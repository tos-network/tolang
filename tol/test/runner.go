// Package toltest provides the P2 test runner for TOL test blocks.
// It discovers *_test.tol files, parses and sema-checks them, then executes
// each test function in an isolated Lua state with injected assert helpers.
//
// Lifecycle order per test function:
//  1. Fresh Lua state (per-test storage isolation)
//  2. setup_suite deploy statements (suite-level contracts, re-executed per test)
//  3. setup deploy statements (per-test contracts)
//  4. test function body
//  5. teardown body (always, pass or fail)
//
// Suite-level hooks:
//
//	setup_suite: deploy statements are replayed in each test's isolated state.
//	teardown_suite: runs once after all tests; errors are suppressed.
package toltest

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/tos-network/tolang"
	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/parser"
	"github.com/tos-network/tolang/tol/sema"
	"golang.org/x/crypto/sha3"
)

// Result holds the outcome of a single test function execution.
type Result struct {
	TestBlock string
	FnName    string
	Passed    bool
	Error     string
	Duration  time.Duration
	Tags      []string // from #[tag("...")] on the test function
}

// CoverageOptions controls coverage threshold enforcement.
type CoverageOptions struct {
	// MinFunctionPct, if > 0, causes RunFileWithCoverage to fail if function
	// coverage is below this percentage.
	MinFunctionPct int
	// MinSlotPct, if > 0, causes RunFileWithCoverage to fail if storage slot
	// coverage is below this percentage.
	MinSlotPct int
	// MinLinePct, if > 0, causes RunFileWithCoverage to fail if line coverage
	// is below this percentage (requires VM sourcemap instrumentation).
	MinLinePct int
	// MinBranchPct, if > 0, causes RunFileWithCoverage to fail if branch coverage
	// is below this percentage (requires VM sourcemap instrumentation).
	MinBranchPct int
}

// CoverageError is returned by RunFileWithCoverage when coverage falls below a
// configured minimum threshold.
type CoverageError struct {
	File   string
	Metric string // "function" or "slot"
	Got    int
	Min    int
}

func (e *CoverageError) Error() string {
	return fmt.Sprintf("coverage: %s coverage %d%% is below minimum %d%% in %s",
		e.Metric, e.Got, e.Min, e.File)
}

// Runner discovers and executes TOL test files.
type Runner struct {
	// SourceDir is the directory to search for contract .tol files referenced
	// by deploy statements. Defaults to the test file's directory.
	SourceDir string
	// FuzzSeed is the random seed used for @fuzz tests. 0 means use 42 (deterministic default).
	FuzzSeed int64
	// FuzzCount is the default number of iterations for @fuzz tests. 0 means 100.
	FuzzCount int
	// RunFilter, if non-empty, causes the runner to skip any test/fuzz function
	// whose name does not contain this substring.
	RunFilter string
	// TagFilter, if non-empty, causes the runner to skip any function that does
	// not have this tag in its Tags slice.
	TagFilter string
	// SkipTag, if non-empty, causes the runner to skip any function that has
	// this tag in its Tags slice.
	SkipTag string
	// CoverageOptions controls coverage threshold enforcement used by
	// RunFileWithCoverage.
	CoverageOptions CoverageOptions
	// instanceID is an ever-incrementing counter used to generate unique Lua
	// global names for per-instance storage references. Incremented in
	// executeDeploy; not safe for concurrent use (tests run sequentially).
	instanceID int
	// slotTrackRead and slotTrackWrite record which storage keys (hex strings)
	// were accessed during a RunFileWithCoverage run. Non-nil only during a
	// coverage run; guarded by sequential test execution.
	slotTrackRead  map[string]bool
	slotTrackWrite map[string]bool
	// lineHits records which source lines were executed per contract file during
	// a RunFileWithCoverage run. Non-nil only during a coverage run.
	lineHits map[string]map[int]bool // contractFile → set of hit line numbers
}

// RunFile parses, sema-checks, and runs all test functions in testFile.
// testFile must end with _test.tol.
func (r *Runner) RunFile(testFile string) ([]Result, error) {
	src, err := os.ReadFile(testFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", testFile, err)
	}

	mod, parseDiags := parser.ParseFile(testFile, src)
	if parseDiags.HasErrors() {
		return nil, fmt.Errorf("parse errors in %s: %v", testFile, parseDiags)
	}

	_, semaDiags := sema.CheckWithResolver(testFile, mod, testFileResolver(testFile))
	if semaDiags.HasErrors() {
		return nil, fmt.Errorf("sema errors in %s: %v", testFile, semaDiags)
	}

	sourceDir := r.SourceDir
	if sourceDir == "" {
		sourceDir = filepath.Dir(testFile)
	}

	var results []Result
	for _, td := range mod.Tests {
		res := r.runTestDecl(testFile, sourceDir, mod, td)
		results = append(results, res...)
	}
	return results, nil
}

// RunFileWithCoverage is like RunFile but also returns a CoverageReport that
// lists each contract deployed by the test file with function call status and
// cyclomatic complexity. Coverage is derived from static analysis of the test
// module's call expressions (function-level; branch/line coverage is deferred
// to a later phase requiring VM instrumentation).
//
// Storage slot coverage is collected at runtime via Lua metatable interception.
//
// The constructor is marked as called for every contract that appears in a
// deploy statement. Other functions are marked called when a call expression of
// the form bindingName.fnName(...) is found anywhere in the test bodies.
//
// If CoverageOptions thresholds are set and actual coverage falls below them,
// a *CoverageError is returned alongside the results and report.
func (r *Runner) RunFileWithCoverage(testFile string) ([]Result, *CoverageReport, error) {
	src, err := os.ReadFile(testFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", testFile, err)
	}

	mod, parseDiags := parser.ParseFile(testFile, src)
	if parseDiags.HasErrors() {
		return nil, nil, fmt.Errorf("parse errors in %s: %v", testFile, parseDiags)
	}

	_, semaDiags := sema.CheckWithResolver(testFile, mod, testFileResolver(testFile))
	if semaDiags.HasErrors() {
		return nil, nil, fmt.Errorf("sema errors in %s: %v", testFile, semaDiags)
	}

	sourceDir := r.SourceDir
	if sourceDir == "" {
		sourceDir = filepath.Dir(testFile)
	}

	// Enable slot and line tracking mode for this run.
	r.slotTrackRead = make(map[string]bool)
	r.slotTrackWrite = make(map[string]bool)
	r.lineHits = make(map[string]map[int]bool)

	var results []Result
	for _, td := range mod.Tests {
		res := r.runTestDecl(testFile, sourceDir, mod, td)
		results = append(results, res...)
	}

	cov := r.buildCoverage(sourceDir, mod)

	// Disable tracking after run.
	r.slotTrackRead = nil
	r.slotTrackWrite = nil
	r.lineHits = nil

	// Enforce coverage thresholds.
	opts := r.CoverageOptions
	if opts.MinFunctionPct > 0 {
		got := cov.TotalFunctionPct()
		if got < opts.MinFunctionPct {
			return results, cov, &CoverageError{
				File:   testFile,
				Metric: "function",
				Got:    got,
				Min:    opts.MinFunctionPct,
			}
		}
	}
	if opts.MinSlotPct > 0 {
		got := cov.TotalSlotPct()
		if got < opts.MinSlotPct {
			return results, cov, &CoverageError{
				File:   testFile,
				Metric: "slot",
				Got:    got,
				Min:    opts.MinSlotPct,
			}
		}
	}
	if opts.MinLinePct > 0 {
		got := cov.TotalLinePct()
		if got < opts.MinLinePct {
			return results, cov, &CoverageError{
				File:   testFile,
				Metric: "line",
				Got:    got,
				Min:    opts.MinLinePct,
			}
		}
	}
	if opts.MinBranchPct > 0 {
		got := cov.TotalBranchPct()
		if got < opts.MinBranchPct {
			return results, cov, &CoverageError{
				File:   testFile,
				Metric: "branch",
				Got:    got,
				Min:    opts.MinBranchPct,
			}
		}
	}

	return results, cov, nil
}

