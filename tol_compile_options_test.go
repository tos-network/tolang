package lua

import (
	"bytes"
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

func TestCompileBytecodeDefaultStripsSourceMapAndIsPathIndependent(t *testing.T) {
	a, err := CompileBytecode([]byte(compileOptionsSample), "/tmp/a.tol")
	if err != nil {
		t.Fatalf("compile bytecode a: %v", err)
	}
	b, err := CompileBytecode([]byte(compileOptionsSample), "/var/tmp/b.tol")
	if err != nil {
		t.Fatalf("compile bytecode b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected default bytecode output to be path-independent")
	}
	proto, err := DecodeFunctionProto(a)
	if err != nil {
		t.Fatalf("decode bytecode: %v", err)
	}
	if proto.SourceName != "" {
		t.Fatalf("expected stripped source name, got %q", proto.SourceName)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected stripped debug positions, got %d", len(proto.DbgSourcePositions))
	}
}

func TestCompileArtifactDefaultStripsSourceMapAndIsPathIndependent(t *testing.T) {
	a, err := CompileArtifact([]byte(compileOptionsSample), "/tmp/a.tol")
	if err != nil {
		t.Fatalf("compile artifact a: %v", err)
	}
	b, err := CompileArtifact([]byte(compileOptionsSample), "/var/tmp/b.tol")
	if err != nil {
		t.Fatalf("compile artifact b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected default artifact output to be path-independent")
	}
	art, err := DecodeArtifact(a)
	if err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	proto, err := DecodeFunctionProto(art.Bytecode)
	if err != nil {
		t.Fatalf("decode embedded bytecode: %v", err)
	}
	if proto.SourceName != "" {
		t.Fatalf("expected stripped embedded source name, got %q", proto.SourceName)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected stripped embedded debug positions, got %d", len(proto.DbgSourcePositions))
	}
}

func TestCompilePackageDefaultStripsSourceMapAndIsPathIndependent(t *testing.T) {
	a, err := CompilePackage([]byte(compileOptionsSample), "/tmp/a.tol", nil)
	if err != nil {
		t.Fatalf("compile package a: %v", err)
	}
	b, err := CompilePackage([]byte(compileOptionsSample), "/var/tmp/b.tol", nil)
	if err != nil {
		t.Fatalf("compile package b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected default package output to be path-independent")
	}
	pkg, err := DecodePackage(a)
	if err != nil {
		t.Fatalf("decode package: %v", err)
	}
	var manifest struct {
		Contracts []struct {
			Artifact string `json:"toc"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(pkg.ManifestJSON, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	artifactData := pkg.Files[manifest.Contracts[0].Artifact]
	art, err := DecodeArtifact(artifactData)
	if err != nil {
		t.Fatalf("decode package artifact: %v", err)
	}
	proto, err := DecodeFunctionProto(art.Bytecode)
	if err != nil {
		t.Fatalf("decode embedded bytecode: %v", err)
	}
	if proto.SourceName != "" {
		t.Fatalf("expected stripped package source name, got %q", proto.SourceName)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected stripped package debug positions, got %d", len(proto.DbgSourcePositions))
	}
}

func TestCompileArtifactWithOptions_EnableSourceMapKeepsDebugInfo(t *testing.T) {
	artifactBytes, err := CompileArtifactWithOptions([]byte(compileOptionsSample), "sample.tol", &ArtifactOptions{IncludeSourceMap: true})
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
	if proto.SourceName == "" {
		t.Fatalf("expected source name to be preserved when source maps are enabled")
	}
	if len(proto.DbgSourcePositions) == 0 {
		t.Fatalf("expected debug positions to be preserved when source maps are enabled")
	}
}
