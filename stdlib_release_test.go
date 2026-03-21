package lua

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStdlibReleaseArtifactsAreCurrent(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	type releaseIndexEntry struct {
		Family             string `json:"family"`
		Contract           string `json:"contract"`
		SourcePath         string `json:"source_path"`
		ReleasePackageName string `json:"release_package_name"`
		TOCPath            string `json:"toc_path"`
		ABIPath            string `json:"abi_path"`
		InitPath           string `json:"init_path"`
		TORPath            string `json:"tor_path"`
		BytecodeHash       string `json:"bytecode_hash"`
		PackageHash        string `json:"package_hash"`
	}
	var index struct {
		Version string              `json:"version"`
		Entries []releaseIndexEntry `json:"entries"`
	}

	indexPath := filepath.Join(repoRoot, "stdlib", "releases", "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read release index %s: %v", indexPath, err)
	}
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatalf("unmarshal release index: %v", err)
	}
	if index.Version != "1.0.0" {
		t.Fatalf("release index version: got=%q want=%q", index.Version, "1.0.0")
	}

	catalog := StdlibReleaseCatalog()
	if len(index.Entries) != len(catalog) {
		t.Fatalf("release index entry count: got=%d want=%d", len(index.Entries), len(catalog))
	}

	byContract := make(map[string]StdlibReleaseEntry, len(catalog))
	for _, entry := range catalog {
		byContract[entry.Contract] = entry
	}

	for _, idxEntry := range index.Entries {
		entry, ok := byContract[idxEntry.Contract]
		if !ok {
			t.Fatalf("release index contains unknown contract %q", idxEntry.Contract)
		}
		sourcePath := filepath.Join(repoRoot, entry.SourcePath)
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("read source %s: %v", sourcePath, err)
		}
		built, err := BuildStdlibReleaseArtifacts(source, sourcePath, entry)
		if err != nil {
			t.Fatalf("build release artifacts %s: %v", entry.Contract, err)
		}

		tocBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.TOCPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.TOCPath, err)
		}
		if string(tocBytes) != string(built.ArtifactTOC) {
			t.Fatalf("stale toc for %s; rerun stdlib exporter", entry.Contract)
		}

		abiBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.ABIPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.ABIPath, err)
		}
		if string(abiBytes) != string(built.InterfaceABI) {
			t.Fatalf("stale abi for %s; rerun stdlib exporter", entry.Contract)
		}

		if idxEntry.InitPath != "" {
			initBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.InitPath))
			if err != nil {
				t.Fatalf("read %s: %v", idxEntry.InitPath, err)
			}
			if string(initBytes) != string(built.InitTOC) {
				t.Fatalf("stale init toc for %s; rerun stdlib exporter", entry.Contract)
			}
		} else if len(built.InitTOC) != 0 {
			t.Fatalf("release index missing init path for %s", entry.Contract)
		}

		torBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.TORPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.TORPath, err)
		}
		if string(torBytes) != string(built.PackageTOR) {
			t.Fatalf("stale tor for %s; rerun stdlib exporter", entry.Contract)
		}
		if idxEntry.PackageHash != PackageHash(torBytes) {
			t.Fatalf("package hash mismatch for %s", entry.Contract)
		}

		art, err := DecodeArtifact(tocBytes)
		if err != nil {
			t.Fatalf("decode artifact %s: %v", entry.Contract, err)
		}
		if idxEntry.BytecodeHash != art.BytecodeHash {
			t.Fatalf("bytecode hash mismatch for %s", entry.Contract)
		}

		pkg, err := DecodePackage(torBytes)
		if err != nil {
			t.Fatalf("decode package %s: %v", entry.Contract, err)
		}
		if err := VerifyPackageSignature(torBytes); err != nil {
			t.Fatalf("verify package signature %s: %v", entry.Contract, err)
		}
		if len(pkg.ManifestJSON) == 0 {
			t.Fatalf("package manifest missing for %s", entry.Contract)
		}
	}
}