// buildCoverage scans all test blocks for deploy statements, parses the
// referenced contract files, and marks called functions via static analysis.
// It also populates SlotCoverage from the slot tracking data if available.
func (r *Runner) buildCoverage(sourceDir string, mod *ast.Module) *CoverageReport {
	cov := &CoverageReport{}
	// fileCovByPath deduplicates contract files across multiple test blocks.
	fileCovByPath := map[string]*FileCoverage{}

	for _, td := range mod.Tests {
		var deployStmts []ast.Statement
		if td.SetupSuite != nil {
			for _, s := range td.SetupSuite.Body {
				if s.Kind == "deploy" {
					deployStmts = append(deployStmts, s)
				}
			}
		}
		if td.Setup != nil {
			for _, s := range td.Setup.Body {
				if s.Kind == "deploy" {
					deployStmts = append(deployStmts, s)
				}
			}
		}

		for _, s := range deployStmts {
			if s.Expr == nil || s.Expr.Callee == nil {
				continue
			}
			contractName := s.Expr.Callee.Value
			contractFile := filepath.Join(sourceDir, contractName+".tol")

			if _, seen := fileCovByPath[contractFile]; !seen {
				contractSrc, readErr := os.ReadFile(contractFile)
				if readErr != nil {
					continue
				}
				fc, covErr := ContractFileCoverage(contractFile, contractSrc)
				if covErr != nil {
					continue
				}
				fileCovByPath[contractFile] = fc
				cov.Files = append(cov.Files, fc)
			}

			fc := fileCovByPath[contractFile]

			// Deploying always calls the constructor.
			for _, fn := range fc.Functions {
				if fn.Name == "constructor" {
					fn.Called = true
				}
			}

			// Mark functions called via binding.method() in test bodies.
			if s.Name != "" {
				MarkCalledFunctions(fc, s.Name, mod)
			}
		}
	}

	// Populate slot coverage from the runtime tracking data (when available).
	if r.slotTrackRead != nil || r.slotTrackWrite != nil {
		for _, fc := range cov.Files {
			for i := range fc.Slots {
				key := fc.Slots[i].StorageKey
				if r.slotTrackRead != nil && r.slotTrackRead[key] {
					fc.Slots[i].Read = true
				}
				if r.slotTrackWrite != nil && r.slotTrackWrite[key] {
					fc.Slots[i].Written = true
				}
			}
		}
	}

	// Populate line/branch coverage from the VM line hook data (when available).
	if r.lineHits != nil {
		for _, fc := range cov.Files {
			if hits, ok := r.lineHits[fc.ContractFile]; ok {
				contractSrc, readErr := os.ReadFile(fc.ContractFile)
				if readErr == nil {
					contractMod, parseDiags := parser.ParseFile(fc.ContractFile, contractSrc)
					if !parseDiags.HasErrors() {
						BuildLineCoverage(fc, hits, contractMod)
					}
				}
			}
		}
	}

	return cov
}

// RunDir finds all *_test.tol files in dir (recursively) and runs them.
func (r *Runner) RunDir(dir string) ([]Result, error) {
	var results []Result
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, "_test.tol") {
			res, runErr := r.RunFile(path)
			if runErr != nil {
				results = append(results, Result{
					TestBlock: path,
					FnName:    "<file>",
					Passed:    false,
					Error:     runErr.Error(),
				})
			} else {
				results = append(results, res...)
			}
		}
		return nil
	})
	return results, err
}

func (r *Runner) runTestDecl(testFile, sourceDir string, mod *ast.Module, td ast.TestDecl) []Result {
	// Collect setup_suite statements to replay in each test's isolated state.
	var suiteSetupStmts []ast.Statement
	if td.SetupSuite != nil {
		suiteSetupStmts = td.SetupSuite.Body
	}

	var results []Result
	for _, fn := range td.Fns {
		// Apply runner-level tag filter: skip if fn doesn't have the required tag.
		if r.TagFilter != "" && !hasFnTag(fn.Tags, r.TagFilter) {
			continue
		}
		// Apply runner-level skip-tag filter: skip if fn has the excluded tag.
		if r.SkipTag != "" && hasFnTag(fn.Tags, r.SkipTag) {
			continue
		}
		// Apply run-name filter for normal tests; apply fuzz filter for fuzz functions.
		if fn.Fuzz {
			if r.RunFilter != "" && !strings.Contains(fn.Name, r.RunFilter) {
				continue
			}
		} else {
			if r.RunFilter != "" && !strings.Contains(fn.Name, r.RunFilter) {
				continue
			}
		}

		if fn.Skip {
			results = append(results, Result{
				TestBlock: td.Name,
				FnName:    fn.Name,
				Passed:    true,
				Error:     "skipped",
				Tags:      fn.Tags,
			})
			continue
		}

		if fn.Fuzz {
			start := time.Now()
			fuzzErr := r.runFuzzFn(testFile, sourceDir, mod, td, fn, suiteSetupStmts)
			dur := time.Since(start)
			if fuzzErr != nil {
				results = append(results, Result{
					TestBlock: td.Name,
					FnName:    fn.Name,
					Passed:    false,
					Error:     fuzzErr.Error(),
					Duration:  dur,
					Tags:      fn.Tags,
				})
			} else {
				results = append(results, Result{
					TestBlock: td.Name,
					FnName:    fn.Name,
					Passed:    true,
					Duration:  dur,
					Tags:      fn.Tags,
				})
			}
			continue
		}

		if fn.Cases != nil && len(fn.Cases.Rows) > 0 {
			// Parameterized test: run once per row.
			for rowIdx, row := range fn.Cases.Rows {
				rowName := fmt.Sprintf("%s[%d]", fn.Name, rowIdx)
				start := time.Now()
				execErr := r.runTestFnWithRow(testFile, sourceDir, mod, td, fn, suiteSetupStmts, fn.Cases.Columns, row)
				dur := time.Since(start)
				if execErr != nil {
					results = append(results, Result{
						TestBlock: td.Name,
						FnName:    rowName,
						Passed:    false,
						Error:     execErr.Error(),
						Duration:  dur,
						Tags:      fn.Tags,
					})
				} else {
					results = append(results, Result{
						TestBlock: td.Name,
						FnName:    rowName,
						Passed:    true,
						Duration:  dur,
						Tags:      fn.Tags,
					})
				}
			}
			continue
		}

		start := time.Now()
		execErr := r.runTestFn(testFile, sourceDir, mod, td, fn, suiteSetupStmts)
		dur := time.Since(start)

		if execErr != nil {
			results = append(results, Result{
				TestBlock: td.Name,
				FnName:    fn.Name,
				Passed:    false,
				Error:     execErr.Error(),
				Duration:  dur,
				Tags:      fn.Tags,
			})
		} else {
			results = append(results, Result{
				TestBlock: td.Name,
				FnName:    fn.Name,
				Passed:    true,
				Duration:  dur,
				Tags:      fn.Tags,
			})
		}
	}

	// Run teardown_suite once after all tests; errors are suppressed.
	if td.TeardownSuite != nil && len(td.TeardownSuite.Body) > 0 {
		ls := lua.NewState()
		defer ls.Close()
		injectAssertBuiltins(ls)
		if chunk, err := buildLuaChunk(td.TeardownSuite.Body); err == nil {
			_ = ls.DoString(chunk)
		}
	}

	return results
}

