package metadata

// AgentProfileSchemaVersion is the schema version for AgentContractProfile.
const AgentProfileSchemaVersion = "0.2.0"

// AgentContractProfile is the unified, agent-facing profile for a single contract.
// It consolidates identity, capabilities, functions, events, errors, discovery,
// and human-readable metadata into one structure that agent runtimes can consume
// without chasing multiple JSON files.
type AgentContractProfile struct {
	SchemaVersion  string                 `json:"schema_version"`
	Identity       ProfileIdentity        `json:"identity"`
	Contract       ProfileContract        `json:"contract"`
	Capabilities   []string               `json:"capabilities,omitempty"`
	Functions      []FunctionMeta         `json:"functions"`
	Events         []EventMeta            `json:"events,omitempty"`
	Errors         []ErrorMeta            `json:"errors,omitempty"`
	ServiceKinds   []string               `json:"service_kinds,omitempty"`
	HumanSummary   string                 `json:"human_summary,omitempty"`
	GasModel       *GasModelMeta          `json:"gas_model,omitempty"`
	TypedDiscovery *TypedDiscoveryProfile `json:"typed_discovery,omitempty"`
}

// ProfileIdentity captures artifact identity and content hashes for an agent profile.
type ProfileIdentity struct {
	PackageName    string `json:"package_name,omitempty"`
	PackageVersion string `json:"package_version,omitempty"`
	ContentHash    string `json:"content_hash,omitempty"`
	BytecodeHash   string `json:"bytecode_hash,omitempty"`
	ABIHash        string `json:"abi_hash,omitempty"`
}

// ProfileContract describes the contract-level metadata within an agent profile.
type ProfileContract struct {
	Name          string         `json:"name"`
	Type          string         `json:"type,omitempty"`
	IsAccount     bool           `json:"is_account,omitempty"`
	PolicyProfile *PolicyProfile `json:"policy_profile,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
}

// BuildAgentProfile constructs a unified AgentContractProfile from ContractMetadata.
// It reuses the same inference logic as BuildDiscoveryManifest for service kinds,
// typed discovery, tags, and human summary generation.
func BuildAgentProfile(cm *ContractMetadata) *AgentContractProfile {
	p := &AgentContractProfile{
		SchemaVersion: AgentProfileSchemaVersion,
	}

	// Identity from ArtifactRef.
	p.Identity = ProfileIdentity{
		ContentHash:  cm.ArtifactRef.PackageHash,
		BytecodeHash: cm.ArtifactRef.BytecodeHash,
		ABIHash:      cm.ArtifactRef.ABIHash,
	}
	if cm.ArtifactRef.Version != "" {
		p.Identity.PackageVersion = cm.ArtifactRef.Version
	}

	// Contract.
	p.Contract = ProfileContract{
		Name:      cm.Contract.Name,
		IsAccount: cm.Contract.IsAccount || cm.IsAccount,
	}

	// Contract.Type: infer from metadata.
	p.Contract.Type = InferContractType(cm)

	// PolicyProfile: use existing or derive.
	if cm.PolicyProfile != nil {
		p.Contract.PolicyProfile = cm.PolicyProfile
	} else if p.Contract.IsAccount {
		p.Contract.PolicyProfile = DerivePolicyProfile(cm.Functions)
	}

	// Tags.
	p.Contract.Tags = InferTags(cm)

	// Capabilities.
	if len(cm.Capabilities) > 0 {
		p.Capabilities = cm.Capabilities
	}

	// Functions (direct copy).
	p.Functions = cm.Functions

	// Events.
	p.Events = cm.Events

	// Errors.
	p.Errors = cm.Errors

	// ServiceKinds.
	p.ServiceKinds = InferServiceKinds(cm)

	// HumanSummary.
	summary := GenerateHumanReadable(cm)
	p.HumanSummary = summary.RiskSummary

	// GasModel.
	gm := cm.GasModel
	p.GasModel = &gm

	// TypedDiscovery.
	p.TypedDiscovery = BuildTypedDiscoveryProfile(cm)

	return p
}
