package metadata

import "strings"

// RuntimeBoundarySchemaVersion labels the machine-readable boundary profile
// that distinguishes GTOS-native semantics from the preferred openlib
// developer-facing entrypoints.
const RuntimeBoundarySchemaVersion = "0.1.0"

// RuntimeBoundaryProfile describes which capabilities are protocol-native,
// which developer-facing surfaces should be preferred in openlib, and which
// future system-contract shapes the current native surfaces are expected to
// collapse into.
type RuntimeBoundaryProfile struct {
	SchemaVersion        string   `json:"schema_version"`
	NativeSurfaces       []string `json:"native_surfaces,omitempty"`
	PreferredOpenlib     []string `json:"preferred_openlib,omitempty"`
	FutureSystemSurfaces []string `json:"future_system_surfaces,omitempty"`
	Notes                []string `json:"notes,omitempty"`
}

// BuildRuntimeBoundaryProfile derives a machine-readable boundary profile for
// openlib exports. Generic non-openlib packages return nil because the
// boundary policy is currently specific to the GTOS/openlib stack.
func BuildRuntimeBoundaryProfile(meta *ContractMetadata, packageName string) *RuntimeBoundaryProfile {
	if meta == nil {
		return nil
	}
	family := inferThreatModelFamily(packageName)
	if family == "" {
		return nil
	}

	boundary := &RuntimeBoundaryProfile{
		SchemaVersion: RuntimeBoundarySchemaVersion,
	}

	pkg := strings.TrimSpace(packageName)
	if pkg != "" {
		addUniqueString(&boundary.PreferredOpenlib, pkg)
	}
	if pkg != "" && strings.TrimSpace(meta.Contract.Name) != "" {
		addUniqueString(&boundary.PreferredOpenlib, pkg+"."+strings.TrimSpace(meta.Contract.Name))
	}

	align := BuildProtocolAlignment(meta, packageName)
	if align != nil {
		if align.SettlementBus {
			addUniqueString(&boundary.NativeSurfaces, "settlement_bus")
			addUniqueString(&boundary.FutureSystemSurfaces, "system.settlement")
			addUniqueString(&boundary.FutureSystemSurfaces, "system.receipt")
		}
		if align.RegistryGovernance {
			addUniqueString(&boundary.NativeSurfaces, "protocol_registry")
			addUniqueString(&boundary.NativeSurfaces, "runtime_inspection")
			addUniqueString(&boundary.FutureSystemSurfaces, "system.registry")
		}
		if align.PackageGovernance {
			addUniqueString(&boundary.NativeSurfaces, "package_registry")
			addUniqueString(&boundary.NativeSurfaces, "runtime_inspection")
			addUniqueString(&boundary.FutureSystemSurfaces, "system.package_registry")
		}
	}

	switch family {
	case "privacy":
		addUniqueString(&boundary.NativeSurfaces, "uno_rail")
		addUniqueString(&boundary.FutureSystemSurfaces, "system.uno")
	case "account", "authority", "execution_binding", "session", "recovery", "discovery", "trust":
		addUniqueString(&boundary.NativeSurfaces, "runtime_inspection")
	}

	if len(boundary.PreferredOpenlib) > 0 {
		addUniqueString(&boundary.Notes, "prefer_openlib_entrypoints")
	}
	if len(boundary.NativeSurfaces) > 0 {
		addUniqueString(&boundary.Notes, "native_surfaces_are_protocol_semantics")
	}

	return boundary
}

// BuildBundleRuntimeBoundaryProfile merges contract-level boundary profiles into
// one family-level view.
func BuildBundleRuntimeBoundaryProfile(boundaries ...*RuntimeBoundaryProfile) *RuntimeBoundaryProfile {
	merged := &RuntimeBoundaryProfile{
		SchemaVersion: RuntimeBoundarySchemaVersion,
	}
	for _, boundary := range boundaries {
		if boundary == nil {
			continue
		}
		for _, item := range boundary.NativeSurfaces {
			addUniqueString(&merged.NativeSurfaces, item)
		}
		for _, item := range boundary.PreferredOpenlib {
			addUniqueString(&merged.PreferredOpenlib, item)
		}
		for _, item := range boundary.FutureSystemSurfaces {
			addUniqueString(&merged.FutureSystemSurfaces, item)
		}
		for _, item := range boundary.Notes {
			addUniqueString(&merged.Notes, item)
		}
	}
	if len(merged.NativeSurfaces) == 0 && len(merged.PreferredOpenlib) == 0 &&
		len(merged.FutureSystemSurfaces) == 0 && len(merged.Notes) == 0 {
		return nil
	}
	return merged
}

func addUniqueString(dst *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *dst {
		if existing == value {
			return
		}
	}
	*dst = append(*dst, value)
}