// runTestFn executes a single test function with the full P2 lifecycle.
func (r *Runner) runTestFn(testFile, sourceDir string, mod *ast.Module, td ast.TestDecl, fn ast.TestFn, suiteSetupStmts []ast.Statement) error {
	ls := lua.NewState()
	defer ls.Close()

	// Enable instruction counting so assert_instructions_le can measure gas delta.
	// Use a very large limit to avoid blocking normal tests.
	ls.SetGasLimit(1 << 48)

	// Install line hook when in coverage mode.
	if r.lineHits != nil {
		hits := r.lineHits
		ls.SetLineHook(func(src string, line int) {
			if line <= 0 {
				return
			}
			if hits[src] == nil {
				hits[src] = make(map[int]bool)
			}
			hits[src][line] = true
		})
	}

	injectAssertBuiltins(ls)

	// 0. Inject test block-level let declarations as Lua globals so they are
	// accessible in setup, teardown, and all test_* function bodies.
	if err := injectTestLets(ls, td.Lets); err != nil {
		return fmt.Errorf("test block let: %w", err)
	}

	bindings := map[string]*contractProxy{}

	// 1. Run setup_suite deploy statements (replayed per test for isolation).
	for _, s := range suiteSetupStmts {
		if s.Kind == "deploy" {
			if err := r.executeDeploy(ls, sourceDir, td, s, bindings); err != nil {
				return fmt.Errorf("setup_suite deploy failed: %w", err)
			}
		}
	}

	// 2. Run per-test setup deploy statements.
	if td.Setup != nil {
		for _, s := range td.Setup.Body {
			if s.Kind == "deploy" {
				if err := r.executeDeploy(ls, sourceDir, td, s, bindings); err != nil {
					return fmt.Errorf("setup deploy failed: %w", err)
				}
			}
		}
	}

	// 3. Register all bindings as Lua globals.
	for name, proxy := range bindings {
		ls.SetGlobal(name, proxy.table)
	}

	// 4. Build and execute the test function body.
	chunk, err := buildLuaChunk(fn.Body)
	if err != nil {
		return fmt.Errorf("building test chunk for %s: %w", fn.Name, err)
	}
	var testErr error
	if fn.Timeout > 0 {
		done := make(chan error, 1)
		go func() { done <- ls.DoString(chunk) }()
		select {
		case testErr = <-done:
		case <-time.After(time.Duration(fn.Timeout) * time.Millisecond):
			testErr = fmt.Errorf("timeout: test exceeded %dms", fn.Timeout)
		}
	} else {
		testErr = ls.DoString(chunk)
	}

	// 5. Run teardown regardless of test outcome.
	if td.Teardown != nil && len(td.Teardown.Body) > 0 {
		teardownChunk, buildErr := buildLuaChunk(td.Teardown.Body)
		if buildErr == nil && teardownChunk != "" {
			if tdErr := ls.DoString(teardownChunk); tdErr != nil && testErr == nil {
				// Test passed but teardown failed: surface the teardown error.
				return fmt.Errorf("teardown: %w", tdErr)
			}
			// If the test already failed, teardown errors are suppressed so the
			// original test failure is the reported outcome.
		}
	}

	return testErr
}

// runTestFnWithRow is like runTestFn but injects a row of case values as Lua
// globals before executing the test body.
func (r *Runner) runTestFnWithRow(testFile, sourceDir string, mod *ast.Module, td ast.TestDecl, fn ast.TestFn, suiteSetupStmts []ast.Statement, columns []string, row []*ast.Expr) error {
	ls := lua.NewState()
	defer ls.Close()

	ls.SetGasLimit(1 << 48)

	if r.lineHits != nil {
		hits := r.lineHits
		ls.SetLineHook(func(src string, line int) {
			if line <= 0 {
				return
			}
			if hits[src] == nil {
				hits[src] = make(map[int]bool)
			}
			hits[src][line] = true
		})
	}

	injectAssertBuiltins(ls)

	if err := injectTestLets(ls, td.Lets); err != nil {
		return fmt.Errorf("test block let: %w", err)
	}

	bindings := map[string]*contractProxy{}

	for _, s := range suiteSetupStmts {
		if s.Kind == "deploy" {
			if err := r.executeDeploy(ls, sourceDir, td, s, bindings); err != nil {
				return fmt.Errorf("setup_suite deploy failed: %w", err)
			}
		}
	}

	if td.Setup != nil {
		for _, s := range td.Setup.Body {
			if s.Kind == "deploy" {
				if err := r.executeDeploy(ls, sourceDir, td, s, bindings); err != nil {
					return fmt.Errorf("setup deploy failed: %w", err)
				}
			}
		}
	}

	for name, proxy := range bindings {
		ls.SetGlobal(name, proxy.table)
	}

	// Inject case column values as Lua globals.
	var rowSb strings.Builder
	for i, col := range columns {
		if i >= len(row) {
			break
		}
		rowSb.WriteString(luaIdent(col) + " = ")
		emitExpr(&rowSb, row[i])
		rowSb.WriteString("\n")
	}
	if rowChunk := rowSb.String(); rowChunk != "" {
		if err := ls.DoString(rowChunk); err != nil {
			return fmt.Errorf("injecting case row: %w", err)
		}
	}

	chunk, err := buildLuaChunk(fn.Body)
	if err != nil {
		return fmt.Errorf("building test chunk for %s: %w", fn.Name, err)
	}
	var testErr error
	if fn.Timeout > 0 {
		done := make(chan error, 1)
		go func() { done <- ls.DoString(chunk) }()
		select {
		case testErr = <-done:
		case <-time.After(time.Duration(fn.Timeout) * time.Millisecond):
			testErr = fmt.Errorf("timeout: test exceeded %dms", fn.Timeout)
		}
	} else {
		testErr = ls.DoString(chunk)
	}

	if td.Teardown != nil && len(td.Teardown.Body) > 0 {
		teardownChunk, buildErr := buildLuaChunk(td.Teardown.Body)
		if buildErr == nil && teardownChunk != "" {
			if tdErr := ls.DoString(teardownChunk); tdErr != nil && testErr == nil {
				return fmt.Errorf("teardown: %w", tdErr)
			}
		}
	}

	return testErr
}

