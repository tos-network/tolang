package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	lua "github.com/tos-network/tolang"
)

// repoRoot returns the repository root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// verify/verify_test.go -> repo root is parent of verify/
	return filepath.Dir(filepath.Dir(file))
}

// findFirstTol returns the path to the first .tol file in a directory
// (skipping test files).
func findFirstTol(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tol") && !strings.HasSuffix(e.Name(), "_test.tol") {
			return filepath.Join(dir, e.Name())
		}
	}
	t.Fatalf("no .tol file found in %s", dir)
	return ""
}

func TestVerifySourceMatch(t *testing.T) {
	root := repoRoot(t)
	sourcePath := findFirstTol(t, filepath.Join(root, "examples", "policy_wallet"))

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Compile source to artifact.
	tocBytes, err := lua.CompileArtifact(source, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify: source should match its own compiled artifact.
	result, err := VerifySource(sourcePath, tocBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Match {
		t.Errorf("expected match, got mismatch: %v", result.Errors)
	}
	if !result.BytecodeMatch {
		t.Errorf("bytecode mismatch: expected %s, got %s",
			result.ExpectedBytecodeHash, result.ActualBytecodeHash)
	}
	if !result.ABIMatch {
		t.Errorf("ABI mismatch: expected %s, got %s",
			result.ExpectedABIHash, result.ActualABIHash)
	}
	if result.ContractName == "" {
		t.Error("expected non-empty contract name")
	}
}

func TestVerifySourceMismatch(t *testing.T) {
	root := repoRoot(t)
	sourcePath := findFirstTol(t, filepath.Join(root, "examples", "policy_wallet"))

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Compile the original source.
	tocBytes, err := lua.CompileArtifact(source, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Write a modified source to a temp file: add a harmless comment that
	// won't change compilation, then add a state variable that WILL change it.
	modified := append([]byte(nil), source...)
	// Inject an extra storage variable into the contract body.
	// Find first '{' after "contract" keyword and insert after it.
	idx := strings.Index(string(modified), "contract ")
	if idx < 0 {
		t.Fatal("could not find 'contract' keyword")
	}
	braceIdx := strings.Index(string(modified[idx:]), "{")
	if braceIdx < 0 {
		t.Fatal("could not find opening brace")
	}
	insertAt := idx + braceIdx + 1
	injection := "\n    u256 __verify_test_dummy;\n"
	modifiedSrc := make([]byte, 0, len(modified)+len(injection))
	modifiedSrc = append(modifiedSrc, modified[:insertAt]...)
	modifiedSrc = append(modifiedSrc, []byte(injection)...)
	modifiedSrc = append(modifiedSrc, modified[insertAt:]...)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, filepath.Base(sourcePath))
	if err := os.WriteFile(tmpFile, modifiedSrc, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify: modified source should NOT match original artifact.
	result, err := VerifySource(tmpFile, tocBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Match {
		t.Error("expected mismatch for modified source, got match")
	}
	if result.BytecodeMatch {
		t.Error("expected bytecode mismatch for modified source")
	}
}

func TestVerifyDirectoryPolicyWallet(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "examples", "policy_wallet")

	report, err := VerifyDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) == 0 {
		t.Fatal("expected at least one result")
	}
	if !report.AllMatch {
		for _, r := range report.Results {
			if !r.Match {
				t.Errorf("contract %s (%s) did not match: %v",
					r.ContractName, r.SourcePath, r.Errors)
			}
		}
	}
	if report.SchemaVersion != schemaVersion {
		t.Errorf("expected schema version %s, got %s", schemaVersion, report.SchemaVersion)
	}
}

func TestVerifyDirectoryAgentEconomy(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "examples", "agent_economy")

	report, err := VerifyDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) == 0 {
		t.Fatal("expected at least one result")
	}
	if !report.AllMatch {
		for _, r := range report.Results {
			if !r.Match {
				t.Errorf("contract %s (%s) did not match: %v",
					r.ContractName, r.SourcePath, r.Errors)
			}
		}
	}
}

func TestFormatReport(t *testing.T) {
	report := &VerificationReport{
		SchemaVersion: schemaVersion,
		Timestamp:     "2026-01-01T00:00:00Z",
		AllMatch:      true,
		Results: []VerificationResult{
			{
				ContractName:         "TestContract",
				SourcePath:           "/tmp/Test.tol",
				Match:                true,
				BytecodeMatch:        true,
				ABIMatch:             true,
				SourceHash:           "0xabc",
				ExpectedBytecodeHash: "0x111",
				ActualBytecodeHash:   "0x111",
				ExpectedABIHash:      "0x222",
				ActualABIHash:        "0x222",
			},
		},
	}

	output := FormatReport(report)
	if !strings.Contains(output, "PASS") {
		t.Error("expected PASS in report")
	}
	if !strings.Contains(output, "TestContract") {
		t.Error("expected contract name in report")
	}
	if !strings.Contains(output, "ALL CONTRACTS VERIFIED") {
		t.Error("expected ALL CONTRACTS VERIFIED in report")
	}
	if !strings.Contains(output, schemaVersion) {
		t.Error("expected schema version in report")
	}
}

func TestFormatReportFailure(t *testing.T) {
	report := &VerificationReport{
		SchemaVersion: schemaVersion,
		Timestamp:     "2026-01-01T00:00:00Z",
		AllMatch:      false,
		Results: []VerificationResult{
			{
				ContractName:         "FailContract",
				SourcePath:           "/tmp/Fail.tol",
				Match:                false,
				BytecodeMatch:        false,
				ABIMatch:             true,
				ExpectedBytecodeHash: "0x111",
				ActualBytecodeHash:   "0x999",
				ExpectedABIHash:      "0x222",
				ActualABIHash:        "0x222",
				Errors:               []string{"bytecode mismatch"},
			},
		},
	}

	output := FormatReport(report)
	if !strings.Contains(output, "FAIL") {
		t.Error("expected FAIL in report")
	}
	if !strings.Contains(output, "VERIFICATION FAILED") {
		t.Error("expected VERIFICATION FAILED in report")
	}
	if !strings.Contains(output, "MISMATCH") {
		t.Error("expected MISMATCH in report")
	}
}

func TestVerificationReportAllMatch(t *testing.T) {
	// AllMatch should be true only when all results match.
	report := &VerificationReport{
		AllMatch: true,
		Results: []VerificationResult{
			{Match: true},
			{Match: true},
		},
	}
	for _, r := range report.Results {
		if !r.Match {
			report.AllMatch = false
			break
		}
	}
	if !report.AllMatch {
		t.Error("AllMatch should be true when all results match")
	}

	// Now with a failing result.
	report.Results = append(report.Results, VerificationResult{Match: false})
	report.AllMatch = true
	for _, r := range report.Results {
		if !r.Match {
			report.AllMatch = false
			break
		}
	}
	if report.AllMatch {
		t.Error("AllMatch should be false when any result fails")
	}
}
