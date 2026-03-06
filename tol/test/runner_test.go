package toltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/tos-network/tolang"
)

// newTestLuaState creates a Lua state with all assert builtins injected.
// Used by tests that exercise assert builtins directly without going through
// the full TOL-file runner pipeline (e.g. when anonymous functions are needed).
func newTestLuaState() *lua.LState {
	ls := lua.NewState()
	injectAssertBuiltins(ls)
	return ls
}

func TestRunnerRunDirFindsNoTestFilesInEmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{SourceDir: dir}
	results, err := r.RunDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestRunnerRunFileRejectsNonTestFile(t *testing.T) {
	dir := t.TempDir()
	// Write a valid TOL test file.
	src := []byte("pragma tolang 0.2.0;\ntest MySuite {\n  function test_basic() {\n  }\n}\n")
	path := filepath.Join(dir, "demo_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].FnName != "test_basic" {
		t.Fatalf("unexpected fn name: %s", results[0].FnName)
	}
}

func TestRunnerSkipsFnsWithSkipFlag(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  @skip\n  function test_skip_me() {\n  }\n}\n")
	path := filepath.Join(dir, "suite_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != "skipped" {
		t.Fatalf("expected skipped result, got: %+v", results[0])
	}
}

func TestRunnerRunDirFindTestFiles(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_one() {\n  }\n  function test_two() {\n  }\n}\n")
	path := filepath.Join(dir, "things_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	// Also write a non-test file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "things.tol"), []byte("pragma tolang 0.2.0;\ncontract X{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
}

// P1 lifecycle tests.

func TestRunnerTeardownRunsAfterPassingTest(t *testing.T) {
	dir := t.TempDir()
	// teardown body contains a passing assertion; if teardown runs it passes,
	// and the test result should be passed.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  teardown {\n    assert_eq(1, 1);\n  }\n  function test_ok() {\n  }\n}\n")
	path := filepath.Join(dir, "lifecycle_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("expected test to pass, got error: %s", results[0].Error)
	}
}

func TestRunnerTeardownFailureSurfacesAfterPassingTest(t *testing.T) {
	dir := t.TempDir()
	// test passes; teardown has a failing assertion → result must be failed
	// and error must mention "teardown".
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  teardown {\n    assert_eq(1, 2);\n  }\n  function test_ok() {\n  }\n}\n")
	path := filepath.Join(dir, "teardown_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatalf("expected test to fail due to teardown error")
	}
	if !strings.Contains(results[0].Error, "teardown") {
		t.Fatalf("expected 'teardown' in error, got: %s", results[0].Error)
	}
}

// P2 lifecycle tests.

func TestRunnerTeardownSuiteDoesNotAffectTestResults(t *testing.T) {
	dir := t.TempDir()
	// teardown_suite has a failing assertion; all individual tests pass.
	// teardown_suite errors must be suppressed — results must still pass.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  teardown_suite {\n    assert_eq(1, 2);\n  }\n  function test_one() {\n  }\n  function test_two() {\n  }\n}\n")
	path := filepath.Join(dir, "suite_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("test %s should pass; teardown_suite error must be suppressed, got: %s", res.FnName, res.Error)
		}
	}
}

func TestRunnerTeardownErrorSuppressedAfterFailingTest(t *testing.T) {
	dir := t.TempDir()
	// test fails with assert_eq(1,2); teardown also fails with assert_eq(3,4).
	// The result must surface the test error ("1" / "2"), not the teardown error ("3" / "4").
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  teardown {\n    assert_eq(3, 4);\n  }\n  function test_fail() {\n    assert_eq(1, 2);\n  }\n}\n")
	path := filepath.Join(dir, "suppress_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatalf("expected test to fail")
	}
	// Error must be from the test body (mentions "1" and "2"), not teardown ("3" and "4").
	if strings.Contains(results[0].Error, "teardown") {
		t.Fatalf("teardown error should be suppressed, got: %s", results[0].Error)
	}
	if !strings.Contains(results[0].Error, "1") {
		t.Fatalf("expected test body error in result, got: %s", results[0].Error)
	}
}

