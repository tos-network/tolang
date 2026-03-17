package metadata

import "fmt"

// CompatibilityMatrix defines the cross-repository version compatibility.
type CompatibilityMatrix struct {
	SchemaVersion   string                `json:"schema_version"`
	GeneratedAt     string                `json:"generated_at"`
	BoundaryVersion string                `json:"boundary_version"`
	MetadataVersion string                `json:"metadata_version"`
	Repositories    map[string]RepoCompat `json:"repositories"`
}

// RepoCompat describes the compatibility bounds for a single repository.
type RepoCompat struct {
	Name               string   `json:"name"`
	MinBoundaryVersion string   `json:"min_boundary_version"`
	MaxBoundaryVersion string   `json:"max_boundary_version"`
	MinMetadataVersion string   `json:"min_metadata_version"`
	MaxMetadataVersion string   `json:"max_metadata_version"`
	Features           []string `json:"features"`
}

// CurrentCompatibilityMatrix returns the current compatibility state for the
// GTOS 2046 architecture repositories (TOL, OpenFox, GTOS boundary).
func CurrentCompatibilityMatrix() *CompatibilityMatrix {
	return &CompatibilityMatrix{
		SchemaVersion:   "0.1.0",
		GeneratedAt:     "2026-03-17",
		BoundaryVersion: "0.1.0",
		MetadataVersion: SchemaVersion,
		Repositories: map[string]RepoCompat{
			"tolang": {
				Name:               "TOL Compiler",
				MinBoundaryVersion: "0.1.0",
				MaxBoundaryVersion: "0.1.x",
				MinMetadataVersion: "0.1.0",
				MaxMetadataVersion: "0.1.x",
				Features:           []string{"abi_json", "metadata_extract", "human_readable", "discovery_manifest", "artifact_ref"},
			},
			"openfox": {
				Name:               "OpenFox Runtime",
				MinBoundaryVersion: "0.1.0",
				MaxBoundaryVersion: "0.1.x",
				MinMetadataVersion: "0.1.0",
				MaxMetadataVersion: "0.1.x",
				Features:           []string{"intent_routing", "approval_ux", "discovery_client", "policy_enforcement"},
			},
			"gtos": {
				Name:               "GTOS Boundary",
				MinBoundaryVersion: "0.1.0",
				MaxBoundaryVersion: "0.1.x",
				MinMetadataVersion: "0.1.0",
				MaxMetadataVersion: "0.1.x",
				Features:           []string{"artifact_storage", "cross_repo_ref", "schema_validation"},
			},
		},
	}
}

// CheckCompatibility verifies if two repo versions are compatible by checking
// that their boundary and metadata version ranges overlap. Returns true if
// compatible, along with a human-readable reason.
func CheckCompatibility(matrix *CompatibilityMatrix, repoA, repoB string) (bool, string) {
	a, okA := matrix.Repositories[repoA]
	b, okB := matrix.Repositories[repoB]

	if !okA {
		return false, fmt.Sprintf("unknown repository: %s", repoA)
	}
	if !okB {
		return false, fmt.Sprintf("unknown repository: %s", repoB)
	}

	// Check boundary version overlap using prefix matching for "0.1.x" style ranges.
	if !rangesOverlap(a.MinBoundaryVersion, a.MaxBoundaryVersion, b.MinBoundaryVersion, b.MaxBoundaryVersion) {
		return false, fmt.Sprintf(
			"boundary version mismatch: %s supports %s..%s, %s supports %s..%s",
			repoA, a.MinBoundaryVersion, a.MaxBoundaryVersion,
			repoB, b.MinBoundaryVersion, b.MaxBoundaryVersion,
		)
	}

	// Check metadata version overlap.
	if !rangesOverlap(a.MinMetadataVersion, a.MaxMetadataVersion, b.MinMetadataVersion, b.MaxMetadataVersion) {
		return false, fmt.Sprintf(
			"metadata version mismatch: %s supports %s..%s, %s supports %s..%s",
			repoA, a.MinMetadataVersion, a.MaxMetadataVersion,
			repoB, b.MinMetadataVersion, b.MaxMetadataVersion,
		)
	}

	return true, fmt.Sprintf("%s and %s are compatible at boundary %s, metadata %s",
		repoA, repoB, matrix.BoundaryVersion, matrix.MetadataVersion)
}

// rangesOverlap checks if two semver-like ranges overlap. For simplicity,
// we compare the min versions: if they share the same major.minor prefix,
// they are considered overlapping (appropriate for 0.x development).
func rangesOverlap(minA, maxA, minB, maxB string) bool {
	// Extract major.minor from min versions and check they match.
	prefA := semverPrefix(minA)
	prefB := semverPrefix(minB)
	return prefA == prefB
}

// semverPrefix extracts "major.minor" from a semver string like "0.1.0" -> "0.1".
func semverPrefix(v string) string {
	parts := splitDot(v)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

// splitDot splits a string on "." without importing strings (already used elsewhere).
func splitDot(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
