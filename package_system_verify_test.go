package lua

import (
    "os"
    "strings"
    "testing"
    
    "github.com/tos-network/tolang/tol/lower"
    "github.com/tos-network/tolang/tol/parser"
    "github.com/tos-network/tolang/tol/sema"
)

func TestPackageImportParsing(t *testing.T) {
    src := `
pragma tolang 0.2.0;
package tos.registry;
contract Registry {
    function get(bytes32 k) external returns (agent r) { return agent(0); }
}
`
    mod, diags := parser.ParseFile("registry.tol", []byte(src))
    if diags.HasErrors() {
        t.Fatal("parse error:", diags)
    }
    if mod.Package != "tos.registry" {
        t.Errorf("expected package=tos.registry, got %q", mod.Package)
    }
}

func TestPackageImportDecl(t *testing.T) {
    src := `
pragma tolang 0.2.0;
import tos.registry.AgentRegistry;
contract Caller {
    function resolve(agent pkgAddr, bytes32 name) external returns (agent r) {
        let reg: AgentRegistry = AgentRegistry(pkgAddr);
        return reg.lookup(name);
    }
}
`
    mod, diags := parser.ParseFile("caller.tol", []byte(src))
    if diags.HasErrors() {
        t.Fatal("parse error:", diags)
    }
    if len(mod.Imports) == 0 {
        t.Fatal("expected at least one import")
    }
    imp := mod.Imports[0]
    if !imp.IsPackageImport {
        t.Error("expected IsPackageImport=true")
    }
    if imp.PackagePath != "tos.registry" {
        t.Errorf("expected PackagePath=tos.registry, got %q", imp.PackagePath)
    }
    if imp.PackageContract != "AgentRegistry" {
        t.Errorf("expected PackageContract=AgentRegistry, got %q", imp.PackageContract)
    }
    if imp.Name != "AgentRegistry" {
        t.Errorf("expected Name=AgentRegistry, got %q", imp.Name)
    }
}

func TestPackageImportWithAlias(t *testing.T) {
    src := `
pragma tolang 0.2.0;
import tos.registry.AgentRegistry as IRegistry;
contract Caller {
    function foo(agent a) external returns (agent r) { return agent(0); }
}
`
    mod, diags := parser.ParseFile("caller.tol", []byte(src))
    if diags.HasErrors() {
        t.Fatal("parse error:", diags)
    }
    imp := mod.Imports[0]
    if !imp.IsPackageImport {
        t.Error("expected IsPackageImport=true")
    }
    if imp.Name != "IRegistry" {
        t.Errorf("expected Name=IRegistry (alias), got %q", imp.Name)
    }
    if imp.PackageContract != "AgentRegistry" {
        t.Errorf("expected PackageContract=AgentRegistry, got %q", imp.PackageContract)
    }
}

func TestPackageImportSemaResolution(t *testing.T) {
    modSrc := `pragma tolang 0.2.0;
package tos.registry;
contract AgentRegistry {
  function lookup(bytes32 name) external returns (agent addr) { return agent(0); }
}
`
    src := `pragma tolang 0.2.0;
import tos.registry.AgentRegistry;
contract Caller {
  function resolve(agent pkgAddr, bytes32 name) external returns (agent r) {
    let reg: AgentRegistry = AgentRegistry(pkgAddr);
    return reg.lookup(name);
  }
}
`
    os.MkdirAll("/tmp/toltest_pkg/tos/registry", 0755)
    os.WriteFile("/tmp/toltest_pkg/tos/registry/AgentRegistry.tol", []byte(modSrc), 0644)
    
    resolver := NewOSFileResolver("/tmp/toltest_pkg")
    mod, diags := parser.ParseFile("/tmp/toltest_pkg/Caller.tol", []byte(src))
    if diags.HasErrors() {
        t.Fatal("parse error:", diags)
    }
    
    typed, diags := sema.CheckWithResolver("/tmp/toltest_pkg/Caller.tol", mod, resolver)
    if diags.HasErrors() {
        t.Fatal("sema error:", diags)
    }
    
    // Verify interface has package info
    found := false
    for _, iface := range typed.AST.Interfaces {
        if iface.Name == "AgentRegistry" {
            if iface.PackageName != "tos.registry" {
                t.Errorf("expected PackageName=tos.registry, got %q", iface.PackageName)
            }
            if iface.ContractName != "AgentRegistry" {
                t.Errorf("expected ContractName=AgentRegistry, got %q", iface.ContractName)
            }
            found = true
        }
    }
    if !found {
        t.Error("AgentRegistry interface not found in module after sema")
    }
}