// runFuzzFn runs a fuzz_ test function count times with random parameter values.
func (r *Runner) runFuzzFn(testFile, sourceDir string, mod *ast.Module, td ast.TestDecl, fn ast.TestFn, suiteSetupStmts []ast.Statement) error {
	seed := r.FuzzSeed
	if seed == 0 {
		seed = 42
	}
	count := fn.FuzzCount
	if count == 0 {
		count = r.FuzzCount
		if count == 0 {
			count = 100
		}
	}

	rng := rand.New(rand.NewSource(seed))
	setupNames := collectSetupBindingNames(td)

	for i := 0; i < count; i++ {
		ls := lua.NewState()
		ls.SetGasLimit(1 << 48)
		injectAssertBuiltins(ls)

		if err := injectTestLets(ls, td.Lets); err != nil {
			ls.Close()
			return fmt.Errorf("[fuzz iteration %d, seed %d] test block let: %w", i, seed, err)
		}

		bindings := map[string]*contractProxy{}

		for _, s := range suiteSetupStmts {
			if s.Kind == "deploy" {
				if err := r.executeDeploy(ls, sourceDir, td, s, bindings); err != nil {
					ls.Close()
					return fmt.Errorf("[fuzz iteration %d, seed %d] setup_suite deploy failed: %w", i, seed, err)
				}
			}
		}

		if td.Setup != nil {
			for _, s := range td.Setup.Body {
				if s.Kind == "deploy" {
					if err := r.executeDeploy(ls, sourceDir, td, s, bindings); err != nil {
						ls.Close()
						return fmt.Errorf("[fuzz iteration %d, seed %d] setup deploy failed: %w", i, seed, err)
					}
				}
			}
		}

		for name, proxy := range bindings {
			ls.SetGlobal(name, proxy.table)
		}

		// Inject random values for fuzz parameters not provided by setup.
		fuzzChunk := buildFuzzParamChunk(fn.Params, setupNames, rng)
		if fuzzChunk != "" {
			if err := ls.DoString(fuzzChunk); err != nil {
				ls.Close()
				return fmt.Errorf("[fuzz iteration %d, seed %d] injecting fuzz params: %w", i, seed, err)
			}
		}

		chunk, err := buildLuaChunk(fn.Body)
		if err != nil {
			ls.Close()
			return fmt.Errorf("[fuzz iteration %d, seed %d] building chunk: %w", i, seed, err)
		}
		var runErr error
		if fn.Timeout > 0 {
			done := make(chan error, 1)
			go func() { done <- ls.DoString(chunk) }()
			select {
			case runErr = <-done:
			case <-time.After(time.Duration(fn.Timeout) * time.Millisecond):
				runErr = fmt.Errorf("timeout: test exceeded %dms", fn.Timeout)
			}
		} else {
			runErr = ls.DoString(chunk)
		}
		if runErr != nil {
			ls.Close()
			return fmt.Errorf("[fuzz iteration %d, seed %d] %w", i, seed, runErr)
		}
		ls.Close()
	}
	return nil
}

// collectSetupBindingNames returns the set of names produced by setup and setup_suite returns.
func collectSetupBindingNames(td ast.TestDecl) map[string]bool {
	names := map[string]bool{}
	if td.Setup != nil {
		for _, r := range td.Setup.Returns {
			names[r.Name] = true
		}
	}
	if td.SetupSuite != nil {
		for _, r := range td.SetupSuite.Returns {
			names[r.Name] = true
		}
	}
	return names
}

// buildFuzzParamChunk generates a Lua snippet that assigns random values to
// parameters not already provided by setup bindings.
func buildFuzzParamChunk(params []ast.FieldDecl, setupNames map[string]bool, rng *rand.Rand) string {
	var sb strings.Builder
	for _, p := range params {
		if setupNames[p.Name] {
			continue
		}
		sb.WriteString(luaIdent(p.Name) + " = " + generateFuzzValue(p.Type, rng) + "\n")
	}
	return sb.String()
}

// generateFuzzValue returns a random Lua literal for the given TOL type.
func generateFuzzValue(typ string, rng *rand.Rand) string {
	switch typ {
	case "u256":
		hi := rng.Uint64()
		lo := rng.Uint64()
		return fmt.Sprintf("\"0x%016x%016x\"", hi, lo)
	case "agent":
		b := make([]byte, 32)
		rng.Read(b)
		return "\"0x" + hex.EncodeToString(b) + "\""
	case "bool":
		if rng.Intn(2) == 0 {
			return "false"
		}
		return "true"
	default:
		return "\"0x0000000000000000000000000000000000000000000000000000000000000000\""
	}
}

// contractProxy wraps a compiled contract's bytecode and isolated storage.
type contractProxy struct {
	name    string
	table   *lua.LTable
	storage *lua.LTable
}

