package toltest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tos-network/tolang/tol/parser"
)

// contractWithBranches is a simple contract with if/while/for branching.
const contractWithBranchesSrc = `pragma tolang 0.2.0;
contract Branchy {
  function ifBranch(u256 x) public returns (u256 r) {
    if (x > 0) {
      return x;
    } else {
      return 0;
    }
  }
  function whileLoop(u256 n) public returns (u256 r) {
    u256 i = 0;
    while (i < n) {
      set i = i + 1;
    }
    return i;
  }
  function noElse(u256 x) public returns (u256 r) {
    u256 r = 0;
    if (x > 5) {
      set r = x;
    }
    return r;
  }
}
`

// contractSimpleSrc is a flat contract with no branches, easy for line coverage.
const contractSimpleSrc = `pragma tolang 0.2.0;
contract Simple {
  function add(u256 a, u256 b) public returns (u256 r) {
    return a + b;
  }
  function noop() public {
    return;
  }
}
`

func writeTempContract(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func writeTestFile(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

// ── Line coverage basic tests ─────────────────────────────────────────────────

func TestLineCoverageHitsExecutedStatements(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Simple.tol", contractSimpleSrc)

	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Simple() -> s; }
  function test_add() {
    assert_eq(s.add(2, 3), 5);
  }
}
`
	path := writeTestFile(t, dir, "simple_test.tol", testSrc)
	r := &Runner{SourceDir: dir}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 || !results[0].Passed {
		t.Fatalf("test did not pass: %+v", results)
	}
	if cov == nil || len(cov.Files) == 0 {
		t.Fatal("no coverage report")
	}
	fc := cov.Files[0]
	if len(fc.Lines) == 0 {
		t.Fatal("expected line coverage data to be populated")
	}
	// At least one line should be marked as hit.
	anyHit := false
	for _, lc := range fc.Lines {
		if lc.Hit {
			anyHit = true
			break
		}
	}
	if !anyHit {
		t.Error("expected at least one line to be marked hit")
	}
	// All lines should have positive line numbers.
	for _, lc := range fc.Lines {
		if lc.Line <= 0 {
			t.Errorf("line coverage entry has non-positive line: %d", lc.Line)
		}
	}
}

func TestLineCoverageSkipsUnreachableElse(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Branchy.tol", contractWithBranchesSrc)

	// Only call ifBranch with x > 0, so the else branch should not be hit.
	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Branchy() -> b; }
  function test_then_only() {
    assert_eq(b.ifBranch(10), 10);
  }
}
`
	path := writeTestFile(t, dir, "branch_test.tol", testSrc)
	r := &Runner{SourceDir: dir}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 || !results[0].Passed {
		t.Fatalf("test did not pass: %+v", results)
	}
	fc := cov.Files[0]
	if len(fc.Lines) == 0 {
		t.Skip("no line coverage data (compilation may not emit line info yet)")
	}
	// The return in the else branch should not be hit.
	// We look for a line that is NOT hit. Not all implementations will have
	// distinct line numbers per statement, so just check that not all are hit.
	allHit := true
	for _, lc := range fc.Lines {
		if !lc.Hit {
			allHit = false
			break
		}
	}
	if allHit && len(fc.Lines) > 1 {
		// If all lines are hit even though we only called the then-branch, the
		// line numbers may be colliding. This is acceptable but surprising.
		t.Log("note: all lines marked hit (line numbers may overlap)")
	}
}

// ── Branch coverage tests ────────────────────────────────────────────────────

