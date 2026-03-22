package lua

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tos-network/tolang/metadata"
)

func TestOpenlibReleaseArtifactsAreCurrent(t *testing.T) {
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
		DiscoveryPath      string `json:"discovery_path"`
		AgentPackagePath   string `json:"agent_package_path"`
		ProfilePath        string `json:"profile_path"`
		BytecodeHash       string `json:"bytecode_hash"`
		PackageHash        string `json:"package_hash"`
	}
	type releaseIndexBundle struct {
		Family            string   `json:"family"`
		SourcePackagePath string   `json:"source_package_path"`
		Contracts         []string `json:"contracts"`
		TORPath           string   `json:"tor_path"`
		CatalogPath       string   `json:"catalog_path"`
		DiscoveryPath     string   `json:"discovery_path"`
		AgentPackagePath  string   `json:"agent_package_path"`
		PackageHash       string   `json:"package_hash"`
	}
	type releaseBundleCatalogContract struct {
		Contract           string `json:"contract"`
		ReleasePackageName string `json:"release_package_name"`
		TOCPath            string `json:"toc_path"`
		ABIPath            string `json:"abi_path"`
		InitPath           string `json:"init_path,omitempty"`
		TORPath            string `json:"tor_path"`
		DiscoveryPath      string `json:"discovery_path"`
		AgentPackagePath   string `json:"agent_package_path"`
		ProfilePath        string `json:"profile_path"`
		BytecodeHash       string `json:"bytecode_hash"`
		PackageHash        string `json:"package_hash"`
	}
	type releaseBundleCatalog struct {
		Version           string                         `json:"version"`
		Family            string                         `json:"family"`
		SourcePackagePath string                         `json:"source_package_path"`
		TORPath           string                         `json:"tor_path"`
		PackageHash       string                         `json:"package_hash"`
		Contracts         []releaseBundleCatalogContract `json:"contracts"`
	}
	type releaseBundleDiscovery struct {
		SchemaVersion     string                        `json:"schema_version"`
		PackageName       string                        `json:"package_name"`
		PackageVersion    string                        `json:"package_version"`
		SourcePackagePath string                        `json:"source_package_path"`
		TORPath           string                        `json:"tor_path"`
		PackageHash       string                        `json:"package_hash"`
		ServiceKinds      []string                      `json:"service_kinds"`
		Capabilities      []string                      `json:"capabilities"`
		Tags              []string                      `json:"tags"`
		Contracts         []*metadata.DiscoveryManifest `json:"contracts"`
		HumanSummary      string                        `json:"human_summary"`
	}
	type releaseBundleAgentPackage struct {
		PackageName       string                       `json:"package_name"`
		PackageVersion    string                       `json:"package_version"`
		SourcePackagePath string                       `json:"source_package_path"`
		TORPath           string                       `json:"tor_path"`
		PackageHash       string                       `json:"package_hash"`
		Contracts         []*metadata.AgentPackageInfo `json:"contracts"`
		HumanSummary      string                       `json:"human_summary"`
	}
	var index struct {
		Version string               `json:"version"`
		Entries []releaseIndexEntry  `json:"entries"`
		Bundles []releaseIndexBundle `json:"bundles"`
	}

	indexPath := filepath.Join(repoRoot, "openlib", "releases", "index.json")
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

	catalog := OpenlibReleaseCatalog()
	if len(index.Entries) != len(catalog) {
		t.Fatalf("release index entry count: got=%d want=%d", len(index.Entries), len(catalog))
	}

	byContract := make(map[string]OpenlibReleaseEntry, len(catalog))
	byFamily := map[string][]OpenlibReleaseEntry{}
	indexEntriesByFamily := map[string][]releaseIndexEntry{}
	discoveryByContract := map[string]*metadata.DiscoveryManifest{}
	agentPkgByContract := map[string]*metadata.AgentPackageInfo{}
	for _, entry := range catalog {
		byContract[entry.Contract] = entry
		byFamily[entry.Family] = append(byFamily[entry.Family], entry)
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
		built, err := BuildOpenlibReleaseArtifacts(source, sourcePath, entry)
		if err != nil {
			t.Fatalf("build release artifacts %s: %v", entry.Contract, err)
		}

		tocBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.TOCPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.TOCPath, err)
		}
		if string(tocBytes) != string(built.ArtifactTOC) {
			t.Fatalf("stale toc for %s; rerun openlib exporter", entry.Contract)
		}

		abiBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.ABIPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.ABIPath, err)
		}
		if string(abiBytes) != string(built.InterfaceABI) {
			t.Fatalf("stale abi for %s; rerun openlib exporter", entry.Contract)
		}

		if idxEntry.InitPath != "" {
			initBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.InitPath))
			if err != nil {
				t.Fatalf("read %s: %v", idxEntry.InitPath, err)
			}
			if string(initBytes) != string(built.InitTOC) {
				t.Fatalf("stale init toc for %s; rerun openlib exporter", entry.Contract)
			}
		} else if len(built.InitTOC) != 0 {
			t.Fatalf("release index missing init path for %s", entry.Contract)
		}

		torBytes, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.TORPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.TORPath, err)
		}
		if string(torBytes) != string(built.PackageTOR) {
			t.Fatalf("stale tor for %s; rerun openlib exporter", entry.Contract)
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

		meta, err := metadata.ExtractFromABI(art.ABIJSON)
		if err != nil {
			t.Fatalf("extract metadata %s: %v", entry.Contract, err)
		}
		meta.Contract.Name = art.ContractName
		meta.ArtifactRef = metadata.ComputeArtifactRef(torBytes, art.Bytecode, source, art.ABIJSON, meta.ArtifactRef.Version)

		discoveryWant, err := json.MarshalIndent(metadata.BuildDiscoveryManifest(meta, entry.ReleasePackageName), "", "  ")
		if err != nil {
			t.Fatalf("marshal discovery %s: %v", entry.Contract, err)
		}
		discoveryGot, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.DiscoveryPath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.DiscoveryPath, err)
		}
		if string(discoveryGot) != string(discoveryWant) {
			t.Fatalf("stale discovery json for %s; rerun openlib exporter", entry.Contract)
		}
		discoveryByContract[entry.Contract] = metadata.BuildDiscoveryManifest(meta, entry.ReleasePackageName)

		agentPkgWant, err := json.MarshalIndent(metadata.BuildAgentPackageInfo(meta, entry.ReleasePackageName), "", "  ")
		if err != nil {
			t.Fatalf("marshal agent package %s: %v", entry.Contract, err)
		}
		agentPkgGot, err := os.ReadFile(filepath.Join(repoRoot, idxEntry.AgentPackagePath))
		if err != nil {
			t.Fatalf("read %s: %v", idxEntry.AgentPackagePath, err)
		}
		if string(agentPkgGot) != string(agentPkgWant) {
			t.Fatalf("stale agent package json for %s; rerun openlib exporter", entry.Contract)
		}
		agentPkgByContract[entry.Contract] = metadata.BuildAgentPackageInfo(meta, entry.ReleasePackageName)
		indexEntriesByFamily[idxEntry.Family] = append(indexEntriesByFamily[idxEntry.Family], idxEntry)
	}

	wantBundleCount := 0
	for _, entries := range byFamily {
		if len(entries) > 1 {
			wantBundleCount++
		}
	}
	if len(index.Bundles) != wantBundleCount {
		t.Fatalf("release bundle count: got=%d want=%d", len(index.Bundles), wantBundleCount)
	}

	bundleByFamily := make(map[string]releaseIndexBundle, len(index.Bundles))
	for _, bundle := range index.Bundles {
		bundleByFamily[bundle.Family] = bundle
	}

	sources := map[string][]byte{}
	for family, entries := range byFamily {
		if len(entries) < 2 {
			continue
		}
		bundle, ok := bundleByFamily[family]
		if !ok {
			t.Fatalf("release index missing bundle for family %q", family)
		}

		ordered := append([]OpenlibReleaseEntry(nil), entries...)
		sort.Slice(ordered, func(i, j int) bool {
			return ordered[i].Contract < ordered[j].Contract
		})
		for _, entry := range ordered {
			if _, ok := sources[entry.SourcePath]; ok {
				continue
			}
			sourcePath := filepath.Join(repoRoot, entry.SourcePath)
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read source %s: %v", sourcePath, err)
			}
			sources[entry.SourcePath] = source
		}
		bundleWant, err := BuildOpenlibFamilyBundlePackage(ordered, sources)
		if err != nil {
			t.Fatalf("build release family bundle %s: %v", family, err)
		}

		bundleGot, err := os.ReadFile(filepath.Join(repoRoot, bundle.TORPath))
		if err != nil {
			t.Fatalf("read %s: %v", bundle.TORPath, err)
		}
		if string(bundleGot) != string(bundleWant) {
			t.Fatalf("stale family bundle tor for %s; rerun openlib exporter", family)
		}
		if bundle.PackageHash != PackageHash(bundleGot) {
			t.Fatalf("family bundle hash mismatch for %s", family)
		}
		if bundle.SourcePackagePath != ordered[0].SourcePackagePath {
			t.Fatalf("family bundle source package path mismatch for %s: got=%q want=%q", family, bundle.SourcePackagePath, ordered[0].SourcePackagePath)
		}

		wantContracts := make([]string, 0, len(ordered))
		for _, entry := range ordered {
			wantContracts = append(wantContracts, entry.Contract)
		}
		if len(bundle.Contracts) != len(wantContracts) {
			t.Fatalf("family bundle contracts count mismatch for %s: got=%d want=%d", family, len(bundle.Contracts), len(wantContracts))
		}
		for i := range wantContracts {
			if bundle.Contracts[i] != wantContracts[i] {
				t.Fatalf("family bundle contract mismatch for %s at %d: got=%q want=%q", family, i, bundle.Contracts[i], wantContracts[i])
			}
		}

		pkg, err := DecodePackage(bundleGot)
		if err != nil {
			t.Fatalf("decode family bundle package %s: %v", family, err)
		}
		if err := VerifyPackageSignature(bundleGot); err != nil {
			t.Fatalf("verify family bundle signature %s: %v", family, err)
		}
		if len(pkg.ManifestJSON) == 0 {
			t.Fatalf("family bundle manifest missing for %s", family)
		}

		var manifest struct {
			Name      string `json:"name"`
			Package   string `json:"package"`
			Contracts []struct {
				Name string `json:"name"`
			} `json:"contracts"`
		}
		if err := json.Unmarshal(pkg.ManifestJSON, &manifest); err != nil {
			t.Fatalf("unmarshal family bundle manifest %s: %v", family, err)
		}
		if manifest.Name != ordered[0].SourcePackagePath {
			t.Fatalf("family bundle manifest name mismatch for %s: got=%q want=%q", family, manifest.Name, ordered[0].SourcePackagePath)
		}
		if manifest.Package != ordered[0].SourcePackagePath {
			t.Fatalf("family bundle manifest package mismatch for %s: got=%q want=%q", family, manifest.Package, ordered[0].SourcePackagePath)
		}
		if len(manifest.Contracts) != len(wantContracts) {
			t.Fatalf("family bundle manifest contracts count mismatch for %s: got=%d want=%d", family, len(manifest.Contracts), len(wantContracts))
		}
		for i := range wantContracts {
			if manifest.Contracts[i].Name != wantContracts[i] {
				t.Fatalf("family bundle manifest contract mismatch for %s at %d: got=%q want=%q", family, i, manifest.Contracts[i].Name, wantContracts[i])
			}
		}

		contractEntries := append([]releaseIndexEntry(nil), indexEntriesByFamily[family]...)
		sort.Slice(contractEntries, func(i, j int) bool {
			return contractEntries[i].Contract < contractEntries[j].Contract
		})
		catalogWantStruct := releaseBundleCatalog{
			Version:           index.Version,
			Family:            family,
			SourcePackagePath: bundle.SourcePackagePath,
			TORPath:           bundle.TORPath,
			PackageHash:       bundle.PackageHash,
			Contracts:         make([]releaseBundleCatalogContract, 0, len(contractEntries)),
		}
		for _, idxEntry := range contractEntries {
			catalogWantStruct.Contracts = append(catalogWantStruct.Contracts, releaseBundleCatalogContract{
				Contract:           idxEntry.Contract,
				ReleasePackageName: idxEntry.ReleasePackageName,
				TOCPath:            idxEntry.TOCPath,
				ABIPath:            idxEntry.ABIPath,
				InitPath:           idxEntry.InitPath,
				TORPath:            idxEntry.TORPath,
				DiscoveryPath:      idxEntry.DiscoveryPath,
				AgentPackagePath:   idxEntry.AgentPackagePath,
				ProfilePath:        idxEntry.ProfilePath,
				BytecodeHash:       idxEntry.BytecodeHash,
				PackageHash:        idxEntry.PackageHash,
			})
		}
		catalogWant, err := json.MarshalIndent(catalogWantStruct, "", "  ")
		if err != nil {
			t.Fatalf("marshal family bundle catalog %s: %v", family, err)
		}
		catalogGot, err := os.ReadFile(filepath.Join(repoRoot, bundle.CatalogPath))
		if err != nil {
			t.Fatalf("read %s: %v", bundle.CatalogPath, err)
		}
		if string(catalogGot) != string(catalogWant) {
			t.Fatalf("stale family bundle catalog for %s; rerun openlib exporter", family)
		}

		serviceKinds := []string{}
		capabilities := []string{}
		tags := []string{}
		discoveryContracts := make([]*metadata.DiscoveryManifest, 0, len(contractEntries))
		agentContracts := make([]*metadata.AgentPackageInfo, 0, len(contractEntries))
		for _, idxEntry := range contractEntries {
			if disc := discoveryByContract[idxEntry.Contract]; disc != nil {
				discoveryContracts = append(discoveryContracts, disc)
				serviceKinds = append(serviceKinds, disc.ServiceKinds...)
				capabilities = append(capabilities, disc.Capabilities...)
				tags = append(tags, disc.Tags...)
			}
			if pkg := agentPkgByContract[idxEntry.Contract]; pkg != nil {
				agentContracts = append(agentContracts, pkg)
			}
		}

		discoveryWantStruct := releaseBundleDiscovery{
			SchemaVersion:     metadata.DiscoverySchemaVersion,
			PackageName:       bundle.SourcePackagePath,
			PackageVersion:    index.Version,
			SourcePackagePath: bundle.SourcePackagePath,
			TORPath:           bundle.TORPath,
			PackageHash:       bundle.PackageHash,
			ServiceKinds:      bundleDedupStrings(serviceKinds),
			Capabilities:      bundleDedupStrings(capabilities),
			Tags:              bundleDedupStrings(tags),
			Contracts:         discoveryContracts,
			HumanSummary:      "Family bundle for " + bundle.SourcePackagePath + " with contracts: " + strings.Join(wantContracts, ", "),
		}
		discoveryWantBytes, err := json.MarshalIndent(discoveryWantStruct, "", "  ")
		if err != nil {
			t.Fatalf("marshal family bundle discovery %s: %v", family, err)
		}
		discoveryGotBytes, err := os.ReadFile(filepath.Join(repoRoot, bundle.DiscoveryPath))
		if err != nil {
			t.Fatalf("read %s: %v", bundle.DiscoveryPath, err)
		}
		if string(discoveryGotBytes) != string(discoveryWantBytes) {
			t.Fatalf("stale family bundle discovery for %s; rerun openlib exporter", family)
		}

		agentWantStruct := releaseBundleAgentPackage{
			PackageName:       bundle.SourcePackagePath,
			PackageVersion:    index.Version,
			SourcePackagePath: bundle.SourcePackagePath,
			TORPath:           bundle.TORPath,
			PackageHash:       bundle.PackageHash,
			Contracts:         agentContracts,
			HumanSummary:      "Agent package bundle for " + bundle.SourcePackagePath + " with contracts: " + strings.Join(wantContracts, ", "),
		}
		agentWantBytes, err := json.MarshalIndent(agentWantStruct, "", "  ")
		if err != nil {
			t.Fatalf("marshal family bundle agent package %s: %v", family, err)
		}
		agentGotBytes, err := os.ReadFile(filepath.Join(repoRoot, bundle.AgentPackagePath))
		if err != nil {
			t.Fatalf("read %s: %v", bundle.AgentPackagePath, err)
		}
		if string(agentGotBytes) != string(agentWantBytes) {
			t.Fatalf("stale family bundle agent package for %s; rerun openlib exporter", family)
		}
	}
}

func bundleDedupStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
