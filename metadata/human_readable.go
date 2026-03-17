package metadata

import (
	"fmt"
	"strings"
)

// HumanReadableSummary generates a plain-language summary of what a contract does,
// suitable for display to humans and consumption by AI agents before execution.
type HumanReadableSummary struct {
	ContractName   string            `json:"contract_name"`
	Description    string            `json:"description"`
	IsAccount      bool              `json:"is_account"`
	PolicyFeatures []string          `json:"policy_features,omitempty"`
	RiskSummary    string            `json:"risk_summary"`
	Functions      []FunctionSummary `json:"functions"`
	Capabilities   []string          `json:"capabilities,omitempty"`
	TotalGasUpper  uint64            `json:"total_gas_upper,omitempty"`
}

// FunctionSummary is a human+agent readable summary of a single function.
type FunctionSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"` // auto-generated from effects
	RiskLevel   string `json:"risk_level"`
	Payable     bool   `json:"payable"`
	Delegated   bool   `json:"delegated"`
	Verifiable  bool   `json:"verifiable"`
}

// GenerateHumanReadable produces a human+agent readable summary from contract metadata.
func GenerateHumanReadable(meta *ContractMetadata) *HumanReadableSummary {
	s := &HumanReadableSummary{
		ContractName: meta.Contract.Name,
		IsAccount:    meta.IsAccount,
		Capabilities: meta.Capabilities,
	}

	// Build description from manifest and contract info.
	s.Description = generateContractDescription(meta)

	// Summarise each function.
	var totalGas uint64
	for _, fn := range meta.Functions {
		fs := FunctionSummary{
			Name:        fn.Name,
			Description: GenerateFunctionDescription(fn),
			RiskLevel:   fn.RiskLevel,
			Payable:     fn.Mutability == "payable",
			Delegated:   fn.Delegated,
			Verifiable:  fn.Verifiable,
		}
		s.Functions = append(s.Functions, fs)
		totalGas += fn.GasUpper
	}
	s.TotalGasUpper = totalGas

	// Policy features for account contracts.
	if meta.PolicyProfile != nil {
		s.PolicyFeatures = derivePolicyFeatureNames(meta.PolicyProfile)
	}

	s.RiskSummary = GenerateRiskSummary(meta)

	return s
}

// GenerateFunctionDescription auto-generates a description from function effects.
// e.g. "Reads balances, writes balances and allowances, emits Transfer event"
func GenerateFunctionDescription(fn FunctionMeta) string {
	if fn.Effects == nil {
		return "Pure computation with no state access"
	}

	var parts []string

	if len(fn.Effects.Reads) > 0 {
		parts = append(parts, "Reads "+joinWords(fn.Effects.Reads))
	}
	if len(fn.Effects.Writes) > 0 {
		parts = append(parts, "writes "+joinWords(fn.Effects.Writes))
	}
	if len(fn.Effects.Emits) > 0 {
		names := make([]string, len(fn.Effects.Emits))
		for i, e := range fn.Effects.Emits {
			names[i] = e + " event"
		}
		parts = append(parts, "emits "+joinWords(names))
	}
	if len(fn.Effects.Calls) > 0 {
		descs := make([]string, 0, len(fn.Effects.Calls))
		for _, c := range fn.Effects.Calls {
			if c.Interface != "" {
				descs = append(descs, c.Interface)
			} else if c.Capability != "" {
				descs = append(descs, c.Capability)
			}
		}
		if len(descs) > 0 {
			parts = append(parts, "calls "+joinWords(dedup(descs)))
		} else {
			parts = append(parts, fmt.Sprintf("makes %d external call(s)", len(fn.Effects.Calls)))
		}
	}

	if len(parts) == 0 {
		return "Pure computation with no state access"
	}

	// Capitalise the first part; the rest start lower-case because we join with ", ".
	result := strings.Join(parts, ", ")
	return result
}