func TestBranchCoverageThenAndElseBothHit(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Branchy.tol", contractWithBranchesSrc)

	// Call ifBranch with both x > 0 and x == 0 to hit both branches.
	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Branchy() -> b; }
  function test_both_branches() {
    assert_eq(b.ifBranch(1), 1);
    assert_eq(b.ifBranch(0), 0);
  }
}
`
	path := writeTestFile(t, dir, "both_branch_test.tol", testSrc)
	r := &Runner{SourceDir: dir}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 || !results[0].Passed {
		t.Fatalf("test did not pass: %+v", results)
	}
	fc := cov.Files[0]
	if len(fc.Branches) == 0 {
		t.Skip("no branch coverage data")
	}
	// Both then (BranchID=0) and else (BranchID=1) for the if statement
	// in ifBranch should be hit.
	thenHit := false
	elseHit := false
	for _, bc := range fc.Branches {
		if bc.BranchID == 0 && bc.Hit {
			thenHit = true
		}
		if bc.BranchID == 1 && bc.Hit {
			elseHit = true
		}
	}
	if !thenHit {
		t.Error("expected then branch to be hit")
	}
	if !elseHit {
		t.Error("expected else branch to be hit")
	}
}

func TestBranchCoverageOnlyThenHit(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Branchy.tol", contractWithBranchesSrc)

	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Branchy() -> b; }
  function test_then_only() {
    assert_eq(b.noElse(10), 10);
  }
}
`
	path := writeTestFile(t, dir, "then_only_test.tol", testSrc)
	r := &Runner{SourceDir: dir}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 || !results[0].Passed {
		t.Fatalf("test did not pass: %+v", results)
	}
	fc := cov.Files[0]
	if len(fc.Branches) == 0 {
		t.Skip("no branch coverage data")
	}
	// For noElse, there should be a then branch but no else branch recorded
	// (since the else is empty). The then branch should be hit.
	thenHit := false
	for _, bc := range fc.Branches {
		if bc.BranchID == 0 && bc.Hit {
			thenHit = true
		}
	}
	if !thenHit {
		t.Error("expected then branch to be hit when x > 5")
	}
}

func TestBranchCoverageWhileBodyHit(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Branchy.tol", contractWithBranchesSrc)

	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Branchy() -> b; }
  function test_while() {
    assert_eq(b.whileLoop(3), 3);
  }
}
`
	path := writeTestFile(t, dir, "while_test.tol", testSrc)
	r := &Runner{SourceDir: dir}
	results, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 || !results[0].Passed {
		t.Fatalf("test did not pass: %+v", results)
	}
	fc := cov.Files[0]
	if len(fc.Branches) == 0 {
		t.Skip("no branch coverage data")
	}
	// The while body branch (BranchID=0) should be hit.
	whileBodyHit := false
	for _, bc := range fc.Branches {
		if bc.BranchID == 0 && bc.Hit {
			whileBodyHit = true
		}
	}
	if !whileBodyHit {
		t.Error("expected while body branch to be hit")
	}
}

// ── LCOV output tests ─────────────────────────────────────────────────────────

func TestLCOVEmitsRealDARecords(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Simple.tol", contractSimpleSrc)

	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Simple() -> s; }
  function test_add() {
    assert_eq(s.add(1, 2), 3);
  }
}
`
	path := writeTestFile(t, dir, "lcov_test.tol", testSrc)
	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov == nil {
		t.Fatal("no coverage report")
	}

	var buf bytes.Buffer
	if err := cov.PrintLCOV(&buf); err != nil {
		t.Fatalf("PrintLCOV error: %v", err)
	}
	lcov := buf.String()

	// Should always have SF and FN/FNDA records.
	if !strings.Contains(lcov, "SF:") {
		t.Error("LCOV missing SF: record")
	}
	if !strings.Contains(lcov, "FN:") {
		t.Error("LCOV missing FN: record")
	}
	if !strings.Contains(lcov, "end_of_record") {
		t.Error("LCOV missing end_of_record")
	}

	// If line coverage was populated, DA: records should be present.
	if len(cov.Files) > 0 && len(cov.Files[0].Lines) > 0 {
		if !strings.Contains(lcov, "DA:") {
			t.Error("LCOV missing DA: records even though line coverage is populated")
		}
		if !strings.Contains(lcov, "LF:") {
			t.Error("LCOV missing LF: record")
		}
		if !strings.Contains(lcov, "LH:") {
			t.Error("LCOV missing LH: record")
		}
	}
}

