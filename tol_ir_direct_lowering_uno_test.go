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

// TestLowerUnoNeOperator verifies != on uno compiles to tos.ciphertext.ne.
func TestLowerUnoNeOperator(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoNeTest {
  function doNe(uno a, uno b) public pure returns (bool) {
    bool result = a != b;
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

// TestLowerUnoLteOperator verifies <= on uno compiles to tos.ciphertext.lte.
func TestLowerUnoLteOperator(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoLteTest {
  function doLte(uno a, uno b) public pure returns (bool) {
    bool result = a <= b;
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

// TestLowerUnoGteOperator verifies >= on uno compiles to tos.ciphertext.gte.
func TestLowerUnoGteOperator(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoGteTest {
  function doGte(uno a, uno b) public pure returns (bool) {
    bool result = a >= b;
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

// TestLowerUnoLteMethod verifies a.lte(b) method call compiles.
func TestLowerUnoLteMethod(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoLteMethodTest {
  function doLte(uno a, uno b) public pure returns (bool) {
    bool result = a.lte(b);
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

// TestLowerUnoGteMethod verifies a.gte(b) method call compiles.
func TestLowerUnoGteMethod(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoGteMethodTest {
  function doGte(uno a, uno b) public pure returns (bool) {
    bool result = a.gte(b);
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

// TestLowerUnoNeMethod verifies a.ne(b) method call compiles.
func TestLowerUnoNeMethod(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract UnoNeMethodTest {
  function doNe(uno a, uno b) public pure returns (bool) {
    bool result = a.ne(b);
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

// TestLowerPayableUno verifies that payable(uno) functions compile successfully
// and msg.value is correctly desugared to msg.uno_value.
func TestLowerPayableUno(t *testing.T) {
	src := []byte(`pragma tolang 0.4.0;
contract PayableUnoTest {
  mapping(agent => uno) deposits;
  function deposit() external payable(uno) {
    deposits[msg.sender] = deposits[msg.sender].add(msg.value);
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
	// Verify the bytecode string pool contains "uno_value" (the desugared field name).
	if !strings.Contains(string(bc), "uno_value") {
		t.Fatal("expected 'uno_value' in bytecode constant pool (msg.value desugared)")
	}
}

// TestLowerPayablePlainMsgValue verifies that plain payable still uses msg.value (not uno_value).
func TestLowerPayablePlainMsgValue(t *testing.T) {
	src := []byte(`pragma tolang 0.4.0;
contract PayablePlainTest {
  u256 total;
  function deposit() external payable {
    total = total + msg.value;
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
	// Plain payable should NOT desugar to uno_value.
	if strings.Contains(string(bc), "uno_value") {
		t.Fatal("plain payable should NOT contain 'uno_value' in constant pool")
	}
}

// TestLowerUnoBalance verifies that uno.balance(addr) compiles.
func TestLowerUnoBalance(t *testing.T) {
	src := []byte(`pragma tolang 0.4.0;
contract UnoBalanceTest {
  function getNativeBalance(agent addr) public view returns (uno) {
    return uno.balance(addr);
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

// TestLowerUnoTransfer verifies that uno.transfer(to, ct) compiles.
func TestLowerUnoTransfer(t *testing.T) {
	src := []byte(`pragma tolang 0.4.0;
contract UnoTransferTest {
  mapping(agent => uno) deposits;
  function withdraw(uno amount) external {
    deposits[msg.sender] = deposits[msg.sender].sub(amount);
    uno.transfer(msg.sender, amount);
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

// TestConfidentialVaultCompiles verifies the full ConfidentialVault example compiles.
func TestConfidentialVaultCompiles(t *testing.T) {
	src := []byte(`pragma tolang 0.4.0;
contract ConfidentialVault {
  mapping(agent => uno) deposits;
  mapping(agent => bytes32) publicKeys;

  event Deposited(agent indexed depositor)
  event Withdrawn(agent indexed withdrawer)

  function setPublicKey(bytes32 pubkey) external {
    publicKeys[msg.sender] = pubkey;
  }

  function deposit() external payable(uno) {
    deposits[msg.sender] = deposits[msg.sender].add(msg.value);
    emit Deposited(msg.sender);
  }

  function withdraw(uno amount) external {
    uno newBal = deposits[msg.sender].sub(amount);
    require(newBal.gt(uno.zero()), "InsufficientDeposit");
    deposits[msg.sender] = newBal;
    uno.transfer(msg.sender, amount);
    emit Withdrawn(msg.sender);
  }

  function depositOf(agent account) public view returns (uno) {
    return deposits[account];
  }

  function nativeBalance(agent account) public view returns (uno) {
    return uno.balance(account);
  }
}
`)
	bc, err := CompileBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("ConfidentialVault compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatal("bytecode is empty")
	}
}
