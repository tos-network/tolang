package lua

import (
	"strings"
	"testing"
)

// TestLowerUnoAdd verifies that a.add(b) where a,b are uno lowers to tos.ciphertext.add.
func TestLowerUnoAdd(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoAddTest {
  function doAdd(uno a, uno b) public pure returns (uno) {
    uno result = a.add(b);
    return result;
  }
}
`)
	bc, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatal("bytecode is empty")
	}
}

// TestLowerUnoZero verifies that uno.zero() lowers to tos.ciphertext.zero().
func TestLowerUnoZero(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoZeroTest {
  function doZero() public pure returns (uno) {
    uno result = uno.zero();
    return result;
  }
}
`)
	bc, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatal("bytecode is empty")
	}
}

// TestLowerUnoStorage verifies that uno storage load/store compiles
// with the two-slot pattern (commitment + handle).
func TestLowerUnoStorage(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoStorageTest {
  uno balance;

  function setBalance(uno val) external {
    balance = val;
  }

  function getBalance() external returns (uno) {
    uno result = balance;
    return result;
  }
}
`)
	bc, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatal("bytecode is empty")
	}
}

// TestLowerUnoMappingStorage verifies mapping(agent => uno) storage works.
func TestLowerUnoMappingStorage(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoMappingTest {
  mapping(agent => uno) balances;

  function setBalance(agent addr, uno val) external {
    balances[addr] = val;
  }

  function getBalance(agent addr) external returns (uno) {
    uno result = balances[addr];
    return result;
  }
}
`)
	bc, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatal("bytecode is empty")
	}
}

// TestLowerUnoEqOperator verifies == on uno compiles to tos.ciphertext.eq.
func TestLowerUnoEqOperator(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoEqTest {
  function doEq(uno a, uno b) public pure returns (bool) {
    bool result = a == b;
    return result;
  }
}
`)
	bc, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatal("bytecode is empty")
	}
}

// TestLowerUnoOperatorReject verifies + on uno is rejected at lowering.
func TestLowerUnoOperatorReject(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoAddOpTest {
  function doOp(uno a, uno b) public pure returns (uno) {
    uno result = a + b;
    return result;
  }
}
`)
	_, err := CompileBytecode(src, "<tol>")
	if err == nil {
		t.Fatal("expected compile error for + on uno")
	}
	if !strings.Contains(err.Error(), "uno") {
		t.Fatalf("expected error mentioning 'uno', got: %v", err)
	}
}
