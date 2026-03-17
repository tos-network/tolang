package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/tos-network/tolang"
	"github.com/tos-network/tolang/metadata"
)

// projectRoot returns the absolute path to the tolang repo root.
func projectRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location to find go.mod.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

// compileAndExtract compiles a .tol file to an artifact, decodes it, and
// extracts metadata from the ABI JSON. Returns the source, artifact, and metadata.
func compileAndExtract(t *testing.T, tolPath string) ([]byte, *lua.Artifact, *metadata.ContractMetadata) {
	t.Helper()
	source, err := os.ReadFile(tolPath)
	if err != nil {
		t.Fatalf("read %s: %v", tolPath, err)
	}
	artBytes, err := lua.CompileArtifact(source, tolPath)
	if err != nil {
		t.Fatalf("CompileArtifact %s: %v", tolPath, err)
	}
	art, err := lua.DecodeArtifact(artBytes)
	if err != nil {
		t.Fatalf("DecodeArtifact %s: %v", tolPath, err)
	}
	meta, err := metadata.ExtractFromABI(art.ABIJSON)
	if err != nil {
		t.Fatalf("ExtractFromABI %s: %v", tolPath, err)
	}
	// Populate contract name from artifact.
	meta.Contract.Name = art.ContractName
	// Compute artifact ref.
	meta.ArtifactRef = metadata.ComputeArtifactRef(artBytes, art.Bytecode, source, art.ABIJSON, meta.ArtifactRef.Version)
	return source, art, meta
}

// TestPolicyWalletCompileAndMetadata compiles PolicyWallet.tol through the full
// pipeline: compile -> decode artifact -> extract metadata -> policy profile ->
// human-readable summary -> discovery manifest.
func TestPolicyWalletCompileAndMetadata(t *testing.T) {
	root := projectRoot(t)
	tolPath := filepath.Join(root, "examples", "policy_wallet", "PolicyWallet.tol")

	source, art, meta := compileAndExtract(t, tolPath)

	// 1. Artifact basics.
	if art.ContractName != "PolicyWallet" {
		t.Errorf("contract name: got %q, want %q", art.ContractName, "PolicyWallet")
	}
	if len(art.Bytecode) == 0 {
		t.Fatal("bytecode is empty")
	}
	if err := lua.VerifySourceHash(art, source); err != nil {
		t.Fatalf("source hash mismatch: %v", err)
	}

	// 2. Metadata: should be detected as an account contract.
	if !meta.IsAccount {
		t.Error("expected IsAccount=true for PolicyWallet")
	}
	if meta.SchemaVersion == "" {
		t.Error("schema version is empty")
	}

	// 3. Policy profile detection.
	if meta.PolicyProfile == nil {
		t.Fatal("expected non-nil PolicyProfile for account contract")
	}
	pp := meta.PolicyProfile
	if !pp.HasSpendCaps {
		t.Error("expected HasSpendCaps=true")
	}
	if !pp.HasGuardian {
		t.Error("expected HasGuardian=true")
	}
	if !pp.HasRecovery {
		t.Error("expected HasRecovery=true")
	}
	if !pp.HasDelegation {
		t.Error("expected HasDelegation=true")
	}
	if !pp.HasSuspension {
		t.Error("expected HasSuspension=true")
	}

	// 4. Functions should be non-empty.
	if len(meta.Functions) == 0 {
		t.Fatal("expected at least one function in metadata")
	}

	// 5. Human-readable summary.
	summary := metadata.GenerateHumanReadable(meta)
	if summary == nil {
		t.Fatal("GenerateHumanReadable returned nil")
	}
	if summary.ContractName != "PolicyWallet" {
		t.Errorf("summary contract name: got %q, want %q", summary.ContractName, "PolicyWallet")
	}
	if !summary.IsAccount {
		t.Error("summary should have IsAccount=true")
	}

	// Summary should mention key policy features.
	display := summary.FormatForDisplay()
	for _, keyword := range []string{"spend caps", "guardian", "recovery"} {
		if !strings.Contains(strings.ToLower(display), keyword) {
			t.Errorf("human-readable display missing keyword %q", keyword)
		}
	}
	if len(summary.PolicyFeatures) == 0 {
		t.Error("expected non-empty PolicyFeatures in summary")
	}
	if len(summary.Functions) == 0 {
		t.Error("expected non-empty Functions in summary")
	}

	// 6. Discovery manifest.
	dm := metadata.BuildDiscoveryManifest(meta, "policy_wallet")
	if dm == nil {
		t.Fatal("BuildDiscoveryManifest returned nil")
	}
	if dm.ContractType != "policy_wallet" {
		t.Errorf("discovery contract type: got %q, want %q", dm.ContractType, "policy_wallet")
	}
	if dm.SchemaVersion == "" {
		t.Error("discovery schema version is empty")
	}
	if len(dm.ServiceKinds) == 0 {
		t.Error("expected non-empty ServiceKinds")
	}
	// Policy wallet should have account_management and policy_enforcement service kinds.
	serviceKindSet := make(map[string]bool)
	for _, sk := range dm.ServiceKinds {
		serviceKindSet[sk] = true
	}
	if !serviceKindSet["account_management"] {
		t.Error("expected service kind 'account_management'")
	}
	if !serviceKindSet["policy_enforcement"] {
		t.Error("expected service kind 'policy_enforcement'")
	}
	if len(dm.InterfaceMethods) == 0 {
		t.Error("expected non-empty InterfaceMethods")
	}
	if len(dm.Tags) == 0 {
		t.Error("expected non-empty Tags")
	}
	if dm.PolicyProfile == nil {
		t.Error("expected PolicyProfile in discovery manifest")
	}
}