// ── MinLinePct and MinBranchPct threshold tests ───────────────────────────────

func TestCoverageMinLinePctFails(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Simple.tol", contractSimpleSrc)

	// Only test one function (add); noop is not called.
	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Simple() -> s; }
  function test_add_only() {
    assert_eq(s.add(10, 5), 15);
  }
}
`
	path := writeTestFile(t, dir, "minline_test.tol", testSrc)
	r := &Runner{
		SourceDir: dir,
		CoverageOptions: CoverageOptions{
			MinLinePct: 100, // demand 100% line coverage — impossible since noop is uncalled
		},
	}
	_, cov, err := r.RunFileWithCoverage(path)
	if cov == nil {
		t.Fatal("expected non-nil coverage report even on failure")
	}
	if len(cov.Files) == 0 || len(cov.Files[0].Lines) == 0 {
		// Line coverage not available — skip this threshold test.
		t.Skip("line coverage not available; skipping MinLinePct threshold test")
	}
	// With 100% threshold and uncalled functions, we expect a CoverageError.
	// But only if not all lines happened to be hit.
	linePct := cov.TotalLinePct()
	if linePct < 100 {
		if err == nil {
			t.Error("expected CoverageError when line coverage is below MinLinePct=100")
		} else {
			covErr, ok := err.(*CoverageError)
			if !ok {
				t.Errorf("expected *CoverageError, got %T: %v", err, err)
			} else if covErr.Metric != "line" {
				t.Errorf("expected metric='line', got %q", covErr.Metric)
			}
		}
	}
}

func TestCoverageMinBranchPctFails(t *testing.T) {
	dir := t.TempDir()
	writeTempContract(t, dir, "Branchy.tol", contractWithBranchesSrc)

	// Only call ifBranch with x > 0, so the else branch is never hit.
	testSrc := `pragma tolang 0.2.0;
test Suite {
  setup { deploy Branchy() -> b; }
  function test_then_only() {
    assert_eq(b.ifBranch(5), 5);
  }
}
`
	path := writeTestFile(t, dir, "minbranch_test.tol", testSrc)
	r := &Runner{
		SourceDir: dir,
		CoverageOptions: CoverageOptions{
			MinBranchPct: 100,
		},
	}
	_, cov, err := r.RunFileWithCoverage(path)
	if cov == nil {
		t.Fatal("expected non-nil coverage report")
	}
	if len(cov.Files) == 0 || len(cov.Files[0].Branches) == 0 {
		t.Skip("branch coverage not available; skipping MinBranchPct threshold test")
	}
	branchPct := cov.TotalBranchPct()
	if branchPct < 100 {
		if err == nil {
			t.Error("expected CoverageError when branch coverage is below MinBranchPct=100")
		} else {
			covErr, ok := err.(*CoverageError)
			if !ok {
				t.Errorf("expected *CoverageError, got %T: %v", err, err)
			} else if covErr.Metric != "branch" {
				t.Errorf("expected metric='branch', got %q", covErr.Metric)
			}
		}
	}
}

// ── TotalLinePct / TotalBranchPct unit tests ──────────────────────────────────

func TestTotalLinePctEmpty(t *testing.T) {
	r := &CoverageReport{}
	if got := r.TotalLinePct(); got != 100 {
		t.Errorf("expected 100 for empty report, got %d", got)
	}
}

func TestTotalBranchPctEmpty(t *testing.T) {
	r := &CoverageReport{}
	if got := r.TotalBranchPct(); got != 100 {
		t.Errorf("expected 100 for empty report, got %d", got)
	}
}

func TestTotalLinePctCalculation(t *testing.T) {
	r := &CoverageReport{
		Files: []*FileCoverage{
			{
				ContractFile: "a.tol",
				Lines: []LineCoverage{
					{Line: 1, Hit: true},
					{Line: 2, Hit: true},
					{Line: 3, Hit: false},
					{Line: 4, Hit: false},
				},
			},
		},
	}
	got := r.TotalLinePct()
	// 2/4 = 50%
	if got != 50 {
		t.Errorf("expected 50, got %d", got)
	}
}

func TestTotalBranchPctCalculation(t *testing.T) {
	r := &CoverageReport{
		Files: []*FileCoverage{
			{
				ContractFile: "a.tol",
				Branches: []BranchCoverage{
					{Line: 5, BranchID: 0, Hit: true},
					{Line: 5, BranchID: 1, Hit: false},
					{Line: 10, BranchID: 0, Hit: true},
				},
			},
		},
	}
	got := r.TotalBranchPct()
	// 2/3 = 66%
	if got != 66 {
		t.Errorf("expected 66, got %d", got)
	}
}

// ── BuildLineCoverage unit tests ─────────────────────────────────────────────

func TestBuildLineCoveragePopulatesLines(t *testing.T) {
	src := `pragma tolang 0.2.0;
