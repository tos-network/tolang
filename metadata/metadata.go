// Package metadata defines the stable, versioned metadata schema for the GTOS 2046
// architecture. It is the contract between TOL, OpenFox, and GTOS: any consumer
// that reads .toc artifacts can depend on these types remaining backward-compatible
// within a major schema version.
package metadata

// SchemaVersion is the metadata schema version for 2046 boundary compatibility.
const SchemaVersion = "0.1.0"

// ContractMetadata is the top-level metadata structure emitted in .toc artifacts.
type ContractMetadata struct {
	SchemaVersion string         `json:"schema_version"`
	ArtifactRef   ArtifactRef    `json:"artifact_ref"`
	Contract      ContractInfo   `json:"contract"`
	Functions     []FunctionMeta `json:"functions"`
	Events        []EventMeta    `json:"events"`
	Manifest      *ManifestMeta  `json:"manifest,omitempty"`
	GasModel      GasModelMeta   `json:"gas_model"`
	Capabilities  []string       `json:"capabilities,omitempty"`
	IsAccount     bool           `json:"is_account"`
	PolicyProfile *PolicyProfile `json:"policy_profile,omitempty"`
}

// ArtifactRef is the canonical artifact identity for cross-system references.
type ArtifactRef struct {
	PackageHash  string `json:"package_hash"`            // keccak256 of .tor
	BytecodeHash string `json:"bytecode_hash"`           // keccak256 of bytecode
	SourceHash   string `json:"source_hash,omitempty"`   // keccak256 of source
	ABIHash      string `json:"abi_hash"`                // keccak256 of ABI JSON
	Version      string `json:"version,omitempty"`       // from manifest
}

// ContractInfo contains contract-level metadata.
type ContractInfo struct {
	Name          string   `json:"name"`
	BaseContracts []string `json:"base_contracts,omitempty"`
	IsAccount     bool     `json:"is_account"`
	StorageSlots  int      `json:"storage_slots"`
}

// FunctionMeta is the per-function metadata used by OpenFox for intent routing
// and approval UX.
type FunctionMeta struct {
	Name               string       `json:"name"`
	Selector           string       `json:"selector"`
	Visibility         string       `json:"visibility"`
	Mutability         string       `json:"mutability"`                    // "pure", "view", "payable", "nonpayable"
	Params             []ParamMeta  `json:"params"`
	Returns            []ParamMeta  `json:"returns,omitempty"`
	RequiresCapability []string     `json:"requires_capability,omitempty"`
	Effects            *EffectsMeta `json:"effects,omitempty"`
	GasUpper           uint64       `json:"gas_upper,omitempty"`
	Verifiable         bool         `json:"verifiable"`
	Delegated          bool         `json:"delegated"`
	NonComposable      bool         `json:"non_composable"`
	RiskLevel          string       `json:"risk_level,omitempty"` // "low", "medium", "high" - derived from effects
}

// ParamMeta describes a single function parameter or return value.
type ParamMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EffectsMeta captures what a function reads, writes, emits, and calls.
type EffectsMeta struct {
	Reads  []string   `json:"reads,omitempty"`
	Writes []string   `json:"writes,omitempty"`
	Emits  []string   `json:"emits,omitempty"`
	Calls  []CallMeta `json:"calls,omitempty"`
}

// CallMeta describes a single external call made by a function.
type CallMeta struct {
	Capability string `json:"capability,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Selector   string `json:"selector,omitempty"`
	MaxGas     uint64 `json:"max_gas,omitempty"`
}

// EventMeta describes an event emitted by a contract.
type EventMeta struct {
	Name   string      `json:"name"`
	Params []ParamMeta `json:"params"`
}

// ManifestMeta holds metadata from the manifest block.
type ManifestMeta struct {
	Version      string            `json:"version,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Spec         string            `json:"spec,omitempty"`
	SLAUptime    string            `json:"sla_uptime,omitempty"`
	Custom       map[string]string `json:"custom,omitempty"`
}

// GasModelMeta describes the gas cost model used during compilation.
type GasModelMeta struct {
	Version string `json:"version"`
	SLoad   uint64 `json:"sload"`
	SStore  uint64 `json:"sstore"`
	LogBase uint64 `json:"log_base"`
}

// PolicyProfile describes the policy-wallet characteristics of an account contract.
// This helps OpenFox understand what policy features a contract supports.
type PolicyProfile struct {
	HasSpendCaps      bool `json:"has_spend_caps"`
	HasAllowlist      bool `json:"has_allowlist"`
	HasTerminalPolicy bool `json:"has_terminal_policy"`
	HasGuardian       bool `json:"has_guardian"`
	HasRecovery       bool `json:"has_recovery"`
	HasDelegation     bool `json:"has_delegation"`
	HasSuspension     bool `json:"has_suspension"`
}
