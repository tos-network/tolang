package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/tos-network/tolang"
)

func TestDefaultArtifactPath(t *testing.T) {
	input := "/tmp/contract.tol"
	if got, want := defaultArtifactPath(input, "toc"), "/tmp/contract.toc"; got != want {
		t.Fatalf("defaultArtifactPath toc: got=%q want=%q", got, want)
	}
	if got, want := defaultArtifactPath(input, "abi"), "/tmp/contract.abi"; got != want {
		t.Fatalf("defaultArtifactPath abi: got=%q want=%q", got, want)
	}
	if got, want := defaultArtifactPath(input, "tor"), "/tmp/contract.tor"; got != want {
		t.Fatalf("defaultArtifactPath tor: got=%q want=%q", got, want)
	}
}

func TestDispatchSubcommandRouting(t *testing.T) {
	if handled, _ := dispatchSubcommand(nil); handled {
		t.Fatalf("empty args should not be handled by subcommand dispatcher")
	}
	if handled, _ := dispatchSubcommand([]string{"unknown"}); handled {
		t.Fatalf("unknown subcommand should fall back to lua script/flag handler")
	}
	if handled, code := dispatchSubcommand([]string{"--help"}); !handled || code != 0 {
		t.Fatalf("--help should be handled with code 0, got handled=%v code=%d", handled, code)
	}
	if handled, code := dispatchSubcommand([]string{"--version"}); !handled || code != 0 {
		t.Fatalf("--version should be handled with code 0, got handled=%v code=%d", handled, code)
	}
	if handled, code := dispatchSubcommand([]string{"version"}); !handled || code != 0 {
		t.Fatalf("version should be handled with code 0, got handled=%v code=%d", handled, code)
	}
	if handled, code := dispatchSubcommand([]string{"help"}); !handled || code != 0 {
		t.Fatalf("help should be handled with code 0, got handled=%v code=%d", handled, code)
	}
	if handled, code := dispatchSubcommand([]string{"help", "compile"}); !handled || code != 0 {
		t.Fatalf("help compile should be handled with code 0, got handled=%v code=%d", handled, code)
	}
	if handled, code := dispatchSubcommand([]string{"help", "nope"}); !handled || code != 1 {
		t.Fatalf("help unknown should be handled with code 1, got handled=%v code=%d", handled, code)
	}
}

func TestSubcommandHelpExitCodes(t *testing.T) {
	if code := cmdCompile([]string{"--help"}); code != 0 {
		t.Fatalf("compile --help: got=%d want=0", code)
	}
	if code := cmdPack([]string{"--help"}); code != 0 {
		t.Fatalf("pack --help: got=%d want=0", code)
	}
	if code := cmdInspect([]string{"--help"}); code != 0 {
		t.Fatalf("inspect --help: got=%d want=0", code)
	}
	if code := cmdVerify([]string{"--help"}); code != 0 {
		t.Fatalf("verify --help: got=%d want=0", code)
	}
}

func TestCompileHelpIncludesNameOverrideDescription(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	code := cmdCompile([]string{"--help"})
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	_ = r.Close()

	if code != 0 {
		t.Fatalf("compile --help: got=%d want=0", code)
	}
	txt := string(out)
	if !strings.Contains(txt, "interface name for emit=abi") || !strings.Contains(txt, "abi interface name for emit=tor") {
		t.Fatalf("compile help missing expected --name description, got:\n%s", txt)
	}
}

func TestDetectArtifactKindMagicFallback(t *testing.T) {
	src := []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n")
	artifactBytes, err := lua.CompileArtifact(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile artifact: %v", err)
	}
	if got := detectArtifactKind("artifact.bin", artifactBytes); got != kindArtifact {
		t.Fatalf("detect artifact by magic: got=%v want=%v", got, kindArtifact)
	}

	ifaceBytes, err := lua.CompileInterface(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile interface: %v", err)
	}
	if got := detectArtifactKind("artifact.bin", ifaceBytes); got != kindInterface {
		t.Fatalf("detect interface by text validation: got=%v want=%v", got, kindInterface)
	}
}

func TestCmdCompileDefaultArtifactOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if code := cmdCompile([]string{input}); code != 0 {
		t.Fatalf("cmdCompile exit code: got=%d want=0", code)
	}

	out := filepath.Join(dir, "sample.toc")
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output artifact: %v", err)
	}
	if _, err := lua.DecodeArtifact(body); err != nil {
		t.Fatalf("decode output artifact: %v", err)
	}
}

func TestCmdCompileArtifactDefaultDisablesSourceMap(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out := filepath.Join(dir, "sample.toc")
	if code := cmdCompile([]string{"-o", out, input}); code != 0 {
		t.Fatalf("cmdCompile exit code: got=%d want=0", code)
	}
	artifactBody, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output artifact: %v", err)
	}
	art, err := lua.DecodeArtifact(artifactBody)
	if err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	proto, err := lua.DecodeFunctionProto(art.Bytecode)
	if err != nil {
		t.Fatalf("decode embedded bytecode: %v", err)
	}
	if len(proto.DbgSourcePositions) != 0 {
		t.Fatalf("expected no sourcemap by default, got %d dbg entries", len(proto.DbgSourcePositions))
	}
}

func TestCmdCompileArtifactSourceMapFlagEnablesSourceMap(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out := filepath.Join(dir, "sample.toc")
	if code := cmdCompile([]string{"--sourcemap", "-o", out, input}); code != 0 {
		t.Fatalf("cmdCompile --sourcemap exit code: got=%d want=0", code)
	}
	artifactBody, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output artifact: %v", err)
	}
	art, err := lua.DecodeArtifact(artifactBody)
	if err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	proto, err := lua.DecodeFunctionProto(art.Bytecode)
	if err != nil {
		t.Fatalf("decode embedded bytecode: %v", err)
	}
	if len(proto.DbgSourcePositions) == 0 {
		t.Fatalf("expected sourcemap entries with --sourcemap")
	}
}

func TestCmdCompileDefaultInterfaceAndPackageOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if code := cmdCompile([]string{"--emit", "abi", input}); code != 0 {
		t.Fatalf("cmdCompile abi exit code: got=%d want=0", code)
	}
	ifacePath := filepath.Join(dir, "sample.abi")
	abiBody, err := os.ReadFile(ifacePath)
	if err != nil {
		t.Fatalf("read output abi: %v", err)
	}
	if err := lua.ValidateInterface(abiBody); err != nil {
		t.Fatalf("validate output abi: %v", err)
	}

	if code := cmdCompile([]string{"--emit", "tor", input}); code != 0 {
		t.Fatalf("cmdCompile tor exit code: got=%d want=0", code)
	}
	torPath := filepath.Join(dir, "sample.tor")
	torBody, err := os.ReadFile(torPath)
	if err != nil {
		t.Fatalf("read output tor: %v", err)
	}
	if _, err := lua.DecodePackage(torBody); err != nil {
		t.Fatalf("decode output tor: %v", err)
	}
}

