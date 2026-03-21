package lua

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStdlibComposedExamplesCompile(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	testCases := []struct {
		file         string
		contractName string
		functions    []string
	}{
		{
			file:         filepath.Join(repoRoot, "examples/stdlib_composed/PolicySponsoredCheckout.tol"),
			contractName: "PolicySponsoredCheckout",
			functions:    []string{"preauthorize", "executeCheckout", "dailyRemaining", "receiptStatus"},
		},
		{
			file:         filepath.Join(repoRoot, "examples/stdlib_composed/PrivateServiceOrder.tol"),
			contractName: "PrivateServiceOrder",
			functions:    []string{"ready", "settleReadyOrder", "customerVaultBalance", "serviceManifest"},
		},
		{
			file:         filepath.Join(repoRoot, "examples/stdlib_composed/PrivateEscrowCheckout.tol"),
			contractName: "PrivateEscrowCheckout",
			functions:    []string{"prepare", "settleAndRelease", "failAndRefund", "confidentialBalance", "receiptState"},
		},
		{
			file:         filepath.Join(repoRoot, "examples/stdlib_composed/SponsoredPrivateEscrowCheckout.tol"),
			contractName: "SponsoredPrivateEscrowCheckout",
			functions:    []string{"preflight", "executeSponsoredRelease", "abortSponsoredRefund", "sponsorRemaining", "receiptStatus", "confidentialBalance"},
		},
		{
			file:         filepath.Join(repoRoot, "examples/stdlib_composed/PrivateDisputeEscrow.tol"),
			contractName: "PrivateDisputeEscrow",
			functions:    []string{"openOrder", "settleOrder", "disputeOrder", "resolveDispute", "escrowStatus", "receiptStatus"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.contractName, func(t *testing.T) {
			source, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("read example %s: %v", tc.file, err)
			}

			if _, err := BuildIRWithResolver(source, tc.file, NewOSFileResolver(filepath.Dir(tc.file))); err != nil {
				t.Fatalf("build IR %s: %v", tc.contractName, err)
			}

			artifactBytes, err := CompileArtifact(source, tc.file)
			if err != nil {
				t.Fatalf("compile artifact %s: %v", tc.contractName, err)
			}
			artifact, err := DecodeArtifact(artifactBytes)
			if err != nil {
				t.Fatalf("decode artifact %s: %v", tc.contractName, err)
			}
			if artifact.ContractName != tc.contractName {
				t.Fatalf("artifact contract mismatch: got=%q want=%q", artifact.ContractName, tc.contractName)
			}

			pkgBytes, err := CompilePackage(source, tc.file, &PackageOptions{
				PackageName:    tc.contractName,
				PackageVersion: "1.0.0",
			})
			if err != nil {
				t.Fatalf("compile package %s: %v", tc.contractName, err)
			}
			pkg, err := DecodePackage(pkgBytes)
			if err != nil {
				t.Fatalf("decode package %s: %v", tc.contractName, err)
			}

			var manifest struct {
				Name      string `json:"name"`
				Package   string `json:"package"`
				Version   string `json:"version"`
				Contracts []struct {
					Name string `json:"name"`
					TOC  string `json:"toc"`
					ABI  string `json:"abi"`
				} `json:"contracts"`
			}
			if err := json.Unmarshal(pkg.ManifestJSON, &manifest); err != nil {
				t.Fatalf("manifest json %s: %v", tc.contractName, err)
			}
			if manifest.Name != tc.contractName {
				t.Fatalf("package name mismatch: got=%q want=%q", manifest.Name, tc.contractName)
			}
			if manifest.Version != "1.0.0" {
				t.Fatalf("package version mismatch: got=%q want=%q", manifest.Version, "1.0.0")
			}

			var abi struct {
				Functions []struct {
					Name string `json:"name"`
				} `json:"functions"`
				Manifest *struct {
					Name string `json:"name"`
				} `json:"manifest"`
			}
			if err := json.Unmarshal(artifact.ABIJSON, &abi); err != nil {
				t.Fatalf("artifact abi %s: %v", tc.contractName, err)
			}
			if abi.Manifest == nil || abi.Manifest.Name != tc.contractName {
				t.Fatalf("artifact abi manifest missing or wrong for %s", tc.contractName)
			}
			gotFns := make(map[string]bool, len(abi.Functions))
			for _, fn := range abi.Functions {
				gotFns[fn.Name] = true
			}
			for _, fn := range tc.functions {
				if !gotFns[fn] {
					t.Fatalf("example %s missing function %q", tc.contractName, fn)
				}
			}

			foundTOC := false
			for _, c := range manifest.Contracts {
				if c.Name == tc.contractName {
					if _, ok := pkg.Files[c.TOC]; !ok {
						t.Fatalf("package missing toc %q for %s", c.TOC, tc.contractName)
					}
					if _, ok := pkg.Files[c.ABI]; !ok {
						t.Fatalf("package missing abi %q for %s", c.ABI, tc.contractName)
					}
					foundTOC = true
				}
			}
			if !foundTOC {
				t.Fatalf("package manifest missing contract %q", tc.contractName)
			}
		})
	}
}