func TestPackageImportLoweringHasPackageInfo(t *testing.T) {
    modSrc := `pragma tolang 0.2.0;
package tos.registry;
contract AgentRegistry {
  function lookup(bytes32 name) external returns (agent addr) { return agent(0); }
}
`
    src := `pragma tolang 0.2.0;
import tos.registry.AgentRegistry;
contract Caller {
  function resolve(agent pkgAddr, bytes32 name) external returns (agent r) {
    let reg: AgentRegistry = AgentRegistry(pkgAddr);
    return reg.lookup(name);
  }
}
`
    os.MkdirAll("/tmp/toltest_pkg/tos/registry", 0755)
    os.WriteFile("/tmp/toltest_pkg/tos/registry/AgentRegistry.tol", []byte(modSrc), 0644)
    
    resolver := NewOSFileResolver("/tmp/toltest_pkg")
    mod, diags := parser.ParseFile("/tmp/toltest_pkg/Caller.tol", []byte(src))
    if diags.HasErrors() {
        t.Fatal("parse error:", diags)
    }
    typed, diags := sema.CheckWithResolver("/tmp/toltest_pkg/Caller.tol", mod, resolver)
    if diags.HasErrors() {
        t.Fatal("sema error:", diags)
    }
    prog, err := lower.FromTyped(typed)
    if err != nil {
        t.Fatal("lower error:", err)
    }
    
    // Check lowered interface has package info
    found := false
    for _, iface := range prog.Interfaces {
        if iface.Name == "AgentRegistry" {
            if iface.PackageName != "tos.registry" {
                t.Errorf("lowered: expected PackageName=tos.registry, got %q", iface.PackageName)
            }
            if iface.ContractName != "AgentRegistry" {
                t.Errorf("lowered: expected ContractName=AgentRegistry, got %q", iface.ContractName)
            }
            found = true
        }
    }
    if !found {
        t.Error("AgentRegistry not found in lowered interfaces")
    }
    
    // Test full compilation succeeds
    _, err = BuildIRFromLowered(prog, "/tmp/toltest_pkg/Caller.tol")
    if err != nil {
        t.Fatal("IR error:", err)
    }
}

// TestQualifiedConstantAccess verifies Step 9b: ContractName.CONST_NAME inlines the constant.
func TestQualifiedConstantAccess(t *testing.T) {
	// Provider contract with constants and enums
	providerSrc := `pragma tolang 0.2.0;
package tos.registry;
contract AgentRegistry {
  constant MAX_FEE: uint256 = 1000;
  enum Kind { Free, Pro, Enterprise }
  function lookup(bytes32 name) external returns (agent addr) { return agent(0); }
}
`
	// Caller uses qualified constant and enum access
	callerSrc := `pragma tolang 0.2.0;
import tos.registry.AgentRegistry;
contract Caller {
  function getFee() external returns (uint256 f) {
    let fee: uint256 = AgentRegistry.MAX_FEE;
    return fee;
  }
  function getKind() external returns (uint256 k) {
    let kind: uint256 = AgentRegistry.Kind.Pro;
    return kind;
  }
}
`
	os.MkdirAll("/tmp/toltest_qual/tos/registry", 0755)
	os.WriteFile("/tmp/toltest_qual/tos/registry/AgentRegistry.tol", []byte(providerSrc), 0644)

	resolver := NewOSFileResolver("/tmp/toltest_qual")
	mod, diags := parser.ParseFile("/tmp/toltest_qual/Caller.tol", []byte(callerSrc))
	if diags.HasErrors() {
		t.Fatal("parse error:", diags)
	}
	typed, diags := sema.CheckWithResolver("/tmp/toltest_qual/Caller.tol", mod, resolver)
	if diags.HasErrors() {
		t.Fatal("sema error:", diags)
	}
	prog, err := lower.FromTyped(typed)
	if err != nil {
		t.Fatal("lower error:", err)
	}

	// Verify constants propagated into the lowered interface
	found := false
	for _, iface := range prog.Interfaces {
		if iface.Name == "AgentRegistry" {
			found = true
			if len(iface.Constants) == 0 {
				t.Error("expected constants in lowered AgentRegistry interface")
			} else if iface.Constants[0].Name != "MAX_FEE" {
				t.Errorf("expected constant MAX_FEE, got %q", iface.Constants[0].Name)
			}
			if len(iface.Enums) == 0 {
				t.Error("expected enums in lowered AgentRegistry interface")
			} else if iface.Enums[0].Name != "Kind" {
				t.Errorf("expected enum Kind, got %q", iface.Enums[0].Name)
			}
		}
	}
	if !found {
		t.Fatal("AgentRegistry not found in lowered interfaces")
	}

	// Full compilation must succeed
	_, err = BuildIRFromLowered(prog, "/tmp/toltest_qual/Caller.tol")
	if err != nil {
		t.Fatal("IR lowering error:", err)
	}
}