// TestAgentEconomyContracts compiles each agent economy contract and verifies the
// full metadata pipeline: compile -> extract -> human-readable -> discovery.
func TestAgentEconomyContracts(t *testing.T) {
	root := projectRoot(t)
	agentDir := filepath.Join(root, "examples", "agent_economy")

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		t.Fatalf("read agent_economy dir: %v", err)
	}

	var tolFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tol") {
			tolFiles = append(tolFiles, filepath.Join(agentDir, e.Name()))
		}
	}
	if len(tolFiles) == 0 {
		t.Fatal("no .tol files found in examples/agent_economy/")
	}

	for _, tolPath := range tolFiles {
		name := strings.TrimSuffix(filepath.Base(tolPath), ".tol")
		t.Run(name, func(t *testing.T) {
			_, art, meta := compileAndExtract(t, tolPath)

			// Basic artifact checks.
			if art.ContractName == "" {
				t.Error("artifact contract name is empty")
			}
			if len(art.Bytecode) == 0 {
				t.Error("bytecode is empty")
			}
			if len(art.ABIJSON) == 0 {
				t.Error("ABI JSON is empty")
			}

			// Metadata should have functions.
			if len(meta.Functions) == 0 {
				t.Error("expected at least one function in metadata")
			}

			// Human-readable summary.
			summary := metadata.GenerateHumanReadable(meta)
			if summary == nil {
				t.Fatal("GenerateHumanReadable returned nil")
			}
			if len(summary.Functions) == 0 {
				t.Error("expected non-empty Functions in summary")
			}
			if summary.RiskSummary == "" {
				t.Error("expected non-empty RiskSummary")
			}

			// Discovery manifest.
			dm := metadata.BuildDiscoveryManifest(meta, name)
			if dm == nil {
				t.Fatal("BuildDiscoveryManifest returned nil")
			}
			if dm.ContractType == "" {
				t.Error("discovery contract type is empty")
			}
			if len(dm.ServiceKinds) == 0 {
				t.Error("expected non-empty ServiceKinds")
			}
			// Every contract should at least have "query" service kind.
			found := false
			for _, sk := range dm.ServiceKinds {
				if sk == "query" {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected 'query' in ServiceKinds")
			}
			if len(dm.Tags) == 0 {
				t.Error("expected non-empty Tags")
			}
			if dm.HumanSummary == "" {
				t.Error("expected non-empty HumanSummary")
			}

			t.Logf("contract=%s type=%s services=%v tags=%v",
				art.ContractName, dm.ContractType, dm.ServiceKinds, dm.Tags)
		})
	}
}

