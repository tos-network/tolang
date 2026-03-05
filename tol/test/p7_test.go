package toltest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// counterWithStorageSrc is a contract with a storage slot for testing slot coverage.
const counterWithStorageSrc = `pragma tolang 0.2.0;
contract Counter {
  u256 count;
  function setCount(u256 v) public { set count = v; return; }
  function getCount() public returns (u256 v) { return count; }
}
`

// ── P7: Storage-slot coverage ─────────────────────────────────────────────────

func TestSlotCoverageTracksReadAndWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test writes via setCount and reads via getCount.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_rw() {
    c.setCount(42);
    assert_eq(c.getCount(), 42);
  }
}
`)
	path := filepath.Join(dir, "slot_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{SourceDir: dir}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected test to pass: %+v", results)
	}
	if cov == nil {
		t.Fatal("expected non-nil coverage report")
	}
	if len(cov.Files) == 0 {
		t.Fatal("expected at least one file in coverage report")
	}
	fc := cov.Files[0]
	if len(fc.Slots) == 0 {
		t.Fatal("expected slot coverage entries for Counter contract")
	}
	// The 'count' slot should be both read and written.
	var countSlot *SlotCoverage
	for i := range fc.Slots {
		if fc.Slots[i].Name == "count" {
			countSlot = &fc.Slots[i]
			break
		}
	}
	if countSlot == nil {
		t.Fatal("slot 'count' not found in coverage")
	}
	if !countSlot.Written {
		t.Errorf("expected slot 'count' to be written (setCount was called)")
	}
	if !countSlot.Read {
		t.Errorf("expected slot 'count' to be read (getCount was called)")
	}
	if countSlot.StorageKey == "" {
		t.Errorf("expected non-empty storage key for slot 'count'")
	}
}

func TestSlotCoverageNotTrackedWhenNotCovered(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test does not call setCount or getCount, so slot should be uncovered.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_no_slot_access() {
    assert_eq(1, 1);
  }
}
`)
	path := filepath.Join(dir, "no_slot_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov == nil || len(cov.Files) == 0 {
		t.Fatal("expected non-nil coverage report")
	}
	fc := cov.Files[0]
	if len(fc.Slots) == 0 {
		t.Fatal("expected slot coverage entries")
	}
	var countSlot *SlotCoverage
	for i := range fc.Slots {
		if fc.Slots[i].Name == "count" {
			countSlot = &fc.Slots[i]
			break
		}
	}
	if countSlot == nil {
		t.Fatal("slot 'count' not found")
	}
	// Neither read nor written — constructor doesn't touch count.
	if countSlot.Read {
		t.Errorf("expected slot 'count' NOT to be read")
	}
	if countSlot.Written {
		t.Errorf("expected slot 'count' NOT to be written")
	}
}

// ── P7: -covermin enforcement ─────────────────────────────────────────────────

func TestCoverageMinFunctionPassesWhenAboveThreshold(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Both functions called → 100% coverage; threshold of 80 should pass.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_both() {
    c.setCount(1);
    c.getCount();
  }
}
`)
	path := filepath.Join(dir, "cmin_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		SourceDir:       dir,
		CoverageOptions: CoverageOptions{MinFunctionPct: 80},
	}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected test to pass: %+v", results)
	}
	if cov == nil {
		t.Fatal("expected non-nil coverage report")
	}
}

func TestCoverageMinFunctionFailsWhenBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	// Contract has setCount and getCount; only setCount is called → 50% of non-constructor functions.
	// With constructor included: constructor=called, setCount=called, getCount=NOT called → 66% (2/3).
	// Threshold of 90 should fail.
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_only_write() {
    c.setCount(99);
  }
}
`)
	path := filepath.Join(dir, "cmin_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		SourceDir:       dir,
		CoverageOptions: CoverageOptions{MinFunctionPct: 90},
	}
	_, cov, err := r.RunFileWithCoverage(path)
	if err == nil {
		t.Fatal("expected CoverageError, got nil error")
	}
	covErr, ok := err.(*CoverageError)
	if !ok {
		t.Fatalf("expected *CoverageError, got: %T: %v", err, err)
	}
	if covErr.Metric != "function" {
		t.Errorf("expected metric 'function', got: %s", covErr.Metric)
	}
	if covErr.Min != 90 {
		t.Errorf("expected min=90, got: %d", covErr.Min)
	}
	if covErr.Got >= 90 {
		t.Errorf("expected got < 90, got: %d", covErr.Got)
	}
	if cov == nil {
		t.Error("coverage report should still be returned with the error")
	}
	// Error message should be informative.
	if !strings.Contains(err.Error(), "function") {
		t.Errorf("expected 'function' in error message, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "90") {
		t.Errorf("expected '90' in error message, got: %s", err.Error())
	}
}

func TestCoverageMinSlotPassesWhenAboveThreshold(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the slot → 100% slot coverage; threshold of 50 should pass.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_write() {
    c.setCount(5);
  }
}
`)
	path := filepath.Join(dir, "slot_min_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{
		SourceDir:       dir,
		CoverageOptions: CoverageOptions{MinSlotPct: 50},
	}
	_, _, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// ── P7: Coverage output formats ───────────────────────────────────────────────

func TestCoverageReportPrintJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_json() {
    c.setCount(1);
  }
}
`)
	path := filepath.Join(dir, "json_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if err := cov.PrintJSON(&buf); err != nil {
		t.Fatalf("PrintJSON failed: %v", err)
	}

	// Must be valid JSON.
	var parsed interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("PrintJSON produced invalid JSON: %v\nOutput:\n%s", err, buf.String())
	}

	// Must contain "files" key.
	m, ok := parsed.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSON object, got: %T", parsed)
	}
	if _, hasFiles := m["files"]; !hasFiles {
		t.Errorf("expected 'files' key in JSON output, got: %s", buf.String())
	}
}

