package lua

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCompileArtifactRoundTrip(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  u256 total;
  mapping(agent => u256) balances;

  event Tick(u256 v);

  function ping(agent owner, u256 amount) public {
    return;
  }
}
`)
	artifactBytes, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected artifact compile error: %v", err)
	}
	if !IsArtifact(artifactBytes) {
		t.Fatalf("expected artifact magic")
	}

	art, err := DecodeArtifact(artifactBytes)
	if err != nil {
		t.Fatalf("unexpected artifact decode error: %v", err)
	}
	if art.Version != ArtifactFormatVersion {
		t.Fatalf("unexpected artifact version: got=%d want=%d", art.Version, ArtifactFormatVersion)
	}
	if art.ContractName != "Demo" {
		t.Fatalf("unexpected contract name: %q", art.ContractName)
	}
	if art.SourceHash != keccak256Hex(src) {
		t.Fatalf("unexpected source hash: got=%s want=%s", art.SourceHash, keccak256Hex(src))
	}
	if art.BytecodeHash != keccak256Hex(art.Bytecode) {
		t.Fatalf("unexpected bytecode hash: got=%s want=%s", art.BytecodeHash, keccak256Hex(art.Bytecode))
	}
	if _, err := DecodeFunctionProto(art.Bytecode); err != nil {
		t.Fatalf("decoded toc contains invalid bytecode: %v", err)
	}

	var abi struct {
		ABIVersion string `json:"abi_version"`
		Kind       string `json:"kind"`
		Functions  []struct {
			Name       string   `json:"name"`
			Visibility string   `json:"visibility"`
			Selector   string   `json:"selector"`
			Params     []string `json:"params"`
		} `json:"functions"`
		Errors []struct {
			Name     string   `json:"name"`
			Kind     string   `json:"kind"`
			Selector string   `json:"selector"`
			Params   []string `json:"params"`
		} `json:"errors"`
		Events []struct {
			Name string `json:"name"`
		} `json:"events"`
	}
	if err := json.Unmarshal(art.ABIJSON, &abi); err != nil {
		t.Fatalf("invalid abi json: %v", err)
	}
	if len(abi.Functions) != 1 {
		t.Fatalf("unexpected abi function count: %d", len(abi.Functions))
	}
	if abi.ABIVersion != "1.0" {
		t.Fatalf("unexpected abi_version: got=%q want=1.0", abi.ABIVersion)
	}
	if abi.Kind != "contract" {
		t.Fatalf("unexpected kind: got=%q want=contract", abi.Kind)
	}
	if abi.Functions[0].Name != "ping" {
		t.Fatalf("unexpected function name: %q", abi.Functions[0].Name)
	}
	wantSel := abiSelectorHex("ping", []string{"agent", "u256"})
	if abi.Functions[0].Selector != wantSel {
		t.Fatalf("unexpected function selector: got=%s want=%s", abi.Functions[0].Selector, wantSel)
	}
	if len(abi.Events) != 1 || abi.Events[0].Name != "Tick" {
		t.Fatalf("unexpected abi events: %+v", abi.Events)
	}
	if len(abi.Errors) != 0 {
		t.Fatalf("unexpected abi errors: %+v", abi.Errors)
	}

	var storage struct {
		Slots []struct {
			Name          string `json:"name"`
			Type          string `json:"type"`
			CanonicalHash string `json:"canonical_hash"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(art.StorageLayoutJSON, &storage); err != nil {
		t.Fatalf("invalid storage json: %v", err)
	}
	if len(storage.Slots) != 2 {
		t.Fatalf("unexpected storage slot count: %d", len(storage.Slots))
	}
	if storage.Slots[0].Name != "total" || storage.Slots[0].Type != "u256" {
		t.Fatalf("unexpected first storage slot: %+v", storage.Slots[0])
	}
	wantSlotHash := keccak256Hex([]byte("tol.slot.Demo.total"))
	if storage.Slots[0].CanonicalHash != wantSlotHash {
		t.Fatalf("unexpected slot hash: got=%s want=%s", storage.Slots[0].CanonicalHash, wantSlotHash)
	}
}