func TestCmdVerifyArtifactSourceMismatchExitCode(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(srcPath, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	artifactPath := filepath.Join(dir, "sample.toc")
	if code := cmdCompile([]string{"-o", artifactPath, srcPath}); code != 0 {
		t.Fatalf("compile artifact exit code: got=%d want=0", code)
	}

	mismatchPath := filepath.Join(dir, "mismatch.tol")
	if err := os.WriteFile(mismatchPath, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function pong() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write mismatch source: %v", err)
	}

	if code := cmdVerify([]string{"--source", mismatchPath, artifactPath}); code != 2 {
		t.Fatalf("verify mismatch exit code: got=%d want=2", code)
	}
}

func TestCmdPackDirectory(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(filepath.Join(pkgDir, "bytecode"), 0o755); err != nil {
		t.Fatalf("mkdir bytecode: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "interfaces"), 0o755); err != nil {
		t.Fatalf("mkdir interfaces: %v", err)
	}

	src := []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n")
	artifactBytes, err := lua.CompileArtifact(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile artifact: %v", err)
	}
	ifaceBytes, err := lua.CompileInterface(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile interface: %v", err)
	}

	artifactPath := filepath.Join(pkgDir, "bytecode", "Sample.toc")
	ifacePath := filepath.Join(pkgDir, "interfaces", "ISample.abi")
	if err := os.WriteFile(artifactPath, artifactBytes, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(ifacePath, ifaceBytes, 0o644); err != nil {
		t.Fatalf("write interface: %v", err)
	}

	manifest := []byte(`{
  "name": "sample-pack",
  "version": "1.0.0",
  "contracts": [
    {"name":"Sample","toc":"bytecode/Sample.toc","abi":"interfaces/ISample.abi"}
  ]
}`)
	if err := os.WriteFile(filepath.Join(pkgDir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	out := filepath.Join(dir, "out.tor")
	if code := cmdPack([]string{"-o", out, pkgDir}); code != 0 {
		t.Fatalf("cmdPack exit code: got=%d want=0", code)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read packed tor: %v", err)
	}
	if _, err := lua.DecodePackage(body); err != nil {
		t.Fatalf("decode packed tor: %v", err)
	}
}

func TestCmdCompileArtifactWithABISidecar(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	out := filepath.Join(dir, "out.toc")
	if code := cmdCompile([]string{"--abi", "-o", out, input}); code != 0 {
		t.Fatalf("compile with --abi exit code: got=%d want=0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("toc output missing: %v", err)
	}
	abiPath := filepath.Join(dir, "out.abi.json")
	abi, err := os.ReadFile(abiPath)
	if err != nil {
		t.Fatalf("abi sidecar missing: %v", err)
	}
	if !json.Valid(abi) {
		t.Fatalf("abi sidecar must be valid json: %s", string(abi))
	}
}

func TestCmdCompileInterfaceNameOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	out := filepath.Join(dir, "iface.abi")
	if code := cmdCompile([]string{"--emit", "abi", "--name", "ISampleX", "-o", out, input}); code != 0 {
		t.Fatalf("compile abi exit code: got=%d want=0", code)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read abi output: %v", err)
	}
	info, err := lua.InspectInterface(body)
	if err != nil {
		t.Fatalf("inspect abi: %v", err)
	}
	if info.InterfaceName != "ISampleX" {
		t.Fatalf("abi name override: got=%q want=%q", info.InterfaceName, "ISampleX")
	}
}

func TestCmdCompilePackageDefaultsAndNameOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "my_contract.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	out := filepath.Join(dir, "out.tor")
	if code := cmdCompile([]string{"--emit", "tor", "--name", "ISampleZ", "-o", out, input}); code != 0 {
		t.Fatalf("compile tor exit code: got=%d want=0", code)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read tor output: %v", err)
	}
	tor, err := lua.DecodePackage(body)
	if err != nil {
		t.Fatalf("decode tor: %v", err)
	}

	var manifest struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Contracts []struct {
			Name string `json:"name"`
			Interface string `json:"abi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(tor.ManifestJSON, &manifest); err != nil {
		t.Fatalf("decode manifest json: %v", err)
	}
	if manifest.Name != "my_contract" {
		t.Fatalf("default tor package name: got=%q want=%q", manifest.Name, "my_contract")
	}
	if manifest.Version != "0.0.0" {
		t.Fatalf("default tor package version: got=%q want=%q", manifest.Version, "0.0.0")
	}
	if len(manifest.Contracts) != 1 {
		t.Fatalf("manifest contracts len: got=%d want=1", len(manifest.Contracts))
	}
	ifacePath := manifest.Contracts[0].Interface
	abiBody, ok := tor.Files[ifacePath]
	if !ok {
		t.Fatalf("manifest abi path %q missing from archive files", ifacePath)
	}
	info, err := lua.InspectInterface(abiBody)
	if err != nil {
		t.Fatalf("inspect tor abi: %v", err)
	}
	if info.InterfaceName != "ISampleZ" {
		t.Fatalf("tor abi name override: got=%q want=%q", info.InterfaceName, "ISampleZ")
	}
}

func TestCmdInspectAndVerifyWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	src := []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	artBytes, err := lua.CompileArtifact(src, srcPath)
	if err != nil {
		t.Fatalf("compile artifact: %v", err)
	}
	ifaceBytes, err := lua.CompileInterface(src, srcPath)
	if err != nil {
		t.Fatalf("compile interface: %v", err)
	}
	pkgBytes, err := lua.CompilePackage(src, srcPath, &lua.PackageOptions{})
	if err != nil {
		t.Fatalf("compile package: %v", err)
	}

	artBinPath := filepath.Join(dir, "artifact.bin")
	ifaceBinPath := filepath.Join(dir, "interface.bin")
	pkgBinPath := filepath.Join(dir, "package.bin")
	if err := os.WriteFile(artBinPath, artBytes, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(ifaceBinPath, ifaceBytes, 0o644); err != nil {
		t.Fatalf("write interface: %v", err)
	}
	if err := os.WriteFile(pkgBinPath, pkgBytes, 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}

	if code := cmdInspect([]string{artBinPath}); code != 0 {
		t.Fatalf("inspect artifact without extension: got=%d want=0", code)
	}
	if code := cmdInspect([]string{ifaceBinPath}); code != 0 {
		t.Fatalf("inspect interface without extension: got=%d want=0", code)
	}
	if code := cmdInspect([]string{pkgBinPath}); code != 0 {
		t.Fatalf("inspect package without extension: got=%d want=0", code)
	}
	if code := cmdVerify([]string{artBinPath}); code != 0 {
		t.Fatalf("verify artifact without extension: got=%d want=0", code)
	}
	if code := cmdVerify([]string{ifaceBinPath}); code != 0 {
		t.Fatalf("verify interface without extension: got=%d want=0", code)
	}
	if code := cmdVerify([]string{pkgBinPath}); code != 0 {
		t.Fatalf("verify package without extension: got=%d want=0", code)
	}
}

func TestCmdVerifySourceFlagRejectedForNonArtifact(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n")
	srcPath := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	ifaceBytes, err := lua.CompileInterface(src, srcPath)
	if err != nil {
		t.Fatalf("compile abi: %v", err)
	}
	ifacePath := filepath.Join(dir, "sample.abi")
	if err := os.WriteFile(ifacePath, ifaceBytes, 0o644); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if code := cmdVerify([]string{"--source", srcPath, ifacePath}); code != 1 {
		t.Fatalf("verify abi with --source: got=%d want=1", code)
	}

	tor, err := lua.CompilePackage(src, srcPath, &lua.PackageOptions{})
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}
	torPath := filepath.Join(dir, "sample.tor")
	if err := os.WriteFile(torPath, tor, 0o644); err != nil {
		t.Fatalf("write tor: %v", err)
	}
	if code := cmdVerify([]string{"--source", srcPath, torPath}); code != 1 {
		t.Fatalf("verify tor with --source: got=%d want=1", code)
	}
}

func TestCmdCompileRejectsInvalidFlagCombinations(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("pragma tolang 0.2.0;\n\ncontract Sample {\n  function ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if code := cmdCompile([]string{"--emit", "abi", "--abi", input}); code != 1 {
		t.Fatalf("--abi flag with emit=abi: got=%d want=1", code)
	}
	if code := cmdCompile([]string{"--emit", "toc", "--package-name", "x", input}); code != 1 {
		t.Fatalf("--package-name with emit=toc: got=%d want=1", code)
	}
	if code := cmdCompile([]string{"--emit", "toc", "--include-source", input}); code != 1 {
		t.Fatalf("--include-source with emit=toc: got=%d want=1", code)
	}
}

func TestCmdPackRejectsMissingOutputOrBadInput(t *testing.T) {
	dir := t.TempDir()
	if code := cmdPack([]string{dir}); code != 1 {
		t.Fatalf("pack missing -o should fail: got=%d want=1", code)
	}

	file := filepath.Join(dir, "not_dir.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if code := cmdPack([]string{"-o", filepath.Join(dir, "x.tor"), file}); code != 1 {
		t.Fatalf("pack non-directory input should fail: got=%d want=1", code)
	}
}

func TestCmdInspectRejectsUnknownArtifact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(path, []byte("not-an-artifact"), 0o644); err != nil {
		t.Fatalf("write bad artifact: %v", err)
	}
	if code := cmdInspect([]string{path}); code != 1 {
		t.Fatalf("inspect unknown artifact should fail: got=%d want=1", code)
	}
}

func TestCmdVerifyRejectsUnknownArtifact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(path, []byte("not-an-artifact"), 0o644); err != nil {
		t.Fatalf("write bad artifact: %v", err)
	}
	if code := cmdVerify([]string{path}); code != 1 {
		t.Fatalf("verify unknown artifact should fail: got=%d want=1", code)
	}
}

// ── tol test subcommand ───────────────────────────────────────────────────────

func TestCmdTestHelp(t *testing.T) {
	if code := cmdTest([]string{"--help"}); code != 0 {
		t.Fatalf("tol test --help: got=%d want=0", code)
	}
}

func TestCmdTestPassingTests(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_basic() {\n    assert_eq(1, 1);\n  }\n}\n")
	path := filepath.Join(dir, "basic_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if code := cmdTest([]string{path}); code != 0 {
		t.Fatalf("passing test should exit 0: got=%d", code)
	}
}

func TestCmdTestFailingTestExitsOne(t *testing.T) {
	dir := t.TempDir()
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_fail() {\n    assert_eq(1, 2);\n  }\n}\n")
	path := filepath.Join(dir, "fail_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if code := cmdTest([]string{path}); code != 1 {
		t.Fatalf("failing test should exit 1: got=%d", code)
	}
}

func TestCmdTestCompileErrorExitsTwo(t *testing.T) {
	dir := t.TempDir()
	src := []byte("this is not valid tol source !@#$\n")
	path := filepath.Join(dir, "bad_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if code := cmdTest([]string{path}); code != 2 {
		t.Fatalf("compile error should exit 2: got=%d", code)
	}
}

func TestCmdTestRunFilter(t *testing.T) {
	dir := t.TempDir()
	// Two tests; only test_alpha should run with -run alpha.
	// test_beta asserts false; if it runs, the exit code would be 1.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_alpha() {\n    assert_eq(1, 1);\n  }\n  function test_beta() {\n    assert_eq(1, 2);\n  }\n}\n")
	path := filepath.Join(dir, "filter_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if code := cmdTest([]string{"-run", "alpha", path}); code != 0 {
		t.Fatalf("-run alpha should only run test_alpha and pass: got=%d", code)
	}
}

func TestCmdTestSkipTagFilter(t *testing.T) {
	dir := t.TempDir()
	// test_slow is tagged "slow" and asserts false; with -skip slow it should be excluded.
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_fast() {\n    assert_eq(1, 1);\n  }\n  @tag(\"slow\")\n  function test_slow() {\n    assert_eq(1, 2);\n  }\n}\n")
	path := filepath.Join(dir, "skip_tag_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if code := cmdTest([]string{"-skip", "slow", path}); code != 0 {
		t.Fatalf("-skip slow should exclude tagged test: got=%d", code)
	}
}

func TestCmdTestDirectorySearch(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	src := []byte("pragma tolang 0.2.0;\ntest Suite {\n  function test_ok() {\n    assert_eq(1, 1);\n  }\n}\n")
	path := filepath.Join(sub, "ok_test.tol")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if code := cmdTest([]string{dir}); code != 0 {
		t.Fatalf("directory search should find and pass tests: got=%d", code)
	}
}

func TestCmdTestDispatchedViaSubcommand(t *testing.T) {
	if handled, _ := dispatchSubcommand([]string{"test", "--help"}); !handled {
		t.Fatalf("test subcommand should be handled by dispatcher")
	}
}
