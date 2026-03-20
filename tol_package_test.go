package lua

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func mustCompileTestArtifact(t *testing.T) []byte {
	t.Helper()
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	artifactBytes, err := CompileArtifact(src, "<tol>")
	if err != nil {
		t.Fatalf("compile artifact: %v", err)
	}
	return artifactBytes
}

func mustCompileTestInterface(t *testing.T) []byte {
	t.Helper()
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	ifaceData, err := CompileInterface(src, "<tol>")
	if err != nil {
		t.Fatalf("compile interface: %v", err)
	}
	return ifaceData
}

func TestEncodeDecodePackageRoundTrip(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	artifactData := mustCompileTestArtifact(t)
	ifaceData := mustCompileTestInterface(t)
	files := map[string][]byte{
		"bytecode/Demo.toc":    artifactData,
		"interfaces/IDemo.abi": ifaceData,
	}

	pkgA, err := EncodePackage(manifest, files)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	pkgB, err := EncodePackage(manifest, files)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if !bytes.Equal(pkgA, pkgB) {
		t.Fatalf("expected deterministic package bytes")
	}
	if !IsPackage(pkgA) {
		t.Fatalf("expected package magic")
	}

	decoded, err := DecodePackage(pkgA)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if string(decoded.ManifestJSON) != string(manifest) {
		t.Fatalf("unexpected manifest: %s", string(decoded.ManifestJSON))
	}
	if !bytes.Equal(decoded.Files["bytecode/Demo.toc"], artifactData) {
		t.Fatalf("unexpected bytecode entry")
	}
	if !bytes.Equal(decoded.Files["interfaces/IDemo.abi"], ifaceData) {
		t.Fatalf("unexpected interface entry")
	}
}

func TestEncodePackageRejectsInvalidManifestJSON(t *testing.T) {
	if _, err := EncodePackage([]byte("{"), nil); err == nil {
		t.Fatalf("expected invalid manifest json error")
	}
}

func TestEncodePackageRejectsManifestMissingName(t *testing.T) {
	if _, err := EncodePackage([]byte(`{"version":"1.0.0"}`), nil); err == nil {
		t.Fatalf("expected missing name error")
	}
}

func TestEncodePackageRejectsManifestMissingVersion(t *testing.T) {
	if _, err := EncodePackage([]byte(`{"name":"demo"}`), nil); err == nil {
		t.Fatalf("expected missing version error")
	}
}

func TestEncodePackageRejectsPathEscape(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0"}`)
	if _, err := EncodePackage(manifest, map[string][]byte{"../x": []byte("x")}); err == nil {
		t.Fatalf("expected path escape error")
	}
}

func TestEncodePackageRejectsMissingManifestReferencedArtifact(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	if _, err := EncodePackage(manifest, map[string][]byte{
		"interfaces/IDemo.abi": []byte("i"),
	}); err == nil {
		t.Fatalf("expected missing manifest referenced artifact error")
	}
}

func TestEncodePackageRejectsManifestContractMissingName(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"toc":"bytecode/Demo.toc"}]}`)
	if _, err := EncodePackage(manifest, map[string][]byte{
		"bytecode/Demo.toc": mustCompileTestArtifact(t),
	}); err == nil {
		t.Fatalf("expected missing contract name error")
	}
}

func TestEncodePackageRejectsManifestContractMissingArtifactAndInterface(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo"}]}`)
	if _, err := EncodePackage(manifest, map[string][]byte{
		"bytecode/Demo.toc": mustCompileTestArtifact(t),
	}); err == nil {
		t.Fatalf("expected missing artifact/interface reference error")
	}
}

func TestEncodePackageRejectsDuplicateManifestContractNames(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"},{"name":"Demo","abi":"interfaces/IDemo.abi"}]}`)
	if _, err := EncodePackage(manifest, map[string][]byte{
		"bytecode/Demo.toc":    mustCompileTestArtifact(t),
		"interfaces/IDemo.abi": mustCompileTestInterface(t),
	}); err == nil {
		t.Fatalf("expected duplicate contract name error")
	}
}