func TestPackageImportManifest(t *testing.T) {
    src := `pragma tolang 0.2.0;
package tos.registry;
contract Registry {
  function get(bytes32 k) external returns (agent r) { return agent(0); }
}
`
    pkgBytes, err := CompilePackage([]byte(src), "registry.tol", nil)
    if err != nil {
        t.Fatal("CompilePackage error:", err)
    }
    pkg, err := DecodePackage(pkgBytes)
    if err != nil {
        t.Fatal("DecodePackage error:", err)
    }
    // Check manifest has "package" field
    if !strings.Contains(string(pkg.ManifestJSON), `"package":"tos.registry"`) {
        t.Errorf("manifest does not contain package field, got: %s", pkg.ManifestJSON)
    }
    // Check pkgName was set from package declaration
    if !strings.Contains(string(pkg.ManifestJSON), `"name":"tos.registry"`) {
        t.Errorf("manifest name should be tos.registry, got: %s", pkg.ManifestJSON)
    }
}

// TestTolLangAutoImport verifies that contracts in tol/lang/ are automatically
// available without an explicit import statement.
func TestTolLangAutoImport(t *testing.T) {
	// Platform contract in tol.lang
	platformSrc := `pragma tolang 0.2.0;
package tol.lang;
contract Token {
  function totalSupply() external returns (uint256 s) { return 0; }
}
`
	// User contract — no import, just uses Token directly
	userSrc := `pragma tolang 0.2.0;
contract MyContract {
  function supply(agent tok) external returns (uint256 s) {
    let t: Token = Token(tok);
    return t.totalSupply();
  }
}
`
	os.MkdirAll("/tmp/toltest_toslang/tol/lang", 0755)
	os.WriteFile("/tmp/toltest_toslang/tol/lang/Token.tol", []byte(platformSrc), 0644)

	resolver := NewOSFileResolver("/tmp/toltest_toslang")
	mod, diags := parser.ParseFile("/tmp/toltest_toslang/MyContract.tol", []byte(userSrc))
	if diags.HasErrors() {
		t.Fatal("parse error:", diags)
	}
	_, diags = sema.CheckWithResolver("/tmp/toltest_toslang/MyContract.tol", mod, resolver)
	if diags.HasErrors() {
		t.Fatal("sema error (Token should be auto-imported from tol.lang):", diags)
	}
	// Token should be in m.Interfaces after auto-import
	found := false
	for _, iface := range mod.Interfaces {
		if iface.Name == "Token" && iface.PackageName == "tol.lang" {
			found = true
		}
	}
	if !found {
		t.Error("Token from tol.lang not auto-imported into module interfaces")
	}
}

// TestTolLangReservedRejected verifies that compiling a .tor with package tol.lang fails.
func TestTolLangReservedRejected(t *testing.T) {
	src := `pragma tolang 0.2.0;
package tol.lang;
contract Fake {
  function noop() external {}
}
`
	_, err := CompilePackage([]byte(src), "fake.tol", nil)
	if err == nil {
		t.Fatal("expected error for reserved package tol.lang, got nil")
	}
	if !strings.Contains(err.Error(), "TOL2097") {
		t.Errorf("expected TOL2097 in error, got: %v", err)
	}
}
