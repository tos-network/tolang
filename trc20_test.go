package lua

import (
	"strings"
	"testing"
)

// trc20Source is the TRC20 contract from TOL_SPEC.md §19, adapted for current
// implementation (set syntax only, no sstore() call form).
const trc20Source = `
pragma tolang 0.2.0;
contract TRC20 {
  u256 total_supply;
  mapping(agent => u256) balances;
  mapping(agent => mapping(agent => u256)) allowances;

  event Transfer(agent from indexed, agent to indexed, u256 value)
  event Approval(agent owner indexed, agent spender indexed, u256 value)

  constructor(agent owner, u256 supply) {
    set total_supply = supply;
    set balances[owner] = supply;
    return;
  }

  function totalSupply() public view returns (u256 s) {
    let s: u256 = total_supply;
    return s;
  }

  function balanceOf(agent owner) public view returns (u256 balance) {
    let b: u256 = balances[owner];
    return b;
  }

  function transfer(agent to, u256 amount) public returns (bool ok) {
    let from: agent = msg.sender;
    let from_bal: u256 = balances[from];
    require(from_bal >= amount, "INSUFFICIENT_BALANCE");
    set balances[from] = from_bal - amount;
    let to_bal: u256 = balances[to];
    set balances[to] = to_bal + amount;
    emit Transfer(from, to, amount);
    return true;
  }

  function approve(agent spender, u256 amount) public returns (bool ok) {
    let owner: agent = msg.sender;
    set allowances[owner][spender] = amount;
    emit Approval(owner, spender, amount);
    return true;
  }

  function transferFrom(agent from, agent to, u256 amount) public returns (bool ok) {
    let spender: agent = msg.sender;
    let allow: u256 = allowances[from][spender];
    require(allow >= amount, "INSUFFICIENT_ALLOWANCE");
    set allowances[from][spender] = allow - amount;
    let from_bal: u256 = balances[from];
    require(from_bal >= amount, "INSUFFICIENT_BALANCE");
    set balances[from] = from_bal - amount;
    let to_bal: u256 = balances[to];
    set balances[to] = to_bal + amount;
    emit Transfer(from, to, amount);
    return true;
  }

  fallback { revert "UNKNOWN_SELECTOR"; }
}
`

// trc20State compiles the TRC20 contract, loads it into a fresh LState, and
// sets up host globals required at runtime: emit (no-op), msg table.
func trc20State(t *testing.T) (*LState, LValue) {
	t.Helper()
	bc, err := CompileBytecode([]byte(trc20Source), "TRC20")
	if err != nil {
		t.Fatalf("TRC20 compile error: %v", err)
	}
	L := NewState()
	// emit(eventName, ...args) - record last event name and args for assertions.
	L.SetGlobal("emit", L.NewFunction(func(L *LState) int {
		if L.GetTop() >= 1 {
			L.SetGlobal("__last_event", L.CheckAny(1))
		}
		return 0
	}))
	// msg table - caller sets msg.sender before each call.
	msgTable := L.NewTable()
	L.SetField(msgTable, "sender", LString("0x0000000000000000000000000000000000000000000000000000000000000000"))
	L.SetField(msgTable, "value", lu256FromInt(0))
	L.SetGlobal("msg", msgTable)

	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("TRC20 DoBytecode error: %v", err)
	}
	tos := L.GetGlobal("tos")
	return L, tos
}

// setSender sets msg.sender to addr in the given LState.
func setSender(L *LState, addr string) {
	msgTable := L.GetGlobal("msg").(*LTable)
	L.SetField(msgTable, "sender", LString(addr))
}

// callTRC20 invokes tos.oninvoke(selector, args...) and returns the first
// return value as a string (empty string if no return value).
func callTRC20(t *testing.T, L *LState, tos LValue, fnSig string, args ...LValue) string {
	t.Helper()
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("tos.oninvoke not set")
	}
	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature(fnSig)))
	for _, a := range args {
		L.Push(a)
	}
	if err := L.PCall(1+len(args), MultRet, nil); err != nil {
		t.Fatalf("call %s failed: %v", fnSig, err)
	}
	if L.GetTop() > 0 {
		v := LVAsString(L.Get(-1))
		L.Pop(L.GetTop())
		return v
	}
	return ""
}

// callTRC20Err expects the call to fail and returns the error message.
func callTRC20Err(t *testing.T, L *LState, tos LValue, fnSig string, args ...LValue) string {
	t.Helper()
	oninvoke := L.GetField(tos, "oninvoke")
	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature(fnSig)))
	for _, a := range args {
		L.Push(a)
	}
	err := L.PCall(1+len(args), MultRet, nil)
	if err == nil {
		t.Fatalf("expected error calling %s, got none", fnSig)
	}
	return err.Error()
}