// executeDeploy compiles the named contract, calls its constructor, and
// registers a proxy table that routes method calls to the contract's compiled
// Lua functions while switching __tol_storage to this instance's isolated table.
// If contractName matches a mock in td.Mocks, the mock is used instead.
func (r *Runner) executeDeploy(ls *lua.LState, sourceDir string, td ast.TestDecl, s ast.Statement, bindings map[string]*contractProxy) error {
	if s.Expr == nil || s.Expr.Callee == nil {
		return fmt.Errorf("deploy statement has no contract name")
	}
	contractName := s.Expr.Callee.Value

	// Check if this contract name matches a mock declaration.
	if mockDecl := findMock(td.Mocks, contractName); mockDecl != nil {
		return r.executeMockDeploy(ls, mockDecl, s.Name, bindings)
	}

	contractFile := filepath.Join(sourceDir, contractName+".tol")

	src, err := os.ReadFile(contractFile)
	if err != nil {
		return fmt.Errorf("contract file '%s' not found: %w", contractFile, err)
	}

	bc, err := lua.CompileBytecodeWithOptions(src, contractFile, &lua.CompileOptions{
		IncludeSourceMap: true,
	})
	if err != nil {
		return fmt.Errorf("compiling contract '%s': %w", contractName, err)
	}
	// Compile the init artifact (constructor-only) separately from the runtime artifact.
	// The init artifact defines __tol_constructor; the runtime artifact defines oninvoke.
	initBC, initErr := lua.CompileInitBytecode(src, contractFile)
	if initErr != nil {
		return fmt.Errorf("compiling init artifact for '%s': %w", contractName, initErr)
	}

	// Create an isolated storage table for this contract instance.
	// In coverage mode, wrap it with a metatable so every read/write is recorded.
	storage := r.newTrackedStorage(ls)
	ls.SetGlobal("__tol_storage", storage)

	// Load the runtime artifact — defines all contract functions and tos.oninvoke.
	if err := ls.DoBytecode(bc); err != nil {
		return fmt.Errorf("loading contract '%s': %w", contractName, err)
	}
	// Load the init artifact — defines __tol_constructor (and tos.oncreate wrapper).
	// Loaded after runtime so that runtime functions (storage helpers, etc.) are available.
	if initErr == nil {
		if err := ls.DoBytecode(initBC); err != nil {
			return fmt.Errorf("loading init artifact for '%s': %w", contractName, err)
		}
	}

	// Call the constructor with the args from the deploy statement, if one exists.
	if ctor := ls.GetGlobal("__tol_constructor"); ctor != lua.LNil {
		var ctorCall strings.Builder
		ctorCall.WriteString("__tol_constructor(")
		for i, arg := range s.Expr.Args {
			if i > 0 {
				ctorCall.WriteString(", ")
			}
			emitExpr(&ctorCall, arg)
		}
		ctorCall.WriteString(")")
		if ctorErr := ls.DoString(ctorCall.String()); ctorErr != nil {
			return fmt.Errorf("constructor for '%s' failed: %w", contractName, ctorErr)
		}
	}

	// Build a proxy table that wraps each contract function so that:
	//   1. __tol_storage is switched to this instance's isolated table on entry.
	//   2. The real Lua function (defined as a global by DoBytecode) is called.
	//   3. __tol_storage is restored on return (even on error).
	//
	// This is done via a small Lua snippet so that variadic return values pass
	// through without manual Go stack management.
	r.instanceID++
	instKey := fmt.Sprintf("__tol_inst_%d", r.instanceID)
	ls.SetGlobal(instKey, storage)

	proxyTable := ls.NewTable()
	fc, parseErr := ContractFileCoverage(contractFile, src)
	if parseErr == nil && fc != nil && len(fc.Functions) > 0 {
		var sb strings.Builder
		// __is captures the storage pointer for this specific instance.
		fmt.Fprintf(&sb, "local __is = %s\n", instKey)
		// __mp(fn) returns a wrapper that switches storage around the call.
		sb.WriteString("local function __mp(fn)\n")
		sb.WriteString("  return function(...)\n")
		sb.WriteString("    local prev = __tol_storage\n")
		sb.WriteString("    __tol_storage = __is\n")
		sb.WriteString("    local ok, r1, r2, r3, r4 = pcall(fn, ...)\n")
		sb.WriteString("    __tol_storage = prev\n")
		sb.WriteString("    if not ok then error(r1, 2) end\n")
		sb.WriteString("    return r1, r2, r3, r4\n")
		sb.WriteString("  end\n")
		sb.WriteString("end\n")
		sb.WriteString("local __tbl = {}\n")
		for _, fn := range fc.Functions {
			if fn.Name == "constructor" || fn.Name == "fallback" {
				continue
			}
			// Only wrap functions that actually exist as Lua globals after DoBytecode.
			if ls.GetGlobal(fn.Name) == lua.LNil {
				continue
			}
			fmt.Fprintf(&sb, "__tbl.%s = __mp(%s)\n", fn.Name, fn.Name)
		}
		sb.WriteString("__tol_proxy_result = __tbl\n")
		if doErr := ls.DoString(sb.String()); doErr == nil {
			if v := ls.GetGlobal("__tol_proxy_result"); v != lua.LNil {
				if tbl, ok := v.(*lua.LTable); ok {
					proxyTable = tbl
				}
			}
		}
	}

	// Patch tos.emit so contract emit calls are captured in __tol_events.
	_ = ls.DoString(`
if type(tos) == "table" then
  tos.emit = emit
end
`)

	// Expose __storage and __contract on the proxy table so inspect() can read slots.
	ls.SetField(proxyTable, "__storage", storage)
	ls.SetField(proxyTable, "__contract", lua.LString(contractName))

	bindingName := s.Name
	if bindingName != "" {
		bindings[bindingName] = &contractProxy{
			name:    contractName,
			table:   proxyTable,
			storage: storage,
		}
	}
	return nil
}

// newTrackedStorage creates a new Lua table for contract storage.
// When coverage mode is active (r.slotTrackRead != nil), the returned table has
// a metatable that intercepts every __index (read) and __newindex (write) and
// records the accessed key into r.slotTrackRead / r.slotTrackWrite.
//
// The tracked table uses a separate raw backing table so that data is not stored
// in the proxy table itself (which would prevent __newindex from being called on
// repeated writes to the same key in Lua 5.x).
func (r *Runner) newTrackedStorage(ls *lua.LState) *lua.LTable {
	outer := ls.NewTable()
	if r.slotTrackRead == nil {
		return outer
	}

	// rawBacking holds the actual storage values; outer is always empty so
	// __newindex is invoked on every write (Lua 5.x only calls __newindex when
	// the key is absent from the raw table).
	rawBacking := ls.NewTable()
	readMap := r.slotTrackRead
	writeMap := r.slotTrackWrite

	mt := ls.NewTable()
	ls.SetField(mt, "__index", ls.NewFunction(func(L *lua.LState) int {
		key := L.Get(2).String()
		readMap[key] = true
		val := rawBacking.RawGetString(key)
		L.Push(val)
		return 1
	}))
	ls.SetField(mt, "__newindex", ls.NewFunction(func(L *lua.LState) int {
		key := L.Get(2).String()
		val := L.Get(3)
		writeMap[key] = true
		rawBacking.RawSetString(key, val)
		return 0
	}))
	ls.SetMetatable(outer, mt)
	return outer
}

// findMock returns the first MockDecl with the given name, or nil.
func findMock(mocks []ast.MockDecl, name string) *ast.MockDecl {
	for i := range mocks {
		if mocks[i].Name == name {
			return &mocks[i]
		}
	}
	return nil
}

// executeMockDeploy creates a Lua table implementing the mock contract's methods
// and registers it under bindingName in bindings.
func (r *Runner) executeMockDeploy(ls *lua.LState, md *ast.MockDecl, bindingName string, bindings map[string]*contractProxy) error {
	var sb strings.Builder
	tblName := "__tol_mock_" + luaIdent(md.Name)
	sb.WriteString(tblName + " = {}\n")
	for _, mm := range md.Methods {
		sb.WriteString("function " + tblName + "." + luaIdent(mm.Name) + "(")
		for i, param := range mm.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(luaIdent(param.Name))
		}
		sb.WriteString(")\n")
		for _, stmt := range mm.Body {
			if err := emitStmt(&sb, stmt, 1); err != nil {
				return fmt.Errorf("mock method '%s.%s': %w", md.Name, mm.Name, err)
			}
		}
		sb.WriteString("end\n")
	}
	if err := ls.DoString(sb.String()); err != nil {
		return fmt.Errorf("executing mock '%s': %w", md.Name, err)
	}
	if bindingName != "" {
		mockTbl := ls.GetGlobal(tblName)
		if tbl, ok := mockTbl.(*lua.LTable); ok {
			bindings[bindingName] = &contractProxy{
				name:    md.Name,
				table:   tbl,
				storage: ls.NewTable(),
			}
		}
	}
	return nil
}

