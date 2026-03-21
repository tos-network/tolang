package metadata

import (
	"strings"
)

// DiscoverySchemaVersion is the version of the discovery manifest schema.
const DiscoverySchemaVersion = "0.1.0"

// DiscoveryManifest is the standardized manifest format that OpenFox discovery clients
// can parse to understand what a TOL package provides.
type DiscoveryManifest struct {
	SchemaVersion    string                 `json:"schema_version"`
	PackageName      string                 `json:"package_name"`
	PackageVersion   string                 `json:"package_version"`
	ArtifactRef      ArtifactRef            `json:"artifact_ref"`
	ContractType     string                 `json:"contract_type"` // "token", "policy_wallet", "task_escrow", "oracle", "payment", "delegation", "custom"
	ServiceKinds     []string               `json:"service_kinds"` // what discovery service kinds this supports
	Capabilities     []string               `json:"capabilities"`
	Errors           []ErrorMeta            `json:"errors,omitempty"`
	InterfaceMethods []DiscoveryMethod      `json:"interface_methods"`
	PolicyProfile    *PolicyProfile         `json:"policy_profile,omitempty"`
	TypedDiscovery   *TypedDiscoveryProfile `json:"typed_discovery,omitempty"`
	HumanSummary     string                 `json:"human_summary"`
	Tags             []string               `json:"tags"`
}

// TypedDiscoveryProfile is the normalized routing-facing discovery shape that
// agent runtimes can consume without dereferencing ad hoc refs first.
type TypedDiscoveryProfile struct {
	SchemaVersion   string                 `json:"schema_version"`
	ServiceKind     string                 `json:"service_kind,omitempty"`
	CapabilityKind  string                 `json:"capability_kind,omitempty"`
	Pricing         *TypedDiscoveryPricing `json:"pricing,omitempty"`
	Privacy         *TypedDiscoveryPrivacy `json:"privacy,omitempty"`
	Receipts        *TypedDiscoveryReceipt `json:"receipts,omitempty"`
	SLA             *TypedDiscoverySLA     `json:"sla,omitempty"`
	Refs            *TypedDiscoveryRefs    `json:"refs,omitempty"`
	SupportedFields []string               `json:"supported_fields,omitempty"`
}

type TypedDiscoveryPricing struct {
	Kind    string `json:"kind,omitempty"`
	BaseFee string `json:"base_fee,omitempty"`
}

type TypedDiscoveryPrivacy struct {
	Mode            string `json:"mode,omitempty"`
	DisclosureReady bool   `json:"disclosure_ready,omitempty"`
}

type TypedDiscoveryReceipt struct {
	Mode string `json:"mode,omitempty"`
}

type TypedDiscoverySLA struct {
	DurationMS string `json:"duration_ms,omitempty"`
}

type TypedDiscoveryRefs struct {
	ManifestRef   string `json:"manifest_ref,omitempty"`
	CapabilityRef string `json:"capability_ref,omitempty"`
	VersionRef    string `json:"version_ref,omitempty"`
	TrustFloorRef string `json:"trust_floor_ref,omitempty"`
}

// DiscoveryMethod describes a single method in the discovery manifest.
type DiscoveryMethod struct {
	Name         string        `json:"name"`
	Selector     string        `json:"selector"`
	RiskLevel    string        `json:"risk_level"`
	Payable      bool          `json:"payable"`
	Delegated    bool          `json:"delegated"`
	Verifiable   bool          `json:"verifiable"`
	FailureModes []FailureMode `json:"failure_modes,omitempty"`
	Summary      string        `json:"summary"`
}