// P3 builtin tests.

func TestRunnerAssertGePassesWhenEqual(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_ge() {\n    assert_ge(5, 5);\n    assert_ge(6, 5);\n  }\n}\n")
	path := filepath.Join(dir, "ge_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestRunnerAssertGeFailsWhenLess(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_ge_fail() {\n    assert_ge(4, 5);\n  }\n}\n")
	path := filepath.Join(dir, "ge_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
}

func TestRunnerAssertLePassesWhenEqual(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_le() {\n    assert_le(5, 5);\n    assert_le(4, 5);\n  }\n}\n")
	path := filepath.Join(dir, "le_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestRunnerAssertLeFailsWhenGreater(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_le_fail() {\n    assert_le(6, 5);\n  }\n}\n")
	path := filepath.Join(dir, "le_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
}

func TestRunnerAssertTrueAndFalse(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_bool() {\n    assert_true(1 == 1);\n    assert_false(1 == 2);\n  }\n}\n")
	path := filepath.Join(dir, "bool_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestRunnerAssertTrueFailsOnFalse(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_true_fail() {\n    assert_true(1 == 2);\n  }\n}\n")
	path := filepath.Join(dir, "true_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
}

func TestRunnerAssertAllPassesWhenAllPass(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_all_pass() {\n    assert_all {\n      assert_eq(1, 1);\n      assert_eq(2, 2);\n    }\n  }\n}\n")
	path := filepath.Join(dir, "all_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestRunnerAssertAllCollectsAllFailures(t *testing.T) {
	dir := t.TempDir()
	// Two assertions fail; assert_all must collect both and report them together.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_all_fail() {\n    assert_all {\n      assert_eq(1, 2);\n      assert_eq(3, 4);\n    }\n  }\n}\n")
	path := filepath.Join(dir, "all_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatalf("expected test to fail")
	}
	// Must mention both failures ("2 of 2").
	if !strings.Contains(results[0].Error, "2 of 2") {
		t.Fatalf("expected combined failure count in error, got: %s", results[0].Error)
	}
}

func TestRunnerAssertAllPartialFailure(t *testing.T) {
	dir := t.TempDir()
	// First assertion passes; second fails — assert_all still collects and reports the failure.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_partial() {\n    assert_all {\n      assert_eq(1, 1);\n      assert_eq(1, 2);\n    }\n  }\n}\n")
	path := filepath.Join(dir, "partial_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
	if !strings.Contains(results[0].Error, "1 of 2") {
		t.Fatalf("expected '1 of 2' in error, got: %s", results[0].Error)
	}
}

// Proxy dispatch tests — verify that contract methods are actually callable.

// counterSrc is a minimal TOL contract with a storage-backed counter.
const counterSrc = `pragma tolang 0.2.0;
contract Counter {
  u256 count;
  function setCount(u256 v) public { set count = v; return; }
  function getCount() public returns (u256 v) { return count; }
}
`

func TestProxyCallNoReturnValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// setCount returns nothing; calling it must not error.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy Counter() -> c; }\n  function test_set() {\n    c.setCount(42);\n  }\n}\n")
	path := filepath.Join(dir, "proxy_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("expected test to pass, got error: %s", results[0].Error)
	}
}

func TestProxyCallWithReturnValueAndAssert(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// Set count to 7, then assert getCount() == 7.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy Counter() -> c; }\n  function test_get() {\n    c.setCount(7);\n    assert_eq(c.getCount(), 7);\n  }\n}\n")
	path := filepath.Join(dir, "proxy_ret_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("expected test to pass, got error: %s", results[0].Error)
	}
}

func TestProxyStorageIsolationBetweenTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// Two test functions each deploy a fresh Counter.
	// The second test must see count=0 (not the value set by the first test).
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy Counter() -> c; }\n  function test_first() {\n    c.setCount(99);\n    assert_eq(c.getCount(), 99);\n  }\n  function test_second() {\n    assert_eq(c.getCount(), 0);\n  }\n}\n")
	path := filepath.Join(dir, "iso_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Errorf("test %s failed (storage not isolated): %s", res.FnName, res.Error)
		}
	}
}

