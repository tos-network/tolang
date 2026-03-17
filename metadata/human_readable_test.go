package metadata

import (
	"strings"
	"testing"
)

func TestGenerateHumanReadable_Basic(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		Contract: ContractInfo{
			Name:      "TestToken",
			IsAccount: false,
		},
		Functions: []FunctionMeta{
			{
				Name:       "transfer",
				Selector:   "0xa9059cbb",
				Visibility: "external",
				Mutability: "payable",
				Effects: &EffectsMeta{
					Reads:  []string{"balances"},
					Writes: []string{"balances"},
					Emits:  []string{"Transfer"},
					Calls:  []CallMeta{{Capability: "token", Interface: "ITRC20"}},
				},
				GasUpper:  65000,
				Verifiable: true,
				RiskLevel: "high",
			},
			{
				Name:       "balanceOf",
				Selector:   "0x70a08231",
				Visibility: "public",
				Mutability: "view",
				Effects: &EffectsMeta{
					Reads: []string{"balances"},
				},
				GasUpper:  2100,
				RiskLevel: "low",
			},
		},
		Events: []EventMeta{
			{Name: "Transfer"},
		},
		Manifest: &ManifestMeta{
			Version: "1.0.0",
			Spec:    "TRC-20",
		},
		Capabilities: []string{"token_send"},
		IsAccount:    false,
	}

	summary := GenerateHumanReadable(meta)

	if summary.ContractName != "TestToken" {
		t.Errorf("ContractName = %q, want %q", summary.ContractName, "TestToken")
	}
	if summary.IsAccount {
		t.Error("expected IsAccount to be false")
	}
	if len(summary.Functions) != 2 {
		t.Fatalf("len(Functions) = %d, want 2", len(summary.Functions))
	}
	if summary.Functions[0].Name != "transfer" {
		t.Errorf("Functions[0].Name = %q, want %q", summary.Functions[0].Name, "transfer")
	}
	if !summary.Functions[0].Payable {
		t.Error("expected transfer to be payable")
	}
	if !summary.Functions[0].Verifiable {
		t.Error("expected transfer to be verifiable")
	}
	if summary.TotalGasUpper != 67100 {
		t.Errorf("TotalGasUpper = %d, want 67100", summary.TotalGasUpper)
	}
	if len(summary.Capabilities) != 1 || summary.Capabilities[0] != "token_send" {
		t.Errorf("Capabilities = %v, want [token_send]", summary.Capabilities)
	}
	if summary.RiskSummary == "" {
		t.Error("expected non-empty RiskSummary")
	}
}

func TestGenerateHumanReadable_AccountContract(t *testing.T) {
	meta := &ContractMetadata{
		Contract:  ContractInfo{Name: "PolicyWallet", IsAccount: true},
		IsAccount: true,
		Functions: []FunctionMeta{
			{Name: "set_spend_cap", RiskLevel: "medium", Effects: &EffectsMeta{Writes: []string{"caps"}}},
		},
		PolicyProfile: &PolicyProfile{
			HasSpendCaps: true,
			HasGuardian:  true,
		},
	}

	summary := GenerateHumanReadable(meta)

	if !summary.IsAccount {
		t.Error("expected IsAccount to be true")
	}
	if len(summary.PolicyFeatures) != 2 {
		t.Fatalf("len(PolicyFeatures) = %d, want 2", len(summary.PolicyFeatures))
	}
	if summary.PolicyFeatures[0] != "spend caps" {
		t.Errorf("PolicyFeatures[0] = %q, want %q", summary.PolicyFeatures[0], "spend caps")
	}
}

func TestGenerateFunctionDescription(t *testing.T) {
	tests := []struct {
		name string
		fn   FunctionMeta
		want string
	}{
		{
			name: "no effects",
			fn:   FunctionMeta{},
			want: "Pure computation with no state access",
		},
		{
			name: "reads only",
			fn: FunctionMeta{
				Effects: &EffectsMeta{Reads: []string{"balances"}},
			},
			want: "Reads balances",
		},
		{
			name: "reads and writes",
			fn: FunctionMeta{
				Effects: &EffectsMeta{
					Reads:  []string{"balances"},
					Writes: []string{"balances", "allowances"},
				},
			},
			want: "Reads balances, writes balances and allowances",
		},
		{
			name: "full effects",
			fn: FunctionMeta{
				Effects: &EffectsMeta{
					Reads:  []string{"balances"},
					Writes: []string{"balances"},
					Emits:  []string{"Transfer"},
					Calls:  []CallMeta{{Interface: "ITRC20"}},
				},
			},
			want: "Reads balances, writes balances, emits Transfer event, calls ITRC20",
		},
		{
			name: "calls without interface",
			fn: FunctionMeta{
				Effects: &EffectsMeta{
					Calls: []CallMeta{{Capability: "token"}},
				},
			},
			want: "calls token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateFunctionDescription(tt.fn)
			if got != tt.want {
				t.Errorf("GenerateFunctionDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateRiskSummary(t *testing.T) {
	meta := &ContractMetadata{
		Functions: []FunctionMeta{
			{Mutability: "payable", RiskLevel: "medium", Effects: &EffectsMeta{Writes: []string{"a"}}},
			{Mutability: "view", RiskLevel: "low", Effects: &EffectsMeta{Reads: []string{"a"}}},
			{Mutability: "nonpayable", RiskLevel: "high", Effects: &EffectsMeta{Writes: []string{"b"}, Calls: []CallMeta{{Capability: "x"}}}},
		},
	}

	risk := GenerateRiskSummary(meta)

	if !strings.HasPrefix(risk, "High risk:") {
		t.Errorf("expected risk to start with 'High risk:', got %q", risk)
	}
	if !strings.Contains(risk, "2 functions write state") {
		t.Errorf("expected '2 functions write state' in %q", risk)
	}
	if !strings.Contains(risk, "1 function is payable") {
		t.Errorf("expected '1 function is payable' in %q", risk)
	}
}

func TestGenerateRiskSummary_LowRisk(t *testing.T) {
	meta := &ContractMetadata{
		Functions: []FunctionMeta{
			{Mutability: "view", RiskLevel: "low"},
			{Mutability: "pure", RiskLevel: "low"},
		},
	}

	risk := GenerateRiskSummary(meta)
	if !strings.HasPrefix(risk, "Low risk:") {
		t.Errorf("expected risk to start with 'Low risk:', got %q", risk)
	}
}

func TestFormatForDisplay(t *testing.T) {
	summary := &HumanReadableSummary{
		ContractName:  "TestToken",
		Description:   "A test token contract",
		IsAccount:     false,
		RiskSummary:   "Low risk: 0 functions write state, 0 functions are payable, no external calls",
		Functions: []FunctionSummary{
			{Name: "transfer", Description: "Writes balances", RiskLevel: "medium", Payable: true},
		},
		Capabilities:  []string{"token_send"},
		TotalGasUpper: 65000,
	}

	output := summary.FormatForDisplay()

	if !strings.Contains(output, "TestToken") {
		t.Error("expected output to contain contract name")
	}
	if !strings.Contains(output, "transfer") {
		t.Error("expected output to contain function name")
	}
	if !strings.Contains(output, "token_send") {
		t.Error("expected output to contain capabilities")
	}
	if !strings.Contains(output, "65000") {
		t.Error("expected output to contain gas upper bound")
	}
}
