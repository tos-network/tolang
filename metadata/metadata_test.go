package metadata

import (
	"encoding/json"
	"testing"
)

// sampleABI is a realistic ABI JSON blob matching the format emitted by the
// current TOL compiler, including compiler-emitted mutability and named_params.
const sampleABI = `{
  "gas_model": {
    "version": "tolang/0.2.0",
    "sload": 2100,
    "sstore": 20000,
    "log_base": 375
  },
  "functions": [
    {
      "name": "transfer",
      "visibility": "external",
      "mutability": "payable",
      "selector": "0xa9059cbb",
      "params": ["address", "uint256"],
      "returns": ["bool"],
      "named_params": [{"name": "to", "type": "address"}, {"name": "amount", "type": "uint256"}],
      "named_returns": [{"name": "success", "type": "bool"}],
      "doc": {
        "effects": {
          "reads": ["balances"],
          "writes": ["balances"],
          "emits": ["Transfer"],
          "calls": [{"cap": "token", "iface": "ITRC20", "selector": "0x12345678", "max_gas": 50000}]
        },
        "gas_upper": 65000,
        "non_composable": false
      },
      "requires_capability": "token_send",
      "pay_amount_wei": "1000000",
      "verifiable": true,
      "delegated": false
    },
    {
      "name": "balanceOf",
      "visibility": "public",
      "mutability": "view",
      "selector": "0x70a08231",
      "params": ["address"],
      "returns": ["uint256"],
      "named_params": [{"name": "owner", "type": "address"}],
      "named_returns": [{"name": "balance", "type": "uint256"}],
      "doc": {
        "effects": {
          "reads": ["balances"]
        },
        "gas_upper": 2100
      },
      "verifiable": false,
      "delegated": false
    },
    {
      "name": "verify_transfer",
      "visibility": "external",
      "selector": "0xdeadbeef",
      "params": ["bytes", "address", "uint256", "bool"],
      "returns": ["bool"],
      "verifiable_stub": true
    }
  ],
  "events": [
    {
      "name": "Transfer",
      "params": ["address", "address", "uint256"],
      "named_params": [{"name": "from", "type": "address"}, {"name": "to", "type": "address"}, {"name": "value", "type": "uint256"}]
    }
  ],
  "manifest": {
    "name": "TestToken",
    "version": "1.0.0",
    "extra": {
      "spec": "TRC-20",
      "sla_uptime": "99.9%"
    }
  },
  "account_contract": false
}`

// sampleABILegacy is an ABI JSON blob without the newer compiler-emitted
// fields (mutability, named_params, named_returns). This exercises the
// backward-compatible fallback paths in ExtractFromABI.
const sampleABILegacy = `{
  "gas_model": {
    "version": "tolang/0.2.0",
    "sload": 2100,
    "sstore": 20000,
    "log_base": 375
  },
  "functions": [
    {
      "name": "transfer",
      "visibility": "external",
      "selector": "0xa9059cbb",
      "params": ["address", "uint256"],
      "returns": ["bool"],
      "doc": {
        "effects": {
          "reads": ["balances"],
          "writes": ["balances"],
          "emits": ["Transfer"]
        },
        "gas_upper": 65000
      },
      "pay_amount_wei": "1000000",
      "verifiable": false,
      "delegated": false
    },
    {
      "name": "balanceOf",
      "visibility": "public",
      "selector": "0x70a08231",
      "params": ["address"],
      "returns": ["uint256"],
      "doc": {
        "effects": { "reads": ["balances"] },
        "gas_upper": 2100
      }
    }
  ],
  "events": [
    {"name": "Transfer", "params": ["address", "address", "uint256"]}
  ],
  "account_contract": false
}`