// ── Test block-level let declarations ────────────────────────────────────────

func TestBlockLetAvailableInTestBody(t *testing.T) {
	dir := t.TempDir()
	// Block-level let declares 'expected' at the test block scope;
	// the test body reads it directly.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  let expected: u256 = 42;\n  function test_reads_let() {\n    assert_eq(expected, 42);\n  }\n}\n")
	path := filepath.Join(dir, "let_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("test failed: %s", results[0].Error)
	}
}

func TestBlockLetMultipleDeclarations(t *testing.T) {
	dir := t.TempDir()
	// Two block-level lets used together in an assertion.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  let a: u256 = 10;\n  let b: u256 = 32;\n  function test_sum() {\n    assert_eq(a + b, 42);\n  }\n}\n")
	path := filepath.Join(dir, "let2_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Errorf("expected pass; got %v", results)
	}
}

func TestBlockLetVisibleAcrossMultipleTestFns(t *testing.T) {
	dir := t.TempDir()
	// The same block-level let must be visible in every test function.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  let magic: u256 = 7;\n  function test_first() {\n    assert_eq(magic, 7);\n  }\n  function test_second() {\n    assert_eq(magic, 7);\n  }\n}\n")
	path := filepath.Join(dir, "let3_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Errorf("test %s failed: %s", res.FnName, res.Error)
		}
	}
}

func TestBlockLetVisibleInSetup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// Block-level let 'initVal' is used inside the setup body (via assert_eq).
	// This verifies the let is injected before setup runs.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  let initVal: u256 = 0;\n  setup { deploy Counter() -> c; assert_eq(initVal, 0); }\n  function test_check() {\n    assert_eq(initVal, 0);\n  }\n}\n")
	path := filepath.Join(dir, "letsetup_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Errorf("expected pass; got error: %v", results[0].Error)
	}
}

func TestBlockLetWithStringValue(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  let name: string = \"hello\";\n  function test_str() {\n    assert_eq(name, \"hello\");\n  }\n}\n")
	path := filepath.Join(dir, "letstr_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Errorf("expected pass; got %v", results)
	}
}

func TestBlockLetWrongValueFails(t *testing.T) {
	dir := t.TempDir()
	// The let sets 'x = 1' but the test asserts x == 2, so it must fail.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  let x: u256 = 1;\n  function test_wrong() {\n    assert_eq(x, 2);\n  }\n}\n")
	path := filepath.Join(dir, "letwrong_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("expected test to fail but it passed")
	}
	if !strings.Contains(results[0].Error, "assert_eq") {
		t.Errorf("expected assert_eq failure message, got: %s", results[0].Error)
	}
}

// ── P5: assert_event / assert_no_event ───────────────────────────────────────

// emitterSrc is a minimal TOL contract that emits a Transfer event.
const emitterSrc = `pragma tolang 0.2.0;
contract Emitter {
  event Transfer(agent from, agent to, u256 amount)
  function transfer(agent from, agent to, u256 amount) public {
    emit Transfer(from, to, amount);
    return;
  }
}
`

func TestAssertEventPassesOnEmit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Emitter.tol"), []byte(emitterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Emitter() -> tok; }
  function test_event() {
    tok.transfer(1, 2, 100);
    assert_event("Transfer");
  }
}
`)
	path := filepath.Join(dir, "event_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestAssertEventFailsWhenNotEmitted(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  function test_no_event() {
    assert_event("Transfer");
  }
}
`)
	path := filepath.Join(dir, "event_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
	if !strings.Contains(results[0].Error, "Transfer") {
		t.Fatalf("expected event name in error, got: %s", results[0].Error)
	}
}

