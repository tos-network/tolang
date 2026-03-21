package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/tos-network/tolang"
	"github.com/tos-network/tolang/metadata"
)

// compileSourceAsAndExtract compiles source bytes using a synthetic compile path,
// which lets package-style imports resolve from the repo parent directory.
func compileSourceAsAndExtract(t *testing.T, sourcePath, compileName string) ([]byte, *lua.Artifact, *metadata.ContractMetadata) {
	t.Helper()

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}

	artBytes, err := lua.CompileArtifact(source, compileName)
	if err != nil {
		t.Fatalf("CompileArtifact %s as %s: %v", sourcePath, compileName, err)
	}

	art, err := lua.DecodeArtifact(artBytes)
	if err != nil {
		t.Fatalf("DecodeArtifact %s: %v", sourcePath, err)
	}

	meta, err := metadata.ExtractFromABI(art.ABIJSON)
	if err != nil {
		t.Fatalf("ExtractFromABI %s: %v", sourcePath, err)
	}

	meta.Contract.Name = art.ContractName
	meta.ArtifactRef = metadata.ComputeArtifactRef(artBytes, art.Bytecode, source, art.ABIJSON, meta.ArtifactRef.Version)
	return source, art, meta
}

func TestStdlibComposedExamplesMetadataAndDiscovery(t *testing.T) {
	root := projectRoot(t)
	baseDir := filepath.Dir(root)

	testCases := []struct {
		file         string
		contractName string
		packageName  string
		functions    []string
	}{
		{
			file:         "PolicySponsoredCheckout.tol",
			contractName: "PolicySponsoredCheckout",
			packageName:  "tolang.examples.stdlib_composed.policy_sponsored_checkout",
			functions:    []string{"preauthorize", "executeCheckout", "dailyRemaining", "receiptStatus"},
		},
		{
			file:         "PrivateServiceOrder.tol",
			contractName: "PrivateServiceOrder",
			packageName:  "tolang.examples.stdlib_composed.private_service_order",
			functions:    []string{"ready", "settleReadyOrder", "customerVaultBalance", "serviceManifest"},
		},
		{
			file:         "PrivateEscrowCheckout.tol",
			contractName: "PrivateEscrowCheckout",
			packageName:  "tolang.examples.stdlib_composed.private_escrow_checkout",
			functions:    []string{"prepare", "settleAndRelease", "failAndRefund", "confidentialBalance", "receiptState"},
		},
		{
			file:         "SponsoredPrivateEscrowCheckout.tol",
			contractName: "SponsoredPrivateEscrowCheckout",
			packageName:  "tolang.examples.stdlib_composed.sponsored_private_escrow_checkout",
			functions:    []string{"preflight", "executeSponsoredRelease", "abortSponsoredRefund", "sponsorRemaining", "receiptStatus", "confidentialBalance"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.contractName, func(t *testing.T) {
			sourcePath := filepath.Join(root, "examples", "stdlib_composed", tc.file)
			compileName := filepath.Join(baseDir, tc.file)

			source, art, meta := compileSourceAsAndExtract(t, sourcePath, compileName)

			if art.ContractName != tc.contractName {
				t.Fatalf("contract name: got %q want %q", art.ContractName, tc.contractName)
			}
			if len(art.Bytecode) == 0 {
				t.Fatal("bytecode is empty")
			}
			if len(art.ABIJSON) == 0 {
				t.Fatal("abi json is empty")
			}
			if err := lua.VerifySourceHash(art, source); err != nil {
				t.Fatalf("source hash mismatch: %v", err)
			}

			if meta.Manifest == nil {
				t.Fatal("manifest metadata is nil")
			}
			if meta.Manifest.Version != "1.0.0" {
				t.Fatalf("manifest version: got %q want %q", meta.Manifest.Version, "1.0.0")
			}
			if len(meta.Functions) != len(tc.functions) {
				t.Fatalf("function count: got %d want %d", len(meta.Functions), len(tc.functions))
			}

			functionSet := make(map[string]bool, len(meta.Functions))
			for _, fn := range meta.Functions {
				functionSet[fn.Name] = true
			}
			for _, want := range tc.functions {
				if !functionSet[want] {
					t.Fatalf("missing function metadata for %q", want)
				}
			}

			summary := metadata.GenerateHumanReadable(meta)
			if summary == nil {
				t.Fatal("GenerateHumanReadable returned nil")
			}
			if summary.ContractName != tc.contractName {
				t.Fatalf("summary contract name: got %q want %q", summary.ContractName, tc.contractName)
			}
			if summary.Description == "" {
				t.Fatal("summary description is empty")
			}
			if summary.RiskSummary == "" {
				t.Fatal("summary risk summary is empty")
			}
			if !strings.Contains(strings.ToLower(summary.RiskSummary), "external calls") {
				t.Fatalf("expected risk summary to mention external calls, got %q", summary.RiskSummary)
			}

			dm := metadata.BuildDiscoveryManifest(meta, tc.packageName)
			if dm == nil {
				t.Fatal("BuildDiscoveryManifest returned nil")
			}
			if dm.PackageName != tc.packageName {
				t.Fatalf("discovery package name: got %q want %q", dm.PackageName, tc.packageName)
			}
			if dm.PackageVersion != "1.0.0" {
				t.Fatalf("discovery package version: got %q want %q", dm.PackageVersion, "1.0.0")
			}
			if dm.ContractType != "custom" {
				t.Fatalf("discovery contract type: got %q want %q", dm.ContractType, "custom")
			}
			if dm.HumanSummary == "" {
				t.Fatal("discovery human summary is empty")
			}
			if len(dm.InterfaceMethods) != len(tc.functions) {
				t.Fatalf("interface method count: got %d want %d", len(dm.InterfaceMethods), len(tc.functions))
			}

			serviceKinds := make(map[string]bool, len(dm.ServiceKinds))
			for _, sk := range dm.ServiceKinds {
				serviceKinds[sk] = true
			}
			if !serviceKinds["query"] {
				t.Fatalf("expected query service kind in %v", dm.ServiceKinds)
			}

			tagSet := make(map[string]bool, len(dm.Tags))
			for _, tag := range dm.Tags {
				tagSet[tag] = true
			}
			if !tagSet["custom"] {
				t.Fatalf("expected custom tag in %v", dm.Tags)
			}

			pkg := metadata.BuildAgentPackageInfo(meta, tc.packageName)
			if pkg == nil {
				t.Fatal("BuildAgentPackageInfo returned nil")
			}
			if pkg.PackageName != tc.packageName {
				t.Fatalf("agent package name: got %q want %q", pkg.PackageName, tc.packageName)
			}
			if pkg.PackageVersion != "1.0.0" {
				t.Fatalf("agent package version: got %q want %q", pkg.PackageVersion, "1.0.0")
			}
			if pkg.Capabilities == nil {
				t.Fatal("agent package capabilities is nil")
			}
			if pkg.Capabilities.ContractName != tc.contractName {
				t.Fatalf("capability manifest contract name: got %q want %q", pkg.Capabilities.ContractName, tc.contractName)
			}
			if pkg.Discovery == nil {
				t.Fatal("agent package discovery is nil")
			}
			if pkg.Discovery.ContractType != "custom" {
				t.Fatalf("agent package contract type: got %q want %q", pkg.Discovery.ContractType, "custom")
			}
			if pkg.HumanSummary == "" {
				t.Fatal("agent package human summary is empty")
			}
			if pkg.ArtifactRef.BytecodeHash == "" || pkg.ArtifactRef.ABIHash == "" {
				t.Fatalf("agent package artifact hashes missing: %+v", pkg.ArtifactRef)
			}
		})
	}
}
