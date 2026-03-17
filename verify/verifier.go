package verify

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// VerifyDirectory verifies all .tol files in a directory by checking
// deterministic compilation — each .tol file is compiled twice and the
// resulting artifacts are compared. This confirms the compiler produces
// stable output but does NOT verify against on-chain deployed code.
// For on-chain verification, use VerifyDeployed or VerifyDeployedBatch.
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

// --- On-chain verification ---

// SimpleRPCClient implements RPCClient via JSON-RPC against a GTOS node.
type SimpleRPCClient struct {
	Endpoint string
}

// jsonRPCRequest is the JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// jsonRPCResponse is the JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is the JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// GetCode fetches the deployed contract code at the given address by calling
// tos_getCode(address, "latest") via HTTP JSON-RPC.
func (c *SimpleRPCClient) GetCode(address string) ([]byte, error) {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tos_getCode",
		Params:  []interface{}{address, "latest"},
		ID:      1,
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}

	resp, err := http.Post(c.Endpoint, "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rpc response: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	var hexCode string
	if err := json.Unmarshal(rpcResp.Result, &hexCode); err != nil {
		return nil, fmt.Errorf("unmarshal rpc result: %w", err)
	}

	hexCode = strings.TrimPrefix(hexCode, "0x")
	if hexCode == "" {
		return nil, fmt.Errorf("no code at address %s", address)
	}

	code, err := hex.DecodeString(hexCode)
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	return code, nil
}

// VerifyDeployed compiles the source at sourcePath and compares the resulting
// artifact against the on-chain code fetched from contractAddress via rpc.
// It tries to decode both as .tor packages first; if that fails it falls back
// to .toc artifact comparison.
func VerifyDeployed(sourcePath string, contractAddress string, rpc RPCClient) (*VerificationResult, error) {
	result := &VerificationResult{
		SourcePath:      sourcePath,
		ContractAddress: contractAddress,
	}

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("read source: %v", err))
		return result, err
	}
	result.SourceHash = keccak256Hex(source)

	// Fetch deployed code from chain.
	deployedCode, err := rpc.GetCode(contractAddress)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("fetch on-chain code: %v", err))
		return result, err
	}

	// Try .tor package comparison first.
	deployedPkg, pkgErr := lua.DecodePackage(deployedCode)
	if pkgErr == nil {
		// Deployed code is a .tor package. Compile source as package too.
		compiledBytes, err := lua.CompilePackage(source, sourcePath, nil)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("compile package: %v", err))
			return result, err
		}

		pkgResult, err := VerifyPackage(compiledBytes, deployedCode)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("verify package: %v", err))
			return result, err
		}

		// Copy package result fields.
		result.ContractName = pkgResult.ContractName
		result.ExpectedBytecodeHash = pkgResult.ExpectedBytecodeHash
		result.ActualBytecodeHash = pkgResult.ActualBytecodeHash
		result.BytecodeMatch = pkgResult.BytecodeMatch
		result.ExpectedABIHash = pkgResult.ExpectedABIHash
		result.ActualABIHash = pkgResult.ActualABIHash
		result.ABIMatch = pkgResult.ABIMatch
		result.Match = pkgResult.Match
		result.Errors = append(result.Errors, pkgResult.Errors...)

		// Also compare the .toc entries inside the package for ABI verification.
		_ = deployedPkg // used above via VerifyPackage
		return result, nil
	}

	// Fall back to .toc artifact comparison.
	compiledBytes, err := lua.CompileArtifact(source, sourcePath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("compile artifact: %v", err))
		return result, err
	}

	artResult, err := VerifySource(sourcePath, deployedCode)
	if err != nil {
		// VerifySource already populated result.Errors; but we need our own result.
		result.Errors = append(result.Errors, fmt.Sprintf("verify artifact: %v", err))
		return result, err
	}

	_ = compiledBytes // used indirectly via VerifySource re-compilation

	result.ContractName = artResult.ContractName
	result.ExpectedBytecodeHash = artResult.ExpectedBytecodeHash
	result.ActualBytecodeHash = artResult.ActualBytecodeHash
	result.BytecodeMatch = artResult.BytecodeMatch
	result.ExpectedABIHash = artResult.ExpectedABIHash
	result.ActualABIHash = artResult.ActualABIHash
	result.ABIMatch = artResult.ABIMatch
	result.Match = artResult.Match
	result.Errors = append(result.Errors, artResult.Errors...)

	return result, nil
}

// VerifyDeployedBatch verifies multiple contracts against their deployed
// on-chain code. The contracts map keys are source file paths and values
// are the corresponding contract addresses.
func VerifyDeployedBatch(contracts map[string]string, rpc RPCClient) (*VerificationReport, error) {
	report := &VerificationReport{
		SchemaVersion: schemaVersion,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		AllMatch:      true,
	}

	for sourcePath, address := range contracts {
		result, err := VerifyDeployed(sourcePath, address, rpc)
		if err != nil {
			// VerifyDeployed returns partial results on error; include them.
			report.AllMatch = false
			report.Results = append(report.Results, *result)
			continue
		}
		report.Results = append(report.Results, *result)
		if !result.Match {
			report.AllMatch = false
		}
	}

	return report, nil
}
