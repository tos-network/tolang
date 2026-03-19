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

// internalNamedParam mirrors tocABIParam — a parameter with both name and type.
type internalNamedParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type internalFunction struct {
	Name               string               `json:"name"`
	Visibility         string               `json:"visibility"`
	Mutability         string               `json:"mutability,omitempty"` // compiler-emitted: "pure", "view", "payable", "nonpayable"
	Selector           string               `json:"selector"`
	Params             []string             `json:"params,omitempty"`
	Returns            []string             `json:"returns,omitempty"`
	NamedParams        []internalNamedParam `json:"named_params,omitempty"`
	NamedReturns       []internalNamedParam `json:"named_returns,omitempty"`
	Doc                *internalDoc         `json:"doc,omitempty"`
	RequiresCapability string               `json:"requires_capability,omitempty"`
	PayAmountTomi      string               `json:"pay_amount_tomi,omitempty"`
	Verifiable         bool                 `json:"verifiable,omitempty"`
	Delegated          bool                 `json:"delegated,omitempty"`
	VerifiableStub     bool                 `json:"verifiable_stub,omitempty"`
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
	Name        string               `json:"name"`
	Params      []string             `json:"params,omitempty"`
	NamedParams []internalNamedParam `json:"named_params,omitempty"`
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
		// Use compiler-emitted mutability when available; fall back to heuristic
		// for ABI JSON produced by older compiler versions.
		mut := fn.Mutability
		if mut == "" {
			mut = deriveMutability(fn)
		}
		fm := FunctionMeta{
			Name:       fn.Name,
			Selector:   fn.Selector,
			Visibility: fn.Visibility,
			Mutability: mut,
			Params:     extractParams(fn.NamedParams, fn.Params),
			Returns:    extractParams(fn.NamedReturns, fn.Returns),
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
			Params: extractParams(ev.NamedParams, ev.Params),
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

// DerivePolicyProfile detects policy wallet features from function metadata.
// It combines two signals:
//  1. Capability declarations (requires_capability) — the most reliable signal
//     because they are compiler-verified annotations.
//  2. Function name patterns — a fallback heuristic for contracts that use
//     conventional naming but lack capability annotations.
//
// Both signals are checked so that the profile is accurate for contracts
// compiled with either older or newer compiler versions.
func DerivePolicyProfile(functions []FunctionMeta) *PolicyProfile {
	pp := &PolicyProfile{}
	for _, fn := range functions {
		// Signal 1: capability-based detection (compiler-verified).
		// Uses if-chains (not switch) so that a single capability can
		// set multiple flags and ordering doesn't cause false matches.
		for _, cap := range fn.RequiresCapability {
			capLower := strings.ToLower(cap)
			if strings.Contains(capLower, "spend") && !strings.Contains(capLower, "suspend") {
				pp.HasSpendCaps = true
			}
			if strings.Contains(capLower, "allowlist") || strings.Contains(capLower, "whitelist") {
				pp.HasAllowlist = true
			}
			if strings.Contains(capLower, "terminal") || strings.Contains(capLower, "lock") || strings.Contains(capLower, "freeze") {
				pp.HasTerminalPolicy = true
			}
			if strings.Contains(capLower, "guardian") {
				pp.HasGuardian = true
			}
			if strings.Contains(capLower, "recover") {
				pp.HasRecovery = true
			}
			if strings.Contains(capLower, "delegat") {
				pp.HasDelegation = true
			}
			if strings.Contains(capLower, "suspend") || strings.Contains(capLower, "pause") {
				pp.HasSuspension = true
			}
		}

		// Signal 2: delegated flag from compiler annotation.
		if fn.Delegated {
			pp.HasDelegation = true
		}

		// Signal 3: function name heuristic (fallback for contracts without
		// capability annotations; kept for backward compatibility).
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

// extractParams converts ABI parameter info into ParamMeta values.
// When the compiler provides named_params (name+type pairs), those are used
// directly so that real source-level parameter names are preserved. When only
// the type-only params list is available (older compiler output), synthetic
// names (arg0, arg1, ...) are generated as a fallback.
func extractParams(named []internalNamedParam, types []string) []ParamMeta {
	// Prefer compiler-emitted named parameters.
	if len(named) > 0 {
		out := make([]ParamMeta, len(named))
		for i, np := range named {
			name := np.Name
			if name == "" {
				name = fmt.Sprintf("arg%d", i)
			}
			out[i] = ParamMeta{
				Name: name,
				Type: np.Type,
			}
		}
		return out
	}
	// Fallback: type-only list from older ABI JSON.
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

// deriveMutability is a fallback heuristic that infers mutability from ABI
// function effects and pay annotations. It is used only when the compiler did
// not emit a "mutability" field (i.e., ABI JSON from older compiler versions).
// Current compilers emit mutability directly; see ExtractFromABI.
func deriveMutability(fn internalFunction) string {
	if fn.PayAmountTomi != "" {
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
