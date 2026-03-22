package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lua "github.com/tos-network/tolang"
	"github.com/tos-network/tolang/metadata"
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
	ProfilePath       string   `json:"profile_path"`
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
	ProtocolAlignment *metadata.ProtocolAlignment   `json:"protocol_alignment,omitempty"`
	HumanSummary      string                        `json:"human_summary"`
}

type releaseBundleAgentPackage struct {
	PackageName       string                       `json:"package_name"`
	PackageVersion    string                       `json:"package_version"`
	SourcePackagePath string                       `json:"source_package_path"`
	TORPath           string                       `json:"tor_path"`
	PackageHash       string                       `json:"package_hash"`
	Contracts         []*metadata.AgentPackageInfo `json:"contracts"`
	ProtocolAlignment *metadata.ProtocolAlignment  `json:"protocol_alignment,omitempty"`
	HumanSummary      string                       `json:"human_summary"`
}

type releaseBundleProfile = metadata.AgentBundleProfile

type releaseIndex struct {
	Version string               `json:"version"`
	Entries []releaseIndexEntry  `json:"entries"`
	Bundles []releaseIndexBundle `json:"bundles,omitempty"`
}

func main() {
	repoRoot, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}
	releaseRoot := filepath.Join(repoRoot, "openlib", "releases")
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		fatalf("mkdir %s: %v", releaseRoot, err)
	}

	entries := lua.OpenlibReleaseCatalog()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Family != entries[j].Family {
			return entries[i].Family < entries[j].Family
		}
		return entries[i].Contract < entries[j].Contract
	})

	index := releaseIndex{Version: "1.0.0"}
	sources := make(map[string][]byte, len(entries))
	familyEntries := make(map[string][]lua.OpenlibReleaseEntry)
	familyReleaseEntries := make(map[string][]releaseIndexEntry)
	contractDiscovery := make(map[string]*metadata.DiscoveryManifest, len(entries))
	contractAgentPackages := make(map[string]*metadata.AgentPackageInfo, len(entries))
	contractProfiles := make(map[string]*metadata.AgentContractProfile, len(entries))
	for _, entry := range entries {
		sourcePath := filepath.Join(repoRoot, entry.SourcePath)
		source := sources[entry.SourcePath]
		if len(source) == 0 {
			var err error
			source, err = os.ReadFile(sourcePath)
			if err != nil {
				fatalf("read %s: %v", sourcePath, err)
			}
			sources[entry.SourcePath] = source
		}
		built, err := lua.BuildOpenlibReleaseArtifacts(source, sourcePath, entry)
		if err != nil {
			fatalf("build release artifacts for %s: %v", entry.Contract, err)
		}

		dir := filepath.Join(releaseRoot, entry.Family)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fatalf("mkdir %s: %v", dir, err)
		}
		tocRel := filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, entry.Contract+".toc"))
		abiRel := filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, "I"+entry.Contract+".abi"))
		torRel := filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, entry.Contract+".tor"))
		if err := os.WriteFile(filepath.Join(repoRoot, tocRel), built.ArtifactTOC, 0o644); err != nil {
			fatalf("write %s: %v", tocRel, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, abiRel), built.InterfaceABI, 0o644); err != nil {
			fatalf("write %s: %v", abiRel, err)
		}
		initRel := ""
		if len(built.InitTOC) > 0 {
			initRel = filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, entry.Contract+"_init.toc"))
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
		meta, err := metadata.ExtractFromABI(art.ABIJSON)
		if err != nil {
			fatalf("extract metadata %s: %v", entry.Contract, err)
		}
		meta.Contract.Name = art.ContractName
		meta.ArtifactRef = metadata.ComputeArtifactRef(built.PackageTOR, art.Bytecode, source, art.ABIJSON, meta.ArtifactRef.Version)
		discovery := metadata.BuildDiscoveryManifest(meta, entry.ReleasePackageName)
		agentPkg := metadata.BuildAgentPackageInfo(meta, entry.ReleasePackageName)
		contractDiscovery[entry.Contract] = discovery
		contractAgentPackages[entry.Contract] = agentPkg
		discoveryBytes, err := json.MarshalIndent(discovery, "", "  ")
		if err != nil {
			fatalf("marshal discovery %s: %v", entry.Contract, err)
		}
		agentPkgBytes, err := json.MarshalIndent(agentPkg, "", "  ")
		if err != nil {
			fatalf("marshal agent package %s: %v", entry.Contract, err)
		}
		discoveryRel := filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, entry.Contract+".discovery.json"))
		agentPkgRel := filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, entry.Contract+".agentpkg.json"))
		if err := os.WriteFile(filepath.Join(repoRoot, discoveryRel), discoveryBytes, 0o644); err != nil {
			fatalf("write %s: %v", discoveryRel, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, agentPkgRel), agentPkgBytes, 0o644); err != nil {
			fatalf("write %s: %v", agentPkgRel, err)
		}
		profile := metadata.BuildAgentProfile(meta, entry.ReleasePackageName)
		profile.Identity.PackageName = entry.ReleasePackageName
		contractProfiles[entry.Contract] = profile
		profileBytes, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			fatalf("marshal profile %s: %v", entry.Contract, err)
		}
		profileRel := filepath.ToSlash(filepath.Join("openlib", "releases", entry.Family, entry.Contract+".profile.json"))
		if err := os.WriteFile(filepath.Join(repoRoot, profileRel), profileBytes, 0o644); err != nil {
			fatalf("write %s: %v", profileRel, err)
		}
		idxEntry := releaseIndexEntry{
			Family:             entry.Family,
			Contract:           entry.Contract,
			SourcePath:         entry.SourcePath,
			ReleasePackageName: entry.ReleasePackageName,
			TOCPath:            tocRel,
			ABIPath:            abiRel,
			InitPath:           initRel,
			TORPath:            torRel,
			DiscoveryPath:      discoveryRel,
			AgentPackagePath:   agentPkgRel,
			ProfilePath:        profileRel,
			BytecodeHash:       art.BytecodeHash,
			PackageHash:        lua.PackageHash(built.PackageTOR),
		}
		index.Entries = append(index.Entries, idxEntry)
		familyEntries[entry.Family] = append(familyEntries[entry.Family], entry)
		familyReleaseEntries[entry.Family] = append(familyReleaseEntries[entry.Family], idxEntry)
	}

	families := make([]string, 0, len(familyEntries))
	for family := range familyEntries {
		if len(familyEntries[family]) > 1 {
			families = append(families, family)
		}
	}
	sort.Strings(families)
	for _, family := range families {
		entries := append([]lua.OpenlibReleaseEntry(nil), familyEntries[family]...)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Contract < entries[j].Contract
		})
		bundleBytes, err := lua.BuildOpenlibFamilyBundlePackage(entries, sources)
		if err != nil {
			fatalf("build family bundle %s: %v", family, err)
		}
		bundleRel := filepath.ToSlash(filepath.Join("openlib", "releases", family, family+".tor"))
		if err := os.WriteFile(filepath.Join(repoRoot, bundleRel), bundleBytes, 0o644); err != nil {
			fatalf("write %s: %v", bundleRel, err)
		}
		contracts := make([]string, 0, len(entries))
		for _, entry := range entries {
			contracts = append(contracts, entry.Contract)
		}
		bundleCatalogRel := filepath.ToSlash(filepath.Join("openlib", "releases", family, family+".bundle.json"))
		contractEntries := append([]releaseIndexEntry(nil), familyReleaseEntries[family]...)
		sort.Slice(contractEntries, func(i, j int) bool {
			return contractEntries[i].Contract < contractEntries[j].Contract
		})
		bundleCatalog := releaseBundleCatalog{
			Version:           index.Version,
			Family:            family,
			SourcePackagePath: entries[0].SourcePackagePath,
			TORPath:           bundleRel,
			PackageHash:       lua.PackageHash(bundleBytes),
			Contracts:         make([]releaseBundleCatalogContract, 0, len(contractEntries)),
		}
		for _, idxEntry := range contractEntries {
			bundleCatalog.Contracts = append(bundleCatalog.Contracts, releaseBundleCatalogContract{
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
		bundleCatalogBytes, err := json.MarshalIndent(bundleCatalog, "", "  ")
		if err != nil {
			fatalf("marshal family bundle catalog %s: %v", family, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, bundleCatalogRel), bundleCatalogBytes, 0o644); err != nil {
			fatalf("write %s: %v", bundleCatalogRel, err)
		}
		bundleDiscoveryRel := filepath.ToSlash(filepath.Join("openlib", "releases", family, family+".bundle.discovery.json"))
		bundleAgentPkgRel := filepath.ToSlash(filepath.Join("openlib", "releases", family, family+".bundle.agentpkg.json"))
		bundleProfileRel := filepath.ToSlash(filepath.Join("openlib", "releases", family, family+".bundle.profile.json"))
		serviceKinds := []string{}
		capabilities := []string{}
		tags := []string{}
		discoveryContracts := make([]*metadata.DiscoveryManifest, 0, len(contractEntries))
		agentContracts := make([]*metadata.AgentPackageInfo, 0, len(contractEntries))
		profileContracts := make([]*metadata.AgentContractProfile, 0, len(contractEntries))
		contractNames := make([]string, 0, len(contractEntries))
		for _, idxEntry := range contractEntries {
			contractNames = append(contractNames, idxEntry.Contract)
			if disc := contractDiscovery[idxEntry.Contract]; disc != nil {
				discoveryContracts = append(discoveryContracts, disc)
				serviceKinds = append(serviceKinds, disc.ServiceKinds...)
				capabilities = append(capabilities, disc.Capabilities...)
				tags = append(tags, disc.Tags...)
			}
			if pkg := contractAgentPackages[idxEntry.Contract]; pkg != nil {
				agentContracts = append(agentContracts, pkg)
			}
			if profile := contractProfiles[idxEntry.Contract]; profile != nil {
				profileContracts = append(profileContracts, profile)
			}
		}
		bundleDiscovery := releaseBundleDiscovery{
			SchemaVersion:     metadata.DiscoverySchemaVersion,
			PackageName:       entries[0].SourcePackagePath,
			PackageVersion:    index.Version,
			SourcePackagePath: entries[0].SourcePackagePath,
			TORPath:           bundleRel,
			PackageHash:       lua.PackageHash(bundleBytes),
			ServiceKinds:      dedupStrings(serviceKinds),
			Capabilities:      dedupStrings(capabilities),
			Tags:              dedupStrings(tags),
			Contracts:         discoveryContracts,
			ProtocolAlignment: metadata.BuildBundleProtocolAlignment(protocolAlignmentsFromDiscovery(discoveryContracts)...),
			HumanSummary:      fmt.Sprintf("Family bundle for %s with contracts: %s", entries[0].SourcePackagePath, strings.Join(contractNames, ", ")),
		}
		bundleDiscoveryBytes, err := json.MarshalIndent(bundleDiscovery, "", "  ")
		if err != nil {
			fatalf("marshal family bundle discovery %s: %v", family, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, bundleDiscoveryRel), bundleDiscoveryBytes, 0o644); err != nil {
			fatalf("write %s: %v", bundleDiscoveryRel, err)
		}
		bundleAgentPkg := releaseBundleAgentPackage{
			PackageName:       entries[0].SourcePackagePath,
			PackageVersion:    index.Version,
			SourcePackagePath: entries[0].SourcePackagePath,
			TORPath:           bundleRel,
			PackageHash:       lua.PackageHash(bundleBytes),
			Contracts:         agentContracts,
			ProtocolAlignment: metadata.BuildBundleProtocolAlignment(protocolAlignmentsFromAgentPackages(agentContracts)...),
			HumanSummary:      fmt.Sprintf("Agent package bundle for %s with contracts: %s", entries[0].SourcePackagePath, strings.Join(contractNames, ", ")),
		}
		bundleAgentPkgBytes, err := json.MarshalIndent(bundleAgentPkg, "", "  ")
		if err != nil {
			fatalf("marshal family bundle agent package %s: %v", family, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, bundleAgentPkgRel), bundleAgentPkgBytes, 0o644); err != nil {
			fatalf("write %s: %v", bundleAgentPkgRel, err)
		}
		bundleProfile := metadata.BuildAgentBundleProfile(family, entries[0].SourcePackagePath, index.Version, profileContracts)
		bundleProfileBytes, err := json.MarshalIndent(bundleProfile, "", "  ")
		if err != nil {
			fatalf("marshal family bundle profile %s: %v", family, err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, bundleProfileRel), bundleProfileBytes, 0o644); err != nil {
			fatalf("write %s: %v", bundleProfileRel, err)
		}
		index.Bundles = append(index.Bundles, releaseIndexBundle{
			Family:            family,
			SourcePackagePath: entries[0].SourcePackagePath,
			Contracts:         contracts,
			TORPath:           bundleRel,
			CatalogPath:       bundleCatalogRel,
			DiscoveryPath:     bundleDiscoveryRel,
			AgentPackagePath:  bundleAgentPkgRel,
			ProfilePath:       bundleProfileRel,
			PackageHash:       lua.PackageHash(bundleBytes),
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

func dedupStrings(items []string) []string {
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

func protocolAlignmentsFromDiscovery(items []*metadata.DiscoveryManifest) []*metadata.ProtocolAlignment {
	alignments := make([]*metadata.ProtocolAlignment, 0, len(items))
	for _, item := range items {
		if item != nil {
			alignments = append(alignments, item.ProtocolAlignment)
		}
	}
	return alignments
}

func protocolAlignmentsFromAgentPackages(items []*metadata.AgentPackageInfo) []*metadata.ProtocolAlignment {
	alignments := make([]*metadata.ProtocolAlignment, 0, len(items))
	for _, item := range items {
		if item != nil {
			alignments = append(alignments, item.ProtocolAlignment)
		}
	}
	return alignments
}
