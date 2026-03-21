package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	lua "github.com/tos-network/tolang"
)

type releaseIndexEntry struct {
	Family             string `json:"family"`
	Contract           string `json:"contract"`
	SourcePath         string `json:"source_path"`
	ReleasePackageName string `json:"release_package_name"`
	TOCPath            string `json:"toc_path"`
	ABIPath            string `json:"abi_path"`
	InitPath           string `json:"init_path,omitempty"`
	TORPath            string `json:"tor_path"`
	BytecodeHash       string `json:"bytecode_hash"`
	PackageHash        string `json:"package_hash"`
}

type releaseIndex struct {
	Version string              `json:"version"`
	Entries []releaseIndexEntry `json:"entries"`
}

func main() {
	repoRoot, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}
	releaseRoot := filepath.Join(repoRoot, "stdlib", "releases")
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		fatalf("mkdir %s: %v", releaseRoot, err)
	}

	entries := lua.StdlibReleaseCatalog()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Family != entries[j].Family {
			return entries[i].Family < entries[j].Family
		}
		return entries[i].Contract < entries[j].Contract
	})

	index := releaseIndex{Version: "1.0.0"}
	for _, entry := range entries {
		sourcePath := filepath.Join(repoRoot, entry.SourcePath)
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			fatalf("read %s: %v", sourcePath, err)
		}
		built, err := lua.BuildStdlibReleaseArtifacts(source, sourcePath, entry)
		if err != nil {
			fatalf("build release artifacts for %s: %v", entry.Contract, err)
		}

		dir := filepath.Join(releaseRoot, entry.Family)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fatalf("mkdir %s: %v", dir, err)
		}
		tocRel := filepath.ToSlash(filepath.Join("stdlib", "releases", entry.Family, entry.Contract+".toc"))
		abiRel := filepath.ToSlash(filepath.Join("stdlib", "releases", entry.Family, "I"+entry.Contract+".abi"))
		torRel := filepath.ToSlash(filepath.Join("stdlib", "releases", entry.Family, entry.Contract+".tor"))
		if err := os.WriteFile(filepath.Join(repoRoot, tocRel), built.ArtifactTOC, 0o644); err != nil {
			fatalf("write %s: %v", tocRel, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, abiRel), built.InterfaceABI, 0o644); err != nil {
			fatalf("write %s: %v", abiRel, err)
		}
		initRel := ""
		if len(built.InitTOC) > 0 {
			initRel = filepath.ToSlash(filepath.Join("stdlib", "releases", entry.Family, entry.Contract+"_init.toc"))
			if err := os.WriteFile(filepath.Join(repoRoot, initRel), built.InitTOC, 0o644); err != nil {
				fatalf("write %s: %v", initRel, err)
			}
		}
		if err := os.WriteFile(filepath.Join(repoRoot, torRel), built.PackageTOR, 0o644); err != nil {
			fatalf("write %s: %v", torRel, err)
		}

		art, err := lua.DecodeArtifact(built.ArtifactTOC)
		if err != nil {
			fatalf("decode artifact %s: %v", entry.Contract, err)
		}
		index.Entries = append(index.Entries, releaseIndexEntry{
			Family:             entry.Family,
			Contract:           entry.Contract,
			SourcePath:         entry.SourcePath,
			ReleasePackageName: entry.ReleasePackageName,
			TOCPath:            tocRel,
			ABIPath:            abiRel,
			InitPath:           initRel,
			TORPath:            torRel,
			BytecodeHash:       art.BytecodeHash,
			PackageHash:        lua.PackageHash(built.PackageTOR),
		})
	}

	indexBytes, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		fatalf("marshal release index: %v", err)
	}
	indexPath := filepath.Join(releaseRoot, "index.json")
	if err := os.WriteFile(indexPath, indexBytes, 0o644); err != nil {
		fatalf("write %s: %v", indexPath, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
