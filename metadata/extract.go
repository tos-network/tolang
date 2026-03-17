package metadata

import (
	"encoding/json"
	"fmt"
	"strings"
)

// internalABI mirrors the existing ABI JSON structure emitted by the TOL compiler.
// This is an internal type used only for parsing; consumers should use the stable
// metadata types defined in metadata.go.
type internalABI struct {
	GasModel        internalGasModel   `json:"gas_model"`
	Functions       []internalFunction `json:"functions"`
	Events          []internalEvent    `json:"events"`
	Manifest        *internalManifest  `json:"manifest,omitempty"`
	AccountContract bool               `json:"account_contract,omitempty"`
}

type internalGasModel struct {
	Version string `json:"version"`
	Sload   uint64 `json:"sload"`
	Sstore  uint64 `json:"sstore"`
	LogBase uint64 `json:"log_base"`
}

type internalFunction struct {
	Name               string       `json:"name"`
	Visibility         string       `json:"visibility"`
	Selector           string       `json:"selector"`
	Params             []string     `json:"params,omitempty"`
	Returns            []string     `json:"returns,omitempty"`
	Doc                *internalDoc `json:"doc,omitempty"`
	RequiresCapability string       `json:"requires_capability,omitempty"`
	PayAmountWei       string       `json:"pay_amount_wei,omitempty"`
	Verifiable         bool         `json:"verifiable,omitempty"`
	Delegated          bool         `json:"delegated,omitempty"`
	VerifiableStub     bool         `json:"verifiable_stub,omitempty"`
}

type internalDoc struct {
	Notice        string           `json:"notice,omitempty"`
	Effects       *internalEffects `json:"effects,omitempty"`
	GasUpper      uint64           `json:"gas_upper,omitempty"`
	NonComposable bool             `json:"non_composable,omitempty"`
}

type internalEffects struct {
	Reads  []string        `json:"reads,omitempty"`
	Writes []string        `json:"writes,omitempty"`
	Emits  []string        `json:"emits,omitempty"`
	Calls  json.RawMessage `json:"calls,omitempty"`
}

type internalCallRef struct {
	Cap      string `json:"cap,omitempty"`
	Iface    string `json:"iface,omitempty"`
	Selector string `json:"selector,omitempty"`
	MaxGas   uint64 `json:"max_gas,omitempty"`
}

