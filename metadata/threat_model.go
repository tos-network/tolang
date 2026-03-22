package metadata

import "strings"

// ThreatModelSchemaVersion is the additive machine-readable schema version for
// exported openlib threat posture metadata.
const ThreatModelSchemaVersion = "0.1.0"

// ThreatModelProfile carries the family-level trust boundary and failure
// posture in a machine-readable form so agent runtimes do not need to scrape
// docs to understand the commercial risk surface.
type ThreatModelProfile struct {
	SchemaVersion      string   `json:"schema_version"`
	Family             string   `json:"family"`
	Scope              string   `json:"scope,omitempty"`
	TrustBoundary      string   `json:"trust_boundary"`
	CriticalInvariants []string `json:"critical_invariants"`
	FailurePosture     string   `json:"failure_posture"`
	RuntimeDependency  string   `json:"runtime_dependency"`
}

type familyThreatModel struct {
	TrustBoundary      string
	CriticalInvariants []string
	FailurePosture     string
	RuntimeDependency  string
}

var openlibThreatModels = map[string]familyThreatModel{
	"account": {
		TrustBoundary:      "account owner, guardian, delegate executor",
		CriticalInvariants: []string{"daily limit never underflows", "allowlist gates stay authoritative", "delegate budget and expiry stay bounded"},
		FailurePosture:     "fail closed on overspend, suspension, or unauthorized execution",
		RuntimeDependency:  "strong dependency on nested-call atomicity and external-call rollback",
	},
	"authority": {
		TrustBoundary:      "grant writer and revoker",
		CriticalInvariants: []string{"scope must stay unique enough", "nonce consumption must be monotonic", "revocation must dominate reuse"},
		FailurePosture:     "fail closed on stale or replayed authority",
		RuntimeDependency:  "moderate dependency on durable replay semantics across hosts",
	},
	"execution_binding": {
		TrustBoundary:      "approver and receipt-binding consumer",
		CriticalInvariants: []string{"single-use binding must never be consumable twice", "expiry must dominate late execution", "value must stay within ceiling"},
		FailurePosture:     "fail closed on replay, expiry, or over-limit use",
		RuntimeDependency:  "moderate dependency on timestamp correctness and receipt-link consistency",
	},
	"session": {
		TrustBoundary:      "session issuer and terminal-trust policy",
		CriticalInvariants: []string{"session expiry must dominate reuse", "step-up threshold must be enforced before spend", "revoked session must become inert immediately"},
		FailurePosture:     "fail closed on inactive or degraded session",
		RuntimeDependency:  "moderate dependency on trustworthy terminal/session evidence outside chain",
	},
	"agreement": {
		TrustBoundary:      "counterparties and evidence finalizer",
		CriticalInvariants: []string{"agreement state machine must remain monotonic", "accepted terms must not mutate after acceptance", "fulfill and cancel must be exclusive terminal outcomes"},
		FailurePosture:     "fail closed on invalid transition",
		RuntimeDependency:  "low dependency beyond standard storage consistency",
	},
	"settlement": {
		TrustBoundary:      "task poster, worker, dispute resolver",
		CriticalInvariants: []string{"escrowed reward must map to exactly one terminal payout path", "reject, dispute, and resolve states must be exclusive", "reclaim must only occur after expiry"},
		FailurePosture:     "fail closed on wrong status or wrong actor",
		RuntimeDependency:  "strong dependency on host escrow and release correctness plus rollback",
	},
	"sponsor": {
		TrustBoundary:      "sponsor treasury owner and authorized relayer",
		CriticalInvariants: []string{"relayer spend must not exceed budget", "sponsor spend must not exceed deposits", "policy hash must bind the route"},
		FailurePosture:     "fail closed on budget, expiry, or policy mismatch",
		RuntimeDependency:  "very strong dependency on external-call atomicity, revert propagation, and accounting semantics",
	},
	"evidence": {
		TrustBoundary:      "evidence writer and finalizer",
		CriticalInvariants: []string{"evidence id must remain unique", "finalization must be monotonic", "challenge and settlement refs must preserve audit linkage"},
		FailurePosture:     "fail closed on duplicate or inconsistent evidence state",
		RuntimeDependency:  "moderate dependency on off-chain proof systems and evidence availability",
	},
	"receipt": {
		TrustBoundary:      "receipt writer",
		CriticalInvariants: []string{"each receipt id must be unique", "success and failure finalization must be terminal", "binding and proof refs must remain stable once opened"},
		FailurePosture:     "fail closed on duplicate open or repeated finalize",
		RuntimeDependency:  "low dependency in pure storage terms and higher dependency when used as settlement or audit anchor",
	},
	"trust": {
		TrustBoundary:      "registry writer and scoring authority",
		CriticalInvariants: []string{"eligibility and reputation updates must not fabricate stake or status", "slash and suspend semantics must remain explicit and monotonic"},
		FailurePosture:     "fail closed on untrusted registry mutation",
		RuntimeDependency:  "high dependency on external reputation, stake, or slashing systems",
	},
	"privacy": {
		TrustBoundary:      "UNO bridge, resolver, and disclosure policy",
		CriticalInvariants: []string{"confidential balance movement must preserve ciphertext integrity", "release and refund paths must be exclusive", "disclosure permissions must not outlive revocation intent"},
		FailurePosture:     "fail closed on zero or invalid confidential value and unauthorized disclosure",
		RuntimeDependency:  "very strong dependency on UNO native rails, ciphertext helpers, and proof validity",
	},
	"recovery": {
		TrustBoundary:      "guardian set, owner, and recovery controller",
		CriticalInvariants: []string{"freeze and rotate flows must not bypass the active guardian threshold", "recovery must not silently resurrect revoked authority"},
		FailurePosture:     "fail closed during contested recovery and require explicit recovery path",
		RuntimeDependency:  "moderate dependency on external guardian coordination and terminal-loss workflow",
	},
	"discovery": {
		TrustBoundary:      "directory writer and market publisher",
		CriticalInvariants: []string{"active flag must dominate routing", "provider and manifest linkage must stay stable", "capability metadata must not drift from actual contract surface"},
		FailurePosture:     "fail closed on inactive or mismatched service record",
		RuntimeDependency:  "moderate dependency on off-chain discovery consumers respecting metadata",
	},
}