func TestAssertNoEventPassesWhenNone(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  function test_none() {
    assert_no_event();
  }
}
`)
	path := filepath.Join(dir, "no_event_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestAssertNoEventFailsOnEmit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Emitter.tol"), []byte(emitterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Emitter() -> tok; }
  function test_unexpected_event() {
    tok.transfer(1, 2, 50);
    assert_no_event();
  }
}
`)
	path := filepath.Join(dir, "no_event_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
	if !strings.Contains(results[0].Error, "Transfer") {
		t.Fatalf("expected event name in error, got: %s", results[0].Error)
	}
}

// ── P5: #[cases] parameterized tests ─────────────────────────────────────────

func TestCasesParameterizedBasic(t *testing.T) {
	dir := t.TempDir()
	// @cases with a single column: n. Test asserts n > 0.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @cases
  function test_positive() {
    assert_gt(n, 0);
  }
  cases {
    | n |
    | 1 |
    | 5 |
    | 99 |
  }
}
`)
	path := filepath.Join(dir, "cases_basic_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect 3 rows → 3 results, all passing.
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(results), results)
	}
	for _, res := range results {
		if !res.Passed {
			t.Errorf("row %s failed: %s", res.FnName, res.Error)
		}
	}
	// Names should be test_positive[0], test_positive[1], test_positive[2]
	if results[0].FnName != "test_positive[0]" {
		t.Errorf("unexpected fn name: %s", results[0].FnName)
	}
}

func TestCasesMultipleColumns(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @cases
  function test_sum() {
    assert_eq(a + b, expected);
  }
  cases {
    | a | b | expected |
    | 1 | 2 | 3 |
    | 10 | 20 | 30 |
  }
}
`)
	path := filepath.Join(dir, "cases_multi_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
	for _, res := range results {
		if !res.Passed {
			t.Errorf("row %s failed: %s", res.FnName, res.Error)
		}
	}
}

func TestCasesReportsRowIndex(t *testing.T) {
	dir := t.TempDir()
	// Second row has a failing assertion; the error should name test_foo[1].
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @cases
  function test_foo() {
    assert_eq(v, 0);
  }
  cases {
    | v |
    | 0 |
    | 1 |
  }
}
`)
	path := filepath.Join(dir, "cases_idx_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("row 0 should pass, got: %s", results[0].Error)
	}
	if results[1].Passed {
		t.Errorf("row 1 should fail")
	}
	if results[1].FnName != "test_foo[1]" {
		t.Errorf("unexpected fn name for failing row: %s", results[1].FnName)
	}
}

// ── P5: inspect binding.slot ──────────────────────────────────────────────────

func TestInspectReadsScalarSlot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), []byte(counterSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// inspect c.count should return 42 after setCount(42).
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy Counter() -> c; }
  function test_inspect() {
    c.setCount(42);
    assert_eq(inspect c.count, 42);
  }
}
`)
	path := filepath.Join(dir, "inspect_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

// ── P5: assert_instructions_le ────────────────────────────────────────────────

func TestAssertInstructionsLePassesUnderLimit(t *testing.T) {
	dir := t.TempDir()
	// A trivial body (one assert_eq call) should be well under 10000 instructions.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  function test_fast() {
    assert_instructions_le(10000) {
      assert_eq(1, 1);
    }
  }
}
`)
	path := filepath.Join(dir, "instr_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestAssertInstructionsLeFailsOverLimit(t *testing.T) {
	dir := t.TempDir()
	// Limit of 0 instructions: any body will exceed it.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  function test_slow() {
    assert_instructions_le(0) {
      assert_eq(1, 1);
    }
  }
}
`)
	path := filepath.Join(dir, "instr_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected failure, got: %+v", results)
	}
	if !strings.Contains(results[0].Error, "assert_instructions_le") {
		t.Fatalf("expected assert_instructions_le in error, got: %s", results[0].Error)
	}
}

// P6 fuzz tests.