func TestCompileArtifactDeterministic(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public {
    return;
  }
}
`)
	a, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error (first): %v", err)
	}
	b, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error (second): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic artifact bytes")
	}
}

func TestEncodeArtifactRejectsInvalidHash(t *testing.T) {
	_, err := EncodeArtifact(&Artifact{
		Version:      ArtifactFormatVersion,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1, 2, 3},
		SourceHash:   "0x1234",
		BytecodeHash: keccak256Hex([]byte{1, 2, 3}),
	})
	if err == nil {
		t.Fatalf("expected invalid hash error")
	}
}

func TestDecodeArtifactRejectsBytecodeHashMismatch(t *testing.T) {
	artifactBytes, err := EncodeArtifact(&Artifact{
		Version:      ArtifactFormatVersion,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1, 2, 3},
		SourceHash:   keccak256Hex([]byte("src")),
		BytecodeHash: keccak256Hex([]byte{9}),
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if _, err := DecodeArtifact(artifactBytes); err == nil {
		t.Fatalf("expected bytecode hash mismatch")
	}
}

func TestDecodeArtifactRejectsUnsupportedVersion(t *testing.T) {
	artifactBytes, err := EncodeArtifact(&Artifact{
		Version:      ArtifactFormatVersion + 1,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1},
		SourceHash:   keccak256Hex([]byte("src")),
		BytecodeHash: keccak256Hex([]byte{1}),
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if _, err := DecodeArtifact(artifactBytes); err == nil {
		t.Fatalf("expected unsupported version error")
	}
}

func TestDecodeArtifactRejectsInvalidEmbeddedBytecode(t *testing.T) {
	artifactBytes, err := EncodeArtifact(&Artifact{
		Version:      ArtifactFormatVersion,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1, 2, 3},
		SourceHash:   keccak256Hex([]byte("src")),
		BytecodeHash: keccak256Hex([]byte{1, 2, 3}),
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if _, err := DecodeArtifact(artifactBytes); err == nil {
		t.Fatalf("expected invalid embedded bytecode error")
	}
}

func TestDecodeArtifactRejectsEmptyContractName(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	bytecode, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, ArtifactFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, ""); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, bytecode); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(bytecode)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeArtifact(raw.Bytes()); err == nil {
		t.Fatalf("expected empty contract name error")
	}
}

func TestDecodeArtifactRejectsEmptyBytecodePayload(t *testing.T) {
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, ArtifactFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, "Demo"); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(nil)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeArtifact(raw.Bytes()); err == nil {
		t.Fatalf("expected empty bytecode error")
	}
}

func TestVerifyArtifactSourceHashMatches(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	artifactBytes, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected artifact compile error: %v", err)
	}
	art, err := DecodeArtifact(artifactBytes)
	if err != nil {
		t.Fatalf("unexpected artifact decode error: %v", err)
	}
	if err := VerifySourceHash(art, src); err != nil {
		t.Fatalf("unexpected source hash mismatch: %v", err)
	}
}

func TestVerifyArtifactSourceHashMismatch(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	artifactBytes, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected artifact compile error: %v", err)
	}
	art, err := DecodeArtifact(artifactBytes)
	if err != nil {
		t.Fatalf("unexpected artifact decode error: %v", err)
	}
	if err := VerifySourceHash(art, []byte("other source")); err == nil {
		t.Fatalf("expected source hash mismatch")
	}
}

func TestVerifyArtifactSourceHashRejectsNilArtifact(t *testing.T) {
	if err := VerifySourceHash(nil, []byte("x")); err == nil {
		t.Fatalf("expected nil artifact error")
	}
}

func TestDecodeArtifactRejectsInvalidABIJSON(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	bytecode, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, ArtifactFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, "Demo"); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, bytecode); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{")); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{}")); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(bytecode)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeArtifact(raw.Bytes()); err == nil {
		t.Fatalf("expected invalid abi json error")
	}
}

func TestDecodeArtifactRejectsInvalidStorageJSON(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	bytecode, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, ArtifactFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, "Demo"); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, bytecode); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{}")); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{")); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(bytecode)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeArtifact(raw.Bytes()); err == nil {
		t.Fatalf("expected invalid storage json error")
	}
}

// TestArtifactABIGasModelField verifies that the compiled .toc ABI JSON contains a
// top-level "gas_model" field with the correct version string and cost constants
// as specified in TOL_EFFECTS.md §7.4 and §9.
func TestArtifactABIGasModelField(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract GasModelTest {
  function getValue() public view returns (u256 v) {
    return 42;
  }
}
`)
	artifactBytes, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	art, err := DecodeArtifact(artifactBytes)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	var abi struct {
		GasModel struct {
			Version string `json:"version"`
			Sload   uint64 `json:"sload"`
			Sstore  uint64 `json:"sstore"`
			LogBase uint64 `json:"log_base"`
		} `json:"gas_model"`
	}
	if err := json.Unmarshal(art.ABIJSON, &abi); err != nil {
		t.Fatalf("failed to parse ABI JSON: %v", err)
	}

	if abi.GasModel.Version != gasModelVersion {
		t.Errorf("gas_model.version: got=%q want=%q", abi.GasModel.Version, gasModelVersion)
	}
	if abi.GasModel.Sload != gasModelSload {
		t.Errorf("gas_model.sload: got=%d want=%d", abi.GasModel.Sload, gasModelSload)
	}
	if abi.GasModel.Sstore != gasModelSstore {
		t.Errorf("gas_model.sstore: got=%d want=%d", abi.GasModel.Sstore, gasModelSstore)
	}
	if abi.GasModel.LogBase != gasModelLogBase {
		t.Errorf("gas_model.log_base: got=%d want=%d", abi.GasModel.LogBase, gasModelLogBase)
	}
}