func TestExtractFromABI(t *testing.T) {
	meta, err := ExtractFromABI([]byte(sampleABI))
	if err != nil {
		t.Fatalf("ExtractFromABI failed: %v", err)
	}
	if meta.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %q, want %q", meta.SchemaVersion, SchemaVersion)
	}
	// Gas model
	if meta.GasModel.Version != "tolang/0.2.0" {
		t.Errorf("gas_model.version = %q, want %q", meta.GasModel.Version, "tolang/0.2.0")
	}
	if meta.GasModel.SLoad != 2100 {
		t.Errorf("gas_model.sload = %d, want 2100", meta.GasModel.SLoad)
	}
	// Functions: verifiable_stub should be filtered out
	if len(meta.Functions) != 2 {
		t.Fatalf("len(functions) = %d, want 2 (verify_transfer stub should be excluded)", len(meta.Functions))
	}
	// transfer function
	xfer := meta.Functions[0]
	if xfer.Name != "transfer" {
		t.Errorf("functions[0].name = %q, want %q", xfer.Name, "transfer")
	}
	if xfer.Selector != "0xa9059cbb" {
		t.Errorf("functions[0].selector = %q, want %q", xfer.Selector, "0xa9059cbb")
	}
	if xfer.Mutability != "payable" {
		t.Errorf("functions[0].mutability = %q, want %q", xfer.Mutability, "payable")
	}
	if xfer.GasUpper != 65000 {
		t.Errorf("functions[0].gas_upper = %d, want 65000", xfer.GasUpper)
	}
	if !xfer.Verifiable {
		t.Error("functions[0].verifiable should be true")
	}
	if xfer.RiskLevel != "high" {
		t.Errorf("functions[0].risk_level = %q, want %q (writes + calls)", xfer.RiskLevel, "high")
	}
	if len(xfer.RequiresCapability) != 1 || xfer.RequiresCapability[0] != "token_send" {
		t.Errorf("functions[0].requires_capability = %v, want [token_send]", xfer.RequiresCapability)
	}
	if xfer.Effects == nil {
		t.Fatal("functions[0].effects should not be nil")
	}
	if len(xfer.Effects.Calls) != 1 {
		t.Fatalf("functions[0].effects.calls length = %d, want 1", len(xfer.Effects.Calls))
	}
	if xfer.Effects.Calls[0].MaxGas != 50000 {
		t.Errorf("functions[0].effects.calls[0].max_gas = %d, want 50000", xfer.Effects.Calls[0].MaxGas)
	}
	// Params — should use compiler-emitted named_params (real source names).
	if len(xfer.Params) != 2 {
		t.Fatalf("functions[0].params length = %d, want 2", len(xfer.Params))
	}
	if xfer.Params[0].Name != "to" {
		t.Errorf("functions[0].params[0].name = %q, want %q", xfer.Params[0].Name, "to")
	}
	if xfer.Params[0].Type != "address" || xfer.Params[1].Type != "uint256" {
		t.Errorf("functions[0].params types = [%s, %s], want [address, uint256]", xfer.Params[0].Type, xfer.Params[1].Type)
	}
	if xfer.Params[1].Name != "amount" {
		t.Errorf("functions[0].params[1].name = %q, want %q", xfer.Params[1].Name, "amount")
	}
	// Returns — should use compiler-emitted named_returns.
	if len(xfer.Returns) != 1 || xfer.Returns[0].Name != "success" {
		t.Errorf("functions[0].returns[0].name = %q, want %q", xfer.Returns[0].Name, "success")
	}
	// balanceOf
	bal := meta.Functions[1]
	if bal.Mutability != "view" {
		t.Errorf("functions[1].mutability = %q, want %q", bal.Mutability, "view")
	}
	if bal.Params[0].Name != "owner" {
		t.Errorf("functions[1].params[0].name = %q, want %q", bal.Params[0].Name, "owner")
	}
	if bal.RiskLevel != "low" {
		t.Errorf("functions[1].risk_level = %q, want %q (read-only)", bal.RiskLevel, "low")
	}
	// Events — should use compiler-emitted named_params.
	if len(meta.Events) != 1 || meta.Events[0].Name != "Transfer" {
		t.Errorf("events = %v, want [{Transfer ...}]", meta.Events)
	}
	if len(meta.Events[0].Params) != 3 {
		t.Errorf("events[0].params length = %d, want 3", len(meta.Events[0].Params))
	}
	if meta.Events[0].Params[0].Name != "from" {
		t.Errorf("events[0].params[0].name = %q, want %q", meta.Events[0].Params[0].Name, "from")
	}
	if meta.Events[0].Params[1].Name != "to" {
		t.Errorf("events[0].params[1].name = %q, want %q", meta.Events[0].Params[1].Name, "to")
	}
	// Manifest
	if meta.Manifest == nil {
		t.Fatal("manifest should not be nil")
	}
	if meta.Manifest.Version != "1.0.0" {
		t.Errorf("manifest.version = %q, want %q", meta.Manifest.Version, "1.0.0")
	}
	if meta.Manifest.Spec != "TRC-20" {
		t.Errorf("manifest.spec = %q, want %q", meta.Manifest.Spec, "TRC-20")
	}
	if meta.Manifest.SLAUptime != "99.9%" {
		t.Errorf("manifest.sla_uptime = %q, want %q", meta.Manifest.SLAUptime, "99.9%")
	}
	// Capabilities
	if len(meta.Capabilities) != 1 || meta.Capabilities[0] != "token_send" {
		t.Errorf("capabilities = %v, want [token_send]", meta.Capabilities)
	}
	// ArtifactRef version should be populated from manifest
	if meta.ArtifactRef.Version != "1.0.0" {
		t.Errorf("artifact_ref.version = %q, want %q", meta.ArtifactRef.Version, "1.0.0")
	}
}