func TestCoverageReportPrintXML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_xml() {
    c.setCount(1);
  }
}
`)
	path := filepath.Join(dir, "xml_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if err := cov.PrintXML(&buf); err != nil {
		t.Fatalf("PrintXML failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "<report") {
		t.Errorf("expected '<report' in XML output, got:\n%s", out)
	}
	if !strings.Contains(out, "METHOD") {
		t.Errorf("expected 'METHOD' counter in XML output, got:\n%s", out)
	}
}

func TestCoverageReportPrintLCOV(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_lcov() {
    c.setCount(1);
  }
}
`)
	path := filepath.Join(dir, "lcov_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if err := cov.PrintLCOV(&buf); err != nil {
		t.Fatalf("PrintLCOV failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "end_of_record") {
		t.Errorf("expected 'end_of_record' in LCOV output, got:\n%s", out)
	}
	if !strings.Contains(out, "FN:") {
		t.Errorf("expected 'FN:' entries in LCOV output, got:\n%s", out)
	}
	if !strings.Contains(out, "FNF:") {
		t.Errorf("expected 'FNF:' in LCOV output, got:\n%s", out)
	}
}

func TestCoverageReportPrintHTML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterWithStorageSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_html() {
    c.setCount(1);
  }
}
`)
	path := filepath.Join(dir, "html_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if err := cov.PrintHTML(&buf); err != nil {
		t.Fatalf("PrintHTML failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "<html") {
		t.Errorf("expected '<html' in HTML output, got:\n%s", out)
	}
	if !strings.Contains(out, "<table") {
		t.Errorf("expected '<table' in HTML output, got:\n%s", out)
	}
	if !strings.Contains(out, "Counter.tol") {
		t.Errorf("expected 'Counter.tol' in HTML output, got:\n%s", out)
	}
}

// ── P7: TotalFunctionPct / TotalSlotPct ───────────────────────────────────────

func TestTotalFunctionPctEmpty(t *testing.T) {
	r := &CoverageReport{}
	if got := r.TotalFunctionPct(); got != 100 {
		t.Errorf("expected 100%% for empty report, got %d", got)
	}
}

func TestTotalSlotPctEmpty(t *testing.T) {
	r := &CoverageReport{}
	if got := r.TotalSlotPct(); got != 100 {
		t.Errorf("expected 100%% for empty report, got %d", got)
	}
}

func TestTotalFunctionPctPartialCoverage(t *testing.T) {
	r := &CoverageReport{
		Files: []*FileCoverage{
			{
				Functions: []*FuncCoverage{
					{Name: "a", Called: true},
					{Name: "b", Called: false},
				},
			},
		},
	}
	got := r.TotalFunctionPct()
	if got != 50 {
		t.Errorf("expected 50%%, got %d", got)
	}
}

func TestTotalSlotPctPartialCoverage(t *testing.T) {
	r := &CoverageReport{
		Files: []*FileCoverage{
			{
				Slots: []SlotCoverage{
					{Name: "a", Read: true},
					{Name: "b", Written: false},
				},
			},
		},
	}
	got := r.TotalSlotPct()
	if got != 50 {
		t.Errorf("expected 50%%, got %d", got)
	}
}

// ── P7: PrintText shows slot coverage ─────────────────────────────────────────

func TestPrintTextShowsSlotCoverage(t *testing.T) {
	r := &CoverageReport{
		Files: []*FileCoverage{
			{
				ContractFile: "/tmp/token.tol",
				Functions: []*FuncCoverage{
					{Name: "transfer", Called: true, CC: 2},
				},
				Slots: []SlotCoverage{
					{Name: "balances", StorageKey: "0xabcd", Read: true, Written: true},
				},
			},
		},
	}
	var buf bytes.Buffer
	r.PrintText(&buf)
	out := buf.String()
	if !strings.Contains(out, "balances") {
		t.Errorf("expected 'balances' slot in text output, got:\n%s", out)
	}
	if !strings.Contains(out, "Slot coverage") {
		t.Errorf("expected 'Slot coverage' in text output, got:\n%s", out)
	}
}

// ── P7: CoverageError.Error() message format ──────────────────────────────────

func TestCoverageErrorMessage(t *testing.T) {
	e := &CoverageError{
		File:   "foo.tol",
		Metric: "function",
		Got:    55,
		Min:    80,
	}
	msg := e.Error()
	if !strings.Contains(msg, "function") {
		t.Errorf("expected 'function' in error message: %s", msg)
	}
	if !strings.Contains(msg, "55") {
		t.Errorf("expected got '55' in error message: %s", msg)
	}
	if !strings.Contains(msg, "80") {
		t.Errorf("expected min '80' in error message: %s", msg)
	}
	if !strings.Contains(msg, "foo.tol") {
		t.Errorf("expected file 'foo.tol' in error message: %s", msg)
	}
}