func TestEncodePackageRejectsMissingManifestReferencedInterface(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","abi":"interfaces/IDemo.abi"}]}`)
	if _, err := EncodePackage(manifest, map[string][]byte{
		"bytecode/Demo.toc": mustCompileTestArtifact(t),
	}); err == nil {
		t.Fatalf("expected missing manifest referenced interface error")
	}
}

func TestDecodePackageRejectsMissingManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("bytecode/Demo.toc")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodePackage(buf.Bytes()); err == nil {
		t.Fatalf("expected missing manifest error")
	}
}

func TestDecodePackageRejectsInvalidManifestJSON(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte("{")); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodePackage(buf.Bytes()); err == nil {
		t.Fatalf("expected invalid manifest json error")
	}
}

func TestDecodePackageRejectsManifestMissingRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte(`{"name":"demo"}`)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodePackage(buf.Bytes()); err == nil {
		t.Fatalf("expected missing version error")
	}
}

func TestDecodePackageRejectsManifestReferencedMissingFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodePackage(buf.Bytes()); err == nil {
		t.Fatalf("expected manifest referenced missing file error")
	}
}

func TestDecodePackageRejectsManifestContractMissingArtifactAndInterface(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo"}]}`)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodePackage(buf.Bytes()); err == nil {
		t.Fatalf("expected missing artifact/interface reference error")
	}
}

