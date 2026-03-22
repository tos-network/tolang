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
			HasSpendCaps: true,
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