// BuildDiscoveryManifest creates a discovery manifest from contract metadata.
func BuildDiscoveryManifest(meta *ContractMetadata, packageName string) *DiscoveryManifest {
	dm := &DiscoveryManifest{
		SchemaVersion: DiscoverySchemaVersion,
		PackageName:   packageName,
		ArtifactRef:   meta.ArtifactRef,
		Capabilities:  meta.Capabilities,
		Errors:        meta.Errors,
		PolicyProfile: meta.PolicyProfile,
	}

	if meta.Manifest != nil {
		dm.PackageVersion = meta.Manifest.Version
	}

	dm.ContractType = InferContractType(meta)
	dm.ServiceKinds = InferServiceKinds(meta)
	dm.Tags = InferTags(meta)
	dm.TypedDiscovery = BuildTypedDiscoveryProfile(meta)

	// Build interface methods.
	for _, fn := range meta.Functions {
		if fn.Visibility == "internal" || fn.Visibility == "private" {
			continue
		}
		dm.InterfaceMethods = append(dm.InterfaceMethods, DiscoveryMethod{
			Name:         fn.Name,
			Selector:     fn.Selector,
			RiskLevel:    fn.RiskLevel,
			Payable:      fn.Mutability == "payable",
			Delegated:    fn.Delegated,
			Verifiable:   fn.Verifiable,
			FailureModes: fn.FailureModes,
			Summary:      GenerateFunctionDescription(fn),
		})
	}

	// Generate human summary from the human-readable module.
	summary := GenerateHumanReadable(meta)
	dm.HumanSummary = summary.RiskSummary

	return dm
}

// InferContractType infers the contract type from function names and patterns.
func InferContractType(meta *ContractMetadata) string {
	if meta.IsAccount {
		return "policy_wallet"
	}

	// Check manifest spec hint.
	if meta.Manifest != nil && meta.Manifest.Spec != "" {
		spec := strings.ToLower(meta.Manifest.Spec)
		if strings.Contains(spec, "trc-20") || strings.Contains(spec, "erc-20") || strings.Contains(spec, "token") {
			return "token"
		}
	}

	names := collectFunctionNames(meta)

	// Token patterns.
	if containsAny(names, "transfer", "approve", "allowance", "balance_of", "balanceof", "total_supply", "totalsupply") {
		return "token"
	}
	// Task escrow patterns.
	if containsAny(names, "create_task", "complete_task", "dispute", "escrow", "release_funds") {
		return "task_escrow"
	}
	// Oracle patterns.
	if containsAny(names, "submit_price", "get_price", "update_feed", "oracle", "report") {
		return "oracle"
	}
	// Payment patterns.
	if containsAny(names, "pay", "withdraw", "deposit", "split_payment") {
		return "payment"
	}
	// Delegation patterns.
	if containsAny(names, "delegate", "undelegate", "redelegate", "delegation") {
		return "delegation"
	}
	// Discovery patterns.
	if containsAny(
		names,
		"registerservice",
		"servicerefof",
		"servicecount",
		"servicekindof",
		"capabilitykindof",
		"capabilitytypeof",
	) {
		return "discovery"
	}

	return "custom"
}

// InferServiceKinds infers what discovery service kinds this contract supports.
func InferServiceKinds(meta *ContractMetadata) []string {
	var kinds []string

	contractType := InferContractType(meta)

	// All contracts support basic query.
	kinds = append(kinds, "query")

	switch contractType {
	case "token":
		kinds = append(kinds, "token_transfer", "balance_query")
	case "policy_wallet":
		kinds = append(kinds, "account_management", "policy_enforcement")
	case "task_escrow":
		kinds = append(kinds, "task_management", "escrow")
	case "oracle":
		kinds = append(kinds, "data_feed", "price_query")
	case "payment":
		kinds = append(kinds, "payment_processing")
	case "delegation":
		kinds = append(kinds, "delegation_management")
	case "discovery":
		kinds = append(kinds, "discovery_registry", "directory_query")
	}

	// Check for verifiable functions.
	for _, fn := range meta.Functions {
		if fn.Verifiable {
			kinds = append(kinds, "verifiable_compute")
			break
		}
	}

	// Check for delegated functions.
	for _, fn := range meta.Functions {
		if fn.Delegated {
			kinds = append(kinds, "delegated_execution")
			break
		}
	}

	return dedup(kinds)
}