func TestFuzzPassesWhenBodyNeverPanics(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @fuzz
  function fuzz_always_pass(u256 x) {
    assert_true(true);
  }
}
`)
	path := filepath.Join(dir, "fuzz_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir, FuzzCount: 10}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("expected fuzz test to pass, got error: %s", results[0].Error)
	}
}

func TestFuzzFailsAndReportsSeedAndIteration(t *testing.T) {
	dir := t.TempDir()
	// This fuzz test always fails: the assertion always rejects.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @fuzz
  function fuzz_always_fail(u256 x) {
    assert_eq(1, 2);
  }
}
`)
	path := filepath.Join(dir, "fuzz_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir, FuzzCount: 5, FuzzSeed: 7}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatalf("expected fuzz test to fail")
	}
	// Error must mention iteration and seed.
	if !strings.Contains(results[0].Error, "fuzz iteration") {
		t.Fatalf("expected 'fuzz iteration' in error, got: %s", results[0].Error)
	}
	if !strings.Contains(results[0].Error, "7") {
		t.Fatalf("expected seed '7' in error, got: %s", results[0].Error)
	}
}

func TestFuzzCustomCount(t *testing.T) {
	dir := t.TempDir()
	// Use @fuzz(count=3) to override the default count.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @fuzz(count=3)
  function fuzz_custom_count(bool x) {
    assert_true(true);
  }
}
`)
	path := filepath.Join(dir, "fuzz_count_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	// FuzzCount on runner is 100, but fn overrides to 3.
	r := &Runner{SourceDir: dir, FuzzCount: 100}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected fuzz test to pass, got: %+v", results)
	}
}

func TestFuzzSkipsSetupBindings(t *testing.T) {
	dir := t.TempDir()
	// The fuzz function has params x (fuzzed) and y (fuzzed); no setup.
	// The body asserts that both are non-nil (always true for u256 fuzz values).
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @fuzz(count=5)
  function fuzz_two_params(u256 x, u256 y) {
    assert_ne(x, "not_a_valid_check");
    assert_true(true);
  }
}
`)
	path := filepath.Join(dir, "fuzz_bindings_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

// P6 mock tests.

func TestMockMethodReturnsStubValue(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  mock Oracle {
    function get_price() returns (u256 price) {
      return "0x0000000000000000000000000000000000000000000000000000000000000064";
    }
  }
  setup {
    deploy Oracle() -> oracle;
  }
  function test_price() {
    let p = oracle.get_price();
    assert_eq(p, "0x0000000000000000000000000000000000000000000000000000000000000064");
  }
}
`)
	path := filepath.Join(dir, "mock_price_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestMockAndRealContractCoexist(t *testing.T) {
	dir := t.TempDir()
	// Write a real contract that does nothing but return a constant.
	realSrc := []byte(`pragma tolang 0.2.0;
contract Counter {
  function value() returns (u256 v) {
    return "0x0000000000000000000000000000000000000000000000000000000000000001";
  }
}
`)
	if err := os.WriteFile(filepath.Join(dir, "Counter.tol"), realSrc, 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  mock FakeCounter {
    function value() returns (u256 v) {
      return "0x0000000000000000000000000000000000000000000000000000000000000002";
    }
  }
  setup {
    deploy Counter() -> real;
    deploy FakeCounter() -> fake;
  }
  function test_both() {
    let rv = real.value();
    let fv = fake.value();
    assert_eq(rv, "0x0000000000000000000000000000000000000000000000000000000000000001");
    assert_eq(fv, "0x0000000000000000000000000000000000000000000000000000000000000002");
  }
}
`)
	path := filepath.Join(dir, "mock_coexist_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestMockBodyCanCallAsserts(t *testing.T) {
	dir := t.TempDir()
	// A mock whose method calls an assert — this should work since assert builtins
	// are injected into the same Lua state.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  mock Validator {
    function check(u256 x) {
      assert_ne(x, "0xdeadbeef");
    }
  }
  setup {
    deploy Validator() -> v;
  }
  function test_check() {
    v.check("0x0000000000000000000000000000000000000000000000000000000000000001");
  }
}
`)
	path := filepath.Join(dir, "mock_assert_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

// ── assert_revert message matching ───────────────────────────────────────────

func TestAssertRevertWithMessageMatchPasses(t *testing.T) {
	// The body reverts with an error containing "boom"; assert_revert expects
	// "boom" as a substring → should pass (no error raised).
	ls := newTestLuaState()
	defer ls.Close()
	err := ls.DoString(`assert_revert(function() error("boom: something went wrong") end, "boom")`)
	if err != nil {
		t.Fatalf("expected pass (no error), got: %v", err)
	}
}

func TestAssertRevertWithMessageMatchFails(t *testing.T) {
	// The body reverts with "actual error"; assert_revert expects "expected msg"
	// which is not a substring → should raise an error mentioning both strings.
	ls := newTestLuaState()
	defer ls.Close()
	err := ls.DoString(`assert_revert(function() error("actual error") end, "expected msg")`)
	if err == nil {
		t.Fatalf("expected assert_revert to raise an error for message mismatch")
	}
	if !strings.Contains(err.Error(), "expected msg") {
		t.Fatalf("error should mention expected string 'expected msg', got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "actual error") {
		t.Fatalf("error should mention actual error 'actual error', got: %s", err.Error())
	}
}

func TestAssertRevertBlockSyntaxPasses(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  function test_revert_any() {
    assert_revert {
      revert "boom";
    }
  }
  function test_revert_msg() {
    assert_revert("boom") {
      revert "boom now";
    }
  }
}
`)
	path := filepath.Join(dir, "assert_revert_block_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("expected pass, got failure: %+v", res)
		}
	}
}

