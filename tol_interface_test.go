package lua

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompileInterfaceExportsPublicSurface(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  event Tick(u256 v indexed);

  function hidden(u256 x) internal {
    return;
  }

  @selector("0x12345678")
  function ping(address owner, u256 amount) public view returns (bool ok) {
    return;
  }

  function poke() external {
    return;
  }
}
`)
	ifaceData, err := CompileInterface(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected ifaceData compile error: %v", err)
	}
	out := string(ifaceData)
	if !strings.Contains(out, "interface IDemo") {
		t.Fatalf("expected interface header, got:\n%s", out)
	}
	if strings.Contains(out, "function hidden(") {
		t.Fatalf("internal function should not be exported, got:\n%s", out)
	}
	if !strings.Contains(out, `@selector("0x12345678")`) {
		t.Fatalf("expected selector override in interface, got:\n%s", out)
	}
	if !strings.Contains(out, "function ping(address owner, u256 amount) public view returns (bool ok);") {
		t.Fatalf("expected exported ping signature, got:\n%s", out)
	}
	if !strings.Contains(out, "function poke() external;") {
		t.Fatalf("expected exported poke signature, got:\n%s", out)
	}
	if !strings.Contains(out, "event Tick(u256 v indexed);") {
		t.Fatalf("expected event signature, got:\n%s", out)
	}
}

func TestCompileInterfaceDeterministic(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	a, err := CompileInterface(src, "<tol>")
	if err != nil {
		t.Fatalf("compile a: %v", err)
	}
	b, err := CompileInterface(src, "<tol>")
	if err != nil {
		t.Fatalf("compile b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic ifaceData output")
	}
}

func TestBuildInterfaceFromModuleRejectsNil(t *testing.T) {
	if _, err := BuildInterface(nil); err == nil {
		t.Fatalf("expected nil module error")
	}
}

func TestCompileInterfaceWithCustomInterfaceName(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	ifaceData, err := CompileInterfaceWithOptions(src, "<tol>", &InterfaceOptions{
		InterfaceName: "DemoSurface",
	})
	if err != nil {
		t.Fatalf("unexpected ifaceData compile error: %v", err)
	}
	if !strings.Contains(string(ifaceData), "interface DemoSurface {") {
		t.Fatalf("expected custom interface name, got:\n%s", string(ifaceData))
	}
}

func TestValidateInterfaceAcceptsGenerated(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function ping() public { return; }
}
`)
	ifaceData, err := CompileInterface(src, "<tol>")
	if err != nil {
		t.Fatalf("compile ifaceData: %v", err)
	}
	if err := ValidateInterface(ifaceData); err != nil {
		t.Fatalf("expected valid ifaceData text, got: %v", err)
	}
}

func TestValidateInterfaceRejectsMalformed(t *testing.T) {
	if err := ValidateInterface([]byte("not ifaceData")); err == nil {
		t.Fatalf("expected malformed ifaceData error")
	}
}

func TestValidateInterfaceRejectsMissingSemicolon(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  function ping() public
}
`)
	if err := ValidateInterface(ifaceData); err == nil {
		t.Fatalf("expected missing semicolon error")
	}
}

func TestValidateInterfaceRejectsDanglingSelector(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  @selector("0x12345678")
  event Tick(u256 v);
}
`)
	if err := ValidateInterface(ifaceData); err == nil {
		t.Fatalf("expected dangling selector error")
	}
}

func TestValidateInterfaceRejectsSelectorNonStringLiteral(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  @selector(123)
  function ping() public;
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected selector literal error")
	}
	if !strings.Contains(err.Error(), "string literal value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInterfaceRejectsSelectorInvalidHex(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  @selector("0x123")
  function ping() public;
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected selector hex format error")
	}
	if !strings.Contains(err.Error(), "0x followed by 8 hex chars") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInterfaceRejectsDuplicateSelectorAnnotations(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  @selector("0x12345678")
  @selector("0x87654321")
  function ping() public;
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected duplicate selector annotation error")
	}
	if !strings.Contains(err.Error(), "must be followed by function declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInterfaceRejectsMultipleInterfaces(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface IA {
  function a() public;
}

interface IB {
  function b() public;
}
`)
	if err := ValidateInterface(ifaceData); err == nil {
		t.Fatalf("expected multiple interface error")
	}
}

func TestValidateInterfaceRejectsDuplicateFunctionDeclaration(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  function ping() public;
  function ping() external;
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected duplicate function declaration error")
	}
	if !strings.Contains(err.Error(), "duplicate function declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInterfaceRejectsDuplicateEventDeclaration(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  event Tick(u256 v);
  event Tick(u256 v);
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected duplicate event declaration error")
	}
	if !strings.Contains(err.Error(), "duplicate event declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInterfaceRejectsFunctionEventNameCollision(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  function Tick() public;
  event Tick(u256 v);
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected function/event name collision error")
	}
	if !strings.Contains(err.Error(), "name collision") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInterfaceAcceptsComments(t *testing.T) {
	ifaceData := []byte(`
-- file comment
pragma tolang 0.2.0;

interface ISample { -- inline
  function ping() public; -- fn
  event Tick(v: u256); -- event
}
`)
	if err := ValidateInterface(ifaceData); err != nil {
		t.Fatalf("expected comments to be accepted: %v", err)
	}
}

func TestValidateInterfaceAcceptsEventThreeIndexed(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  event Transfer(address a indexed, address b indexed, u256 c indexed, u256 d);
}
`)
	if err := ValidateInterface(ifaceData); err != nil {
		t.Fatalf("expected ifaceData with 3 indexed fields to be accepted: %v", err)
	}
}

func TestValidateInterfaceRejectsEventMoreThanThreeIndexed(t *testing.T) {
	ifaceData := []byte(`
pragma tolang 0.2.0;

interface ISample {
  event TooMany(address a indexed, address b indexed, u256 c indexed, u256 d indexed);
}
`)
	err := ValidateInterface(ifaceData)
	if err == nil {
		t.Fatalf("expected indexed field cap error")
	}
	if !strings.Contains(err.Error(), "at most 3 are allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInspectInterface(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  event Tick(u256 v);
  function ping() public { return; }
}
`)
	ifaceData, err := CompileInterface(src, "<tol>")
	if err != nil {
		t.Fatalf("compile ifaceData: %v", err)
	}
	info, err := InspectInterface(ifaceData)
	if err != nil {
		t.Fatalf("inspect ifaceData: %v", err)
	}
	if info.Version != "0.2.0" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if info.InterfaceName != "IDemo" {
		t.Fatalf("unexpected interface: %s", info.InterfaceName)
	}
	if info.FunctionCount != 1 || info.EventCount != 1 {
		t.Fatalf("unexpected counts: %+v", info)
	}
}