// TestDiscoveryManifestConsistency compiles multiple contracts and verifies that
// their discovery manifests have consistent schema versions and unique artifact refs.
func TestDiscoveryManifestConsistency(t *testing.T) {
	root := projectRoot(t)

	// Collect .tol files from both example directories.
	var tolFiles []string
	for _, dir := range []string{
		filepath.Join(root, "examples", "policy_wallet"),
		filepath.Join(root, "examples", "agent_economy"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tol") {
				tolFiles = append(tolFiles, filepath.Join(dir, e.Name()))
			}
		}
	}
	if len(tolFiles) < 2 {
		t.Fatal("need at least 2 .tol files for consistency test")
	}

	type result struct {
		name     string
		manifest *metadata.DiscoveryManifest
	}
	var results []result

	for _, tolPath := range tolFiles {
		name := strings.TrimSuffix(filepath.Base(tolPath), ".tol")
		_, _, meta := compileAndExtract(t, tolPath)
		dm := metadata.BuildDiscoveryManifest(meta, name)
		results = append(results, result{name: name, manifest: dm})
	}

	// 1. All schema versions should be the same.
	firstSchema := results[0].manifest.SchemaVersion
	for _, r := range results[1:] {
		if r.manifest.SchemaVersion != firstSchema {
			t.Errorf("schema version mismatch: %s has %q, %s has %q",
				results[0].name, firstSchema, r.name, r.manifest.SchemaVersion)
		}
	}

	// 2. Artifact ref bytecode hashes should be unique per contract.
	seenBytecodeHash := make(map[string]string)
	for _, r := range results {
		h := r.manifest.ArtifactRef.BytecodeHash
		if h == "" {
			t.Errorf("%s: bytecode hash is empty", r.name)
			continue
		}
		if prev, exists := seenBytecodeHash[h]; exists {
			t.Errorf("duplicate bytecode hash between %s and %s: %s", prev, r.name, h)
		}
		seenBytecodeHash[h] = r.name
	}

	// 3. Tags should be meaningful (non-empty strings).
	for _, r := range results {
		for _, tag := range r.manifest.Tags {
			if strings.TrimSpace(tag) == "" {
				t.Errorf("%s: empty tag found", r.name)
			}
		}
	}

	// 4. All ABI hashes should be non-empty.
	for _, r := range results {
		if r.manifest.ArtifactRef.ABIHash == "" {
			t.Errorf("%s: ABI hash is empty", r.name)
		}
	}

	t.Logf("verified %d contracts for discovery manifest consistency", len(results))
}

// TestCompatibilityMatrix verifies the current compatibility matrix has all three
// repositories and that cross-repo compatibility checks pass.
func TestCompatibilityMatrix(t *testing.T) {
	matrix := metadata.CurrentCompatibilityMatrix()
	if matrix == nil {
		t.Fatal("CurrentCompatibilityMatrix returned nil")
	}

	// 1. Schema version should be set.
	if matrix.SchemaVersion == "" {
		t.Error("matrix schema version is empty")
	}
	if matrix.BoundaryVersion == "" {
		t.Error("matrix boundary version is empty")
	}
	if matrix.MetadataVersion == "" {
		t.Error("matrix metadata version is empty")
	}

	// 2. All three repos should be present.
	expectedRepos := []string{"tolang", "openfox", "gtos"}
	for _, repo := range expectedRepos {
		rc, ok := matrix.Repositories[repo]
		if !ok {
			t.Errorf("missing repository %q in compatibility matrix", repo)
			continue
		}
		if rc.Name == "" {
			t.Errorf("repository %q has empty name", repo)
		}
		if len(rc.Features) == 0 {
			t.Errorf("repository %q has no features listed", repo)
		}
		if rc.MinBoundaryVersion == "" || rc.MaxBoundaryVersion == "" {
			t.Errorf("repository %q missing boundary version range", repo)
		}
		if rc.MinMetadataVersion == "" || rc.MaxMetadataVersion == "" {
			t.Errorf("repository %q missing metadata version range", repo)
		}
	}

	// 3. Cross-repo compatibility: all pairs should be compatible.
	pairs := [][2]string{
		{"tolang", "openfox"},
		{"tolang", "gtos"},
		{"openfox", "gtos"},
	}
	for _, pair := range pairs {
		ok, reason := metadata.CheckCompatibility(matrix, pair[0], pair[1])
		if !ok {
			t.Errorf("incompatible: %s <-> %s: %s", pair[0], pair[1], reason)
		} else {
			t.Logf("compatible: %s <-> %s: %s", pair[0], pair[1], reason)
		}
	}

	// 4. Unknown repo should return incompatible.
	ok, _ := metadata.CheckCompatibility(matrix, "tolang", "unknown_repo")
	if ok {
		t.Error("expected incompatible for unknown repository")
	}
}