// deployTRC20 loads the init artifact (constructor only) to call tos.oncreate(owner, supply),
// then reloads the runtime artifact so oninvoke dispatch is available.
// This mirrors the init/runtime split: init_code runs once at deploy; runtime handles calls.
func deployTRC20(t *testing.T, owner string, supply int) (*LState, LValue) {
	t.Helper()

	// Load init artifact to execute the constructor.
	initBC, err := CompileInitBytecode([]byte(trc20Source), "TRC20")
	if err != nil {
		t.Fatalf("TRC20 init compile error: %v", err)
	}
	L, tos := trc20State(t)
	if err := L.DoBytecode(initBC); err != nil {
		t.Fatalf("TRC20 init DoBytecode error: %v", err)
	}
	tos = L.GetGlobal("tos")
	oncreate := L.GetField(tos, "oncreate")
	if oncreate == LNil {
		t.Fatalf("tos.oncreate not set")
	}
	L.Push(oncreate)
	L.Push(LString(owner))
	L.Push(lu256FromInt(supply))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("constructor failed: %v", err)
	}

	// Reload runtime artifact so tos.oninvoke dispatch is available.
	runtimeBC, err := CompileBytecode([]byte(trc20Source), "TRC20")
	if err != nil {
		t.Fatalf("TRC20 runtime compile error: %v", err)
	}
	if err := L.DoBytecode(runtimeBC); err != nil {
		t.Fatalf("TRC20 runtime DoBytecode error: %v", err)
	}
	tos = L.GetGlobal("tos")
	return L, tos
}

const (
	alice   = "0x000000000000000000000000000000000000000000000000000000000000a11c"
	bob     = "0x000000000000000000000000000000000000000000000000000000000000b0b0"
	charlie = "0x000000000000000000000000000000000000000000000000000000000000c4a0"
)

// --- Compilation ---

func TestTRC20Compiles(t *testing.T) {
	bc, err := CompileBytecode([]byte(trc20Source), "TRC20")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("empty bytecode")
	}
}

// --- Constructor ---

func TestTRC20ConstructorSetsSupply(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	got := callTRC20(t, L, tos, "totalSupply()")
	if got != "1000" {
		t.Fatalf("totalSupply: got %s want 1000", got)
	}
}

func TestTRC20ConstructorCreditsOwner(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	bal := callTRC20(t, L, tos, "balanceOf(agent)", LString(alice))
	if bal != "1000" {
		t.Fatalf("alice balance after deploy: got %s want 1000", bal)
	}
	bal = callTRC20(t, L, tos, "balanceOf(agent)", LString(bob))
	if bal != "0" {
		t.Fatalf("bob balance after deploy: got %s want 0", bal)
	}
}

// --- balanceOf ---

func TestTRC20BalanceOfZeroForUnknownAgent(t *testing.T) {
	L, tos := deployTRC20(t, alice, 500)
	defer L.Close()

	bal := callTRC20(t, L, tos, "balanceOf(agent)", LString(charlie))
	if bal != "0" {
		t.Fatalf("charlie balance: got %s want 0", bal)
	}
}

// --- transfer ---

func TestTRC20TransferMovesBalance(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	setSender(L, alice)
	callTRC20(t, L, tos, "transfer(agent,u256)", LString(bob), lu256FromInt(300))

	aliceBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(alice))
	if aliceBal != "700" {
		t.Fatalf("alice balance after transfer: got %s want 700", aliceBal)
	}
	bobBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(bob))
	if bobBal != "300" {
		t.Fatalf("bob balance after transfer: got %s want 300", bobBal)
	}
}

func TestTRC20TransferFullBalance(t *testing.T) {
	L, tos := deployTRC20(t, alice, 500)
	defer L.Close()

	setSender(L, alice)
	callTRC20(t, L, tos, "transfer(agent,u256)", LString(bob), lu256FromInt(500))

	aliceBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(alice))
	if aliceBal != "0" {
		t.Fatalf("alice balance after full transfer: got %s want 0", aliceBal)
	}
	bobBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(bob))
	if bobBal != "500" {
		t.Fatalf("bob balance after full transfer: got %s want 500", bobBal)
	}
}

func TestTRC20TransferInsufficientBalanceReverts(t *testing.T) {
	L, tos := deployTRC20(t, alice, 100)
	defer L.Close()

	setSender(L, alice)
	errMsg := callTRC20Err(t, L, tos, "transfer(agent,u256)", LString(bob), lu256FromInt(200))
	if !strings.Contains(errMsg, "INSUFFICIENT_BALANCE") {
		t.Fatalf("expected INSUFFICIENT_BALANCE error, got: %s", errMsg)
	}
}

func TestTRC20TransferDoesNotChangeBalanceOnRevert(t *testing.T) {
	L, tos := deployTRC20(t, alice, 100)
	defer L.Close()

	setSender(L, alice)
	callTRC20Err(t, L, tos, "transfer(agent,u256)", LString(bob), lu256FromInt(999))

	aliceBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(alice))
	if aliceBal != "100" {
		t.Fatalf("alice balance unchanged after failed transfer: got %s want 100", aliceBal)
	}
}

