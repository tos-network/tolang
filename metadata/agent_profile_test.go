package metadata

import (
	"testing"
)

func TestBuildAgentProfile_Token(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		ArtifactRef: ArtifactRef{
			PackageHash:  "0xabc",
			BytecodeHash: "0xdef",
			ABIHash:      "0x123",
			Version:      "1.0.0",
		},
		Contract: ContractInfo{Name: "TestToken", StorageSlots: 3},
		Functions: []FunctionMeta{
			{
				Name:       "transfer",
				Selector:   "0xa9059cbb",
				Visibility: "external",
				Mutability: "payable",
				Verifiable: true,
				RiskLevel:  "high",
				Effects: &EffectsMeta{
					Reads:  []string{"balances"},
					Writes: []string{"balances"},
					Emits:  []string{"Transfer"},
				},
			},
			{
				Name:       "balanceOf",
				Selector:   "0x70a08231",
				Visibility: "public",
				Mutability: "view",
				RiskLevel:  "low",
				Effects:    &EffectsMeta{Reads: []string{"balances"}},
			},
		},
		Events: []EventMeta{
			{Name: "Transfer", Params: []ParamMeta{{Name: "from", Type: "address"}, {Name: "to", Type: "address"}}},
		},
		Errors: []ErrorMeta{
			{Name: "InsufficientBalance", Kind: "custom", Selector: "0xcf479181"},
		},
		Manifest: &ManifestMeta{
			Version: "1.0.0",
			Spec:    "TRC-20",
		},
		GasModel: GasModelMeta{
			Version: "1.0",
			SLoad:   2100,
			SStore:  20000,
			LogBase: 375,
		},
		Capabilities: []string{"token_send"},
	}

	p := BuildAgentProfile(meta)

	if p.SchemaVersion != AgentProfileSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", p.SchemaVersion, AgentProfileSchemaVersion)
	}
	if p.Identity.ContentHash != "0xabc" {
		t.Errorf("Identity.ContentHash = %q, want %q", p.Identity.ContentHash, "0xabc")
	}
	if p.Identity.BytecodeHash != "0xdef" {
		t.Errorf("Identity.BytecodeHash = %q, want %q", p.Identity.BytecodeHash, "0xdef")
	}
	if p.Identity.ABIHash != "0x123" {
		t.Errorf("Identity.ABIHash = %q, want %q", p.Identity.ABIHash, "0x123")
	}
	if p.Identity.PackageVersion != "1.0.0" {
		t.Errorf("Identity.PackageVersion = %q, want %q", p.Identity.PackageVersion, "1.0.0")
	}
	if p.Contract.Name != "TestToken" {
		t.Errorf("Contract.Name = %q, want %q", p.Contract.Name, "TestToken")
	}
	if p.Contract.Type != "token" {
		t.Errorf("Contract.Type = %q, want %q", p.Contract.Type, "token")
	}
	if p.Contract.IsAccount {
		t.Error("expected Contract.IsAccount = false for token")
	}
	if len(p.Functions) != 2 {
		t.Fatalf("len(Functions) = %d, want 2", len(p.Functions))
	}
	if p.Functions[0].Name != "transfer" {
		t.Errorf("Functions[0].Name = %q, want %q", p.Functions[0].Name, "transfer")
	}
	if len(p.Events) != 1 || p.Events[0].Name != "Transfer" {
		t.Errorf("Events = %+v, want one Transfer event", p.Events)
	}
	if len(p.Errors) != 1 || p.Errors[0].Name != "InsufficientBalance" {
		t.Errorf("Errors = %+v, want one InsufficientBalance error", p.Errors)
	}
	if len(p.Capabilities) != 1 || p.Capabilities[0] != "token_send" {
		t.Errorf("Capabilities = %v, want [token_send]", p.Capabilities)
	}
	if len(p.ServiceKinds) == 0 {
		t.Error("expected non-empty ServiceKinds")
	}
	if p.HumanSummary == "" {
		t.Error("expected non-empty HumanSummary")
	}
	if p.GasModel == nil {
		t.Fatal("expected non-nil GasModel")
	}
	if p.GasModel.SLoad != 2100 {
		t.Errorf("GasModel.SLoad = %d, want 2100", p.GasModel.SLoad)
	}
	if p.TypedDiscovery != nil {
		t.Error("expected nil TypedDiscovery for token contract")
	}
	if p.ThreatModel != nil {
		t.Errorf("expected nil ThreatModel for non-openlib token profile, got %+v", p.ThreatModel)
	}
	if p.ProtocolAlignment == nil {
		t.Fatal("expected non-nil ProtocolAlignment")
	}
	if p.ProtocolAlignment.PackageGovernance {
		t.Error("expected token profile built without release package name to remain package-governance agnostic")
	}
	if len(p.ProtocolAlignment.ReleaseArtifacts) == 0 {
		t.Error("expected release artifacts alignment to be populated")
	}
	if len(p.Contract.Tags) == 0 {
		t.Error("expected non-empty Tags")
	}
}

