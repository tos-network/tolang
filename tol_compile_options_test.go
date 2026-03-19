package lua

import (
	"encoding/json"
	"testing"
)

const compileOptionsSample = `pragma tolang 0.2.0;
contract Sample {
  function ping() public view returns (u256 x) {
    u256 x = 1;
    return x;
  }
}
`

func TestCompileBytecodeWithOptions_DisableSourceMap(t *testing.T) {
	bc, err := CompileBytecodeWithOptions([]byte(compileOptionsSample), "sample.tol", &CompileOptions{IncludeSourceMap: false})
	if err != nil {
		t.Fatalf("compile bytecode: %v", err)
	}
	proto, err := DecodeFunctionProto(bc)
	if err != nil {
		t.Fatalf("decode bytecode: %v", err)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected no debug positions, got %d", len(proto.DbgSourcePositions))
	}
}

func TestCompileArtifactWithOptions_DisableSourceMap(t *testing.T) {
	artifactBytes, err := CompileArtifactWithOptions([]byte(compileOptionsSample), "sample.tol", &ArtifactOptions{IncludeSourceMap: false})
	if err != nil {
		t.Fatalf("compile artifact: %v", err)
	}
	art, err := DecodeArtifact(artifactBytes)
	if err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	proto, err := DecodeFunctionProto(art.Bytecode)
	if err != nil {
		t.Fatalf("decode embedded bytecode: %v", err)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected no debug positions in artifact bytecode, got %d", len(proto.DbgSourcePositions))
	}
}

func TestCompilePackageWithOptions_DisableSourceMap(t *testing.T) {
	sm := false
	tor, err := CompilePackage([]byte(compileOptionsSample), "sample.tol", &PackageOptions{
		PackageName:      "sample",
		PackageVersion:   "1.0.0",
		IncludeSourceMap: &sm,
	})
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}
	art, err := DecodePackage(tor)
	if err != nil {
		t.Fatalf("decode tor: %v", err)
	}

	var manifest struct {
		Contracts []struct {
			Artifact string `json:"toc"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(art.ManifestJSON, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Contracts) != 1 {
		t.Fatalf("expected one contract entry, got %d", len(manifest.Contracts))
	}
	artifactPath := manifest.Contracts[0].Artifact
	artifactData, ok := art.Files[artifactPath]
	if !ok {
		t.Fatalf("artifact %q not found in package", artifactPath)
	}
	innerArt, err := DecodeArtifact(artifactData)
	if err != nil {
		t.Fatalf("decode artifact in package: %v", err)
	}
	proto, err := DecodeFunctionProto(innerArt.Bytecode)
	if err != nil {
		t.Fatalf("decode embedded bytecode: %v", err)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected no debug positions in package artifact bytecode, got %d", len(proto.DbgSourcePositions))
	}
}