func TestTRC20MultipleTransfers(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	setSender(L, alice)
	callTRC20(t, L, tos, "transfer(agent,u256)", LString(bob), lu256FromInt(200))
	callTRC20(t, L, tos, "transfer(agent,u256)", LString(charlie), lu256FromInt(300))

	aliceBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(alice))
	if aliceBal != "500" {
		t.Fatalf("alice balance: got %s want 500", aliceBal)
	}
	bobBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(bob))
	if bobBal != "200" {
		t.Fatalf("bob balance: got %s want 200", bobBal)
	}
	charlieBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(charlie))
	if charlieBal != "300" {
		t.Fatalf("charlie balance: got %s want 300", charlieBal)
	}
}

// --- approve ---

func TestTRC20ApproveSetAllowance(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	setSender(L, alice)
	callTRC20(t, L, tos, "approve(agent,u256)", LString(charlie), lu256FromInt(400))

	// Verify via transferFrom: charlie can spend up to 400 of alice's tokens.
	setSender(L, charlie)
	callTRC20(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(charlie), lu256FromInt(100))

	charlieBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(charlie))
	if charlieBal != "100" {
		t.Fatalf("charlie balance after transferFrom: got %s want 100", charlieBal)
	}
	aliceBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(alice))
	if aliceBal != "900" {
		t.Fatalf("alice balance after transferFrom: got %s want 900", aliceBal)
	}
}

func TestTRC20ApproveOverwritesPreviousAllowance(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	setSender(L, alice)
	callTRC20(t, L, tos, "approve(agent,u256)", LString(charlie), lu256FromInt(500))
	callTRC20(t, L, tos, "approve(agent,u256)", LString(charlie), lu256FromInt(50))

	// charlie can only spend 50 now.
	setSender(L, charlie)
	callTRC20Err(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(charlie), lu256FromInt(100))

	// But spending 50 or less works.
	callTRC20(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(charlie), lu256FromInt(50))

	charlieBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(charlie))
	if charlieBal != "50" {
		t.Fatalf("charlie balance: got %s want 50", charlieBal)
	}
}

// --- transferFrom ---

func TestTRC20TransferFromReducesAllowance(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	setSender(L, alice)
	callTRC20(t, L, tos, "approve(agent,u256)", LString(charlie), lu256FromInt(300))

	setSender(L, charlie)
	callTRC20(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(bob), lu256FromInt(100))
	callTRC20(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(bob), lu256FromInt(100))

	bobBal := callTRC20(t, L, tos, "balanceOf(agent)", LString(bob))
	if bobBal != "200" {
		t.Fatalf("bob balance: got %s want 200", bobBal)
	}
	// Remaining allowance = 100; spending 101 should fail.
	errMsg := callTRC20Err(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(bob), lu256FromInt(101))
	if !strings.Contains(errMsg, "INSUFFICIENT_ALLOWANCE") {
		t.Fatalf("expected INSUFFICIENT_ALLOWANCE, got: %s", errMsg)
	}
}

func TestTRC20TransferFromInsufficientAllowanceReverts(t *testing.T) {
	L, tos := deployTRC20(t, alice, 1000)
	defer L.Close()

	// charlie has no allowance → should revert immediately.
	setSender(L, charlie)
	errMsg := callTRC20Err(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(bob), lu256FromInt(1))
	if !strings.Contains(errMsg, "INSUFFICIENT_ALLOWANCE") {
		t.Fatalf("expected INSUFFICIENT_ALLOWANCE, got: %s", errMsg)
	}
}

func TestTRC20TransferFromInsufficientBalanceReverts(t *testing.T) {
	L, tos := deployTRC20(t, alice, 50)
	defer L.Close()

	// Give charlie unlimited allowance but alice only has 50.
	setSender(L, alice)
	callTRC20(t, L, tos, "approve(agent,u256)", LString(charlie), lu256FromInt(9999))

	setSender(L, charlie)
	errMsg := callTRC20Err(t, L, tos, "transferFrom(agent,agent,u256)",
		LString(alice), LString(bob), lu256FromInt(100))
	if !strings.Contains(errMsg, "INSUFFICIENT_BALANCE") {
		t.Fatalf("expected INSUFFICIENT_BALANCE, got: %s", errMsg)
	}
}

// --- fallback ---

func TestTRC20FallbackRevertsOnUnknownSelector(t *testing.T) {
	L, _ := trc20State(t)
	defer L.Close()

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	L.Push(oninvoke)
	L.Push(LString("0xdeadbeef"))
	err := L.PCall(1, 0, nil)
	if err == nil {
		t.Fatalf("expected revert on unknown selector")
	}
	if !strings.Contains(err.Error(), "UNKNOWN_SELECTOR") {
		t.Fatalf("expected UNKNOWN_SELECTOR, got: %v", err)
	}
}

// --- storage isolation: separate deployments don't share state ---

func TestTRC20TwoDeploymentsAreIsolated(t *testing.T) {
	L1, tos1 := deployTRC20(t, alice, 1000)
	defer L1.Close()
	L2, tos2 := deployTRC20(t, alice, 500)
	defer L2.Close()

	bal1 := callTRC20(t, L1, tos1, "totalSupply()")
	bal2 := callTRC20(t, L2, tos2, "totalSupply()")
	if bal1 != "1000" || bal2 != "500" {
		t.Fatalf("deployment isolation: L1=%s L2=%s want 1000/500", bal1, bal2)
	}
}