func TestBuildAgentProfile_AccountContract(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		ArtifactRef: ArtifactRef{
			PackageHash:  "0x111",
			BytecodeHash: "0x222",
			ABIHash:      "0x333",
		},
		Contract:  ContractInfo{Name: "PolicyAccount", IsAccount: true},
		IsAccount: true,
		Functions: []FunctionMeta{
			{
				Name:               "set_spend_cap",
				Selector:           "0x00000001",
				Visibility:         "external",
				Mutability:         "nonpayable",
				RequiresCapability: []string{"SPEND_ADMIN"},
				RiskLevel:          "medium",
				Effects:            &EffectsMeta{Writes: []string{"spend_caps"}},
			},
			{
				Name:       "execute",
				Selector:   "0x00000002",
				Visibility: "external",
				Mutability: "payable",
				Delegated:  true,
				RiskLevel:  "high",
				Effects: &EffectsMeta{
					Writes: []string{"nonce"},
					Calls:  []CallMeta{{Capability: "ANY"}},
				},
			},
		},
		Capabilities: []string{"SPEND_ADMIN"},
		PolicyProfile: &PolicyProfile{
			HasSpendCaps:  true,
			HasDelegation: true,
		},
	}

	p := BuildAgentProfile(meta)

	if p.Contract.Type != "policy_wallet" {
		t.Errorf("Contract.Type = %q, want %q", p.Contract.Type, "policy_wallet")
	}
	if !p.Contract.IsAccount {
		t.Error("expected Contract.IsAccount = true")
	}
	if p.Contract.PolicyProfile == nil {
		t.Fatal("expected non-nil PolicyProfile")
	}
	if !p.Contract.PolicyProfile.HasSpendCaps {
		t.Error("expected HasSpendCaps = true")
	}
	if !p.Contract.PolicyProfile.HasDelegation {
		t.Error("expected HasDelegation = true")
	}

	// ServiceKinds should include account_management and policy_enforcement.
	kindSet := make(map[string]bool)
	for _, k := range p.ServiceKinds {
		kindSet[k] = true
	}
	if !kindSet["account_management"] {
		t.Errorf("expected account_management in ServiceKinds, got %v", p.ServiceKinds)
	}
	if !kindSet["policy_enforcement"] {
		t.Errorf("expected policy_enforcement in ServiceKinds, got %v", p.ServiceKinds)
	}
}

func TestBuildAgentProfile_DerivesPolicyProfileWhenNil(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		Contract:      ContractInfo{Name: "MinimalAccount", IsAccount: true},
		IsAccount:     true,
		Functions: []FunctionMeta{
			{Name: "recover_account", Selector: "0x01", Visibility: "external", Mutability: "nonpayable"},
		},
	}

	p := BuildAgentProfile(meta)

	if p.Contract.PolicyProfile == nil {
		t.Fatal("expected PolicyProfile to be derived")
	}
	if !p.Contract.PolicyProfile.HasRecovery {
		t.Error("expected HasRecovery = true from function name heuristic")
	}
}

func TestBuildAgentProfile_PackageGovernanceAlignment(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		ArtifactRef: ArtifactRef{
			PackageHash:  "0x444",
			BytecodeHash: "0x555",
			ABIHash:      "0x666",
		},
		Contract: ContractInfo{Name: "TaskSettlement"},
		Functions: []FunctionMeta{
			{Name: "openTask", Selector: "0x01", Visibility: "external"},
			{Name: "approveTask", Selector: "0x02", Visibility: "external"},
			{Name: "refundTask", Selector: "0x03", Visibility: "external"},
		},
	}

	p := BuildAgentProfile(meta, "tolang.openlib.settlement.task")
	if p.ProtocolAlignment == nil {
		t.Fatal("expected non-nil ProtocolAlignment")
	}
	if !p.ProtocolAlignment.PackageGovernance {
		t.Error("expected package-governance alignment for release-built profile")
	}
	if !p.ProtocolAlignment.SettlementBus {
		t.Error("expected settlement-bus alignment for settlement contract")
	}
	if p.ThreatModel == nil {
		t.Fatal("expected threat model for openlib settlement profile")
	}
	if p.ThreatModel.Family != "settlement" {
		t.Fatalf("ThreatModel.Family = %q, want settlement", p.ThreatModel.Family)
	}
	if p.ThreatModel.Scope != "contract" {
		t.Fatalf("ThreatModel.Scope = %q, want contract", p.ThreatModel.Scope)
	}
	if p.ThreatModel.FailurePosture == "" || p.ThreatModel.RuntimeDependency == "" {
		t.Fatalf("expected populated threat model, got %+v", p.ThreatModel)
	}
}

