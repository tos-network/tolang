package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AgentPackageInfo is the standard format for discovery-inspectable agent packages.
// It bundles the capability manifest and discovery manifest into a single,
// canonical structure that discovery clients can fetch and parse.
type AgentPackageInfo struct {
	PackageName       string                  `json:"package_name"`
	PackageVersion    string                  `json:"package_version"`
	ArtifactRef       ArtifactRef             `json:"artifact_ref"`
	Errors            []ErrorMeta             `json:"errors,omitempty"`
	Capabilities      *CapabilityManifest     `json:"capabilities"`
	Discovery         *DiscoveryManifest      `json:"discovery"`
	ThreatModel       *ThreatModelProfile     `json:"threat_model,omitempty"`
	HumanSummary      string                  `json:"human_summary"`
	ProtocolAlignment *ProtocolAlignment      `json:"protocol_alignment,omitempty"`
	RuntimeBoundary   *RuntimeBoundaryProfile `json:"runtime_boundary,omitempty"`
}

// BuildAgentPackageInfo builds a full discovery-inspectable package info record
// from contract metadata and a package name. This is the canonical way to produce
// an agent-economy package that discovery clients can inspect.
func BuildAgentPackageInfo(meta *ContractMetadata, packageName string) *AgentPackageInfo {
	capManifest := BuildCapabilityManifest(meta)
	discManifest := BuildDiscoveryManifest(meta, packageName)

	version := ""
	if meta.Manifest != nil {
		version = meta.Manifest.Version
	}

	summary := GenerateHumanReadable(meta)

	return &AgentPackageInfo{
		PackageName:       packageName,
		PackageVersion:    version,
		ArtifactRef:       meta.ArtifactRef,
		Errors:            meta.Errors,
		Capabilities:      capManifest,
		Discovery:         discManifest,
		ThreatModel:       BuildThreatModelProfile(meta, packageName),
		HumanSummary:      summary.RiskSummary,
		ProtocolAlignment: BuildProtocolAlignment(meta, packageName),
		RuntimeBoundary:   BuildRuntimeBoundaryProfile(meta, packageName),
	}
}

// BuildAllAgentEconomyPackages scans the agent_economy example directory under
// repoRoot for .toc files, reads their embedded metadata, and builds an
// AgentPackageInfo for each one.
func BuildAllAgentEconomyPackages(repoRoot string) ([]*AgentPackageInfo, error) {
	dir := filepath.Join(repoRoot, "examples", "agent_economy")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var packages []*AgentPackageInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toc") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		meta, err := extractMetadataFromTOC(data)
		if err != nil {
			// Skip files that don't contain valid embedded metadata.
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".toc")
		packages = append(packages, BuildAgentPackageInfo(meta, name))
	}

	return packages, nil
}

// extractMetadataFromTOC attempts to find and decode embedded JSON metadata
// from a .toc artifact. Returns an error if no valid metadata is found.
func extractMetadataFromTOC(data []byte) (*ContractMetadata, error) {
	// Look for the JSON metadata section by scanning for the opening brace
	// of a schema_version field.
	marker := []byte(`"schema_version"`)
	idx := findBytesIndex(data, marker)
	if idx < 0 {
		return nil, os.ErrNotExist
	}

	// Walk backward to find the opening brace.
	start := idx
	for start > 0 && data[start] != '{' {
		start--
	}
	if data[start] != '{' {
		return nil, os.ErrNotExist
	}

	// Find matching closing brace.
	depth := 0
	end := start
	for end < len(data) {
		switch data[end] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var meta ContractMetadata
				if err := json.Unmarshal(data[start:end+1], &meta); err != nil {
					return nil, err
				}
				return &meta, nil
			}
		}
		end++
	}
	return nil, os.ErrNotExist
}

// findBytesIndex finds the first occurrence of needle in haystack.
func findBytesIndex(haystack, needle []byte) int {
	if len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
