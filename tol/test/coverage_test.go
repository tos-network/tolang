package toltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/parser"
)

// ── CyclomaticComplexity ──────────────────────────────────────────────────────

func TestCCEmptyBody(t *testing.T) {
	if cc := CyclomaticComplexity(nil); cc != 1 {
		t.Fatalf("expected CC=1 for empty body, got %d", cc)
	}
}

func TestCCWithIf(t *testing.T) {
	body := []ast.Statement{
		{Kind: "if", Cond: &ast.Expr{Kind: "ident", Value: "x"}, Then: []ast.Statement{}},
	}
	if cc := CyclomaticComplexity(body); cc != 2 {
		t.Fatalf("expected CC=2 for single if, got %d", cc)
	}
}

func TestCCWithWhile(t *testing.T) {
	body := []ast.Statement{
		{Kind: "while", Cond: &ast.Expr{Kind: "ident", Value: "x"}, Body: []ast.Statement{}},
	}
	if cc := CyclomaticComplexity(body); cc != 2 {
		t.Fatalf("expected CC=2 for while, got %d", cc)
	}
}

func TestCCWithLogicalAnd(t *testing.T) {
	// expr containing && counts as one decision point.
	body := []ast.Statement{{
		Kind: "expr",
		Expr: &ast.Expr{
			Kind: "binary", Op: "&&",
			Left:  &ast.Expr{Kind: "ident", Value: "a"},
			Right: &ast.Expr{Kind: "ident", Value: "b"},
		},
	}}
	if cc := CyclomaticComplexity(body); cc != 2 {
		t.Fatalf("expected CC=2 for &&, got %d", cc)
	}
}

func TestCCWithRequireCall(t *testing.T) {
	// require(...) is a decision point.
	body := []ast.Statement{{
		Kind: "expr",
		Expr: &ast.Expr{
			Kind:   "call",
			Callee: &ast.Expr{Kind: "ident", Value: "require"},
			Args:   []*ast.Expr{{Kind: "ident", Value: "cond"}},
		},
	}}
	if cc := CyclomaticComplexity(body); cc != 2 {
		t.Fatalf("expected CC=2 for require, got %d", cc)
	}
}

func TestCCComplex(t *testing.T) {
	// if + nested while + require → 3 decision points → CC=4.
	body := []ast.Statement{
		{
			Kind: "if",
			Cond: &ast.Expr{Kind: "ident", Value: "x"},
			Then: []ast.Statement{
				{Kind: "while", Cond: &ast.Expr{Kind: "ident", Value: "y"}, Body: []ast.Statement{}},
			},
		},
		{
			Kind: "expr",
			Expr: &ast.Expr{
				Kind:   "call",
				Callee: &ast.Expr{Kind: "ident", Value: "require"},
				Args:   []*ast.Expr{{Kind: "ident", Value: "z"}},
			},
		},
	}
	if cc := CyclomaticComplexity(body); cc != 4 {
		t.Fatalf("expected CC=4 (if + while + require), got %d", cc)
	}
}

// ── ContractFileCoverage ──────────────────────────────────────────────────────

func TestContractFileCoverageListsFunctions(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ncontract C {\n  function increment() {}\n  function get() returns (u256 v) { return 0; }\n}\n")
	cf := filepath.Join(dir, "C.tol")
	os.WriteFile(cf, src, 0o644)

	fc, err := ContractFileCoverage(cf, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fc.Functions))
	}
	names := map[string]bool{}
	for _, fn := range fc.Functions {
		names[fn.Name] = true
		if fn.Called {
			t.Errorf("function %q should start as uncalled", fn.Name)
		}
	}
	if !names["increment"] || !names["get"] {
		t.Fatalf("unexpected function names: %v", names)
	}
}

