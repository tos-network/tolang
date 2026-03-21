package metadata

import (
	"testing"
)

func TestBuildDiscoveryManifest(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		ArtifactRef: ArtifactRef{
			PackageHash:  "0xabc",
			BytecodeHash: "0xdef",
			ABIHash:      "0x123",
			Version:      "1.0.0",
		},
		Contract: ContractInfo{Name: "TestToken"},
		Functions: []FunctionMeta{
			{
				Name:       "transfer",
				Selector:   "0xa9059cbb",
				Visibility: "external",
				Mutability: "payable",
				Verifiable: true,
				RiskLevel:  "high",
				FailureModes: []FailureMode{
					{Name: "Error", Kind: "error", Selector: "0x08c379a0"},
					{Name: "InsufficientBalance", Kind: "custom", Selector: "0xcf479181"},
				},
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
			{
				Name:       "_internal",
				Selector:   "0x00000000",
				Visibility: "internal",
				RiskLevel:  "low",
			},
		},
		Manifest: &ManifestMeta{
			Version: "1.0.0",
			Spec:    "TRC-20",
		},
		Errors: []ErrorMeta{
			{Name: "InsufficientBalance", Kind: "custom", Selector: "0xcf479181"},
		},
		Capabilities: []string{"token_send"},
	}

	dm := BuildDiscoveryManifest(meta, "tolang.tokens.test")

	if dm.SchemaVersion != DiscoverySchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", dm.SchemaVersion, DiscoverySchemaVersion)
	}
	if dm.PackageName != "tolang.tokens.test" {
		t.Errorf("PackageName = %q, want %q", dm.PackageName, "tolang.tokens.test")
	}
	if dm.PackageVersion != "1.0.0" {
		t.Errorf("PackageVersion = %q, want %q", dm.PackageVersion, "1.0.0")
	}
	if dm.ContractType != "token" {
		t.Errorf("ContractType = %q, want %q", dm.ContractType, "token")
	}
	// Internal functions should be excluded.
	if len(dm.InterfaceMethods) != 2 {
		t.Fatalf("len(InterfaceMethods) = %d, want 2 (internal excluded)", len(dm.InterfaceMethods))
	}
	if dm.InterfaceMethods[0].Name != "transfer" {
		t.Errorf("InterfaceMethods[0].Name = %q, want %q", dm.InterfaceMethods[0].Name, "transfer")
	}
	if len(dm.InterfaceMethods[0].FailureModes) != 2 {
		t.Fatalf("len(InterfaceMethods[0].FailureModes) = %d, want 2", len(dm.InterfaceMethods[0].FailureModes))
	}
	if len(dm.Errors) != 1 || dm.Errors[0].Name != "InsufficientBalance" {
		t.Fatalf("Errors = %+v, want one InsufficientBalance entry", dm.Errors)
	}
	if !dm.InterfaceMethods[0].Payable {
		t.Error("expected transfer to be payable")
	}
	if dm.HumanSummary == "" {
		t.Error("expected non-empty HumanSummary")
	}
	if len(dm.Tags) == 0 {
		t.Error("expected non-empty Tags")
	}
	if dm.TypedDiscovery != nil {
		t.Fatalf("token manifest should not expose typed discovery, got %+v", dm.TypedDiscovery)
	}
}