func TestDecodePackageRejectsDuplicateManifestContractNames(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"},{"name":"Demo","abi":"interfaces/IDemo.abi"}]}`)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	tw, err := zw.Create("bytecode/Demo.toc")
	if err != nil {
		t.Fatalf("create artifact entry: %v", err)
	}
	if _, err := tw.Write(mustCompileTestArtifact(t)); err != nil {
		t.Fatalf("write artifact entry: %v", err)
	}
	iw, err := zw.Create("interfaces/IDemo.abi")
	if err != nil {
		t.Fatalf("create interface entry: %v", err)
	}
	if _, err := iw.Write(mustCompileTestInterface(t)); err != nil {
		t.Fatalf("write interface entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodePackage(buf.Bytes()); err == nil {
		t.Fatalf("expected duplicate contract name error")
	}
}

func TestDecodePackageRejectsInvalidArtifactEntry(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	pkg, err := EncodePackage(manifest, map[string][]byte{
		"bytecode/Demo.toc": []byte("not-a-toc"),
	})
	if err != nil {
		t.Fatalf("encode package: %v", err)
	}
	if _, err := DecodePackage(pkg); err == nil {
		t.Fatalf("expected invalid artifact entry error")
	}
}

func TestDecodePackageRejectsInvalidInterfaceEntry(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","abi":"interfaces/IDemo.abi"}]}`)
	pkg, err := EncodePackage(manifest, map[string][]byte{
		"interfaces/IDemo.abi": []byte("not-a-abi"),
	})
	if err != nil {
		t.Fatalf("encode package: %v", err)
	}
	if _, err := DecodePackage(pkg); err == nil {
		t.Fatalf("expected invalid interface entry error")
	}
}

func TestPackageHashStable(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0"}`)
	files := map[string][]byte{"bytecode/Demo.toc": []byte("x")}
	pkgA, err := EncodePackage(manifest, files)
	if err != nil {
		t.Fatalf("encode pkgA: %v", err)
	}
	pkgB, err := EncodePackage(manifest, files)
	if err != nil {
		t.Fatalf("encode pkgB: %v", err)
	}
	if PackageHash(pkgA) != PackageHash(pkgB) {
		t.Fatalf("expected stable package hash")
	}
}

func TestCompilePackageRoundTrip(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	pkg, err := CompilePackage(src, "demo.tol", &PackageOptions{
		PackageName:    "demo",
		PackageVersion: "1.0.0",
		IncludeSource:  true,
	})
	if err != nil {
		t.Fatalf("compile package: %v", err)
	}
	decoded, err := DecodePackage(pkg)
	if err != nil {
		t.Fatalf("decode package: %v", err)
	}
	var manifest struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Contracts []struct {
			Name      string `json:"name"`
			Artifact  string `json:"toc"`
			Interface string `json:"abi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(decoded.ManifestJSON, &manifest); err != nil {
		t.Fatalf("manifest decode: %v", err)
	}
	if manifest.Name != "demo" || manifest.Version != "1.0.0" {
		t.Fatalf("unexpected manifest identity: %+v", manifest)
	}
	if len(manifest.Contracts) != 1 {
		t.Fatalf("unexpected contract entries: %d", len(manifest.Contracts))
	}
	ref := manifest.Contracts[0]
	if _, ok := decoded.Files[ref.Artifact]; !ok {
		t.Fatalf("missing referenced artifact file: %s", ref.Artifact)
	}
	if _, ok := decoded.Files[ref.Interface]; !ok {
		t.Fatalf("missing referenced interface file: %s", ref.Interface)
	}
	if got := string(decoded.Files["sources/demo.tol"]); got == "" {
		t.Fatalf("expected included source file")
	}
}

func TestCompilePackageDeterministic(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	opts := &PackageOptions{
		PackageName:    "demo",
		PackageVersion: "1.0.0",
		IncludeSource:  true,
	}
	a, err := CompilePackage(src, "demo.tol", opts)
	if err != nil {
		t.Fatalf("compile package a: %v", err)
	}
	b, err := CompilePackage(src, "demo.tol", opts)
	if err != nil {
		t.Fatalf("compile package b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic package output")
	}
}

func TestCompilePackageMultiContractDefaultIsPathIndependent(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Alpha {
  function ping() public { return; }
}
contract Beta {
  function pong() public { return; }
}
`)
	a, err := CompilePackage(src, "alpha.tol", nil)
	if err != nil {
		t.Fatalf("compile package a: %v", err)
	}
	b, err := CompilePackage(src, "beta.tol", nil)
	if err != nil {
		t.Fatalf("compile package b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected multi-contract default package output to be path-independent")
	}

	decoded, err := DecodePackage(a)
	if err != nil {
		t.Fatalf("decode package: %v", err)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(decoded.ManifestJSON, &manifest); err != nil {
		t.Fatalf("manifest decode: %v", err)
	}
	if manifest.Name != "alpha" {
		t.Fatalf("expected stable default package name alpha, got %q", manifest.Name)
	}
}

func TestCompilePackageCustomPaths(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	pkg, err := CompilePackage(src, "demo.tol", &PackageOptions{
		PackageName:    "demo",
		PackageVersion: "1.2.3",
		ArtifactPath:   "artifacts/Demo.toc",
		InterfacePath:  "abi/IDemo.abi",
		InterfaceName:  "DemoIface",
		IncludeSource:  true,
		SourcePath:     "src/demo.tol",
	})
	if err != nil {
		t.Fatalf("compile package: %v", err)
	}
	decoded, err := DecodePackage(pkg)
	if err != nil {
		t.Fatalf("decode package: %v", err)
	}
	if _, ok := decoded.Files["artifacts/Demo.toc"]; !ok {
		t.Fatalf("missing custom artifact path")
	}
	if _, ok := decoded.Files["abi/IDemo.abi"]; !ok {
		t.Fatalf("missing custom interface path")
	}
	if !strings.Contains(string(decoded.Files["abi/IDemo.abi"]), "interface DemoIface {") {
		t.Fatalf("expected custom interface name")
	}
	if _, ok := decoded.Files["src/demo.tol"]; !ok {
		t.Fatalf("missing custom source path")
	}
}

func TestCompilePackageDefaultNoSource(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	pkg, err := CompilePackage(src, "demo.tol", nil)
	if err != nil {
		t.Fatalf("compile package: %v", err)
	}
	decoded, err := DecodePackage(pkg)
	if err != nil {
		t.Fatalf("decode package: %v", err)
	}
	for name := range decoded.Files {
		if strings.HasPrefix(name, "sources/") {
			t.Fatalf("unexpected source entry in default mode: %s", name)
		}
	}
}