// injectAssertBuiltins registers assert_eq, assert_ne, assert_gt, assert_lt,
// assert_ge, assert_le, assert_true, assert_false, assert_between, assert_revert,
// assert_event, assert_no_event as Lua global functions that call error() on failure.
// It also sets up the event capture list __tol_events and a global emit function.
func injectAssertBuiltins(ls *lua.LState) {
	// Event capture: __tol_events = {} and a global emit() that appends to it.
	// The global emit is the fallback used by __tol_emit when tos.emit is not set.
	if err := ls.DoString(`
__tol_events = {}
emit = function(name, ...)
  -- __tol_emit passes alternating ("type [indexed]", value) pairs.
  -- Extract only the values (odd-position args starting at arg 2).
  local n = select("#", ...)
  local vals = {}
  for i = 1, n, 2 do
    vals[#vals + 1] = select(i + 1, ...)
  end
  local e = {name=name, args=vals}
  table.insert(__tol_events, e)
end
if type(msg) ~= "table" then msg = {} end
if msg.sender == nil then
  msg.sender = "0x000000000000000000000000000000000000000000000000000000000000dead"
end
if msg.value == nil then msg.value = 0 end
if type(tx) ~= "table" then tx = {} end
if tx.origin == nil then tx.origin = msg.sender end
if type(block) ~= "table" then block = {} end
if block.number == nil then block.number = 0 end
if block.timestamp == nil then block.timestamp = 0 end
if block.timestamp_ms == nil then block.timestamp_ms = 1000000000000 end
`); err != nil {
		// Should never fail; panic would be overkill — continue without event capture.
		_ = err
	}

	ls.Register("assert_event", func(L *lua.LState) int {
		wantName := L.CheckString(1)
		events := L.GetGlobal("__tol_events")
		tbl, ok := events.(*lua.LTable)
		if !ok || tbl == nil {
			L.RaiseError("assert_event: __tol_events is not a table")
			return 0
		}
		// Collect extra positional args to check against event args.
		nExtra := L.GetTop() - 1
		var found bool
		tbl.ForEach(func(_, v lua.LValue) {
			if found {
				return
			}
			ev, isT := v.(*lua.LTable)
			if !isT {
				return
			}
			nameVal := L.GetField(ev, "name")
			if nameVal.String() != wantName {
				return
			}
			// Name matches; check positional args if provided.
			if nExtra == 0 {
				found = true
				return
			}
			argsVal := L.GetField(ev, "args")
			argsTbl, isTbl := argsVal.(*lua.LTable)
			if !isTbl {
				return
			}
			match := true
			for i := 1; i <= nExtra; i++ {
				got := argsTbl.RawGetInt(i)
				want := L.Get(i + 1)
				if !luaValuesEqual(got, want) {
					match = false
					break
				}
			}
			if match {
				found = true
			}
		})
		if !found {
			// Build a description of what was emitted.
			var emitted []string
			tbl.ForEach(func(_, v lua.LValue) {
				ev, isT := v.(*lua.LTable)
				if !isT {
					return
				}
				emitted = append(emitted, ev.RawGetString("name").String())
			})
			if len(emitted) == 0 {
				L.RaiseError("assert_event: no events emitted, expected %q", wantName)
			} else {
				L.RaiseError("assert_event: event %q not found; emitted: %v", wantName, emitted)
			}
		}
		return 0
	})

	ls.Register("assert_no_event", func(L *lua.LState) int {
		events := L.GetGlobal("__tol_events")
		tbl, ok := events.(*lua.LTable)
		if !ok || tbl == nil {
			return 0
		}
		var names []string
		tbl.ForEach(func(_, v lua.LValue) {
			ev, isT := v.(*lua.LTable)
			if !isT {
				return
			}
			names = append(names, ev.RawGetString("name").String())
		})
		if len(names) > 0 {
			L.RaiseError("assert_no_event: expected no events, but got: %v", names)
		}
		return 0
	})

	// __tol_inspect(proxy, slotName) — reads a scalar storage slot value directly.
	ls.Register("__tol_inspect", func(L *lua.LState) int {
		proxy := L.CheckTable(1)
		slotName := L.CheckString(2)
		storageVal := L.GetField(proxy, "__storage")
		storageTbl, ok := storageVal.(*lua.LTable)
		if !ok || storageTbl == nil {
			L.RaiseError("__tol_inspect: proxy has no __storage field")
			return 0
		}
		contractVal := L.GetField(proxy, "__contract")
		contractName := contractVal.String()
		h := sha3.NewLegacyKeccak256()
		h.Write([]byte("tol.slot." + contractName + "." + slotName))
		key := "0x" + hex.EncodeToString(h.Sum(nil))
		val := L.GetField(storageTbl, key)
		if val == lua.LNil {
			L.Push(lua.LUint256Zero)
		} else {
			L.Push(val)
		}
		return 1
	})

	// __tol_gas_used() — returns the number of VM instructions executed so far.
	ls.Register("__tol_gas_used", func(L *lua.LState) int {
		L.Push(lua.Lu256FromUint64(L.GasUsed()))
		return 1
	})

	ls.Register("assert_eq", func(L *lua.LState) int {
		a := L.Get(1)
		b := L.Get(2)
		msg := L.OptString(3, "assert_eq failed")
		if !luaValuesEqual(a, b) {
			L.RaiseError("%s: expected %v == %v", msg, a, b)
		}
		return 0
	})
	ls.Register("assert_ne", func(L *lua.LState) int {
		a := L.Get(1)
		b := L.Get(2)
		msg := L.OptString(3, "assert_ne failed")
		if luaValuesEqual(a, b) {
			L.RaiseError("%s: expected %v != %v", msg, a, b)
		}
		return 0
	})
	ls.Register("assert_gt", func(L *lua.LState) int {
		a := L.CheckUint256(1)
		b := L.CheckUint256(2)
		msg := L.OptString(3, "assert_gt failed")
		if lua.Lu256Cmp(a, b) <= 0 {
			L.RaiseError("%s: expected %v > %v", msg, a, b)
		}
		return 0
	})
	ls.Register("assert_lt", func(L *lua.LState) int {
		a := L.CheckUint256(1)
		b := L.CheckUint256(2)
		msg := L.OptString(3, "assert_lt failed")
		if lua.Lu256Cmp(a, b) >= 0 {
			L.RaiseError("%s: expected %v < %v", msg, a, b)
		}
		return 0
	})
	ls.Register("assert_between", func(L *lua.LState) int {
		v := L.CheckUint256(1)
		lo := L.CheckUint256(2)
		hi := L.CheckUint256(3)
		msg := L.OptString(4, "assert_between failed")
		if lua.Lu256Cmp(v, lo) < 0 || lua.Lu256Cmp(v, hi) > 0 {
			L.RaiseError("%s: expected %v in [%v, %v]", msg, v, lo, hi)
		}
		return 0
	})
	ls.Register("assert_ge", func(L *lua.LState) int {
		a := L.CheckUint256(1)
		b := L.CheckUint256(2)
		msg := L.OptString(3, "assert_ge failed")
		if lua.Lu256Cmp(a, b) < 0 {
			L.RaiseError("%s: expected %v >= %v", msg, a, b)
		}
		return 0
	})
	ls.Register("assert_le", func(L *lua.LState) int {
		a := L.CheckUint256(1)
		b := L.CheckUint256(2)
		msg := L.OptString(3, "assert_le failed")
		if lua.Lu256Cmp(a, b) > 0 {
			L.RaiseError("%s: expected %v <= %v", msg, a, b)
		}
		return 0
	})
	ls.Register("assert_true", func(L *lua.LState) int {
		v := L.Get(1)
		msg := L.OptString(2, "assert_true failed")
		if v == lua.LFalse || v == lua.LNil {
			L.RaiseError("%s: expected true, got %v", msg, v)
		}
		return 0
	})
	ls.Register("assert_false", func(L *lua.LState) int {
		v := L.Get(1)
		msg := L.OptString(2, "assert_false failed")
		if v != lua.LFalse && v != lua.LNil {
			L.RaiseError("%s: expected false, got %v", msg, v)
		}
		return 0
	})
	ls.Register("assert_revert", func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		wantMsg := L.OptString(2, "")
		err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		})
		if err == nil {
			if wantMsg != "" {
				L.RaiseError("assert_revert failed: expected revert with message containing %q but call succeeded", wantMsg)
			} else {
				L.RaiseError("assert_revert failed: expected revert but call succeeded")
			}
			return 0
		}
		// Revert occurred; if a message was specified, verify it is contained in the error.
		if wantMsg != "" {
			gotMsg := err.Error()
			if !strings.Contains(gotMsg, wantMsg) {
				L.RaiseError("assert_revert: expected message containing %q, got %q", wantMsg, gotMsg)
			}
		}
		return 0
	})
}