// GenerateRiskSummary produces an overall risk assessment.
// e.g. "Medium risk: 3 functions write state, 1 function is payable, no external calls"
func GenerateRiskSummary(meta *ContractMetadata) string {
	var writeFns, payableFns, callFns, highFns int
	for _, fn := range meta.Functions {
		if fn.Effects != nil && len(fn.Effects.Writes) > 0 {
			writeFns++
		}
		if fn.Mutability == "payable" {
			payableFns++
		}
		if fn.Effects != nil && len(fn.Effects.Calls) > 0 {
			callFns++
		}
		if fn.RiskLevel == "high" {
			highFns++
		}
	}

	// Determine overall risk level.
	overall := "Low"
	if highFns > 0 {
		overall = "High"
	} else if writeFns > 0 || payableFns > 0 || callFns > 0 {
		overall = "Medium"
	}

	var details []string
	details = append(details, pluralize(writeFns, "function", "writes state", "write state"))
	details = append(details, pluralize(payableFns, "function", "is payable", "are payable"))
	if callFns > 0 {
		details = append(details, pluralize(callFns, "function", "makes external calls", "make external calls"))
	} else {
		details = append(details, "no external calls")
	}

	return fmt.Sprintf("%s risk: %s", overall, strings.Join(details, ", "))
}

// FormatForDisplay formats the summary as human-readable text.
func (s *HumanReadableSummary) FormatForDisplay() string {
	var b strings.Builder

	name := s.ContractName
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Fprintf(&b, "Contract: %s\n", name)
	if s.Description != "" {
		fmt.Fprintf(&b, "  %s\n", s.Description)
	}
	fmt.Fprintf(&b, "\nRisk: %s\n", s.RiskSummary)

	if len(s.PolicyFeatures) > 0 {
		fmt.Fprintf(&b, "Policy features: %s\n", strings.Join(s.PolicyFeatures, ", "))
	}
	if len(s.Capabilities) > 0 {
		fmt.Fprintf(&b, "Required capabilities: %s\n", strings.Join(s.Capabilities, ", "))
	}

	fmt.Fprintf(&b, "\nFunctions (%d):\n", len(s.Functions))
	for _, fn := range s.Functions {
		flags := formatFunctionFlags(fn)
		fmt.Fprintf(&b, "  - %s [%s]%s\n", fn.Name, fn.RiskLevel, flags)
		fmt.Fprintf(&b, "    %s\n", fn.Description)
	}

	if s.TotalGasUpper > 0 {
		fmt.Fprintf(&b, "\nTotal gas upper bound: %d\n", s.TotalGasUpper)
	}

	return b.String()
}

// generateContractDescription builds a short description from metadata.
func generateContractDescription(meta *ContractMetadata) string {
	var parts []string

	if meta.IsAccount {
		parts = append(parts, "Account contract")
	} else {
		parts = append(parts, "Smart contract")
	}

	if meta.Manifest != nil {
		if meta.Manifest.Spec != "" {
			parts = append(parts, fmt.Sprintf("implementing %s", meta.Manifest.Spec))
		}
		if meta.Manifest.Version != "" {
			parts = append(parts, fmt.Sprintf("version %s", meta.Manifest.Version))
		}
	}

	desc := strings.Join(parts, " ")

	desc += fmt.Sprintf(" with %d function(s)", len(meta.Functions))
	if len(meta.Events) > 0 {
		desc += fmt.Sprintf(" and %d event(s)", len(meta.Events))
	}

	return desc
}

// derivePolicyFeatureNames returns human-readable names for active policy features.
func derivePolicyFeatureNames(pp *PolicyProfile) []string {
	var names []string
	if pp.HasSpendCaps {
		names = append(names, "spend caps")
	}
	if pp.HasAllowlist {
		names = append(names, "allowlist")
	}
	if pp.HasTerminalPolicy {
		names = append(names, "terminal policy")
	}
	if pp.HasGuardian {
		names = append(names, "guardian")
	}
	if pp.HasRecovery {
		names = append(names, "recovery")
	}
	if pp.HasDelegation {
		names = append(names, "delegation")
	}
	if pp.HasSuspension {
		names = append(names, "suspension")
	}
	return names
}

// joinWords joins words with commas and "and" for the last element.
func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
	}
}

// pluralize returns "N thing(s) verb" with correct pluralization.
func pluralize(n int, noun, singularVerb, pluralVerb string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s %s", n, noun, singularVerb)
	}
	return fmt.Sprintf("%d %ss %s", n, noun, pluralVerb)
}

// formatFunctionFlags returns a string of flags for display.
func formatFunctionFlags(fn FunctionSummary) string {
	var flags []string
	if fn.Payable {
		flags = append(flags, "payable")
	}
	if fn.Delegated {
		flags = append(flags, "delegated")
	}
	if fn.Verifiable {
		flags = append(flags, "verifiable")
	}
	if len(flags) == 0 {
		return ""
	}
	return " " + strings.Join(flags, ", ")
}