func TestAssertRevertBlockMessageMismatchFails(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  function test_revert_msg_mismatch() {
    assert_revert("expected") {
      revert "actual";
    }
  }
}
`)
	path := filepath.Join(dir, "assert_revert_mismatch_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatalf("expected failure for message mismatch")
	}
	if !strings.Contains(results[0].Error, "expected") {
		t.Fatalf("expected error to contain expected message, got: %s", results[0].Error)
	}
}

func TestWithSenderOverridesContext(t *testing.T) {
	dir := t.TempDir()
	contractSrc := []byte(`pragma tolang 0.2.0;
contract EchoSender {
  function sender() public view returns (agent s) {
    let s: agent = msg.sender;
    return s;
  }
}
`)
	if err := os.WriteFile(filepath.Join(dir, "EchoSender.tol"), contractSrc, 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  setup { deploy EchoSender() -> e; }
  function test_sender_override() {
    with msg.sender = "0x00000000000000000000000000000000000000aa" {
      assert_eq(e.sender(), "0x00000000000000000000000000000000000000aa");
    }
    assert_ne(e.sender(), "0x00000000000000000000000000000000000000aa");
  }
}
`)
	path := filepath.Join(dir, "with_sender_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

// ── #[timeout(ms)] ────────────────────────────────────────────────────────────

func TestTimeoutPassesWhenBodyFast(t *testing.T) {
	dir := t.TempDir()
	// timeout=500ms; body is instant → should pass.
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @timeout(500)
  function test_fast_body() {
    assert_eq(1, 1);
  }
}
`)
	path := filepath.Join(dir, "timeout_pass_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected pass, got: %+v", results)
	}
}

func TestTimeoutFailsWhenBodyExceedsLimit(t *testing.T) {
	dir := t.TempDir()
	// timeout=1ms; body does enough work (a tight Lua loop) to exceed it → fail with "timeout".
	src := []byte(`pragma tolang 0.2.0;
test Suite {
  @timeout(1)
  function test_slow_body() {
    let i: u256 = 0;
    while (i < 1000000) {
      i = i + 1;
    }
  }
}
`)
	path := filepath.Join(dir, "timeout_fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{SourceDir: dir}
	results, err := r.RunFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatalf("expected test to fail due to timeout")
	}
	if !strings.Contains(results[0].Error, "timeout") {
		t.Fatalf("expected 'timeout' in error, got: %s", results[0].Error)
	}
}