// BuildThreatModelProfile derives a family-level threat profile for exported
// openlib metadata surfaces. Unknown non-openlib packages deliberately return
// nil so generic contracts do not inherit openlib-specific risk claims.
func BuildThreatModelProfile(meta *ContractMetadata, packageName string) *ThreatModelProfile {
	family := inferThreatModelFamily(packageName)
	if family == "" {
		return nil
	}
	return buildThreatModelForFamily(family, "contract")
}

// BuildBundleThreatModelProfile emits the family-level threat model for a
// multi-contract release bundle.
func BuildBundleThreatModelProfile(family string) *ThreatModelProfile {
	return buildThreatModelForFamily(strings.TrimSpace(family), "family_bundle")
}

func buildThreatModelForFamily(family, scope string) *ThreatModelProfile {
	model, ok := openlibThreatModels[family]
	if !ok {
		return nil
	}
	return &ThreatModelProfile{
		SchemaVersion:      ThreatModelSchemaVersion,
		Family:             family,
		Scope:              scope,
		TrustBoundary:      model.TrustBoundary,
		CriticalInvariants: append([]string(nil), model.CriticalInvariants...),
		FailurePosture:     model.FailurePosture,
		RuntimeDependency:  model.RuntimeDependency,
	}
}

func inferThreatModelFamily(packageName string) string {
	const (
		openlibPrefix = "tolang.openlib."
		stdlibPrefix  = "tolang.stdlib."
	)
	name := strings.TrimSpace(packageName)
	switch {
	case strings.HasPrefix(name, openlibPrefix):
		rest := strings.TrimPrefix(name, openlibPrefix)
		return familyFromQualifiedPackage(rest)
	case strings.HasPrefix(name, stdlibPrefix):
		rest := strings.TrimPrefix(name, stdlibPrefix)
		return familyFromQualifiedPackage(rest)
	default:
		return ""
	}
}

func familyFromQualifiedPackage(rest string) string {
	if rest == "" {
		return ""
	}
	parts := strings.Split(rest, ".")
	if len(parts) == 0 {
		return ""
	}
	family := strings.TrimSpace(parts[0])
	if _, ok := openlibThreatModels[family]; ok {
		return family
	}
	return ""
}