func TestExtractFromABI_LegacyABI(t *testing.T) {
	// Legacy ABI without mutability, named_params, named_returns fields.
	meta, err := ExtractFromABI([]byte(sampleABILegacy))
	if err != nil {
		t.Fatalf("ExtractFromABI (legacy) failed: %v", err)
	}
	// Mutability should be derived from heuristic fallback.
	xfer := meta.Functions[0]
	if xfer.Mutability != "payable" {
		t.Errorf("legacy transfer mutability = %q, want %q (heuristic from pay_amount_wei)", xfer.Mutability, "payable")
	}
	bal := meta.Functions[1]
	if bal.Mutability != "view" {
		t.Errorf("legacy balanceOf mutability = %q, want %q (heuristic from reads)", bal.Mutability, "view")
	}
	// Params should use synthetic names (arg0, arg1) since named_params is absent.
	if xfer.Params[0].Name != "arg0" {
		t.Errorf("legacy transfer params[0].name = %q, want %q", xfer.Params[0].Name, "arg0")
	}
	if xfer.Params[1].Name != "arg1" {
		t.Errorf("legacy transfer params[1].name = %q, want %q", xfer.Params[1].Name, "arg1")
	}
	// Event params should also use synthetic names.
	if meta.Events[0].Params[0].Name != "arg0" {
		t.Errorf("legacy event params[0].name = %q, want %q", meta.Events[0].Params[0].Name, "arg0")
	}
}

