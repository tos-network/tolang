package metadata

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"
)

// keccak256Hex returns the keccak-256 hash of data as a "0x"-prefixed hex string.
func keccak256Hex(data []byte) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(data)
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

// ComputeArtifactRef computes the canonical ArtifactRef from raw data.
// Any nil byte slice is treated as empty and produces the keccak-256 hash of "".
func ComputeArtifactRef(packageBytes, bytecodeBytes, sourceBytes, abiBytes []byte, version string) ArtifactRef {
	ref := ArtifactRef{
		PackageHash:  keccak256Hex(packageBytes),
		BytecodeHash: keccak256Hex(bytecodeBytes),
		ABIHash:      keccak256Hex(abiBytes),
		Version:      version,
	}
	if sourceBytes != nil {
		ref.SourceHash = keccak256Hex(sourceBytes)
	}
	return ref
}