func TestInferContractType(t *testing.T) {
	tests := []struct {
		name     string
		meta     *ContractMetadata
		wantType string
	}{
		{
			name:     "account contract",
			meta:     &ContractMetadata{IsAccount: true},
			wantType: "policy_wallet",
		},
		{
			name: "token from spec",
			meta: &ContractMetadata{
				Manifest: &ManifestMeta{Spec: "TRC-20"},
			},
			wantType: "token",
		},
		{
			name: "token from functions",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "transfer"},
					{Name: "balanceOf"},
				},
			},
			wantType: "token",
		},
		{
			name: "task escrow",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "create_task"},
					{Name: "complete_task"},
				},
			},
			wantType: "task_escrow",
		},
		{
			name: "oracle",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "submit_price"},
					{Name: "get_price"},
				},
			},
			wantType: "oracle",
		},
		{
			name: "payment",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "deposit"},
					{Name: "withdraw"},
				},
			},
			wantType: "payment",
		},
		{
			name: "delegation",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "delegate"},
					{Name: "undelegate"},
				},
			},
			wantType: "delegation",
		},
		{
			name: "discovery",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "registerService"},
					{Name: "serviceKindOf"},
				},
			},
			wantType: "discovery",
		},
		{
			name: "custom fallback",
			meta: &ContractMetadata{
				Functions: []FunctionMeta{
					{Name: "do_something_unique"},
				},
			},
			wantType: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferContractType(tt.meta)
			if got != tt.wantType {
				t.Errorf("InferContractType() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestBuildDiscoveryManifestTypedDiscoveryProfile(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		ArtifactRef: ArtifactRef{
			PackageHash:  "0xabc",
			BytecodeHash: "0xdef",
			ABIHash:      "0x123",
			Version:      "1.0.0",
		},
		Contract: ContractInfo{Name: "ServiceDirectory"},
		Manifest: &ManifestMeta{Version: "1.0.0"},
		Functions: []FunctionMeta{
			{Name: "registerService", Visibility: "public"},
			{Name: "serviceKindOf", Visibility: "public"},
			{Name: "capabilityTypeOf", Visibility: "public"},
			{Name: "pricingKindOf", Visibility: "public"},
			{Name: "privacyModeOf", Visibility: "public"},
			{Name: "receiptModeOf", Visibility: "public"},
			{Name: "trustFloorRefOf", Visibility: "public"},
			{Name: "manifestRefOf", Visibility: "public"},
			{Name: "capabilityRefOf", Visibility: "public"},
			{Name: "quoteRefOf", Visibility: "public"},
			{Name: "feeOf", Visibility: "public"},
			{Name: "slaOf", Visibility: "public"},
		},
	}

	dm := BuildDiscoveryManifest(meta, "tolang.stdlib.discovery.service_directory")
	if dm.ContractType != "discovery" {
		t.Fatalf("ContractType = %q, want %q", dm.ContractType, "discovery")
	}
	if dm.TypedDiscovery == nil {
		t.Fatal("expected typed discovery profile")
	}
	if dm.TypedDiscovery.ServiceKind != "DISCOVERY" {
		t.Fatalf("typed discovery service kind = %q, want %q", dm.TypedDiscovery.ServiceKind, "DISCOVERY")
	}
	if dm.TypedDiscovery.Pricing == nil || dm.TypedDiscovery.Pricing.Kind != "FREE" {
		t.Fatalf("typed discovery pricing = %+v, want FREE", dm.TypedDiscovery.Pricing)
	}
	if dm.TypedDiscovery.Receipts == nil || dm.TypedDiscovery.Receipts.Mode != "MANUAL_RECEIPT" {
		t.Fatalf("typed discovery receipts = %+v, want MANUAL_RECEIPT", dm.TypedDiscovery.Receipts)
	}
	fields := map[string]bool{}
	for _, field := range dm.TypedDiscovery.SupportedFields {
		fields[field] = true
	}
	for _, want := range []string{
		"service_kind",
		"capability_kind",
		"pricing_kind",
		"privacy_mode",
		"receipt_mode",
		"fee_amount",
		"sla_duration_ms",
		"trust_floor_ref",
		"manifest_ref",
		"capability_ref",
		"quote_ref",
	} {
		if !fields[want] {
			t.Fatalf("missing typed discovery supported field %q in %v", want, dm.TypedDiscovery.SupportedFields)
		}
	}
}

func TestInferServiceKinds(t *testing.T) {
	meta := &ContractMetadata{
		Functions: []FunctionMeta{
			{Name: "transfer", Verifiable: true},
			{Name: "approve", Delegated: true},
		},
	}

	kinds := InferServiceKinds(meta)

	// Should include: query, token_transfer, balance_query, verifiable_compute, delegated_execution
	kindSet := make(map[string]bool)
	for _, k := range kinds {
		kindSet[k] = true
	}

	for _, expected := range []string{"query", "token_transfer", "balance_query", "verifiable_compute", "delegated_execution"} {
		if !kindSet[expected] {
			t.Errorf("expected service kind %q in %v", expected, kinds)
		}
	}
}

func TestInferTags(t *testing.T) {
	meta := &ContractMetadata{
		Functions: []FunctionMeta{
			{Name: "transfer", Verifiable: true},
			{Name: "approve"},
		},
		Manifest:     &ManifestMeta{Spec: "TRC-20"},
		Capabilities: []string{"token_send"},
	}

	tags := InferTags(meta)

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}

	for _, expected := range []string{"token", "trc-20", "token_send", "transferable", "approvable", "verifiable"} {
		if !tagSet[expected] {
			t.Errorf("expected tag %q in %v", expected, tags)
		}
	}
}

func TestInferTags_AccountContract(t *testing.T) {
	meta := &ContractMetadata{
		IsAccount: true,
		Functions: []FunctionMeta{
			{Name: "set_spend_cap"},
		},
		PolicyProfile: &PolicyProfile{
			HasSpendCaps: true,
			HasGuardian:  true,
		},
	}

	tags := InferTags(meta)
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}

	if !tagSet["account"] {
		t.Errorf("expected 'account' tag in %v", tags)
	}
	if !tagSet["spend-caps"] {
		t.Errorf("expected 'spend-caps' tag in %v", tags)
	}
	if !tagSet["guardian"] {
		t.Errorf("expected 'guardian' tag in %v", tags)
	}
}

func TestCheckCompatibility(t *testing.T) {
	matrix := CurrentCompatibilityMatrix()

	ok, reason := CheckCompatibility(matrix, "tolang", "openfox")
	if !ok {
		t.Errorf("expected tolang and openfox to be compatible: %s", reason)
	}

	ok, reason = CheckCompatibility(matrix, "tolang", "gtos")
	if !ok {
		t.Errorf("expected tolang and gtos to be compatible: %s", reason)
	}

	// Unknown repo.
	ok, reason = CheckCompatibility(matrix, "tolang", "unknown")
	if ok {
		t.Error("expected unknown repo to be incompatible")
	}
	if reason == "" {
		t.Error("expected non-empty reason for incompatibility")
	}
}