func TestExtractFromABI_InvalidJSON(t *testing.T) {
	_, err := ExtractFromABI([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDeriveRiskLevel(t *testing.T) {
	tests := []struct {
		name string
		fn   FunctionMeta
		want string
	}{
		{
			name: "no effects",
			fn:   FunctionMeta{},
			want: "low",
		},
		{
			name: "reads only",
			fn: FunctionMeta{
				Effects: &EffectsMeta{Reads: []string{"balances"}},
			},
			want: "low",
		},
		{
			name: "writes only",
			fn: FunctionMeta{
				Effects: &EffectsMeta{Writes: []string{"balances"}},
			},
			want: "medium",
		},
		{
			name: "calls only",
			fn: FunctionMeta{
				Effects: &EffectsMeta{Calls: []CallMeta{{Capability: "token"}}},
			},
			want: "medium",
		},
		{
			name: "writes and calls",
			fn: FunctionMeta{
				Effects: &EffectsMeta{
					Writes: []string{"balances"},
					Calls:  []CallMeta{{Capability: "token"}},
				},
			},
			want: "high",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveRiskLevel(tt.fn)
			if got != tt.want {
				t.Errorf("DeriveRiskLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDerivePolicyProfile(t *testing.T) {
	fns := []FunctionMeta{
		{Name: "set_spend_cap"},
		{Name: "add_to_allowlist"},
		{Name: "freeze"},
		{Name: "set_guardian"},
		{Name: "recover_account"},
		{Name: "delegate_call"},
		{Name: "suspend"},
		{Name: "transfer"}, // not a policy function
	}
	pp := DerivePolicyProfile(fns)
	if !pp.HasSpendCaps {
		t.Error("expected HasSpendCaps")
	}
	if !pp.HasAllowlist {
		t.Error("expected HasAllowlist")
	}
	if !pp.HasTerminalPolicy {
		t.Error("expected HasTerminalPolicy")
	}
	if !pp.HasGuardian {
		t.Error("expected HasGuardian")
	}
	if !pp.HasRecovery {
		t.Error("expected HasRecovery")
	}
	if !pp.HasDelegation {
		t.Error("expected HasDelegation")
	}
	if !pp.HasSuspension {
		t.Error("expected HasSuspension")
	}
}

func TestDerivePolicyProfile_Capabilities(t *testing.T) {
	// Test detection via capability annotations (no matching function names).
	fns := []FunctionMeta{
		{Name: "configure_limits", RequiresCapability: []string{"spend_admin"}},
		{Name: "manage_list", RequiresCapability: []string{"allowlist_admin"}},
		{Name: "lockdown", RequiresCapability: []string{"terminal_control"}},
		{Name: "manage_keys", RequiresCapability: []string{"guardian_mgmt"}},
		{Name: "restore_access", RequiresCapability: []string{"recovery_admin"}},
		{Name: "proxy_call", Delegated: true},
		{Name: "halt", RequiresCapability: []string{"suspend_ops"}},
	}
	pp := DerivePolicyProfile(fns)
	if !pp.HasSpendCaps {
		t.Error("expected HasSpendCaps from spend_admin capability")
	}
	if !pp.HasAllowlist {
		t.Error("expected HasAllowlist from allowlist_admin capability")
	}
	if !pp.HasTerminalPolicy {
		t.Error("expected HasTerminalPolicy from terminal_control capability")
	}
	if !pp.HasGuardian {
		t.Error("expected HasGuardian from guardian_mgmt capability")
	}
	if !pp.HasRecovery {
		t.Error("expected HasRecovery from recovery_admin capability")
	}
	if !pp.HasDelegation {
		t.Error("expected HasDelegation from Delegated flag")
	}
	if !pp.HasSuspension {
		t.Error("expected HasSuspension from suspend_ops capability")
	}
}

func TestDerivePolicyProfile_Empty(t *testing.T) {
	pp := DerivePolicyProfile(nil)
	if pp.HasSpendCaps || pp.HasAllowlist || pp.HasTerminalPolicy ||
		pp.HasGuardian || pp.HasRecovery || pp.HasDelegation || pp.HasSuspension {
		t.Error("expected all policy flags to be false for empty function list")
	}
}

func TestContractMetadataJSONRoundTrip(t *testing.T) {
	original := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		ArtifactRef: ArtifactRef{
			PackageHash:  "0xabc123",
			BytecodeHash: "0xdef456",
			ABIHash:      "0x789012",
			Version:      "1.0.0",
		},
		Contract: ContractInfo{
			Name:         "TestToken",
			IsAccount:    false,
			StorageSlots: 3,
		},
		Functions: []FunctionMeta{
			{
				Name:       "transfer",
				Selector:   "0xa9059cbb",
				Visibility: "external",
				Mutability: "nonpayable",
				Params: []ParamMeta{
					{Name: "to", Type: "address"},
					{Name: "amount", Type: "uint256"},
				},
				Returns: []ParamMeta{
					{Name: "success", Type: "bool"},
				},
				RequiresCapability: []string{"token_send"},
				Effects: &EffectsMeta{
					Reads:  []string{"balances"},
					Writes: []string{"balances"},
					Emits:  []string{"Transfer"},
				},
				GasUpper:  65000,
				RiskLevel: "medium",
			},
		},
		Events: []EventMeta{
			{
				Name: "Transfer",
				Params: []ParamMeta{
					{Name: "from", Type: "address"},
					{Name: "to", Type: "address"},
					{Name: "amount", Type: "uint256"},
				},
			},
		},
		GasModel: GasModelMeta{
			Version: "tolang/0.2.0",
			SLoad:   2100,
			SStore:  20000,
			LogBase: 375,
		},
		Capabilities: []string{"token_send"},
		IsAccount:    false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ContractMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Re-marshal and compare
	data2, err := json.Marshal(&decoded)
	if err != nil {
		t.Fatalf("re-Marshal failed: %v", err)
	}

	if string(data) != string(data2) {
		t.Errorf("JSON round-trip mismatch:\n  got:  %s\n  want: %s", string(data2), string(data))
	}
}

func TestComputeArtifactRef(t *testing.T) {
	ref := ComputeArtifactRef(
		[]byte("package-data"),
		[]byte("bytecode-data"),
		[]byte("source-data"),
		[]byte("abi-data"),
		"1.0.0",
	)
	if ref.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", ref.Version, "1.0.0")
	}
	// All hashes should be 0x-prefixed hex, 66 chars (0x + 64 hex digits)
	for _, h := range []struct {
		name, val string
	}{
		{"package_hash", ref.PackageHash},
		{"bytecode_hash", ref.BytecodeHash},
		{"source_hash", ref.SourceHash},
		{"abi_hash", ref.ABIHash},
	} {
		if len(h.val) != 66 {
			t.Errorf("%s length = %d, want 66", h.name, len(h.val))
		}
		if h.val[:2] != "0x" {
			t.Errorf("%s should be 0x-prefixed: %q", h.name, h.val)
		}
	}
	// SourceHash should be empty when sourceBytes is nil
	ref2 := ComputeArtifactRef([]byte("pkg"), []byte("bc"), nil, []byte("abi"), "")
	if ref2.SourceHash != "" {
		t.Errorf("expected empty source_hash when sourceBytes is nil, got %q", ref2.SourceHash)
	}
}

func TestExtractFromABI_AccountContract(t *testing.T) {
	abiJSON := `{
		"gas_model": {"version": "tolang/0.2.0", "sload": 2100, "sstore": 20000, "log_base": 375},
		"functions": [
			{"name": "set_spend_cap", "visibility": "external", "selector": "0x11111111"},
			{"name": "add_to_allowlist", "visibility": "external", "selector": "0x22222222"},
			{"name": "set_guardian", "visibility": "external", "selector": "0x33333333"},
			{"name": "execute", "visibility": "external", "selector": "0x44444444"}
		],
		"events": [],
		"account_contract": true
	}`
	meta, err := ExtractFromABI([]byte(abiJSON))
	if err != nil {
		t.Fatalf("ExtractFromABI failed: %v", err)
	}
	if !meta.IsAccount {
		t.Error("expected IsAccount to be true")
	}
	if meta.PolicyProfile == nil {
		t.Fatal("expected PolicyProfile to be set for account contract")
	}
	if !meta.PolicyProfile.HasSpendCaps {
		t.Error("expected HasSpendCaps")
	}
	if !meta.PolicyProfile.HasAllowlist {
		t.Error("expected HasAllowlist")
	}
	if !meta.PolicyProfile.HasGuardian {
		t.Error("expected HasGuardian")
	}
	// Non-account contracts should not have a policy profile
	abiJSON2 := `{
		"gas_model": {"version": "tolang/0.2.0", "sload": 2100, "sstore": 20000, "log_base": 375},
		"functions": [{"name": "set_spend_cap", "visibility": "external", "selector": "0x11111111"}],
		"events": [],
		"account_contract": false
	}`
	meta2, err := ExtractFromABI([]byte(abiJSON2))
	if err != nil {
		t.Fatalf("ExtractFromABI failed: %v", err)
	}
	if meta2.PolicyProfile != nil {
		t.Error("expected nil PolicyProfile for non-account contract")
	}
}