// InferTags generates searchable tags from metadata.
func InferTags(meta *ContractMetadata) []string {
	var tags []string

	// Add contract type as a tag.
	contractType := InferContractType(meta)
	tags = append(tags, contractType)

	// Add capabilities as tags.
	tags = append(tags, meta.Capabilities...)

	// Add spec as tag if present.
	if meta.Manifest != nil && meta.Manifest.Spec != "" {
		tags = append(tags, strings.ToLower(meta.Manifest.Spec))
	}

	// Infer semantic tags from function names.
	names := collectFunctionNames(meta)

	if containsAny(names, "transfer", "send") {
		tags = append(tags, "transferable")
	}
	if containsAny(names, "approve", "allowance") {
		tags = append(tags, "approvable")
	}
	if meta.IsAccount {
		tags = append(tags, "account")
	}
	if BuildTypedDiscoveryProfile(meta) != nil {
		tags = append(tags, "typed-discovery")
	}

	// Tag based on policy features.
	if meta.PolicyProfile != nil {
		if meta.PolicyProfile.HasSpendCaps {
			tags = append(tags, "spend-caps")
		}
		if meta.PolicyProfile.HasGuardian {
			tags = append(tags, "guardian")
		}
		if meta.PolicyProfile.HasRecovery {
			tags = append(tags, "recoverable")
		}
		if meta.PolicyProfile.HasDelegation {
			tags = append(tags, "delegatable")
		}
	}

	// Tag based on risk.
	for _, fn := range meta.Functions {
		if fn.Verifiable {
			tags = append(tags, "verifiable")
			break
		}
	}

	return dedup(tags)
}

// BuildTypedDiscoveryProfile derives a normalized typed discovery view from
// release metadata when the contract exposes typed discovery schema APIs.
func BuildTypedDiscoveryProfile(meta *ContractMetadata) *TypedDiscoveryProfile {
	names := collectFunctionNames(meta)
	supported := []string{}
	if containsAny(names, "servicekindof", "setservicekind") {
		supported = append(supported, "service_kind")
	}
	if containsAny(names, "capabilitykindof", "setcapabilitykind", "capabilitytypeof", "setcapabilitytype") {
		supported = append(supported, "capability_kind")
	}
	if containsAny(names, "pricingkindof", "setpricingkind", "feeof", "setservicefee") {
		supported = append(supported, "pricing_kind", "fee_amount")
	}
	if containsAny(names, "privacymodeof", "setprivacymode") {
		supported = append(supported, "privacy_mode")
	}
	if containsAny(names, "receiptmodeof", "setreceiptmode") {
		supported = append(supported, "receipt_mode")
	}
	if containsAny(names, "slaof", "setservicesla") {
		supported = append(supported, "sla_duration_ms")
	}
	if containsAny(names, "trustfloorrefof", "settrustfloorref") {
		supported = append(supported, "trust_floor_ref")
	}
	if containsAny(names, "manifestrefof") {
		supported = append(supported, "manifest_ref")
	}
	if containsAny(names, "capabilityrefof") {
		supported = append(supported, "capability_ref")
	}
	if containsAny(names, "quoterefof") {
		supported = append(supported, "quote_ref")
	}
	if len(supported) == 0 {
		return nil
	}

	profile := &TypedDiscoveryProfile{
		SchemaVersion:   DiscoverySchemaVersion,
		SupportedFields: dedup(supported),
	}

	if InferContractType(meta) == "discovery" {
		profile.ServiceKind = "DISCOVERY"
		profile.CapabilityKind = "READ_ONLY"
		profile.Pricing = &TypedDiscoveryPricing{Kind: "FREE", BaseFee: "0"}
		profile.Privacy = &TypedDiscoveryPrivacy{Mode: "PUBLIC_ONLY"}
		profile.Receipts = &TypedDiscoveryReceipt{Mode: "MANUAL_RECEIPT"}
		profile.SLA = &TypedDiscoverySLA{DurationMS: "0"}
		profile.Refs = &TypedDiscoveryRefs{}
	}

	return profile
}

// collectFunctionNames returns lower-cased function names from metadata.
func collectFunctionNames(meta *ContractMetadata) []string {
	names := make([]string, len(meta.Functions))
	for i, fn := range meta.Functions {
		names[i] = strings.ToLower(fn.Name)
	}
	return names
}

// containsAny returns true if any of the targets appear in the haystack.
func containsAny(haystack []string, targets ...string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, t := range targets {
		if set[t] {
			return true
		}
	}
	return false
}