func luaValuesEqual(a, b lua.LValue) bool {
	if a.Type() != b.Type() {
		return false
	}
	return a.String() == b.String()
}

// injectTestLets sets test block-level let declarations as Lua globals in ls.
// Unlike function-body let (emitted as Lua locals), block-level lets must be
// globals so they are accessible in both setup and all test_* bodies that run
// in the same Lua state.
func injectTestLets(ls *lua.LState, lets []ast.Statement) error {
	if len(lets) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, s := range lets {
		if s.Kind != "let" {
			continue
		}
		sb.WriteString(luaIdent(s.Name) + " = ")
		emitExpr(&sb, s.Expr)
		sb.WriteString("\n")
	}
	chunk := sb.String()
	if chunk == "" {
		return nil
	}
	return ls.DoString(chunk)
}

// buildLuaChunk translates a slice of TOL statements into a Lua source string.
// This is a minimal P0 translator that handles common test patterns.
func buildLuaChunk(stmts []ast.Statement) (string, error) {
	var sb strings.Builder
	for _, s := range stmts {
		if err := emitStmt(&sb, s, 0); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}

func emitStmt(sb *strings.Builder, s ast.Statement, depth int) error {
	indent := strings.Repeat("  ", depth)
	switch s.Kind {
	case "let":
		sb.WriteString(indent + "local " + luaIdent(s.Name))
		if s.Expr != nil {
			sb.WriteString(" = ")
			emitExpr(sb, s.Expr)
		}
		sb.WriteString("\n")
	case "set":
		if s.Target != nil {
			emitExpr(sb, s.Target)
		}
		sb.WriteString(" = ")
		if s.Expr != nil {
			emitExpr(sb, s.Expr)
		}
		sb.WriteString("\n")
	case "expr":
		if s.Expr != nil {
			sb.WriteString(indent)
			emitExpr(sb, s.Expr)
			sb.WriteString("\n")
		}
	case "return":
		sb.WriteString(indent + "return")
		if s.Expr != nil {
			sb.WriteString(" ")
			emitExpr(sb, s.Expr)
		}
		sb.WriteString("\n")
	case "require", "assert":
		sb.WriteString(indent + "if not (")
		if s.Expr != nil {
			emitExpr(sb, s.Expr)
		} else {
			sb.WriteString("false")
		}
		sb.WriteString(") then error(")
		if strings.TrimSpace(s.Text) != "" {
			sb.WriteString(s.Text)
		} else {
			sb.WriteString(fmt.Sprintf("%q", s.Kind+" failed"))
		}
		sb.WriteString(") end\n")
	case "revert":
		sb.WriteString(indent + "error(")
		if s.Expr != nil {
			emitExpr(sb, s.Expr)
		} else {
			sb.WriteString(fmt.Sprintf("%q", "revert"))
		}
		sb.WriteString(")\n")
	case "if":
		sb.WriteString(indent + "if ")
		if s.Cond != nil {
			emitExpr(sb, s.Cond)
		}
		sb.WriteString(" then\n")
		for _, ts := range s.Then {
			emitStmt(sb, ts, depth+1)
		}
		if len(s.Else) > 0 {
			sb.WriteString(indent + "else\n")
			for _, es := range s.Else {
				emitStmt(sb, es, depth+1)
			}
		}
		sb.WriteString(indent + "end\n")
	case "while":
		sb.WriteString(indent + "while ")
		if s.Cond != nil {
			emitExpr(sb, s.Cond)
		}
		sb.WriteString(" do\n")
		for _, bs := range s.Body {
			emitStmt(sb, bs, depth+1)
		}
		sb.WriteString(indent + "end\n")
	case "break":
		sb.WriteString(indent + "break\n")
	case "with":
		scope, key, value, ok := parseWithAssignment(s.Cond)
		if !ok {
			return fmt.Errorf("invalid with statement: expected assignment like msg.sender = <expr>")
		}
		sb.WriteString(indent + "do\n")
		sb.WriteString(indent + "  local __with_tbl = " + scope + "\n")
		sb.WriteString(indent + "  if type(__with_tbl) ~= \"table\" then\n")
		sb.WriteString(indent + "    __with_tbl = {}\n")
		sb.WriteString(indent + "    " + scope + " = __with_tbl\n")
		sb.WriteString(indent + "  end\n")
		sb.WriteString(indent + "  local __with_prev = __with_tbl[" + fmt.Sprintf("%q", key) + "]\n")
		sb.WriteString(indent + "  __with_tbl[" + fmt.Sprintf("%q", key) + "] = ")
		emitExpr(sb, value)
		sb.WriteString("\n")
		sb.WriteString(indent + "  local __with_ok, __with_err = pcall(function()\n")
		for _, bs := range s.Body {
			emitStmt(sb, bs, depth+2)
		}
		sb.WriteString(indent + "  end)\n")
		sb.WriteString(indent + "  __with_tbl[" + fmt.Sprintf("%q", key) + "] = __with_prev\n")
		sb.WriteString(indent + "  if not __with_ok then error(__with_err) end\n")
		sb.WriteString(indent + "end\n")
	case "assert_revert":
		sb.WriteString(indent + "assert_revert(function()\n")
		for _, bs := range s.Body {
			emitStmt(sb, bs, depth+1)
		}
		sb.WriteString(indent + "end")
		if s.Expr != nil {
			sb.WriteString(", ")
			emitExpr(sb, s.Expr)
		}
		sb.WriteString(")\n")
	case "assert_all":
		// assert_all { stmts... } — runs every statement inside via pcall and
		// collects all failures. Reports a combined error if any failed.
		n := len(s.Body)
		sb.WriteString(indent + "do\n")
		sb.WriteString(indent + "  local __aa_errs = {}\n")
		sb.WriteString(indent + "  local __aa_ok, __aa_err\n")
		for _, bs := range s.Body {
			var inner strings.Builder
			emitStmt(&inner, bs, 0)
			chunk := strings.TrimRight(inner.String(), "\n")
			sb.WriteString(indent + "  __aa_ok, __aa_err = pcall(function()\n")
			// indent each line of the inner chunk
			for _, line := range strings.Split(chunk, "\n") {
				sb.WriteString(indent + "    " + line + "\n")
			}
			sb.WriteString(indent + "  end)\n")
			sb.WriteString(indent + "  if not __aa_ok then __aa_errs[#__aa_errs+1] = tostring(__aa_err) end\n")
		}
		sb.WriteString(fmt.Sprintf("%s  if #__aa_errs > 0 then\n", indent))
		sb.WriteString(fmt.Sprintf("%s    error(\"assert_all failed (\" .. #__aa_errs .. \" of %d):\\n  \" .. table.concat(__aa_errs, \"\\n  \"))\n", indent, n))
		sb.WriteString(indent + "  end\n")
		sb.WriteString(indent + "end\n")
	case "assert_instructions_le":
		// assert_instructions_le(N) { stmts... }
		// Measures gas used by the body and fails if it exceeds N.
		sb.WriteString(indent + "do\n")
		sb.WriteString(indent + "  local __ail_before = __tol_gas_used()\n")
		for _, bs := range s.Body {
			emitStmt(sb, bs, depth+1)
		}
		sb.WriteString(indent + "  local __ail_after = __tol_gas_used()\n")
		sb.WriteString(indent + "  local __ail_delta = __ail_after - __ail_before\n")
		sb.WriteString(indent + "  local __ail_limit = ")
		if s.Expr != nil {
			emitExpr(sb, s.Expr)
		} else {
			sb.WriteString("0")
		}
		sb.WriteString("\n")
		sb.WriteString(indent + "  if __ail_delta > __ail_limit then\n")
		sb.WriteString(indent + "    error(string.format(\"assert_instructions_le: %d instructions used, limit %d\", __ail_delta, __ail_limit))\n")
		sb.WriteString(indent + "  end\n")
		sb.WriteString(indent + "end\n")
	case "deploy":
		// deploy statements are handled at the harness level, not as inline Lua.
		// If one appears inside a test body (rather than setup), it's a no-op here.
	default:
		// Unknown statement kinds are silently skipped in P0.
	}
	return nil
}

func emitExpr(sb *strings.Builder, e *ast.Expr) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "ident":
		sb.WriteString(luaIdent(e.Value))
	case "number":
		sb.WriteString(e.Value)
	case "string":
		sb.WriteString(e.Value)
	case "call":
		emitExpr(sb, e.Callee)
		sb.WriteString("(")
		for i, a := range e.Args {
			if i > 0 {
				sb.WriteString(", ")
			}
			emitExpr(sb, a)
		}
		sb.WriteString(")")
	case "member":
		emitExpr(sb, e.Object)
		sb.WriteString(".")
		sb.WriteString(luaIdent(e.Member))
	case "index":
		emitExpr(sb, e.Object)
		sb.WriteString("[")
		emitExpr(sb, e.Index)
		sb.WriteString("]")
	case "binary":
		emitExpr(sb, e.Left)
		sb.WriteString(" " + luaBinaryOp(e.Op) + " ")
		emitExpr(sb, e.Right)
	case "unary":
		sb.WriteString(luaUnaryOp(e.Op))
		emitExpr(sb, e.Right)
	case "paren":
		sb.WriteString("(")
		emitExpr(sb, e.Left)
		sb.WriteString(")")
	case "assign":
		emitExpr(sb, e.Left)
		sb.WriteString(" = ")
		emitExpr(sb, e.Right)
	case "inspect":
		sb.WriteString("__tol_inspect(")
		emitExpr(sb, e.Object)
		sb.WriteString(", \"" + e.Member + "\")")
	default:
		sb.WriteString("nil")
	}
}

