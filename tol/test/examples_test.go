package toltest

import (
	"path/filepath"
	"testing"
)

func TestTRC20ExampleSuite(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	exampleDir := filepath.Join(repoRoot, "examples", "trc20_tol")
	testFile := filepath.Join(exampleDir, "trc20_test.tol")

	r := &Runner{SourceDir: exampleDir}
	results, err := r.RunFile(testFile)
	if err != nil {
		t.Fatalf("unexpected error running %s: %v", testFile, err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 test results, got %d: %+v", len(results), results)
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("example test failed: %s.%s: %s", res.TestBlock, res.FnName, res.Error)
		}
	}
}