func TestContractFileCoverageComputesCC(t *testing.T) {
	dir := t.TempDir()
	// simple: no branches → CC=1; branchy: one if → CC=2.
	src := []byte("pragma tolang 0.2.0;\ncontract C {\n  function simple() {}\n  function branchy() { if (true) { } }\n}\n")
	cf := filepath.Join(dir, "C.tol")
	os.WriteFile(cf, src, 0o644)

	fc, err := ContractFileCoverage(cf, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ccByName := map[string]int{}
	for _, fn := range fc.Functions {
		ccByName[fn.Name] = fn.CC
	}
	if ccByName["simple"] != 1 {
		t.Errorf("expected CC=1 for simple, got %d", ccByName["simple"])
	}
	if ccByName["branchy"] != 2 {
		t.Errorf("expected CC=2 for branchy, got %d", ccByName["branchy"])
	}
}

func TestContractFileCoverageIncludesConstructorAndFallback(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ncontract C {\n  constructor() {}\n  function foo() {}\n  fallback {}\n}\n")
	cf := filepath.Join(dir, "C.tol")
	os.WriteFile(cf, src, 0o644)

	fc, err := ContractFileCoverage(cf, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := map[string]bool{}
	for _, fn := range fc.Functions {
		names[fn.Name] = true
	}
	if !names["constructor"] {
		t.Error("expected constructor in coverage")
	}
	if !names["fallback"] {
		t.Error("expected fallback in coverage")
	}
}

// ── MarkCalledFunctions ───────────────────────────────────────────────────────

func TestMarkCalledFunctionsMarksCalledMethod(t *testing.T) {
	dir := t.TempDir()
	contractSrc := []byte("pragma tolang 0.2.0;\ncontract C {\n  function increment() {}\n  function get() returns (u256 v) { return 0; }\n}\n")
	cf := filepath.Join(dir, "C.tol")
	os.WriteFile(cf, contractSrc, 0o644)

	fc, _ := ContractFileCoverage(cf, contractSrc)

	// Test module: test body calls c.increment() but not c.get().
	testSrc := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy C() -> c; }\n  function test_inc() {\n    c.increment();\n  }\n}\n")
	tf := filepath.Join(dir, "c_test.tol")
	os.WriteFile(tf, testSrc, 0o644)
	testMod, _ := parser.ParseFile(tf, testSrc)

	MarkCalledFunctions(fc, "c", testMod)

	calledByName := map[string]bool{}
	for _, fn := range fc.Functions {
		calledByName[fn.Name] = fn.Called
	}
	if !calledByName["increment"] {
		t.Error("increment should be marked called")
	}
	if calledByName["get"] {
		t.Error("get should not be marked called")
	}
}

func TestMarkCalledFunctionsNothingCalledWhenBindingDiffers(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ncontract C {\n  function foo() {}\n}\n")
	cf := filepath.Join(dir, "C.tol")
	os.WriteFile(cf, src, 0o644)
	fc, _ := ContractFileCoverage(cf, src)

	// Test module calls other.foo() but binding is "c".
	testSrc := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_x() {\n    other.foo();\n  }\n}\n")
	tf := filepath.Join(dir, "c_test.tol")
	os.WriteFile(tf, testSrc, 0o644)
	testMod, _ := parser.ParseFile(tf, testSrc)

	MarkCalledFunctions(fc, "c", testMod)

	for _, fn := range fc.Functions {
		if fn.Called {
			t.Errorf("no function should be called (wrong binding), but %q is", fn.Name)
		}
	}
}

// ── CoverageReport helpers ────────────────────────────────────────────────────

func TestCoverageReportFunctionPercent(t *testing.T) {
	fc := &FileCoverage{
		Functions: []*FuncCoverage{
			{Name: "a", Called: true},
			{Name: "b", Called: true},
			{Name: "c", Called: false},
		},
	}
	r := &CoverageReport{Files: []*FileCoverage{fc}}
	pct := r.FunctionPercent()
	if pct < 66 || pct > 67 {
		t.Fatalf("expected ~66%%, got %.2f", pct)
	}
}

func TestCoverageReportFunctionPercentEmpty(t *testing.T) {
	r := &CoverageReport{}
	if pct := r.FunctionPercent(); pct != 100 {
		t.Fatalf("expected 100%% for empty report, got %.2f", pct)
	}
}

func TestCoverageReportPrintTextContainsSymbols(t *testing.T) {
	dir := t.TempDir()
	fc := &FileCoverage{
		ContractFile: filepath.Join(dir, "foo.tol"),
		Functions: []*FuncCoverage{
			{Name: "transfer", Called: true, CC: 3},
			{Name: "fallback", Called: false, CC: 1},
		},
	}
	r := &CoverageReport{Files: []*FileCoverage{fc}}
	var sb strings.Builder
	r.PrintText(&sb)
	out := sb.String()
	if !strings.Contains(out, "✓") {
		t.Error("expected ✓ in output for called function")
	}
	if !strings.Contains(out, "✗") {
		t.Error("expected ✗ in output for uncalled function")
	}
	if !strings.Contains(out, "CC=3") {
		t.Error("expected CC=3 in output")
	}
	if !strings.Contains(out, "1/2") {
		t.Error("expected 1/2 function coverage fraction")
	}
}

