package verify

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/tos-network/tolang"
	"golang.org/x/crypto/sha3"
)

const schemaVersion = "0.1.0"

// keccak256Hex returns the keccak-256 hash of data as a "0x"-prefixed hex string.
func keccak256Hex(data []byte) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(data)
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

// VerifySource compiles source and compares against deployed artifact bytes.
// sourcePath is the filesystem path to the .tol source file.
// deployedTocBytes is the raw .toc artifact bytes from the deployed contract.
func VerifySource(sourcePath string, deployedTocBytes []byte) (*VerificationResult, error) {
	result := &VerificationResult{
		SourcePath: sourcePath,
	}

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("read source: %v", err))
		return result, err
	}
	result.SourceHash = keccak256Hex(source)

	// Decode the deployed artifact.
	deployed, err := lua.DecodeArtifact(deployedTocBytes)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("decode deployed artifact: %v", err))
		return result, err
	}
	result.ContractName = deployed.ContractName

	// Compile source to artifact.
	compiledBytes, err := lua.CompileArtifact(source, sourcePath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("compile source: %v", err))
		return result, err
	}
	compiled, err := lua.DecodeArtifact(compiledBytes)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("decode compiled artifact: %v", err))
		return result, err
	}

	// Compare bytecode hashes.
	result.ExpectedBytecodeHash = keccak256Hex(compiled.Bytecode)
	result.ActualBytecodeHash = keccak256Hex(deployed.Bytecode)
	result.BytecodeMatch = result.ExpectedBytecodeHash == result.ActualBytecodeHash

	// Compare ABI hashes.
	result.ExpectedABIHash = keccak256Hex(compiled.ABIJSON)
	result.ActualABIHash = keccak256Hex(deployed.ABIJSON)
	result.ABIMatch = result.ExpectedABIHash == result.ActualABIHash

	result.Match = result.BytecodeMatch && result.ABIMatch
	if !result.BytecodeMatch {
		result.Errors = append(result.Errors, "bytecode mismatch")
	}
	if !result.ABIMatch {
		result.Errors = append(result.Errors, "ABI mismatch")
	}
	return result, nil
}

// VerifyArtifact compares two .toc artifacts byte-for-byte via their
// bytecode and ABI hashes.
func VerifyArtifact(expected, actual []byte) (*VerificationResult, error) {
	result := &VerificationResult{}

	exp, err := lua.DecodeArtifact(expected)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("decode expected artifact: %v", err))
		return result, err
	}
	act, err := lua.DecodeArtifact(actual)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("decode actual artifact: %v", err))
		return result, err
	}

	result.ContractName = exp.ContractName

	result.ExpectedBytecodeHash = keccak256Hex(exp.Bytecode)
	result.ActualBytecodeHash = keccak256Hex(act.Bytecode)
	result.BytecodeMatch = result.ExpectedBytecodeHash == result.ActualBytecodeHash

	result.ExpectedABIHash = keccak256Hex(exp.ABIJSON)
	result.ActualABIHash = keccak256Hex(act.ABIJSON)
	result.ABIMatch = result.ExpectedABIHash == result.ActualABIHash

	result.Match = result.BytecodeMatch && result.ABIMatch
	if !result.BytecodeMatch {
		result.Errors = append(result.Errors, "bytecode mismatch")
	}
	if !result.ABIMatch {
		result.Errors = append(result.Errors, "ABI mismatch")
	}
	return result, nil
}

// VerifyPackage compares two .tor packages by comparing the bytecode and ABI
// of each contract entry found in both packages.
func VerifyPackage(expected, actual []byte) (*VerificationResult, error) {
	result := &VerificationResult{}

	exp, err := lua.DecodePackage(expected)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("decode expected package: %v", err))
		return result, err
	}
	act, err := lua.DecodePackage(actual)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("decode actual package: %v", err))
		return result, err
	}

	// Compare all .toc entries in the packages.
	result.BytecodeMatch = true
	result.ABIMatch = true

	for path, expData := range exp.Files {
		if !strings.HasSuffix(path, ".toc") {
			continue
		}
		actData, ok := act.Files[path]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("missing file in actual package: %s", path))
			result.BytecodeMatch = false
			result.ABIMatch = false
			continue
		}
		sub, err := VerifyArtifact(expData, actData)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			result.BytecodeMatch = false
			result.ABIMatch = false
			continue
		}
		if !sub.BytecodeMatch {
			result.BytecodeMatch = false
			result.Errors = append(result.Errors, fmt.Sprintf("%s: bytecode mismatch", path))
		}
		if !sub.ABIMatch {
			result.ABIMatch = false
			result.Errors = append(result.Errors, fmt.Sprintf("%s: ABI mismatch", path))
		}
	}

	result.ExpectedBytecodeHash = keccak256Hex(expected)
	result.ActualBytecodeHash = keccak256Hex(actual)
	result.ExpectedABIHash = keccak256Hex(exp.ManifestJSON)
	result.ActualABIHash = keccak256Hex(act.ManifestJSON)

	result.Match = result.BytecodeMatch && result.ABIMatch
	return result, nil
}