func TestArtifactABIIncludesErrorsAndRevertSchema(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract ErrorDemo {
  error Unauthorized(agent caller);

  function guarded(agent caller) public {
    require(caller != agent(0), "ZERO");
    revert Unauthorized(caller);
  }
}
`)
	artifactBytes, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	art, err := DecodeArtifact(artifactBytes)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	var abi struct {
		Errors []struct {
			Name     string   `json:"name"`
			Kind     string   `json:"kind"`
			Selector string   `json:"selector"`
			Params   []string `json:"params"`
		} `json:"errors"`
		Functions []struct {
			Name string `json:"name"`
			Doc  struct {
				RevertSchema []struct {
					Name     string `json:"name"`
					Kind     string `json:"kind"`
					Selector string `json:"selector"`
				} `json:"revert_schema"`
			} `json:"doc"`
		} `json:"functions"`
	}
	if err := json.Unmarshal(art.ABIJSON, &abi); err != nil {
		t.Fatalf("failed to parse ABI JSON: %v", err)
	}
	if len(abi.Errors) != 1 {
		t.Fatalf("errors length = %d, want 1", len(abi.Errors))
	}
	if abi.Errors[0].Name != "Unauthorized" || abi.Errors[0].Kind != "custom" {
		t.Fatalf("unexpected error entry: %+v", abi.Errors[0])
	}
	wantSel := selectorHexFromSignature("Unauthorized(agent)")
	if abi.Errors[0].Selector != wantSel {
		t.Fatalf("error selector = %q, want %q", abi.Errors[0].Selector, wantSel)
	}
	if len(abi.Functions) != 1 {
		t.Fatalf("functions length = %d, want 1", len(abi.Functions))
	}
	if len(abi.Functions[0].Doc.RevertSchema) != 2 {
		t.Fatalf("revert_schema length = %d, want 2", len(abi.Functions[0].Doc.RevertSchema))
	}
}
