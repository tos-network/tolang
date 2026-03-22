package metadata

import "strings"

// ProtocolAlignmentSchemaVersion labels the additive next-wave alignment schema.
const ProtocolAlignmentSchemaVersion = "0.1.0"

// ProtocolAlignment describes how an agent-facing artifact aligns with the next
// GTOS protocol wave. It is additive and optional: existing consumers can ignore
// it, while newer runtimes can use it to route work toward settlement, registry,
// and package-governance surfaces.
type ProtocolAlignment struct {
	SchemaVersion      string   `json:"schema_version"`
	SettlementBus      bool     `json:"settlement_bus,omitempty"`
	RegistryGovernance bool     `json:"registry_governance,omitempty"`
	PackageGovernance  bool     `json:"package_governance,omitempty"`
	ReleaseArtifacts   []string `json:"release_artifacts,omitempty"`
	Notes              []string `json:"notes,omitempty"`
}

// BuildProtocolAlignment derives the next-wave GTOS alignment markers from the
// contract metadata and optional package name. Package name is used to mark
// release artifacts as package-governed when the artifact is being exported as
// part of a concrete package.
func BuildProtocolAlignment(meta *ContractMetadata, packageName string) *ProtocolAlignment {
	if meta == nil {
		return nil
	}

	align := &ProtocolAlignment{
		SchemaVersion:    ProtocolAlignmentSchemaVersion,
		ReleaseArtifacts: []string{"profile_json", "discovery_json", "agent_package_json"},
	}

	names := collectFunctionNames(meta)
	contractType := InferContractType(meta)
	contractName := strings.ToLower(meta.Contract.Name)

	if meta.IsAccount ||
		strings.Contains(contractName, "settlement") ||
		strings.Contains(contractName, "escrow") ||
		strings.Contains(contractName, "receipt") ||
		strings.Contains(contractName, "payment") ||
		strings.Contains(contractName, "sponsor") ||
		containsAny(names, "settle", "receipt", "refund", "release", "escrow", "slash", "sponsor") {
		align.SettlementBus = true
	}
	if contractType == "payment" || contractType == "task_escrow" || contractType == "agreement" {
		align.SettlementBus = true
	}
	if contractType == "discovery" || containsAny(names, "registerservice", "servicecount", "servicekindof", "capabilitykindof", "capabilitytypeof", "manifestrefof", "trustfloorrefof", "publisher", "revoke") {
		align.RegistryGovernance = true
	}
	if packageName != "" {
		align.PackageGovernance = true
	}

	if !align.SettlementBus && !align.RegistryGovernance && !align.PackageGovernance {
		align.Notes = append(align.Notes, "release_alignment_ready")
	}
	if len(meta.Capabilities) > 0 {
		align.Notes = append(align.Notes, "capability_profile_available")
	}

	return align
}

// MergeProtocolAlignments combines several alignment records into one.
func MergeProtocolAlignments(alignments ...*ProtocolAlignment) *ProtocolAlignment {
	merged := &ProtocolAlignment{
		SchemaVersion: ProtocolAlignmentSchemaVersion,
	}

	releaseArtifacts := map[string]bool{}
	notesSeen := map[string]bool{}
	for _, align := range alignments {
		if align == nil {
			continue
		}
		if align.SettlementBus {
			merged.SettlementBus = true
		}
		if align.RegistryGovernance {
			merged.RegistryGovernance = true
		}
		if align.PackageGovernance {
			merged.PackageGovernance = true
		}
		for _, art := range align.ReleaseArtifacts {
			if art == "" {
				continue
			}
			if !releaseArtifacts[art] {
				releaseArtifacts[art] = true
				merged.ReleaseArtifacts = append(merged.ReleaseArtifacts, art)
			}
		}
		for _, note := range align.Notes {
			if !notesSeen[note] {
				notesSeen[note] = true
				merged.Notes = append(merged.Notes, note)
			}
		}
	}

	if len(merged.ReleaseArtifacts) == 0 {
		merged.ReleaseArtifacts = []string{"profile_json", "discovery_json", "agent_package_json"}
	}
	if !merged.SettlementBus && !merged.RegistryGovernance && !merged.PackageGovernance {
		merged.Notes = append(merged.Notes, "release_alignment_ready")
	}
	return merged
}