// VerifyDirectory verifies all .tol files in a directory against their
// corresponding .toc files (compiled fresh from source). Each .tol file
// is compiled and compared against itself — this validates that compilation
// is deterministic and the source produces the expected artifact.
func VerifyDirectory(dir string) (*VerificationReport, error) {
	report := &VerificationReport{
		SchemaVersion: schemaVersion,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		AllMatch:      true,
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".tol") {
			continue
		}
		// Skip test files.
		if strings.HasSuffix(entry.Name(), "_test.tol") {
			continue
		}

		sourcePath := filepath.Join(dir, entry.Name())
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			report.Results = append(report.Results, VerificationResult{
				SourcePath: sourcePath,
				Errors:     []string{fmt.Sprintf("read source: %v", err)},
			})
			report.AllMatch = false
			continue
		}

		// Compile source to artifact.
		tocBytes, err := lua.CompileArtifact(source, sourcePath)
		if err != nil {
			report.Results = append(report.Results, VerificationResult{
				SourcePath: sourcePath,
				SourceHash: keccak256Hex(source),
				Errors:     []string{fmt.Sprintf("compile: %v", err)},
			})
			report.AllMatch = false
			continue
		}

		// Compile a second time and verify determinism.
		tocBytes2, err := lua.CompileArtifact(source, sourcePath)
		if err != nil {
			report.Results = append(report.Results, VerificationResult{
				SourcePath: sourcePath,
				SourceHash: keccak256Hex(source),
				Errors:     []string{fmt.Sprintf("second compile: %v", err)},
			})
			report.AllMatch = false
			continue
		}

		result, err := VerifyArtifact(tocBytes, tocBytes2)
		if err != nil {
			report.Results = append(report.Results, VerificationResult{
				SourcePath: sourcePath,
				SourceHash: keccak256Hex(source),
				Errors:     []string{fmt.Sprintf("verify artifact: %v", err)},
			})
			report.AllMatch = false
			continue
		}

		result.SourcePath = sourcePath
		result.SourceHash = keccak256Hex(source)
		report.Results = append(report.Results, *result)
		if !result.Match {
			report.AllMatch = false
		}
	}

	return report, nil
}

// VerifyAllExamples compiles and verifies all example contracts from the
// policy_wallet and agent_economy directories. The base directory should
// be the tolang repository root.
func VerifyAllExamples(repoRoot string) (*VerificationReport, error) {
	report := &VerificationReport{
		SchemaVersion: schemaVersion,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		AllMatch:      true,
	}

	dirs := []string{
		filepath.Join(repoRoot, "examples", "policy_wallet"),
		filepath.Join(repoRoot, "examples", "agent_economy"),
	}

	for _, dir := range dirs {
		sub, err := VerifyDirectory(dir)
		if err != nil {
			return nil, fmt.Errorf("verify %s: %w", dir, err)
		}
		report.Results = append(report.Results, sub.Results...)
		if !sub.AllMatch {
			report.AllMatch = false
		}
	}

	return report, nil
}

// FormatReport generates a human-readable verification report.
func FormatReport(report *VerificationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verification Report (schema %s)\n", report.SchemaVersion)
	fmt.Fprintf(&b, "Timestamp: %s\n", report.Timestamp)
	fmt.Fprintf(&b, "Contracts: %d\n", len(report.Results))
	b.WriteString(strings.Repeat("-", 60))
	b.WriteString("\n")

	for _, r := range report.Results {
		status := "PASS"
		if !r.Match {
			status = "FAIL"
		}
		name := r.ContractName
		if name == "" {
			name = filepath.Base(r.SourcePath)
		}
		fmt.Fprintf(&b, "[%s] %s\n", status, name)
		if r.SourcePath != "" {
			fmt.Fprintf(&b, "  Source: %s\n", r.SourcePath)
		}
		if r.SourceHash != "" {
			fmt.Fprintf(&b, "  Source hash: %s\n", r.SourceHash)
		}
		fmt.Fprintf(&b, "  Bytecode: %s (expected %s, got %s)\n",
			boolStatus(r.BytecodeMatch), r.ExpectedBytecodeHash, r.ActualBytecodeHash)
		fmt.Fprintf(&b, "  ABI:      %s (expected %s, got %s)\n",
			boolStatus(r.ABIMatch), r.ExpectedABIHash, r.ActualABIHash)
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "  ERROR: %s\n", e)
		}
	}

	b.WriteString(strings.Repeat("-", 60))
	b.WriteString("\n")
	if report.AllMatch {
		b.WriteString("Overall: ALL CONTRACTS VERIFIED\n")
	} else {
		b.WriteString("Overall: VERIFICATION FAILED\n")
	}
	return b.String()
}

func boolStatus(ok bool) string {
	if ok {
		return "MATCH"
	}
	return "MISMATCH"
}