func TestBuildAgentBundleProfile(t *testing.T) {
	contracts := []*AgentContractProfile{
		{
			SchemaVersion: AgentProfileSchemaVersion,
			Contract:      ProfileContract{Name: "ConfidentialEscrow", Type: "payment"},
			ProtocolAlignment: &ProtocolAlignment{
				SchemaVersion:     ProtocolAlignmentSchemaVersion,
				SettlementBus:     true,
				PackageGovernance: true,
				ReleaseArtifacts:  []string{"profile_json", "discovery_json"},
			},
		},
		{
			SchemaVersion: AgentProfileSchemaVersion,
			Contract:      ProfileContract{Name: "ConfidentialVault", Type: "custom"},
			ProtocolAlignment: &ProtocolAlignment{
				SchemaVersion:      ProtocolAlignmentSchemaVersion,
				RegistryGovernance: true,
				PackageGovernance:  true,
				ReleaseArtifacts:   []string{"agent_package_json"},
			},
		},
	}

	bundle := BuildAgentBundleProfile("privacy", "tolang.openlib.privacy", "1.0.0", contracts)
	if bundle == nil {
		t.Fatal("expected non-nil bundle profile")
	}
	if bundle.SchemaVersion != AgentBundleProfileSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", bundle.SchemaVersion, AgentBundleProfileSchemaVersion)
	}
	if bundle.Family != "privacy" || bundle.PackageName != "tolang.openlib.privacy" || bundle.PackageVersion != "1.0.0" {
		t.Fatalf("unexpected bundle identity %+v", bundle)
	}
	if len(bundle.Contracts) != 2 {
		t.Fatalf("len(Contracts) = %d, want 2", len(bundle.Contracts))
	}
	if bundle.ProtocolAlignment == nil {
		t.Fatal("expected merged protocol alignment")
	}
	if bundle.ThreatModel == nil {
		t.Fatal("expected bundle threat model")
	}
	if bundle.ThreatModel.Family != "privacy" || bundle.ThreatModel.Scope != "family_bundle" {
		t.Fatalf("unexpected bundle threat model %+v", bundle.ThreatModel)
	}
	if !bundle.ProtocolAlignment.SettlementBus || !bundle.ProtocolAlignment.RegistryGovernance || !bundle.ProtocolAlignment.PackageGovernance {
		t.Fatalf("unexpected merged protocol alignment %+v", bundle.ProtocolAlignment)
	}
	for _, want := range []string{"bundle_profile_json", "bundle_discovery_json", "bundle_agent_package_json"} {
		if !containsString(bundle.ProtocolAlignment.ReleaseArtifacts, want) {
			t.Fatalf("expected bundle release artifact %q in %v", want, bundle.ProtocolAlignment.ReleaseArtifacts)
		}
	}
	if bundle.HumanSummary == "" {
		t.Fatal("expected non-empty human summary")
	}
}

func TestBuildThreatModelProfile_OpenlibFamily(t *testing.T) {
	meta := &ContractMetadata{
		Contract: ContractInfo{Name: "SponsorPolicyRelay"},
		Functions: []FunctionMeta{
			{Name: "relay", Visibility: "external"},
		},
	}

	profile := BuildThreatModelProfile(meta, "tolang.openlib.sponsor")
	if profile == nil {
		t.Fatal("expected sponsor threat model")
	}
	if profile.Family != "sponsor" {
		t.Fatalf("Family = %q, want sponsor", profile.Family)
	}
	if profile.Scope != "contract" {
		t.Fatalf("Scope = %q, want contract", profile.Scope)
	}
	if profile.TrustBoundary == "" || len(profile.CriticalInvariants) == 0 {
		t.Fatalf("expected populated threat model, got %+v", profile)
	}
}