// ── RunFileWithCoverage integration ──────────────────────────────────────────

func TestRunnerCoverageConstructorMarkedCalled(t *testing.T) {
	dir := t.TempDir()
	contractSrc := []byte("pragma tolang 0.2.0;\ncontract Counter {\n  constructor() {}\n  function increment() {}\n  function get() returns (u256 v) { return 0; }\n}\n")
	os.WriteFile(filepath.Join(dir, "Counter.tol"), contractSrc, 0o644)

	// Test deploys Counter; bodies only use assert_eq (no contract method calls).
	testSrc := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy Counter() -> c; }\n  function test_noop() {\n    assert_eq(1, 1);\n  }\n}\n")
	tf := filepath.Join(dir, "counter_test.tol")
	os.WriteFile(tf, testSrc, 0o644)

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(tf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cov.Files) != 1 {
		t.Fatalf("expected 1 contract file in coverage, got %d", len(cov.Files))
	}
	fc := cov.Files[0]
	ctorCalled := false
	for _, fn := range fc.Functions {
		if fn.Name == "constructor" && fn.Called {
			ctorCalled = true
		}
	}
	if !ctorCalled {
		t.Error("constructor should be marked called because contract was deployed")
	}
}

func TestRunnerCoverageTracksCalledAndUncalledFunctions(t *testing.T) {
	dir := t.TempDir()
	contractSrc := []byte("pragma tolang 0.2.0;\ncontract Counter {\n  function increment() {}\n  function get() returns (u256 v) { return 0; }\n}\n")
	os.WriteFile(filepath.Join(dir, "Counter.tol"), contractSrc, 0o644)

	// Test calls c.increment() but not c.get().
	testSrc := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy Counter() -> c; }\n  function test_inc() {\n    c.increment();\n  }\n}\n")
	tf := filepath.Join(dir, "counter_test.tol")
	os.WriteFile(tf, testSrc, 0o644)

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(tf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cov.Files) != 1 {
		t.Fatalf("expected 1 file in coverage, got %d", len(cov.Files))
	}

	calledByName := map[string]bool{}
	for _, fn := range cov.Files[0].Functions {
		calledByName[fn.Name] = fn.Called
	}
	if !calledByName["increment"] {
		t.Error("increment should be marked called")
	}
	if calledByName["get"] {
		t.Error("get should not be marked called")
	}
}

func TestRunnerCoverageFunctionPercentAfterPartialCoverage(t *testing.T) {
	dir := t.TempDir()
	contractSrc := []byte("pragma tolang 0.2.0;\ncontract C {\n  function a() {}\n  function b() {}\n  function c() {}\n  function d() {}\n}\n")
	os.WriteFile(filepath.Join(dir, "C.tol"), contractSrc, 0o644)

	// Tests call c.a() and c.b() only — 2 of 4 functions → 50%.
	testSrc := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy C() -> c; }\n  function test_ab() {\n    c.a();\n    c.b();\n  }\n}\n")
	tf := filepath.Join(dir, "c_test.tol")
	os.WriteFile(tf, testSrc, 0o644)

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(tf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pct := cov.FunctionPercent()
	if pct < 49 || pct > 51 {
		t.Fatalf("expected ~50%% function coverage, got %.2f", pct)
	}
}

func TestRunnerCoverageNoContractFileSkipsGracefully(t *testing.T) {
	dir := t.TempDir()
	// Test references "Missing" contract which doesn't exist on disk.
	testSrc := []byte("pragma tolang 0.2.0;\ntest Suite {\n  setup { deploy Missing() -> m; }\n  function test_noop() { assert_eq(1, 1); }\n}\n")
	tf := filepath.Join(dir, "missing_test.tol")
	os.WriteFile(tf, testSrc, 0o644)

	r := &Runner{SourceDir: dir}
	_, cov, err := r.RunFileWithCoverage(tf)
	// Test execution itself fails because contract file not found, but coverage
	// building should not return an error — it silently skips missing files.
	_ = err
	if cov == nil {
		t.Fatal("coverage report should not be nil even when contract is missing")
	}
	if len(cov.Files) != 0 {
		t.Fatalf("expected 0 files in coverage for missing contract, got %d", len(cov.Files))
	}
}