type internalManifest struct {
	Name        string            `json:"name,omitempty"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type internalEvent struct {
	Name   string   `json:"name"`
	Params []string `json:"params,omitempty"`
}

// ExtractFromABI parses existing ABI JSON (as emitted by the current TOL compiler)
// and returns a structured ContractMetadata object. This bridges the existing
// compiler output to the 2046 stable schema.
//
// The returned metadata has an empty ArtifactRef; callers should populate it
// using ComputeArtifactRef when the raw artifact bytes are available.
func ExtractFromABI(abiJSON []byte) (*ContractMetadata, error) {
	var raw internalABI
	if err := json.Unmarshal(abiJSON, &raw); err != nil {
		return nil, fmt.Errorf("metadata: parse ABI JSON: %w", err)
	}

	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		GasModel: GasModelMeta{
			Version: raw.GasModel.Version,
			SLoad:   raw.GasModel.Sload,
			SStore:  raw.GasModel.Sstore,
			LogBase: raw.GasModel.LogBase,
		},
		IsAccount: raw.AccountContract,
	}

	// Extract manifest.
	if raw.Manifest != nil {
		m := &ManifestMeta{
			Version: raw.Manifest.Version,
		}
		if len(raw.Manifest.Extra) > 0 {
			m.Custom = raw.Manifest.Extra
			if v, ok := raw.Manifest.Extra["spec"]; ok {
				m.Spec = v
			}
			if v, ok := raw.Manifest.Extra["sla_uptime"]; ok {
				m.SLAUptime = v
			}
		}
		meta.Manifest = m
		if raw.Manifest.Version != "" {
			meta.ArtifactRef.Version = raw.Manifest.Version
		}
	}

	// Extract functions (skip verifiable stubs from the stable metadata).
	var allCaps []string
	for _, fn := range raw.Functions {
		if fn.VerifiableStub {
			continue
		}
		fm := FunctionMeta{
			Name:       fn.Name,
			Selector:   fn.Selector,
			Visibility: fn.Visibility,
			Mutability: deriveMutability(fn),
			Params:     convertParamTypes(fn.Params),
			Returns:    convertParamTypes(fn.Returns),
			Verifiable: fn.Verifiable,
			Delegated:  fn.Delegated,
		}
		if fn.RequiresCapability != "" {
			caps := strings.Split(fn.RequiresCapability, ",")
			fm.RequiresCapability = caps
			allCaps = append(allCaps, caps...)
		}
		if fn.Doc != nil {
			fm.GasUpper = fn.Doc.GasUpper
			fm.NonComposable = fn.Doc.NonComposable
			if fn.Doc.Effects != nil {
				fm.Effects = convertEffects(fn.Doc.Effects)
			}
		}
		fm.RiskLevel = DeriveRiskLevel(fm)
		meta.Functions = append(meta.Functions, fm)
	}
	if len(allCaps) > 0 {
		meta.Capabilities = dedup(allCaps)
	}

	// Extract events.
	for _, ev := range raw.Events {
		meta.Events = append(meta.Events, EventMeta{
			Name:   ev.Name,
			Params: convertParamTypes(ev.Params),
		})
	}

	// Contract info (name is not in ABI JSON; caller should set it).
	meta.Contract = ContractInfo{
		IsAccount: raw.AccountContract,
	}

	// Policy profile detection for account contracts.
	if raw.AccountContract {
		pp := DerivePolicyProfile(meta.Functions)
		meta.PolicyProfile = pp
	}

	return meta, nil
}

// DeriveRiskLevel computes a risk level from function effects.
//   - "high": writes to storage AND makes external calls
//   - "medium": writes to storage OR makes external calls
//   - "low": pure reads or no effects
func DeriveRiskLevel(fn FunctionMeta) string {
	if fn.Effects == nil {
		return "low"
	}
	hasWrites := len(fn.Effects.Writes) > 0
	hasCalls := len(fn.Effects.Calls) > 0
	if hasWrites && hasCalls {
		return "high"
	}
	if hasWrites || hasCalls {
		return "medium"
	}
	return "low"
}

// DerivePolicyProfile inspects function names and patterns to detect policy
// wallet features. This is a heuristic based on naming conventions used in
// the TOL account contract ecosystem.
func DerivePolicyProfile(functions []FunctionMeta) *PolicyProfile {
	pp := &PolicyProfile{}
	for _, fn := range functions {
		name := strings.ToLower(fn.Name)
		switch {
		case strings.Contains(name, "spend_cap") || strings.Contains(name, "spendcap"):
			pp.HasSpendCaps = true
		case strings.Contains(name, "allowlist") || strings.Contains(name, "whitelist"):
			pp.HasAllowlist = true
		case strings.Contains(name, "terminal") || name == "lock" || name == "freeze":
			pp.HasTerminalPolicy = true
		case strings.Contains(name, "guardian"):
			pp.HasGuardian = true
		case strings.Contains(name, "recover") || strings.Contains(name, "recovery"):
			pp.HasRecovery = true
		case strings.Contains(name, "delegat"):
			pp.HasDelegation = true
		case strings.Contains(name, "suspend") || strings.Contains(name, "pause"):
			pp.HasSuspension = true
		}
	}
	return pp
}

// convertParamTypes converts a list of ABI type strings (e.g. ["uint256", "address"])
// into ParamMeta values. The existing ABI JSON stores only types, not names, so
// parameter names are synthesized as "arg0", "arg1", etc.
func convertParamTypes(types []string) []ParamMeta {
	if len(types) == 0 {
		return nil
	}
	out := make([]ParamMeta, len(types))
	for i, t := range types {
		out[i] = ParamMeta{
			Name: fmt.Sprintf("arg%d", i),
			Type: t,
		}
	}
	return out
}

// convertEffects translates internal effects to the stable schema.
func convertEffects(eff *internalEffects) *EffectsMeta {
	if eff == nil {
		return nil
	}
	out := &EffectsMeta{
		Reads:  eff.Reads,
		Writes: eff.Writes,
		Emits:  eff.Emits,
	}
	if len(eff.Calls) > 0 {
		var calls []internalCallRef
		if err := json.Unmarshal(eff.Calls, &calls); err == nil {
			for _, c := range calls {
				out.Calls = append(out.Calls, CallMeta{
					Capability: c.Cap,
					Interface:  c.Iface,
					Selector:   c.Selector,
					MaxGas:     c.MaxGas,
				})
			}
		}
	}
	return out
}

// deriveMutability infers the Solidity-style mutability from the existing ABI
// function data.  The current ABI doesn't store mutability directly, so we
// approximate from effects and pay annotations.
func deriveMutability(fn internalFunction) string {
	if fn.PayAmountWei != "" {
		return "payable"
	}
	if fn.Doc == nil || fn.Doc.Effects == nil {
		return "view"
	}
	if len(fn.Doc.Effects.Writes) > 0 || len(fn.Doc.Effects.Emits) > 0 {
		return "nonpayable"
	}
	if len(fn.Doc.Effects.Reads) > 0 {
		return "view"
	}
	return "pure"
}

// dedup returns a deduplicated copy of a string slice, preserving order.
func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
