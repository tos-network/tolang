package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	toltest "github.com/tos-network/tolang/tol/test"
)

// cmdTest implements the `tol test` subcommand.
//
// Usage: tol test [flags] [path...]
//
// Exit codes:
//
//	0 - all tests passed
//	1 - one or more tests failed
//	2 - compile/parse error in a test or contract file
//	3 - coverage below -covermin threshold
func cmdTest(args []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var runFilter, tagFilter, skipTag, fuzzFilter string
	var verbose, cover bool
	var coverMin int

	fs.StringVar(&runFilter, "run", "", "run only tests whose name contains this substring")
	fs.StringVar(&tagFilter, "tag", "", "run only tests with this tag (comma-separated for multiple)")
	fs.StringVar(&skipTag, "skip", "", "skip tests with this tag")
	fs.BoolVar(&verbose, "v", false, "verbose output (print PASS results too)")
	fs.BoolVar(&cover, "cover", false, "enable coverage collection")
	fs.IntVar(&coverMin, "covermin", 0, "fail if function coverage % is below this value (0 = disabled)")
	fs.StringVar(&fuzzFilter, "fuzz", "", "run only fuzz functions whose name contains this substring")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol test [flags] [path...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "path... defaults to ./... (current directory, recursive).")
		fmt.Fprintln(os.Stderr, "Each path is a *_test.tol file or a directory (searched recursively).")
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	// Collect test files.
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"./..."}
	}

	testFiles, err := collectTestFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}

	// Build runner with filters.
	runner := &toltest.Runner{
		RunFilter: runFilter,
		TagFilter: tagFilter,
		SkipTag:   skipTag,
	}

	// When -fuzz is set, override RunFilter to target fuzz functions only.
	// We set RunFilter to the fuzz pattern; normal test_ functions will not
	// match because we'll also require the fuzz_ prefix check in the output
	// filtering below.
	if fuzzFilter != "" {
		runner.RunFilter = fuzzFilter
	}

	totalTests := 0
	totalFailures := 0
	anyCompileError := false

	var allCoverReports []*toltest.CoverageReport
	var allCoverFiles []string // parallel to allCoverReports: the test file path

	for _, testFile := range testFiles {
		var results []toltest.Result
		var covReport *toltest.CoverageReport

		if cover || coverMin > 0 {
			var runErr error
			results, covReport, runErr = runner.RunFileWithCoverage(testFile)
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "compile error in %s: %v\n", testFile, runErr)
				anyCompileError = true
				continue
			}
			allCoverReports = append(allCoverReports, covReport)
			allCoverFiles = append(allCoverFiles, testFile)
		} else {
			var runErr error
			results, runErr = runner.RunFile(testFile)
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "compile error in %s: %v\n", testFile, runErr)
				anyCompileError = true
				continue
			}
		}

		// When -fuzz is set, filter results to only fuzz_ functions.
		if fuzzFilter != "" {
			filtered := results[:0]
			for _, r := range results {
				if strings.HasPrefix(r.FnName, "fuzz_") {
					filtered = append(filtered, r)
				}
			}
			results = filtered
		}

		fileTests := 0
		fileFailures := 0

		for _, res := range results {
			fileTests++
			if res.Error == "skipped" {
				// Print SKIP line always (it is never verbose-only).
				fmt.Printf("--- SKIP: %s/%s   (#[skip])\n", res.TestBlock, res.FnName)
				continue
			}
			if res.Passed {
				if verbose {
					fmt.Printf("--- PASS: %s/%s   (%.3fs)\n",
						res.TestBlock, res.FnName, res.Duration.Seconds())
				}
			} else {
				fileFailures++
				fmt.Printf("--- FAIL: %s/%s\n", res.TestBlock, res.FnName)
				if res.Error != "" {
					// Indent the error message.
					for _, line := range strings.Split(res.Error, "\n") {
						fmt.Printf("    %s\n", line)
					}
				}
			}
		}

		totalTests += fileTests
		totalFailures += fileFailures

		// Print per-file summary line.
		rel := relOrAbs(testFile)
		if fileFailures > 0 {
			fmt.Printf("FAIL    %s    (%d tests, %d failures)\n", rel, fileTests, fileFailures)
		} else {
			fmt.Printf("ok      %s    (%d tests, %d failures)\n", rel, fileTests, fileFailures)
		}
	}

	// Print coverage report.
	if (cover || coverMin > 0) && len(allCoverReports) > 0 {
		fmt.Println("\nCoverage:")
		for i, covReport := range allCoverReports {
			testFileRel := relOrAbs(allCoverFiles[i])
			for _, fc := range covReport.Files {
				total, called := 0, 0
				ccSum := 0
				for _, fn := range fc.Functions {
					total++
					if fn.Called {
						called++
					}
					ccSum += fn.CC
				}
				pct := 0.0
				if total > 0 {
					pct = float64(called) / float64(total) * 100
				}
				ccAvg := 0.0
				if total > 0 {
					ccAvg = float64(ccSum) / float64(total)
				}
				contractRel := relOrAbs(fc.ContractFile)
				fmt.Printf("  %s -> %s   function: %d/%d (%.0f%%)   CC avg: %.1f\n",
					testFileRel, contractRel, called, total, pct, ccAvg)
			}
		}
	}

	// Determine exit code.
	if anyCompileError {
		return 2
	}
	if coverMin > 0 {
		for _, covReport := range allCoverReports {
			if covReport.FunctionPercent() < float64(coverMin) {
				fmt.Fprintf(os.Stderr, "FAIL: function coverage %.0f%% is below -covermin %d%%\n",
					covReport.FunctionPercent(), coverMin)
				return 3
			}
		}
	}
	if totalFailures > 0 {
		return 1
	}
	return 0
}

// collectTestFiles expands a list of path arguments (files or directories, or
// "./..." recursive glob) into a deduplicated list of *_test.tol file paths.
func collectTestFiles(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var result []string

	addFile := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			result = append(result, abs)
		}
	}

	for _, p := range paths {
		// Handle ./... or any path ending with /... as recursive dir search.
		if strings.HasSuffix(p, "/...") || p == "./..." || p == "..." {
			dir := strings.TrimSuffix(p, "/...")
			if dir == "." || dir == "" || dir == "..." {
				dir = "."
			}
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return nil, fmt.Errorf("resolving path %q: %w", p, err)
			}
			if err := filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(path, "_test.tol") {
					addFile(path)
				}
				return nil
			}); err != nil {
				return nil, fmt.Errorf("walking %q: %w", dir, err)
			}
			continue
		}

		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", p, err)
		}

		if info.IsDir() {
			if err := filepath.WalkDir(p, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !d.IsDir() && strings.HasSuffix(path, "_test.tol") {
					addFile(path)
				}
				return nil
			}); err != nil {
				return nil, fmt.Errorf("walking %q: %w", p, err)
			}
		} else {
			if !strings.HasSuffix(p, "_test.tol") {
				return nil, fmt.Errorf("file %q is not a _test.tol file", p)
			}
			addFile(p)
		}
	}

	return result, nil
}

// relOrAbs returns a relative path from cwd if possible, otherwise the absolute.
func relOrAbs(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	// Prefix with "./" for readability (similar to go test output).
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}