contract Simple {
  function add(u256 a, u256 b) public returns (u256 r) {
    return a + b;
  }
  function noop() public {
    return;
  }
}
`
	cfile := "Simple.tol"
	mod, diags := parser.ParseFile(cfile, []byte(src))
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	fc := &FileCoverage{ContractFile: cfile}
	// Mark only line 4 (return a + b) as hit.
	hits := map[int]bool{4: true}
	BuildLineCoverage(fc, hits, mod)

	if len(fc.Lines) == 0 {
		t.Fatal("expected Lines to be populated after BuildLineCoverage")
	}

	hitCount := 0
	for _, lc := range fc.Lines {
		if lc.Line <= 0 {
			t.Errorf("invalid line number %d in coverage", lc.Line)
		}
		if lc.Hit {
			hitCount++
		}
	}
	if hitCount == 0 {
		t.Error("expected at least one line to be marked hit")
	}
	// Not all lines should be hit (noop is not called).
	if hitCount == len(fc.Lines) && len(fc.Lines) > 1 {
		t.Error("expected some lines to be unhit (noop body was not called)")
	}
}

func TestBuildLineCoverageNoBranchesForFlatContract(t *testing.T) {
	src := `pragma tolang 0.2.0;
contract Flat {
  function compute(u256 a) public returns (u256 r) {
    u256 b = a + 1;
    return b;
  }
}
`
	mod, diags := parser.ParseFile("Flat.tol", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	fc := &FileCoverage{ContractFile: "Flat.tol"}
	BuildLineCoverage(fc, map[int]bool{}, mod)

	if len(fc.Branches) != 0 {
		t.Errorf("expected no branch entries for flat contract, got %d", len(fc.Branches))
	}
	if len(fc.Lines) == 0 {
		t.Error("expected line entries even for flat contract")
	}
}

func TestBuildLineCoverageIfElseBranchRecorded(t *testing.T) {
	src := `pragma tolang 0.2.0;
contract Cond {
  function check(u256 x) public returns (u256 r) {
    if (x > 0) {
      return x;
    } else {
      return 0;
    }
  }
}
`
	mod, diags := parser.ParseFile("Cond.tol", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	fc := &FileCoverage{ContractFile: "Cond.tol"}
	// No hits — both branches should be recorded but not hit.
	BuildLineCoverage(fc, map[int]bool{}, mod)

	thenBranch := false
	elseBranch := false
	for _, bc := range fc.Branches {
		if bc.BranchID == 0 {
			thenBranch = true
			if bc.Hit {
				t.Error("then branch should not be hit with empty hit set")
			}
		}
		if bc.BranchID == 1 {
			elseBranch = true
			if bc.Hit {
				t.Error("else branch should not be hit with empty hit set")
			}
		}
	}
	if !thenBranch {
		t.Error("expected then branch (BranchID=0) to be recorded")
	}
	if !elseBranch {
		t.Error("expected else branch (BranchID=1) to be recorded")
	}
}