func stripParenExpr(e *ast.Expr) *ast.Expr {
	for e != nil && e.Kind == "paren" {
		e = e.Left
	}
	return e
}

func parseWithAssignment(cond *ast.Expr) (scope, key string, value *ast.Expr, ok bool) {
	root := stripParenExpr(cond)
	if root == nil || root.Kind != "assign" || root.Right == nil {
		return "", "", nil, false
	}
	left := stripParenExpr(root.Left)
	if left == nil || left.Kind != "member" {
		return "", "", nil, false
	}
	obj := stripParenExpr(left.Object)
	if obj == nil || obj.Kind != "ident" {
		return "", "", nil, false
	}
	scope = strings.TrimSpace(obj.Value)
	switch scope {
	case "msg", "tx", "block":
		// supported test context scopes
	default:
		return "", "", nil, false
	}
	key = strings.TrimSpace(left.Member)
	if key == "" {
		return "", "", nil, false
	}
	return scope, key, root.Right, true
}

// luaIdent maps a TOL identifier to a safe Lua identifier.
func luaIdent(name string) string {
	if name == "" {
		return "_"
	}
	return name
}

func luaBinaryOp(op string) string {
	switch op {
	case "&&":
		return "and"
	case "||":
		return "or"
	case "!=":
		return "~="
	case "**":
		return "^" // Lua power operator → OP_POW → lu256Pow
	default:
		return op
	}
}

func luaUnaryOp(op string) string {
	switch op {
	case "!":
		return "not "
	case "~":
		return "~"
	default:
		return op
	}
}

// hasFnTag reports whether tags contains the given tag name.
func hasFnTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// testFileResolver returns a sema.FileResolver that resolves import paths
// relative to the directory containing the given test file.
// Supports local paths (.tol, .abi, .toc, .tor) and github.com/@rev imports.
func testFileResolver(testFile string) sema.FileResolver {
	return lua.NewOSFileResolver(filepath.Dir(testFile))
}
