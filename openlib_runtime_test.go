package lua

import (
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tos-network/tolang/metadata"
)

const (
	openlibMerchant = "0x000000000000000000000000000000000000000000000000000000000000d00d"
	openlibService  = "0x0000000000000000000000000000000000000000000000000000000000005eed"
)

type openlibRuntimeHost struct {
	msgTable   *LTable
	blockTable *LTable
	tosTable   *LTable

	agentProps map[string]map[string]LValue

	nativeUnoBalances map[string]*big.Int

	callHook        func(addr, value, data string) (bool, string, bool)
	packageCallHook func(addr, contractName, calldata string) []LValue

	callCount    int
	lastCallAddr string
	lastCallData string
	lastCallCost string

	escrowCount    int
	lastEscrowAddr string
	lastEscrowAmt  string
	lastEscrowTag  string

	releaseCount    int
	lastReleaseAddr string
	lastReleaseAmt  string
	lastReleaseTag  string

	packageCallCount        int
	lastPackageAddr         string
	lastPackageContractName string
	lastPackageCalldata     string

	unoTransferCount      int
	lastUnoTransferAddr   string
	lastUnoTransferAmount string

	capturedResult string
	hasResult      bool

	emittedEvents []openlibEmittedEvent
}

type openlibEmittedEvent struct {
	Name string
	Args []string // alternating type, value pairs
}

var openlibResultSentinel = &struct{}{}

type openlibRuntimeHostSnapshot struct {
	agentProps        map[string]map[string]LValue
	nativeUnoBalances map[string]*big.Int

	callCount    int
	lastCallAddr string
	lastCallData string
	lastCallCost string

	escrowCount    int
	lastEscrowAddr string
	lastEscrowAmt  string
	lastEscrowTag  string

	releaseCount    int
	lastReleaseAddr string
	lastReleaseAmt  string
	lastReleaseTag  string

	packageCallCount        int
	lastPackageAddr         string
	lastPackageContractName string
	lastPackageCalldata     string

	unoTransferCount      int
	lastUnoTransferAddr   string
	lastUnoTransferAmount string

	capturedResult string
	hasResult      bool

	msgSender   LValue
	msgValue    LValue
	msgUno      LValue
	tosCalldata LValue
}

func cloneLValueMap(src map[string]LValue) map[string]LValue {
	if src == nil {
		return nil
	}
	out := make(map[string]LValue, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneAgentProps(src map[string]map[string]LValue) map[string]map[string]LValue {
	if src == nil {
		return nil
	}
	out := make(map[string]map[string]LValue, len(src))
	for addr, props := range src {
		out[addr] = cloneLValueMap(props)
	}
	return out
}

func cloneNativeUnoBalances(src map[string]*big.Int) map[string]*big.Int {
	if src == nil {
		return nil
	}
	out := make(map[string]*big.Int, len(src))
	for addr, bal := range src {
		if bal == nil {
			out[addr] = nil
			continue
		}
		out[addr] = new(big.Int).Set(bal)
	}
	return out
}

func snapshotRuntimeHost(host *openlibRuntimeHost) openlibRuntimeHostSnapshot {
	if host == nil {
		return openlibRuntimeHostSnapshot{}
	}
	snap := openlibRuntimeHostSnapshot{
		agentProps:              cloneAgentProps(host.agentProps),
		nativeUnoBalances:       cloneNativeUnoBalances(host.nativeUnoBalances),
		callCount:               host.callCount,
		lastCallAddr:            host.lastCallAddr,
		lastCallData:            host.lastCallData,
		lastCallCost:            host.lastCallCost,
		escrowCount:             host.escrowCount,
		lastEscrowAddr:          host.lastEscrowAddr,
		lastEscrowAmt:           host.lastEscrowAmt,
		lastEscrowTag:           host.lastEscrowTag,
		releaseCount:            host.releaseCount,
		lastReleaseAddr:         host.lastReleaseAddr,
		lastReleaseAmt:          host.lastReleaseAmt,
		lastReleaseTag:          host.lastReleaseTag,
		packageCallCount:        host.packageCallCount,
		lastPackageAddr:         host.lastPackageAddr,
		lastPackageContractName: host.lastPackageContractName,
		lastPackageCalldata:     host.lastPackageCalldata,
		unoTransferCount:        host.unoTransferCount,
		lastUnoTransferAddr:     host.lastUnoTransferAddr,
		lastUnoTransferAmount:   host.lastUnoTransferAmount,
		capturedResult:          host.capturedResult,
		hasResult:               host.hasResult,
	}
	if host.msgTable != nil {
		snap.msgSender = host.msgTable.RawGetString("sender")
		snap.msgValue = host.msgTable.RawGetString("value")
		snap.msgUno = host.msgTable.RawGetString("uno_value")
	}
	if host.tosTable != nil {
		snap.tosCalldata = host.tosTable.RawGetString("calldata")
	}
	return snap
}

func restoreRuntimeHost(host *openlibRuntimeHost, snap openlibRuntimeHostSnapshot) {
	if host == nil {
		return
	}
	host.agentProps = cloneAgentProps(snap.agentProps)
	host.nativeUnoBalances = cloneNativeUnoBalances(snap.nativeUnoBalances)
	host.callCount = snap.callCount
	host.lastCallAddr = snap.lastCallAddr
	host.lastCallData = snap.lastCallData
	host.lastCallCost = snap.lastCallCost
	host.escrowCount = snap.escrowCount
	host.lastEscrowAddr = snap.lastEscrowAddr
	host.lastEscrowAmt = snap.lastEscrowAmt
	host.lastEscrowTag = snap.lastEscrowTag
	host.releaseCount = snap.releaseCount
	host.lastReleaseAddr = snap.lastReleaseAddr
	host.lastReleaseAmt = snap.lastReleaseAmt
	host.lastReleaseTag = snap.lastReleaseTag
	host.packageCallCount = snap.packageCallCount
	host.lastPackageAddr = snap.lastPackageAddr
	host.lastPackageContractName = snap.lastPackageContractName
	host.lastPackageCalldata = snap.lastPackageCalldata
	host.unoTransferCount = snap.unoTransferCount
	host.lastUnoTransferAddr = snap.lastUnoTransferAddr
	host.lastUnoTransferAmount = snap.lastUnoTransferAmount
	host.capturedResult = snap.capturedResult
	host.hasResult = snap.hasResult
	restoreRuntimeHostCallContext(host, snap)
}

func restoreRuntimeHostCallContext(host *openlibRuntimeHost, snap openlibRuntimeHostSnapshot) {
	if host == nil {
		return
	}
	if host.msgTable != nil {
		host.msgTable.RawSetString("sender", snap.msgSender)
		host.msgTable.RawSetString("value", snap.msgValue)
		host.msgTable.RawSetString("uno_value", snap.msgUno)
	}
	if host.tosTable != nil {
		host.tosTable.RawSetString("calldata", snap.tosCalldata)
	}
}

func TestRuntimeHostSnapshotRestoresPersistentStateAndCallContext(t *testing.T) {
	L := NewState()
	defer L.Close()

	host := installOpenlibRuntimeHost(L)
	openlibSetSender(host, alice)
	openlibSetValue(host, 7)
	openlibSetUnoValue(host, openlibUnoFromInt(3))
	host.tosTable.RawSetString("calldata", LString("0x1234"))
	openlibSetNativeUnoBalance(host, alice, 11)
	host.unoTransferCount = 1
	host.lastUnoTransferAddr = alice
	host.lastUnoTransferAmount = openlibUnoStringFromBigInt(big.NewInt(11))
	openlibSetAgentProp(host, alice, "is_registered", LTrue)

	snap := snapshotRuntimeHost(host)

	openlibSetSender(host, bob)
	openlibSetValue(host, 99)
	openlibSetUnoValue(host, openlibUnoFromInt(25))
	host.tosTable.RawSetString("calldata", LString("0xbeef"))
	openlibSetNativeUnoBalance(host, alice, 42)
	host.unoTransferCount = 8
	host.lastUnoTransferAddr = bob
	host.lastUnoTransferAmount = openlibUnoStringFromBigInt(big.NewInt(42))
	openlibSetAgentProp(host, bob, "is_registered", LTrue)

	restoreRuntimeHost(host, snap)

	if got := LVAsString(host.msgTable.RawGetString("sender")); got != alice {
		t.Fatalf("sender after restore: got=%q want=%q", got, alice)
	}
	if got := LVAsString(host.msgTable.RawGetString("value")); got != LVAsString(lu256FromInt(7)) {
		t.Fatalf("value after restore: got=%q want=%q", got, LVAsString(lu256FromInt(7)))
	}
	if got := LVAsString(host.msgTable.RawGetString("uno_value")); got != LVAsString(openlibUnoFromInt(3)) {
		t.Fatalf("uno_value after restore: got=%q want=%q", got, LVAsString(openlibUnoFromInt(3)))
	}
	if got := LVAsString(host.tosTable.RawGetString("calldata")); got != "0x1234" {
		t.Fatalf("calldata after restore: got=%q want=%q", got, "0x1234")
	}
	if got := openlibNativeUnoBalance(host, alice); got.Cmp(big.NewInt(11)) != 0 {
		t.Fatalf("native UNO balance after restore: got=%s want=11", got.String())
	}
	if _, ok := host.agentProps[bob]; ok {
		t.Fatalf("agentProps for bob should have been rolled back")
	}
	if host.unoTransferCount != 1 {
		t.Fatalf("unoTransferCount after restore: got=%d want=1", host.unoTransferCount)
	}
	if host.lastUnoTransferAddr != alice {
		t.Fatalf("lastUnoTransferAddr after restore: got=%q want=%q", host.lastUnoTransferAddr, alice)
	}
}

func TestOpenlibSetValueStringRejectsInvalidInput(t *testing.T) {
	L := NewState()
	defer L.Close()

	host := installOpenlibRuntimeHost(L)
	host.msgTable.RawSetString("value", lu256FromInt(9))

	if err := openlibSetValueString(host, "not-a-number"); err == nil {
		t.Fatal("expected invalid value string to return error")
	}
	if got := LVAsString(host.msgTable.RawGetString("value")); got != LVAsString(lu256FromInt(9)) {
		t.Fatalf("msg.value changed on invalid input: got=%q want=%q", got, LVAsString(lu256FromInt(9)))
	}
}

func openlibBytes32(hexNibble string) string {
	return "0x" + strings.Repeat(hexNibble, 64)
}

func openlibHex64FromBigInt(v *big.Int) string {
	if v == nil {
		return strings.Repeat("0", 64)
	}
	abs := new(big.Int).Abs(v)
	hex := abs.Text(16)
	if len(hex) > 64 {
		hex = hex[len(hex)-64:]
	}
	return strings.Repeat("0", 64-len(hex)) + hex
}

func openlibNormalizeHex64(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "0x")
	if len(s) > 64 {
		s = s[len(s)-64:]
	}
	return strings.Repeat("0", 64-len(s)) + s
}

func openlibUnoStringFromBigInt(v *big.Int) string {
	if v == nil || v.Sign() == 0 {
		return "0x" + strings.Repeat("0", 128)
	}
	sign := big.NewInt(0)
	mag := new(big.Int).Set(v)
	if mag.Sign() < 0 {
		sign.SetInt64(1)
		mag.Neg(mag)
	}
	return "0x" + openlibHex64FromBigInt(sign) + openlibHex64FromBigInt(mag)
}

func openlibUnoFromInt(v int) LValue {
	return LString(openlibUnoStringFromBigInt(big.NewInt(int64(v))))
}

func openlibParseUnoString(s string) *big.Int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return big.NewInt(0)
	}
	if !strings.HasPrefix(s, "0x") {
		if out, ok := new(big.Int).SetString(s, 10); ok {
			return out
		}
		return big.NewInt(0)
	}
	hex := strings.TrimPrefix(s, "0x")
	if len(hex) <= 64 {
		if out, ok := new(big.Int).SetString(hex, 16); ok {
			return out
		}
		return big.NewInt(0)
	}
	if len(hex) < 128 {
		hex = strings.Repeat("0", 128-len(hex)) + hex
	}
	commitHex := hex[:64]
	handleHex := hex[len(hex)-64:]
	mag, ok := new(big.Int).SetString(handleHex, 16)
	if !ok {
		return big.NewInt(0)
	}
	sign, ok := new(big.Int).SetString(commitHex, 16)
	if ok && sign.Sign() != 0 {
		mag.Neg(mag)
	}
	return mag
}

func openlibUnoCommitment(ct string) string {
	ct = strings.TrimSpace(strings.ToLower(ct))
	ct = strings.TrimPrefix(ct, "0x")
	if len(ct) < 128 {
		ct = strings.Repeat("0", 128-len(ct)) + ct
	}
	return "0x" + ct[:64]
}

func openlibUnoHandle(ct string) string {
	ct = strings.TrimSpace(strings.ToLower(ct))
	ct = strings.TrimPrefix(ct, "0x")
	if len(ct) < 128 {
		ct = strings.Repeat("0", 128-len(ct)) + ct
	}
	return "0x" + ct[len(ct)-64:]
}

func openlibNativeUnoBalance(host *openlibRuntimeHost, addr string) *big.Int {
	if host == nil {
		return big.NewInt(0)
	}
	if v, ok := host.nativeUnoBalances[addr]; ok && v != nil {
		return new(big.Int).Set(v)
	}
	return big.NewInt(0)
}

func deployOpenlibContract(t *testing.T, relPath string, ctorArgs ...LValue) (*LState, LValue, *openlibRuntimeHost) {
	return deployOpenlibContractWithCompileName(t, relPath, "", ctorArgs...)
}

func deployOpenlibSourceContract(t *testing.T, source []byte, compileName string, ctorArgs ...LValue) (*LState, LValue, *openlibRuntimeHost) {
	t.Helper()

	runtimeBC, err := CompileBytecode(source, compileName)
	if err != nil {
		t.Fatalf("compile runtime %s: %v", compileName, err)
	}
	initBC, err := CompileInitBytecode(source, compileName)
	if err != nil {
		t.Fatalf("compile init %s: %v", compileName, err)
	}

	L := NewState()
	host := installOpenlibRuntimeHost(L)

	if err := L.DoBytecode(runtimeBC); err != nil {
		t.Fatalf("load runtime %s: %v", compileName, err)
	}
	if err := L.DoBytecode(initBC); err != nil {
		t.Fatalf("load init %s: %v", compileName, err)
	}

	tos := L.GetGlobal("tos")
	oncreate := L.GetField(tos, "oncreate")
	if oncreate != LNil {
		L.Push(oncreate)
		for _, arg := range ctorArgs {
			openlibRememberAgentValue(host, arg)
			L.Push(arg)
		}
		if err := L.PCall(len(ctorArgs), 0, nil); err != nil {
			t.Fatalf("constructor %s failed: %v", compileName, err)
		}
	} else if len(ctorArgs) != 0 {
		t.Fatalf("%s missing tos.oncreate", compileName)
	}

	if err := L.DoBytecode(runtimeBC); err != nil {
		t.Fatalf("reload runtime %s: %v", compileName, err)
	}
	tos = L.GetGlobal("tos")
	return L, tos, host
}

func deployOpenlibContractWithCompileName(t *testing.T, relPath string, compileName string, ctorArgs ...LValue) (*LState, LValue, *openlibRuntimeHost) {
	t.Helper()

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	sourcePath := filepath.Join(repoRoot, relPath)
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if compileName == "" {
		compileName = sourcePath
	}

	runtimeBC, err := CompileBytecode(source, compileName)
	if err != nil {
		t.Fatalf("compile runtime %s: %v", relPath, err)
	}
	initBC, err := CompileInitBytecode(source, compileName)
	if err != nil {
		t.Fatalf("compile init %s: %v", relPath, err)
	}

	L := NewState()
	host := installOpenlibRuntimeHost(L)

	if err := L.DoBytecode(runtimeBC); err != nil {
		t.Fatalf("load runtime %s: %v", relPath, err)
	}
	if err := L.DoBytecode(initBC); err != nil {
		t.Fatalf("load init %s: %v", relPath, err)
	}

	tos := L.GetGlobal("tos")
	oncreate := L.GetField(tos, "oncreate")
	if oncreate != LNil {
		L.Push(oncreate)
		for _, arg := range ctorArgs {
			openlibRememberAgentValue(host, arg)
			L.Push(arg)
		}
		if err := L.PCall(len(ctorArgs), 0, nil); err != nil {
			t.Fatalf("constructor %s failed: %v", relPath, err)
		}
	} else if len(ctorArgs) != 0 {
		t.Fatalf("%s missing tos.oncreate", relPath)
	}

	if err := L.DoBytecode(runtimeBC); err != nil {
		t.Fatalf("reload runtime %s: %v", relPath, err)
	}
	tos = L.GetGlobal("tos")
	return L, tos, host
}

func openlibEncodeStaticCalldata(fnSig string, words ...string) string {
	var b strings.Builder
	b.WriteString(selectorHexFromSignature(fnSig))
	for _, word := range words {
		b.WriteString(openlibNormalizeHex64(word))
	}
	return b.String()
}

func installOpenlibRuntimeHost(L *LState) *openlibRuntimeHost {
	host := &openlibRuntimeHost{
		agentProps:        map[string]map[string]LValue{},
		nativeUnoBalances: map[string]*big.Int{},
	}

	msgTable := L.NewTable()
	L.SetField(msgTable, "sender", LString("0x"+strings.Repeat("0", 64)))
	L.SetField(msgTable, "value", lu256FromInt(0))
	L.SetField(msgTable, "uno_value", LString(openlibUnoStringFromBigInt(nil)))

	blockTable := L.NewTable()
	L.SetField(blockTable, "number", lu256FromInt(0))
	L.SetField(blockTable, "timestamp_ms", lu256FromInt(0))

	txTable := L.NewTable()
	L.SetField(txTable, "origin", LString("0x"+strings.Repeat("0", 64)))

	tosTable := L.NewTable()
	ctTable := L.NewTable()
	L.SetField(tosTable, "msg", msgTable)
	L.SetField(tosTable, "block", blockTable)
	L.SetField(tosTable, "tx", txTable)
	L.SetField(tosTable, "ciphertext", ctTable)
	L.SetField(tosTable, "call", L.NewFunction(func(L *LState) int {
		host.callCount++
		host.lastCallAddr = LVAsString(L.CheckAny(1))
		host.lastCallCost = LVAsString(L.CheckAny(2))
		host.lastCallData = LVAsString(L.CheckAny(3))
		if host.callHook != nil {
			ok, ret, handled := host.callHook(host.lastCallAddr, host.lastCallCost, host.lastCallData)
			if handled {
				L.Push(LBool(ok))
				L.Push(LString(ret))
				return 2
			}
		}
		L.Push(LTrue)
		L.Push(LString("0x"))
		return 2
	}))
	L.SetField(tosTable, "package_call", L.NewFunction(func(L *LState) int {
		host.packageCallCount++
		host.lastPackageAddr = LVAsString(L.CheckAny(1))
		host.lastPackageContractName = LVAsString(L.CheckAny(2))
		host.lastPackageCalldata = LVAsString(L.CheckAny(3))
		if host.packageCallHook == nil {
			L.RaiseError("package_call not stubbed for %s", host.lastPackageContractName)
			return 0
		}
		rets := host.packageCallHook(host.lastPackageAddr, host.lastPackageContractName, host.lastPackageCalldata)
		for _, ret := range rets {
			L.Push(ret)
		}
		return len(rets)
	}))
	L.SetField(tosTable, "escrow", L.NewFunction(func(L *LState) int {
		host.escrowCount++
		if L.GetTop() >= 1 {
			host.lastEscrowAddr = LVAsString(L.CheckAny(1))
		}
		if L.GetTop() >= 2 {
			host.lastEscrowAmt = LVAsString(L.CheckAny(2))
		}
		if L.GetTop() >= 3 {
			host.lastEscrowTag = LVAsString(L.CheckAny(3))
		}
		return 0
	}))
	L.SetField(tosTable, "release", L.NewFunction(func(L *LState) int {
		host.releaseCount++
		if L.GetTop() >= 1 {
			host.lastReleaseAddr = LVAsString(L.CheckAny(1))
		}
		if L.GetTop() >= 2 {
			host.lastReleaseAmt = LVAsString(L.CheckAny(2))
		}
		if L.GetTop() >= 3 {
			host.lastReleaseTag = LVAsString(L.CheckAny(3))
		}
		return 0
	}))
	L.SetField(tosTable, "agentload", L.NewFunction(func(L *LState) int {
		addr := LVAsString(L.CheckAny(1))
		field := "is_registered"
		if L.GetTop() >= 2 {
			field = LVAsString(L.CheckAny(2))
		}
		if field == "registered" {
			field = "is_registered"
		}
		if field == "is_registered" {
			if addr == "0" ||
				addr == "0x0000000000000000000000000000000000000000" ||
				addr == "0x0000000000000000000000000000000000000000000000000000000000000000" {
				L.Push(lu256FromInt(1))
				return 1
			}
		}
		props := host.agentProps[addr]
		if props == nil {
			if field == "is_registered" {
				L.Push(lu256FromInt(0))
				return 1
			}
			L.Push(LNil)
			return 1
		}
		if field == "is_registered" {
			L.Push(lu256FromInt(1))
			return 1
		}
		if v, ok := props[field]; ok {
			L.Push(v)
			return 1
		}
		L.Push(LNil)
		return 1
	}))
	L.SetField(tosTable, "min_agent_stake", L.NewFunction(func(L *LState) int {
		L.Push(lu256FromInt(0))
		return 1
	}))
	L.SetField(ctTable, "zero", L.NewFunction(func(L *LState) int {
		L.Push(LString(openlibUnoStringFromBigInt(nil)))
		return 1
	}))
	L.SetField(ctTable, "from_parts", L.NewFunction(func(L *LState) int {
		commitment := openlibNormalizeHex64(LVAsString(L.CheckAny(1)))
		handle := openlibNormalizeHex64(LVAsString(L.CheckAny(2)))
		L.Push(LString("0x" + commitment + handle))
		return 1
	}))
	L.SetField(ctTable, "commitment", L.NewFunction(func(L *LState) int {
		L.Push(LString(openlibUnoCommitment(LVAsString(L.CheckAny(1)))))
		return 1
	}))
	L.SetField(ctTable, "handle", L.NewFunction(func(L *LState) int {
		L.Push(LString(openlibUnoHandle(LVAsString(L.CheckAny(1)))))
		return 1
	}))
	L.SetField(ctTable, "add", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LString(openlibUnoStringFromBigInt(new(big.Int).Add(left, right))))
		return 1
	}))
	L.SetField(ctTable, "sub", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LString(openlibUnoStringFromBigInt(new(big.Int).Sub(left, right))))
		return 1
	}))
	L.SetField(ctTable, "gt", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LBool(left.Cmp(right) > 0))
		return 1
	}))
	L.SetField(ctTable, "gte", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LBool(left.Cmp(right) >= 0))
		return 1
	}))
	L.SetField(ctTable, "lt", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LBool(left.Cmp(right) < 0))
		return 1
	}))
	L.SetField(ctTable, "lte", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LBool(left.Cmp(right) <= 0))
		return 1
	}))
	L.SetField(ctTable, "eq", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LBool(left.Cmp(right) == 0))
		return 1
	}))
	L.SetField(ctTable, "ne", L.NewFunction(func(L *LState) int {
		left := openlibParseUnoString(LVAsString(L.CheckAny(1)))
		right := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		L.Push(LBool(left.Cmp(right) != 0))
		return 1
	}))
	balanceFn := L.NewFunction(func(L *LState) int {
		addr := LVAsString(L.CheckAny(1))
		openlibRememberAgentString(host, addr)
		L.Push(LString(openlibUnoStringFromBigInt(openlibNativeUnoBalance(host, addr))))
		return 1
	})
	L.SetField(ctTable, "balance", balanceFn)
	L.SetField(tosTable, "uno_balance", balanceFn)
	transferFn := L.NewFunction(func(L *LState) int {
		addr := LVAsString(L.CheckAny(1))
		amount := openlibParseUnoString(LVAsString(L.CheckAny(2)))
		openlibRememberAgentString(host, addr)
		host.unoTransferCount++
		host.lastUnoTransferAddr = addr
		host.lastUnoTransferAmount = openlibUnoStringFromBigInt(amount)
		next := openlibNativeUnoBalance(host, addr)
		next.Add(next, amount)
		host.nativeUnoBalances[addr] = next
		return 0
	})
	L.SetField(ctTable, "transfer", transferFn)
	L.SetField(tosTable, "uno_transfer", transferFn)

	L.SetGlobal("emit", L.NewFunction(func(L *LState) int {
		n := L.GetTop()
		if n >= 1 {
			ev := openlibEmittedEvent{Name: LVAsString(L.Get(1))}
			for i := 2; i <= n; i++ {
				ev.Args = append(ev.Args, LVAsString(L.Get(i)))
			}
			host.emittedEvents = append(host.emittedEvents, ev)
		}
		return 0
	}))
	L.SetGlobal("tos", tosTable)
	L.SetGlobal("msg", msgTable)
	L.SetGlobal("block", blockTable)
	L.SetGlobal("tx", txTable)
	hostUD := L.NewUserData()
	hostUD.Value = host
	L.SetGlobal("__openlib_runtime_host", hostUD)

	host.msgTable = msgTable
	host.blockTable = blockTable
	host.tosTable = tosTable
	return host
}

func openlibSetSender(host *openlibRuntimeHost, sender string) {
	openlibRememberAgentString(host, sender)
	host.msgTable.RawSetString("sender", LString(sender))
}

func openlibSetValue(host *openlibRuntimeHost, value int) {
	host.msgTable.RawSetString("value", lu256FromInt(value))
}

func openlibSetValueString(host *openlibRuntimeHost, value string) error {
	parsed, err := parseUint256(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	host.msgTable.RawSetString("value", parsed)
	return nil
}

func isOpenlibResultSignal(err error) bool {
	apiErr, ok := err.(*ApiError)
	if !ok {
		return false
	}
	ud, ok := apiErr.Object.(*LUserData)
	if !ok {
		return false
	}
	return ud.Value == openlibResultSentinel
}

func installOpenlibResultCapture(L *LState, host *openlibRuntimeHost) func() {
	prevResult := host.tosTable.RawGetString("result")
	host.hasResult = false
	host.capturedResult = ""
	host.tosTable.RawSetString("result", L.NewFunction(func(L *LState) int {
		encFn := L.GetGlobal("__tol_abi_encode_v2")
		if encFn == LNil {
			L.RaiseError("tos.result: __tol_abi_encode_v2 not available")
			return 0
		}
		nargs := L.GetTop()
		if nargs == 0 || nargs%2 != 0 {
			L.RaiseError("tos.result: expected pairs of type and value")
			return 0
		}
		base := L.GetTop()
		args := make([]LValue, 0, nargs)
		for i := 1; i <= nargs; i += 2 {
			typ := L.CheckAny(i)
			val := L.CheckAny(i + 1)
			args = append(args, val, typ)
		}
		if err := L.CallByParam(P{Fn: encFn, NRet: 1, Protect: true}, args...); err != nil {
			L.SetTop(base)
			L.RaiseError("tos.result: %v", err)
			return 0
		}
		ret := L.Get(-1)
		L.SetTop(base)
		host.capturedResult = LVAsString(ret)
		host.hasResult = true
		ud := L.NewUserData()
		ud.Value = openlibResultSentinel
		L.Error(ud, 0)
		return 0
	}))
	return func() {
		host.tosTable.RawSetString("result", prevResult)
	}
}

func openlibSetUnoValue(host *openlibRuntimeHost, value LValue) {
	host.msgTable.RawSetString("uno_value", value)
}

func openlibSetNativeUnoBalance(host *openlibRuntimeHost, addr string, value int) {
	openlibRememberAgentString(host, addr)
	host.nativeUnoBalances[addr] = big.NewInt(int64(value))
}

func openlibSetTimestamp(host *openlibRuntimeHost, value int) {
	host.blockTable.RawSetString("timestamp_ms", lu256FromInt(value))
}

func openlibSetAgentProp(host *openlibRuntimeHost, addr string, field string, value LValue) {
	props := host.agentProps[addr]
	if props == nil {
		props = map[string]LValue{}
		host.agentProps[addr] = props
	}
	props[field] = value
}

func openlibRememberAgentValue(host *openlibRuntimeHost, value LValue) {
	if host == nil {
		return
	}
	if s, ok := value.(LString); ok {
		openlibRememberAgentString(host, string(s))
	}
}

func openlibRememberAgentString(host *openlibRuntimeHost, addr string) {
	if host == nil {
		return
	}
	if !strings.HasPrefix(addr, "0x") || len(addr) != 66 {
		return
	}
	props := host.agentProps[addr]
	if props == nil {
		props = map[string]LValue{}
		host.agentProps[addr] = props
	}
	if _, ok := props["suspended"]; !ok {
		props["suspended"] = lu256FromInt(0)
	}
	if _, ok := props["stake"]; !ok {
		props["stake"] = lu256FromInt(0)
	}
	if _, ok := props["reputation"]; !ok {
		props["reputation"] = lu256FromInt(0)
	}
}

// snapshotLuaStorage deep-copies the __tol_storage and __tol_transient_storage
// Lua tables, simulating the StateDB snapshot that the on-chain LVM takes
// before every top-level call or nested tos.call.
func snapshotLuaStorage(L *LState) (storage map[string]LValue, transient map[string]LValue) {
	storage = make(map[string]LValue)
	transient = make(map[string]LValue)
	if tbl, ok := L.GetGlobal("__tol_storage").(*LTable); ok {
		tbl.ForEach(func(k, v LValue) {
			storage[LVAsString(k)] = v
		})
	}
	if tbl, ok := L.GetGlobal("__tol_transient_storage").(*LTable); ok {
		tbl.ForEach(func(k, v LValue) {
			transient[LVAsString(k)] = v
		})
	}
	return
}

// revertLuaStorage restores __tol_storage and __tol_transient_storage to a
// previously captured snapshot, simulating the StateDB revert that the on-chain
// LVM performs when a call reverts.
func revertLuaStorage(L *LState, storageSnap, transientSnap map[string]LValue) {
	if tbl, ok := L.GetGlobal("__tol_storage").(*LTable); ok {
		// Collect all current keys, then remove any not in snapshot.
		var keys []string
		tbl.ForEach(func(k, _ LValue) {
			keys = append(keys, LVAsString(k))
		})
		for _, k := range keys {
			if _, exists := storageSnap[k]; !exists {
				tbl.RawSetString(k, LNil)
			}
		}
		// Restore snapshot values.
		for k, v := range storageSnap {
			tbl.RawSetString(k, v)
		}
	}
	if tbl, ok := L.GetGlobal("__tol_transient_storage").(*LTable); ok {
		var keys []string
		tbl.ForEach(func(k, _ LValue) {
			keys = append(keys, LVAsString(k))
		})
		for _, k := range keys {
			if _, exists := transientSnap[k]; !exists {
				tbl.RawSetString(k, LNil)
			}
		}
		for k, v := range transientSnap {
			tbl.RawSetString(k, v)
		}
	}
}

func invokeOpenlib(t *testing.T, L *LState, tos LValue, fnSig string, args ...LValue) LValue {
	t.Helper()

	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatal("tos.oninvoke not set")
	}

	// Snapshot storage before the call — if the call reverts, we restore.
	storageSnap, transientSnap := snapshotLuaStorage(L)
	hostSnap := snapshotRuntimeHost(hostFromTables(L))

	base := L.GetTop()
	prevCalldata := L.GetField(tos, "calldata")
	L.SetField(tos, "calldata", LNil)
	defer L.SetField(tos, "calldata", prevCalldata)
	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature(fnSig)))
	for _, arg := range args {
		openlibRememberAgentValue(hostFromTables(L), arg)
		L.Push(arg)
	}
	if err := L.PCall(1+len(args), MultRet, nil); err != nil {
		revertLuaStorage(L, storageSnap, transientSnap)
		restoreRuntimeHost(hostFromTables(L), hostSnap)
		t.Fatalf("invoke %s failed: %v", fnSig, err)
	}

	if L.GetTop() == base {
		return LNil
	}
	ret := L.Get(-1)
	L.SetTop(base)
	return ret
}

func invokeOpenlibMulti(t *testing.T, L *LState, tos LValue, fnSig string, args ...LValue) []LValue {
	t.Helper()

	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatal("tos.oninvoke not set")
	}

	storageSnap, transientSnap := snapshotLuaStorage(L)
	hostSnap := snapshotRuntimeHost(hostFromTables(L))

	base := L.GetTop()
	prevCalldata := L.GetField(tos, "calldata")
	L.SetField(tos, "calldata", LNil)
	defer L.SetField(tos, "calldata", prevCalldata)
	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature(fnSig)))
	for _, arg := range args {
		openlibRememberAgentValue(hostFromTables(L), arg)
		L.Push(arg)
	}
	if err := L.PCall(1+len(args), MultRet, nil); err != nil {
		revertLuaStorage(L, storageSnap, transientSnap)
		restoreRuntimeHost(hostFromTables(L), hostSnap)
		t.Fatalf("invoke %s failed: %v", fnSig, err)
	}

	n := L.GetTop() - base
	rets := make([]LValue, 0, n)
	for i := 0; i < n; i++ {
		rets = append(rets, L.Get(base+1+i))
	}
	L.SetTop(base)
	return rets
}

func invokeOpenlibErr(t *testing.T, L *LState, tos LValue, fnSig string, args ...LValue) string {
	t.Helper()

	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatal("tos.oninvoke not set")
	}

	// Snapshot storage — this simulates the StateDB snapshot taken by
	// LVM.Call before execution.  On revert the snapshot is restored,
	// rolling back any __tol_storage mutations made before the error.
	storageSnap, transientSnap := snapshotLuaStorage(L)
	hostSnap := snapshotRuntimeHost(hostFromTables(L))

	base := L.GetTop()
	prevCalldata := L.GetField(tos, "calldata")
	L.SetField(tos, "calldata", LNil)
	defer L.SetField(tos, "calldata", prevCalldata)
	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature(fnSig)))
	for _, arg := range args {
		openlibRememberAgentValue(hostFromTables(L), arg)
		L.Push(arg)
	}
	err := L.PCall(1+len(args), MultRet, nil)
	L.SetTop(base)
	if err == nil {
		t.Fatalf("expected error for %s", fnSig)
	}
	// Revert storage to pre-call state — matches on-chain behavior where
	// a reverted transaction's StateDB snapshot is restored.
	revertLuaStorage(L, storageSnap, transientSnap)
	restoreRuntimeHost(hostFromTables(L), hostSnap)
	return extractApiRevertMsg(err)
}

func hostFromTables(L *LState) *openlibRuntimeHost {
	// The test harness always installs the same table objects once per state.
	// We stash the host pointer in a lightuserdata-like global table slot by name.
	if hv := L.GetGlobal("__openlib_runtime_host"); hv != LNil {
		if ud, ok := hv.(*LUserData); ok {
			if host, ok := ud.Value.(*openlibRuntimeHost); ok {
				return host
			}
		}
	}
	return nil
}

func TestPolicyAccountRuntimeDelegateAndSuspension(t *testing.T) {
	L, tos, host := deployOpenlibContract(
		t,
		"openlib/account/PolicyAccount.tol",
		LString(alice),
		LString(bob),
		lu256FromInt(1000),
		lu256FromInt(400),
	)
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "setAllowlistEnabled(bool)", LTrue)
	invokeOpenlib(t, L, tos, "setAllowlisted(agent,bool)", LString(openlibMerchant), LTrue)
	invokeOpenlib(t, L, tos, "authorizeDelegate(agent,u256,u256)", LString(charlie), lu256FromInt(300), lu256FromInt(1000))

	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 100)
	if got := invokeOpenlib(t, L, tos, "execute(agent,bytes,u256)", LString(openlibMerchant), LString("0x1234"), lu256FromInt(200)); !LVAsBool(got) {
		t.Fatalf("execute should return true, got %v", got)
	}
	if host.lastCallAddr != openlibMerchant {
		t.Fatalf("host call addr: got=%q want=%q", host.lastCallAddr, openlibMerchant)
	}
	if host.lastCallData != "0x1234" {
		t.Fatalf("host call data: got=%q want=%q", host.lastCallData, "0x1234")
	}
	if host.lastCallCost != "200" {
		t.Fatalf("host call value: got=%q want=%q", host.lastCallCost, "200")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingDaily()")); got != "800" {
		t.Fatalf("remainingDaily: got=%s want=800", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "delegateRemaining(agent)", LString(charlie))); got != "100" {
		t.Fatalf("delegateRemaining: got=%s want=100", got)
	}

	errMsg := invokeOpenlibErr(t, L, tos, "execute(agent,bytes,u256)", LString(openlibMerchant), LString("0xab"), lu256FromInt(150))
	if !strings.Contains(errMsg, "OVER_ALLOWANCE") {
		t.Fatalf("expected OVER_ALLOWANCE, got %q", errMsg)
	}

	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "suspend()")
	if got := invokeOpenlib(t, L, tos, "isSuspended()"); !LVAsBool(got) {
		t.Fatal("isSuspended should be true after guardian suspend")
	}
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "unsuspend()")
	if got := invokeOpenlib(t, L, tos, "isSuspended()"); LVAsBool(got) {
		t.Fatal("isSuspended should be false after owner unsuspend")
	}
}

func TestAgentCastRuntimeZeroAndRegistrySemantics(t *testing.T) {
	const source = `pragma tolang 0.4.0;

contract AgentCastProbe {
    agent owner;

    constructor(agent _owner) {
        require(_owner != agent(0), "ZERO_OWNER");
        set owner = _owner;
    }

    function zero() public pure returns (agent out) {
        return agent(0);
    }

    function cast(agent who) public view returns (agent out) {
        return agent(who);
    }

    function callerAgent() public view returns (agent out) {
        return agent(msg.sender);
    }
}
`

	alice := openlibBytes32("a")
	bob := "0x00000000000000000000000000000000000000bb"
	zero := openlibBytes32("0")

	L, tos, host := deployOpenlibSourceContract(t, []byte(source), "<AgentCastProbe.tol>", LString(alice))
	defer L.Close()

	if got := LVAsString(invokeOpenlib(t, L, tos, "zero()")); got != zero {
		t.Fatalf("zero agent: got=%s want=%s", got, zero)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "cast(agent)", LString(alice))); got != alice {
		t.Fatalf("cast registered: got=%s want=%s", got, alice)
	}
	if errMsg := invokeOpenlibErr(t, L, tos, "cast(agent)", LString(bob)); !strings.Contains(errMsg, "AgentNotFound") {
		t.Fatalf("cast unregistered: got=%s want contains AgentNotFound", errMsg)
	}

	openlibSetSender(host, alice)
	if got := LVAsString(invokeOpenlib(t, L, tos, "callerAgent()")); got != alice {
		t.Fatalf("caller registered: got=%s want=%s", got, alice)
	}

	openlibSetSender(host, bob)
	if errMsg := invokeOpenlibErr(t, L, tos, "callerAgent()"); !strings.Contains(errMsg, "AgentNotFound") {
		t.Fatalf("caller unregistered: got=%s want contains AgentNotFound", errMsg)
	}
}

func TestAuthorityBookRuntimeGrantConsumeAndRevoke(t *testing.T) {
	scope := openlibBytes32("1")
	policy := openlibBytes32("2")
	binding := openlibBytes32("3")

	L, tos, host := deployOpenlibContract(t, "openlib/authority/AuthorityBook.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "grant(agent,bytes32,u256,u256,bytes32,u256)", LString(bob), LString(scope), lu256FromInt(500), lu256FromInt(1000), LString(policy), lu256FromInt(10))
	if got := invokeOpenlib(t, L, tos, "isActive(agent,bytes32)", LString(bob), LString(scope)); !LVAsBool(got) {
		t.Fatal("authority should be active after grant")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingOf(agent,bytes32)", LString(bob), LString(scope))); got != "500" {
		t.Fatalf("remainingOf: got=%s want=500", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "policyHashOf(agent,bytes32)", LString(bob), LString(scope))); got != policy {
		t.Fatalf("policyHashOf: got=%s want=%s", got, policy)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	if got := invokeOpenlib(t, L, tos, "consume(agent,bytes32,u256,u256,bytes32)", LString(bob), LString(scope), lu256FromInt(200), lu256FromInt(10), LString(binding)); !LVAsBool(got) {
		t.Fatalf("consume should return true, got %v", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingOf(agent,bytes32)", LString(bob), LString(scope))); got != "300" {
		t.Fatalf("remaining after consume: got=%s want=300", got)
	}

	errMsg := invokeOpenlibErr(t, L, tos, "consume(agent,bytes32,u256,u256,bytes32)", LString(bob), LString(scope), lu256FromInt(1), lu256FromInt(10), LString(binding))
	if !strings.Contains(errMsg, "NONCE_REPLAY") {
		t.Fatalf("expected NONCE_REPLAY, got %q", errMsg)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "revoke(agent,bytes32)", LString(bob), LString(scope))
	if got := invokeOpenlib(t, L, tos, "isActive(agent,bytes32)", LString(bob), LString(scope)); LVAsBool(got) {
		t.Fatal("authority should be inactive after revoke")
	}
}

func TestExecutionBindingBookRuntimeApproveAndConsume(t *testing.T) {
	bindingID := openlibBytes32("4")
	policy := openlibBytes32("5")
	sponsorPolicy := openlibBytes32("6")
	proof := openlibBytes32("7")
	intent := openlibBytes32("8")
	receipt := openlibBytes32("9")

	L, tos, host := deployOpenlibContract(t, "openlib/execution_binding/ExecutionBindingBook.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(
		t,
		L,
		tos,
		"approve(bytes32,agent,agent,u256,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(bindingID),
		LString(bob),
		LString(openlibMerchant),
		lu256FromInt(250),
		lu256FromInt(1000),
		LString(policy),
		LString(sponsorPolicy),
		LString(proof),
		LString(intent),
	)
	if got := invokeOpenlib(t, L, tos, "isConsumable(bytes32)", LString(bindingID)); !LVAsBool(got) {
		t.Fatal("binding should be consumable after approve")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "executorOf(bytes32)", LString(bindingID))); got != bob {
		t.Fatalf("executorOf: got=%s want=%s", got, bob)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "policyHashOf(bytes32)", LString(bindingID))); got != policy {
		t.Fatalf("policyHashOf: got=%s want=%s", got, policy)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "proofRefOf(bytes32)", LString(bindingID))); got != proof {
		t.Fatalf("proofRefOf: got=%s want=%s", got, proof)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	if got := invokeOpenlib(t, L, tos, "consume(bytes32,u256,bytes32)", LString(bindingID), lu256FromInt(200), LString(receipt)); !LVAsBool(got) {
		t.Fatalf("consume should return true, got %v", got)
	}
	if got := invokeOpenlib(t, L, tos, "isConsumable(bytes32)", LString(bindingID)); LVAsBool(got) {
		t.Fatal("binding should not be consumable after consume")
	}
	errMsg := invokeOpenlibErr(t, L, tos, "consume(bytes32,u256,bytes32)", LString(bindingID), lu256FromInt(1), LString(receipt))
	if !strings.Contains(errMsg, "ALREADY_CONSUMED") {
		t.Fatalf("expected ALREADY_CONSUMED, got %q", errMsg)
	}
}

func TestSessionBookRuntimeStepUpAndRevoke(t *testing.T) {
	sessionID := openlibBytes32("a")
	terminalID := openlibBytes32("b")

	L, tos, host := deployOpenlibContract(t, "openlib/session/SessionBook.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(
		t,
		L,
		tos,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(sessionID),
		LString(bob),
		LString(terminalID),
		lu256FromInt(1),
		lu256FromInt(2),
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(100),
		LFalse,
	)
	if got := invokeOpenlib(t, L, tos, "requiresStepUp(bytes32,u256)", LString(sessionID), lu256FromInt(50)); LVAsBool(got) {
		t.Fatal("50 should not require step up")
	}
	if got := invokeOpenlib(t, L, tos, "requiresStepUp(bytes32,u256)", LString(sessionID), lu256FromInt(150)); !LVAsBool(got) {
		t.Fatal("150 should require step up")
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	if got := invokeOpenlib(t, L, tos, "consume(bytes32,u256,bool)", LString(sessionID), lu256FromInt(80), LFalse); !LVAsBool(got) {
		t.Fatalf("consume under threshold should return true, got %v", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingOf(bytes32)", LString(sessionID))); got != "420" {
		t.Fatalf("remaining after first spend: got=%s want=420", got)
	}
	errMsg := invokeOpenlibErr(t, L, tos, "consume(bytes32,u256,bool)", LString(sessionID), lu256FromInt(150), LFalse)
	if !strings.Contains(errMsg, "STEP_UP_REQUIRED") {
		t.Fatalf("expected STEP_UP_REQUIRED, got %q", errMsg)
	}
	if got := invokeOpenlib(t, L, tos, "consume(bytes32,u256,bool)", LString(sessionID), lu256FromInt(150), LTrue); !LVAsBool(got) {
		t.Fatalf("consume with step up should return true, got %v", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingOf(bytes32)", LString(sessionID))); got != "270" {
		t.Fatalf("remaining after second spend: got=%s want=270", got)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "revokeSession(bytes32)", LString(sessionID))
	if got := invokeOpenlib(t, L, tos, "isActive(bytes32)", LString(sessionID)); LVAsBool(got) {
		t.Fatal("session should be inactive after revoke")
	}
}

func TestReceiptBookRuntimeLifecycle(t *testing.T) {
	receiptID := openlibBytes32("c")
	policy := openlibBytes32("d")
	binding := openlibBytes32("e")
	proof := openlibBytes32("f")
	external := openlibBytes32("1")
	result := openlibBytes32("2")
	settlement := openlibBytes32("3")
	failedReceipt := openlibBytes32("4")
	failedResult := openlibBytes32("5")

	L, tos, host := deployOpenlibContract(t, "openlib/receipt/ReceiptBook.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(
		t,
		L,
		tos,
		"openReceipt(bytes32,agent,agent,agent,agent,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(receiptID),
		LString(alice),
		LString(charlie),
		LString(bob),
		LString(openlibMerchant),
		lu256FromInt(77),
		LString(policy),
		LString(binding),
		LString(proof),
		LString(external),
	)
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(receiptID))); got != "1" {
		t.Fatalf("status after open: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "bindingRefOf(bytes32)", LString(receiptID))); got != binding {
		t.Fatalf("bindingRefOf: got=%s want=%s", got, binding)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "proofRefOf(bytes32)", LString(receiptID))); got != proof {
		t.Fatalf("proofRefOf: got=%s want=%s", got, proof)
	}
	if got := invokeOpenlib(t, L, tos, "isFinalized(bytes32)", LString(receiptID)); LVAsBool(got) {
		t.Fatal("receipt should not be finalized while open")
	}

	invokeOpenlib(t, L, tos, "finalizeSuccess(bytes32,bytes32,bytes32)", LString(receiptID), LString(result), LString(settlement))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(receiptID))); got != "2" {
		t.Fatalf("status after success: got=%s want=2", got)
	}
	if got := invokeOpenlib(t, L, tos, "isFinalized(bytes32)", LString(receiptID)); !LVAsBool(got) {
		t.Fatal("receipt should be finalized after success")
	}

	invokeOpenlib(
		t,
		L,
		tos,
		"openReceipt(bytes32,agent,agent,agent,agent,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(failedReceipt),
		LString(alice),
		LString(charlie),
		LString(bob),
		LString(openlibMerchant),
		lu256FromInt(1),
		LString(policy),
		LString(binding),
		LString(proof),
		LString(external),
	)
	invokeOpenlib(t, L, tos, "finalizeFailure(bytes32,bytes32)", LString(failedReceipt), LString(failedResult))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(failedReceipt))); got != "3" {
		t.Fatalf("status after failure: got=%s want=3", got)
	}
}

func TestSponsorPolicyRelayRuntimeDepositRelayAndWithdraw(t *testing.T) {
	policy := openlibBytes32("6")
	binding := openlibBytes32("7")
	receipt := openlibBytes32("8")
	orderID := openlibBytes32("9")
	sponsorAddr := openlibBytes32("a")

	L, tos, host := deployOpenlibContract(t, "openlib/sponsor/SponsorPolicyRelay.tol", LString(alice))
	defer L.Close()

	targetL, targetTOS, targetHost := deployOpenlibSourceContract(t, []byte(openlibCallTargetSource), "<openlib-call-target>")
	defer targetL.Close()
	targetDep := &openlibDeployedPackageContract{name: "CallTargetRecorder", addr: openlibService, L: targetL, tos: targetTOS, host: targetHost}
	attachActualCallRouter(t, host, sponsorAddr, targetDep)

	openlibSetSender(host, alice)
	openlibSetValue(host, 1000)
	invokeOpenlib(t, L, tos, "deposit()")
	openlibSetValue(host, 0)
	invokeOpenlib(t, L, tos, "authorizeRelayer(agent,u256,u256,bytes32)", LString(bob), lu256FromInt(400), lu256FromInt(1000), LString(policy))
	if got := invokeOpenlib(t, L, tos, "isRelayerActive(agent)", LString(bob)); !LVAsBool(got) {
		t.Fatal("relayer should be active after authorization")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingOf(agent)", LString(bob))); got != "400" {
		t.Fatalf("remainingOf: got=%s want=400", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "policyHashOf(agent)", LString(bob))); got != policy {
		t.Fatalf("policyHashOf: got=%s want=%s", got, policy)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	if got := invokeOpenlib(
		t,
		L,
		tos,
		"relay(agent,bytes,agent,u256,bytes32,bytes32,bytes32)",
		LString(openlibService),
		LString(openlibEncodeStaticCalldata("record(bytes32,u256)", orderID, "fa")),
		LString(charlie),
		lu256FromInt(250),
		LString(policy),
		LString(binding),
		LString(receipt),
	); !LVAsBool(got) {
		t.Fatalf("relay should return true, got %v", got)
	}
	if host.lastCallAddr != openlibService {
		t.Fatalf("host call addr: got=%q want=%q", host.lastCallAddr, openlibService)
	}
	if host.lastCallData != openlibEncodeStaticCalldata("record(bytes32,u256)", orderID, "fa") {
		t.Fatalf("host call data: got=%q want=%q", host.lastCallData, openlibEncodeStaticCalldata("record(bytes32,u256)", orderID, "fa"))
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingOf(agent)", LString(bob))); got != "150" {
		t.Fatalf("remaining after relay: got=%s want=150", got)
	}
	if got := LVAsString(invokeOpenlib(t, targetL, targetTOS, "lastOrderId()")); got != orderID {
		t.Fatalf("target lastOrderId: got=%s want=%s", got, orderID)
	}
	if got := LVAsString(invokeOpenlib(t, targetL, targetTOS, "lastAmount()")); got != "250" {
		t.Fatalf("target lastAmount: got=%s want=250", got)
	}
	if got := LVAsString(invokeOpenlib(t, targetL, targetTOS, "callCount()")); got != "1" {
		t.Fatalf("target callCount: got=%s want=1", got)
	}
	errMsg := invokeOpenlibErr(
		t,
		L,
		tos,
		"relay(agent,bytes,agent,u256,bytes32,bytes32,bytes32)",
		LString(openlibService),
		LString("0xcafe"),
		LString(charlie),
		lu256FromInt(200),
		LString(policy),
		LString(binding),
		LString(receipt),
	)
	if !strings.Contains(errMsg, "OVER_BUDGET") {
		t.Fatalf("expected OVER_BUDGET, got %q", errMsg)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "withdraw(u256)", lu256FromInt(300))
	if host.lastReleaseAddr != alice {
		t.Fatalf("release addr: got=%q want=%q", host.lastReleaseAddr, alice)
	}
	if host.lastReleaseAmt != "300" {
		t.Fatalf("release amount: got=%q want=%q", host.lastReleaseAmt, "300")
	}
}

func TestEvidenceBookRuntimeChallengeAndFinalize(t *testing.T) {
	evidenceID1 := openlibBytes32("9")
	evidenceID2 := openlibBytes32("a")
	claim1 := openlibBytes32("b")
	claim2 := openlibBytes32("c")
	proof1 := openlibBytes32("d")
	proof2 := openlibBytes32("e")
	challenge := openlibBytes32("f")

	L, tos, host := deployOpenlibContract(t, "openlib/evidence/EvidenceBook.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "setAttester(agent,bool)", LString(bob), LTrue)
	openlibSetTimestamp(host, 100)
	invokeOpenlib(t, L, tos, "openEvidence(bytes32,bytes32,agent,u256,u256)", LString(evidenceID1), LString(claim1), LString(bob), lu256FromInt(1000), lu256FromInt(50))

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 120)
	invokeOpenlib(t, L, tos, "fulfill(bytes32,u256,bytes32)", LString(evidenceID1), lu256FromInt(42), LString(proof1))
	if got := LVAsString(invokeOpenlib(t, L, tos, "readValue(bytes32)", LString(evidenceID1))); got != "42" {
		t.Fatalf("readValue after fulfill: got=%s want=42", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "proofRefOf(bytes32)", LString(evidenceID1))); got != proof1 {
		t.Fatalf("proofRefOf: got=%s want=%s", got, proof1)
	}

	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 160)
	invokeOpenlib(t, L, tos, "challenge(bytes32,bytes32)", LString(evidenceID1), LString(challenge))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(evidenceID1))); got != "3" {
		t.Fatalf("status after challenge: got=%s want=3", got)
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 200)
	invokeOpenlib(t, L, tos, "openEvidence(bytes32,bytes32,agent,u256,u256)", LString(evidenceID2), LString(claim2), LString(bob), lu256FromInt(1000), lu256FromInt(50))
	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 220)
	invokeOpenlib(t, L, tos, "fulfill(bytes32,u256,bytes32)", LString(evidenceID2), lu256FromInt(7), LString(proof2))
	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 271)
	invokeOpenlib(t, L, tos, "finalize(bytes32)", LString(evidenceID2))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(evidenceID2))); got != "4" {
		t.Fatalf("status after finalize: got=%s want=4", got)
	}
	if got := invokeOpenlib(t, L, tos, "isFinalized(bytes32)", LString(evidenceID2)); !LVAsBool(got) {
		t.Fatal("evidence should be finalized after finalize()")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "readValue(bytes32)", LString(evidenceID2))); got != "7" {
		t.Fatalf("readValue after finalize: got=%s want=7", got)
	}
}

func TestRecoveryControllerRuntimeGuardiansFreezeAndRecovery(t *testing.T) {
	reason := openlibBytes32("1")

	L, tos, host := deployOpenlibContract(
		t,
		"openlib/recovery/RecoveryController.tol",
		LString(alice),
		lu256FromInt(2),
		lu256FromInt(50),
	)
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "addGuardian(agent)", LString(bob))
	invokeOpenlib(t, L, tos, "addGuardian(agent)", LString(charlie))
	invokeOpenlib(t, L, tos, "freeze(bytes32)", LString(reason))
	if got := invokeOpenlib(t, L, tos, "isFrozen()"); !LVAsBool(got) {
		t.Fatal("controller should be frozen after freeze()")
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	invokeOpenlib(t, L, tos, "startRecovery(agent)", LString(openlibMerchant))
	if got := invokeOpenlib(t, L, tos, "isRecoveryActive()"); !LVAsBool(got) {
		t.Fatal("recovery should be active after startRecovery")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "approvalCount()")); got != "1" {
		t.Fatalf("approvalCount after start: got=%s want=1", got)
	}

	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "approveRecovery()")
	if got := LVAsString(invokeOpenlib(t, L, tos, "approvalCount()")); got != "2" {
		t.Fatalf("approvalCount after second guardian: got=%s want=2", got)
	}

	openlibSetTimestamp(host, 149)
	errMsg := invokeOpenlibErr(t, L, tos, "executeRecovery()")
	if !strings.Contains(errMsg, "TIMELOCK") {
		t.Fatalf("expected TIMELOCK, got %q", errMsg)
	}

	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "executeRecovery()")
	if got := LVAsString(invokeOpenlib(t, L, tos, "currentController()")); got != openlibMerchant {
		t.Fatalf("currentController after execute: got=%s want=%s", got, openlibMerchant)
	}
	if got := invokeOpenlib(t, L, tos, "isRecoveryActive()"); LVAsBool(got) {
		t.Fatal("recovery should be inactive after executeRecovery")
	}
	if got := invokeOpenlib(t, L, tos, "isFrozen()"); LVAsBool(got) {
		t.Fatal("recovery execution should clear frozen flag")
	}
}

func TestTrustRegistryRuntimeBondEligibilityAndOverride(t *testing.T) {
	subject := openlibService

	L, tos, host := deployOpenlibContract(
		t,
		"openlib/trust/TrustRegistry.tol",
		LString(alice),
		lu256FromInt(100),
		lu256FromInt(5),
	)
	defer L.Close()

	openlibSetAgentProp(host, subject, "stake", lu256FromInt(150))
	openlibSetAgentProp(host, subject, "suspended", lu256FromInt(0))

	// Reputation is now stored in the contract mapping (not agent props).
	// Owner must call updateReputation to set it.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "updateReputation(agent,i256,bytes32)", LString(subject), lu256FromInt(10), LString(openlibBytes32("0")))

	if got := invokeOpenlib(t, L, tos, "isEligible(agent)", LString(subject)); !LVAsBool(got) {
		t.Fatal("subject should be eligible with sufficient stake/reputation")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "snapshotStakeOf(agent)", LString(subject))); got != "150" {
		t.Fatalf("snapshotStakeOf: got=%s want=150", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "snapshotReputationOf(agent)", LString(subject))); got != "10" {
		t.Fatalf("snapshotReputationOf: got=%s want=10", got)
	}

	openlibSetSender(host, bob)
	openlibSetValue(host, 90)
	invokeOpenlib(t, L, tos, "depositBond()")
	openlibSetValue(host, 0)
	if got := LVAsString(invokeOpenlib(t, L, tos, "bondedAmountOf(agent)", LString(bob))); got != "90" {
		t.Fatalf("bondedAmountOf after deposit: got=%s want=90", got)
	}
	if host.lastEscrowAddr != bob || host.lastEscrowAmt != "90" {
		t.Fatalf("escrow call mismatch: addr=%q amt=%q", host.lastEscrowAddr, host.lastEscrowAmt)
	}
	invokeOpenlib(t, L, tos, "withdrawBond(u256)", lu256FromInt(40))
	if got := LVAsString(invokeOpenlib(t, L, tos, "bondedAmountOf(agent)", LString(bob))); got != "50" {
		t.Fatalf("bondedAmountOf after withdraw: got=%s want=50", got)
	}
	if host.lastReleaseAddr != bob || host.lastReleaseAmt != "40" {
		t.Fatalf("release call mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}

	openlibSetAgentProp(host, subject, "stake", lu256FromInt(10))
	if got := invokeOpenlib(t, L, tos, "isEligible(agent)", LString(subject)); LVAsBool(got) {
		t.Fatal("subject should be ineligible after stake drops below floor")
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "setOverride(agent,bool)", LString(subject), LTrue)
	if got := invokeOpenlib(t, L, tos, "isEligible(agent)", LString(subject)); !LVAsBool(got) {
		t.Fatal("manual override should force eligibility")
	}
}

func TestServiceDirectoryRuntimeRegisterUpdateAndDeactivate(t *testing.T) {
	manifest1 := openlibBytes32("2")
	capability1 := openlibBytes32("3")
	version1 := openlibBytes32("4")
	quote1 := openlibBytes32("5")
	manifest2 := openlibBytes32("6")
	capability2 := openlibBytes32("7")
	version2 := openlibBytes32("8")
	quote2 := openlibBytes32("9")

	L, tos, host := deployOpenlibContract(t, "openlib/discovery/ServiceDirectory.tol")
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(
		t,
		L,
		tos,
		"registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifest1),
		LString(capability1),
		LString(version1),
		LString(quote1),
	)
	if got := LVAsString(invokeOpenlib(t, L, tos, "providerOf(u256)", lu256FromInt(1))); got != alice {
		t.Fatalf("providerOf: got=%s want=%s", got, alice)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "manifestRefOf(u256)", lu256FromInt(1))); got != manifest1 {
		t.Fatalf("manifestRefOf: got=%s want=%s", got, manifest1)
	}
	if got := invokeOpenlib(t, L, tos, "isActive(u256)", lu256FromInt(1)); !LVAsBool(got) {
		t.Fatal("service should be active after registration")
	}

	invokeOpenlib(
		t,
		L,
		tos,
		"updateManifest(u256,bytes32,bytes32,bytes32)",
		lu256FromInt(1),
		LString(manifest2),
		LString(capability2),
		LString(version2),
	)
	invokeOpenlib(t, L, tos, "updateQuote(u256,bytes32)", lu256FromInt(1), LString(quote2))
	if got := LVAsString(invokeOpenlib(t, L, tos, "manifestRefOf(u256)", lu256FromInt(1))); got != manifest2 {
		t.Fatalf("manifestRefOf after update: got=%s want=%s", got, manifest2)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "capabilityRefOf(u256)", lu256FromInt(1))); got != capability2 {
		t.Fatalf("capabilityRefOf after update: got=%s want=%s", got, capability2)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "quoteRefOf(u256)", lu256FromInt(1))); got != quote2 {
		t.Fatalf("quoteRefOf after update: got=%s want=%s", got, quote2)
	}

	invokeOpenlib(t, L, tos, "deactivate(u256)", lu256FromInt(1))
	if got := invokeOpenlib(t, L, tos, "isActive(u256)", lu256FromInt(1)); LVAsBool(got) {
		t.Fatal("service should be inactive after deactivate")
	}
}

func TestCommercialAgreementRuntimeOpenAcceptFulfillAndExpire(t *testing.T) {
	quote1 := openlibBytes32("a")
	terms1 := openlibBytes32("b")
	acceptance1 := openlibBytes32("c")
	settlement1 := openlibBytes32("d")
	quote2 := openlibBytes32("e")
	terms2 := openlibBytes32("f")

	L, tos, host := deployOpenlibContract(t, "openlib/agreement/CommercialAgreement.tol")
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	invokeOpenlib(
		t,
		L,
		tos,
		"createOffer(agent,u256,u256,bytes32,bytes32)",
		LString(bob),
		lu256FromInt(250),
		lu256FromInt(500),
		LString(quote1),
		LString(terms1),
	)
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(1))); got != "1" {
		t.Fatalf("status after create: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "quoteRefOf(u256)", lu256FromInt(1))); got != quote1 {
		t.Fatalf("quoteRefOf: got=%s want=%s", got, quote1)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "accept(u256,bytes32)", lu256FromInt(1), LString(acceptance1))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(1))); got != "2" {
		t.Fatalf("status after accept: got=%s want=2", got)
	}
	invokeOpenlib(t, L, tos, "fulfill(u256,bytes32)", lu256FromInt(1), LString(settlement1))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(1))); got != "4" {
		t.Fatalf("status after fulfill: got=%s want=4", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "settlementRefOf(u256)", lu256FromInt(1))); got != settlement1 {
		t.Fatalf("settlementRefOf: got=%s want=%s", got, settlement1)
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 200)
	invokeOpenlib(
		t,
		L,
		tos,
		"createOffer(agent,u256,u256,bytes32,bytes32)",
		LString(charlie),
		lu256FromInt(10),
		lu256FromInt(220),
		LString(quote2),
		LString(terms2),
	)
	openlibSetTimestamp(host, 220)
	invokeOpenlib(t, L, tos, "expire(u256)", lu256FromInt(2))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(2))); got != "5" {
		t.Fatalf("status after expire: got=%s want=5", got)
	}
}

func TestTaskSettlementRuntimeLifecycleAndDispute(t *testing.T) {
	taskRef1 := openlibBytes32("1")
	receiptRef1 := openlibBytes32("2")
	resultRef1 := openlibBytes32("3")
	proofRef1 := openlibBytes32("4")
	settlementRef1 := openlibBytes32("5")
	taskRef2 := openlibBytes32("6")
	receiptRef2 := openlibBytes32("7")
	resultRef2 := openlibBytes32("8")
	proofRef2 := openlibBytes32("9")
	disputeProof2 := openlibBytes32("a")
	settlementRef2 := openlibBytes32("b")
	taskRef3 := openlibBytes32("c")
	receiptRef3 := openlibBytes32("d")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/TaskSettlement.tol", LString(charlie))
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 100)
	invokeOpenlib(
		t,
		L,
		tos,
		"openTask(bytes32,u256,bytes32)",
		LString(taskRef1),
		lu256FromInt(500),
		LString(receiptRef1),
	)
	openlibSetValue(host, 0)
	if host.lastEscrowAddr != alice || host.lastEscrowAmt != "100" {
		t.Fatalf("openTask escrow mismatch: addr=%q amt=%q", host.lastEscrowAddr, host.lastEscrowAmt)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(1))); got != "1" {
		t.Fatalf("status task 1 after open: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "rewardOf(u256)", lu256FromInt(1))); got != "100" {
		t.Fatalf("rewardOf task 1: got=%s want=100", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "receiptRefOf(u256)", lu256FromInt(1))); got != receiptRef1 {
		t.Fatalf("receiptRefOf task 1: got=%s want=%s", got, receiptRef1)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(1))
	invokeOpenlib(t, L, tos, "submitTask(u256,bytes32,bytes32)", lu256FromInt(1), LString(resultRef1), LString(proofRef1))
	if got := LVAsString(invokeOpenlib(t, L, tos, "proofRefOf(u256)", lu256FromInt(1))); got != proofRef1 {
		t.Fatalf("proofRefOf task 1: got=%s want=%s", got, proofRef1)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "approveTask(u256,bytes32)", lu256FromInt(1), LString(settlementRef1))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(1))); got != "4" {
		t.Fatalf("status task 1 after approve: got=%s want=4", got)
	}
	if host.lastReleaseAddr != bob || host.lastReleaseAmt != "100" {
		t.Fatalf("approveTask release mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 200)
	openlibSetValue(host, 70)
	invokeOpenlib(
		t,
		L,
		tos,
		"openTask(bytes32,u256,bytes32)",
		LString(taskRef2),
		lu256FromInt(700),
		LString(receiptRef2),
	)
	openlibSetValue(host, 0)
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(2))); got != "1" {
		t.Fatalf("status task 2 after open: got=%s want=1", got)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 250)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(2))
	invokeOpenlib(t, L, tos, "submitTask(u256,bytes32,bytes32)", lu256FromInt(2), LString(resultRef2), LString(proofRef2))

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "rejectTask(u256,bytes32)", lu256FromInt(2), LString(openlibBytes32("e")))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(2))); got != "5" {
		t.Fatalf("status task 2 after reject: got=%s want=5", got)
	}

	openlibSetSender(host, bob)
	openlibSetValue(host, 30)
	invokeOpenlib(t, L, tos, "disputeTask(u256,bytes32)", lu256FromInt(2), LString(disputeProof2))
	openlibSetValue(host, 0)
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(2))); got != "6" {
		t.Fatalf("status task 2 after dispute: got=%s want=6", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "proofRefOf(u256)", lu256FromInt(2))); got != disputeProof2 {
		t.Fatalf("proofRefOf task 2 after dispute: got=%s want=%s", got, disputeProof2)
	}
	if host.lastEscrowAddr != bob || host.lastEscrowAmt != "30" {
		t.Fatalf("disputeTask escrow mismatch: addr=%q amt=%q", host.lastEscrowAddr, host.lastEscrowAmt)
	}

	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "resolveDispute(u256,bool,bytes32)", lu256FromInt(2), LFalse, LString(settlementRef2))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(2))); got != "7" {
		t.Fatalf("status task 2 after resolve: got=%s want=7", got)
	}
	if host.lastReleaseAddr != alice || host.lastReleaseAmt != "100" {
		t.Fatalf("resolveDispute release mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 300)
	openlibSetValue(host, 40)
	invokeOpenlib(
		t,
		L,
		tos,
		"openTask(bytes32,u256,bytes32)",
		LString(taskRef3),
		lu256FromInt(320),
		LString(receiptRef3),
	)
	openlibSetValue(host, 0)
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(3))); got != "1" {
		t.Fatalf("status task 3 after open: got=%s want=1", got)
	}

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 301)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(3))
	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 321)
	invokeOpenlib(t, L, tos, "reclaimExpired(u256)", lu256FromInt(3))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(3))); got != "8" {
		t.Fatalf("status task 3 after reclaim: got=%s want=8", got)
	}
	if host.lastReleaseAddr != alice || host.lastReleaseAmt != "40" {
		t.Fatalf("reclaimExpired release mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}
}

func TestConfidentialVaultRuntimeDepositWithdrawAndDisclosure(t *testing.T) {
	pubkey := openlibBytes32("d")
	disclosureRef := openlibBytes32("e")

	L, tos, host := deployOpenlibContract(t, "openlib/privacy/ConfidentialVault.tol")
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "registerPublicKey(bytes32)", LString(pubkey))
	openlibSetUnoValue(host, openlibUnoFromInt(50))
	invokeOpenlib(t, L, tos, "deposit()")
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	if got := LVAsString(invokeOpenlib(t, L, tos, "balanceOf(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(50)) {
		t.Fatalf("balanceOf after deposit: got=%s want=%s", got, LVAsString(openlibUnoFromInt(50)))
	}

	invokeOpenlib(t, L, tos, "authorizeAuditor(agent,bytes32)", LString(bob), LString(disclosureRef))
	if got := invokeOpenlib(t, L, tos, "canAudit(agent,agent)", LString(alice), LString(bob)); !LVAsBool(got) {
		t.Fatal("auditor should be authorized")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "disclosureRefOf(agent,agent)", LString(alice), LString(bob))); got != disclosureRef {
		t.Fatalf("disclosureRefOf: got=%s want=%s", got, disclosureRef)
	}

	openlibSetNativeUnoBalance(host, bob, 12)
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(bob))); got != LVAsString(openlibUnoFromInt(12)) {
		t.Fatalf("nativeBalance before withdraw: got=%s want=%s", got, LVAsString(openlibUnoFromInt(12)))
	}

	invokeOpenlib(t, L, tos, "withdraw(uno)", openlibUnoFromInt(30))
	if got := LVAsString(invokeOpenlib(t, L, tos, "balanceOf(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(20)) {
		t.Fatalf("balanceOf after withdraw: got=%s want=%s", got, LVAsString(openlibUnoFromInt(20)))
	}
	if host.lastUnoTransferAddr != alice || host.lastUnoTransferAmount != LVAsString(openlibUnoFromInt(30)) {
		t.Fatalf("uno.transfer mismatch: addr=%q amount=%q", host.lastUnoTransferAddr, host.lastUnoTransferAmount)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(30)) {
		t.Fatalf("nativeBalance after withdraw: got=%s want=%s", got, LVAsString(openlibUnoFromInt(30)))
	}

	errMsg := invokeOpenlibErr(t, L, tos, "withdraw(uno)", openlibUnoFromInt(25))
	if !strings.Contains(errMsg, "INSUFFICIENT_BALANCE") {
		t.Fatalf("expected INSUFFICIENT_BALANCE, got %q", errMsg)
	}

	invokeOpenlib(t, L, tos, "revokeAuditor(agent)", LString(bob))
	if got := invokeOpenlib(t, L, tos, "canAudit(agent,agent)", LString(alice), LString(bob)); LVAsBool(got) {
		t.Fatal("auditor should be revoked")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "disclosureRefOf(agent,agent)", LString(alice), LString(bob))); got != openlibBytes32("0") {
		t.Fatalf("disclosureRefOf after revoke: got=%s want=%s", got, openlibBytes32("0"))
	}
}

func TestConfidentialEscrowRuntimeOpenReleaseRefundAndReclaim(t *testing.T) {
	escrowOne := openlibBytes32("1")
	escrowTwo := openlibBytes32("2")
	escrowThree := openlibBytes32("3")
	receiptOne := openlibBytes32("4")
	receiptTwo := openlibBytes32("5")
	receiptThree := openlibBytes32("6")
	settlementRef := openlibBytes32("7")
	reasonRef := openlibBytes32("8")

	L, tos, host := deployOpenlibContract(t, "openlib/privacy/ConfidentialEscrow.tol", LString(charlie))
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	openlibSetUnoValue(host, openlibUnoFromInt(50))
	invokeOpenlib(t, L, tos, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowOne), LString(bob), lu256FromInt(500), LString(receiptOne))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(escrowOne))); got != "1" {
		t.Fatalf("status escrow one after open: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "payerOf(bytes32)", LString(escrowOne))); got != alice {
		t.Fatalf("payerOf escrow one: got=%s want=%s", got, alice)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "payeeOf(bytes32)", LString(escrowOne))); got != bob {
		t.Fatalf("payeeOf escrow one: got=%s want=%s", got, bob)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "receiptRefOf(bytes32)", LString(escrowOne))); got != receiptOne {
		t.Fatalf("receiptRefOf escrow one: got=%s want=%s", got, receiptOne)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "amountOf(bytes32)", LString(escrowOne))); got != LVAsString(openlibUnoFromInt(50)) {
		t.Fatalf("amountOf escrow one: got=%s want=%s", got, LVAsString(openlibUnoFromInt(50)))
	}

	openlibSetSender(host, bob)
	errMsg := invokeOpenlibErr(t, L, tos, "releaseEscrow(bytes32,bytes32)", LString(escrowOne), LString(settlementRef))
	if !strings.Contains(errMsg, "NOT_AUTHORIZED") {
		t.Fatalf("expected NOT_AUTHORIZED, got %q", errMsg)
	}

	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "releaseEscrow(bytes32,bytes32)", LString(escrowOne), LString(settlementRef))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(escrowOne))); got != "2" {
		t.Fatalf("status escrow one after release: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(bob))); got != LVAsString(openlibUnoFromInt(50)) {
		t.Fatalf("nativeBalance bob after release: got=%s want=%s", got, LVAsString(openlibUnoFromInt(50)))
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 150)
	openlibSetUnoValue(host, openlibUnoFromInt(20))
	invokeOpenlib(t, L, tos, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowTwo), LString(bob), lu256FromInt(0), LString(receiptTwo))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	invokeOpenlib(t, L, tos, "refundEscrow(bytes32,bytes32)", LString(escrowTwo), LString(reasonRef))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(escrowTwo))); got != "3" {
		t.Fatalf("status escrow two after refund: got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(20)) {
		t.Fatalf("nativeBalance alice after refund: got=%s want=%s", got, LVAsString(openlibUnoFromInt(20)))
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 200)
	openlibSetUnoValue(host, openlibUnoFromInt(15))
	invokeOpenlib(t, L, tos, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowThree), LString(bob), lu256FromInt(210), LString(receiptThree))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	openlibSetTimestamp(host, 205)
	errMsg = invokeOpenlibErr(t, L, tos, "reclaimExpired(bytes32)", LString(escrowThree))
	if !strings.Contains(errMsg, "NOT_EXPIRED") {
		t.Fatalf("expected NOT_EXPIRED, got %q", errMsg)
	}
	openlibSetTimestamp(host, 210)
	invokeOpenlib(t, L, tos, "reclaimExpired(bytes32)", LString(escrowThree))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(escrowThree))); got != "3" {
		t.Fatalf("status escrow three after reclaim: got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(35)) {
		t.Fatalf("nativeBalance alice after reclaim: got=%s want=%s", got, LVAsString(openlibUnoFromInt(35)))
	}
}

func TestConfidentialPaymentRuntimeSingleBatchReleaseAndRefund(t *testing.T) {
	paymentOne := openlibBytes32("1")
	paymentTwo := openlibBytes32("2")
	batchOne := openlibBytes32("3")
	batchTwo := openlibBytes32("4")
	receiptOne := openlibBytes32("5")
	receiptTwo := openlibBytes32("6")
	receiptThree := openlibBytes32("7")
	receiptFour := openlibBytes32("8")
	settlementOne := openlibBytes32("9")
	settlementTwo := openlibBytes32("a")
	reasonOne := openlibBytes32("b")
	reasonTwo := openlibBytes32("c")

	L, tos, host := deployOpenlibContract(t, "openlib/privacy/ConfidentialPayment.tol", LString(charlie))
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetUnoValue(host, openlibUnoFromInt(40))
	invokeOpenlib(t, L, tos, "pay(bytes32,agent,bytes32)", LString(paymentOne), LString(bob), LString(receiptOne))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(paymentOne))); got != "1" {
		t.Fatalf("status payment one after pay: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "payerOf(bytes32)", LString(paymentOne))); got != alice {
		t.Fatalf("payerOf payment one: got=%s want=%s", got, alice)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "payeeOf(bytes32)", LString(paymentOne))); got != bob {
		t.Fatalf("payeeOf payment one: got=%s want=%s", got, bob)
	}

	openlibSetSender(host, bob)
	errMsg := invokeOpenlibErr(t, L, tos, "releasePayment(bytes32,bytes32)", LString(paymentOne), LString(settlementOne))
	if !strings.Contains(errMsg, "NOT_COORDINATOR") {
		t.Fatalf("expected NOT_COORDINATOR, got %q", errMsg)
	}

	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "releasePayment(bytes32,bytes32)", LString(paymentOne), LString(settlementOne))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(paymentOne))); got != "2" {
		t.Fatalf("status payment one after release: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(bob))); got != LVAsString(openlibUnoFromInt(40)) {
		t.Fatalf("nativeBalance bob after payment release: got=%s want=%s", got, LVAsString(openlibUnoFromInt(40)))
	}

	openlibSetSender(host, alice)
	openlibSetUnoValue(host, openlibUnoFromInt(15))
	invokeOpenlib(t, L, tos, "pay(bytes32,agent,bytes32)", LString(paymentTwo), LString(bob), LString(receiptTwo))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "refund(bytes32,bytes32)", LString(paymentTwo), LString(reasonOne))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(paymentTwo))); got != "3" {
		t.Fatalf("status payment two after refund: got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(15)) {
		t.Fatalf("nativeBalance alice after payment refund: got=%s want=%s", got, LVAsString(openlibUnoFromInt(15)))
	}

	openlibSetSender(host, alice)
	openlibSetUnoValue(host, openlibUnoFromInt(30))
	invokeOpenlib(t, L, tos, "batchPay(bytes32,bytes32)", LString(batchOne), LString(receiptThree))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "addPayee(bytes32,agent,uno)", LString(batchOne), LString(bob), openlibUnoFromInt(10))
	invokeOpenlib(t, L, tos, "addPayee(bytes32,agent,uno)", LString(batchOne), LString(openlibMerchant), openlibUnoFromInt(20))
	invokeOpenlib(t, L, tos, "releaseBatch(bytes32,bytes32)", LString(batchOne), LString(settlementTwo))
	if got := LVAsString(invokeOpenlib(t, L, tos, "batchStatusOf(bytes32)", LString(batchOne))); got != "2" {
		t.Fatalf("batchStatus batch one after release: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(bob))); got != LVAsString(openlibUnoFromInt(50)) {
		t.Fatalf("nativeBalance bob after batch release: got=%s want=%s", got, LVAsString(openlibUnoFromInt(50)))
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(openlibMerchant))); got != LVAsString(openlibUnoFromInt(20)) {
		t.Fatalf("nativeBalance merchant after batch release: got=%s want=%s", got, LVAsString(openlibUnoFromInt(20)))
	}

	openlibSetSender(host, alice)
	openlibSetUnoValue(host, openlibUnoFromInt(25))
	invokeOpenlib(t, L, tos, "batchPay(bytes32,bytes32)", LString(batchTwo), LString(receiptFour))
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "addPayee(bytes32,agent,uno)", LString(batchTwo), LString(bob), openlibUnoFromInt(10))
	errMsg = invokeOpenlibErr(t, L, tos, "releaseBatch(bytes32,bytes32)", LString(batchTwo), LString(settlementTwo))
	if !strings.Contains(errMsg, "BATCH_TOTAL_MISMATCH") {
		t.Fatalf("expected BATCH_TOTAL_MISMATCH, got %q", errMsg)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "batchStatusOf(bytes32)", LString(batchTwo))); got != "1" {
		t.Fatalf("batchStatus batch two after failed release: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(bob))); got != LVAsString(openlibUnoFromInt(50)) {
		t.Fatalf("nativeBalance bob should be unchanged after failed batch release: got=%s want=%s", got, LVAsString(openlibUnoFromInt(50)))
	}
	invokeOpenlib(t, L, tos, "refundBatch(bytes32,bytes32)", LString(batchTwo), LString(reasonTwo))
	if got := LVAsString(invokeOpenlib(t, L, tos, "batchStatusOf(bytes32)", LString(batchTwo))); got != "3" {
		t.Fatalf("batchStatus batch two after refund: got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(40)) {
		t.Fatalf("nativeBalance alice after batch refund: got=%s want=%s", got, LVAsString(openlibUnoFromInt(40)))
	}
}

func TestConfidentialTreasuryRuntimeSignerSpendAndAuditorLifecycle(t *testing.T) {
	spendOne := openlibBytes32("1")
	spendTwo := openlibBytes32("2")
	purposeOne := openlibBytes32("3")
	purposeTwo := openlibBytes32("4")
	settlementRef := openlibBytes32("5")
	disclosureRef := openlibBytes32("6")

	L, tos, host := deployOpenlibContract(t, "openlib/privacy/ConfidentialTreasury.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetUnoValue(host, openlibUnoFromInt(100))
	invokeOpenlib(t, L, tos, "deposit()")
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	if got := LVAsString(invokeOpenlib(t, L, tos, "totalBalance()")); got != LVAsString(openlibUnoFromInt(100)) {
		t.Fatalf("totalBalance after deposit: got=%s want=%s", got, LVAsString(openlibUnoFromInt(100)))
	}

	invokeOpenlib(t, L, tos, "addSigner(agent)", LString(bob))
	if got := invokeOpenlib(t, L, tos, "isSigner(agent)", LString(bob)); !LVAsBool(got) {
		t.Fatal("bob should be an authorized signer")
	}

	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "authorizeSpend(bytes32,agent,uno,bytes32)", LString(spendOne), LString(charlie), openlibUnoFromInt(30), LString(purposeOne))
	if got := LVAsString(invokeOpenlib(t, L, tos, "spendStatusOf(bytes32)", LString(spendOne))); got != "1" {
		t.Fatalf("spendStatus spend one after authorize: got=%s want=1", got)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "executeSpend(bytes32,bytes32)", LString(spendOne), LString(settlementRef))
	if got := LVAsString(invokeOpenlib(t, L, tos, "spendStatusOf(bytes32)", LString(spendOne))); got != "2" {
		t.Fatalf("spendStatus spend one after execute: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "totalBalance()")); got != LVAsString(openlibUnoFromInt(70)) {
		t.Fatalf("totalBalance after execute: got=%s want=%s", got, LVAsString(openlibUnoFromInt(70)))
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nativeBalance(agent)", LString(charlie))); got != LVAsString(openlibUnoFromInt(30)) {
		t.Fatalf("nativeBalance charlie after execute: got=%s want=%s", got, LVAsString(openlibUnoFromInt(30)))
	}

	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "authorizeSpend(bytes32,agent,uno,bytes32)", LString(spendTwo), LString(openlibMerchant), openlibUnoFromInt(10), LString(purposeTwo))
	invokeOpenlib(t, L, tos, "cancelSpend(bytes32)", LString(spendTwo))
	if got := LVAsString(invokeOpenlib(t, L, tos, "spendStatusOf(bytes32)", LString(spendTwo))); got != "3" {
		t.Fatalf("spendStatus spend two after cancel: got=%s want=3", got)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "authorizeAuditor(agent,bytes32)", LString(openlibService), LString(disclosureRef))
	if got := invokeOpenlib(t, L, tos, "canAudit(agent)", LString(openlibService)); !LVAsBool(got) {
		t.Fatal("auditor should be authorized")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "disclosureRefOf(agent)", LString(openlibService))); got != disclosureRef {
		t.Fatalf("disclosureRefOf: got=%s want=%s", got, disclosureRef)
	}
	invokeOpenlib(t, L, tos, "revokeAuditor(agent)", LString(openlibService))
	if got := invokeOpenlib(t, L, tos, "canAudit(agent)", LString(openlibService)); LVAsBool(got) {
		t.Fatal("auditor should be revoked")
	}
}

func TestConfidentialAllowanceRuntimeApproveTransferAndExpiry(t *testing.T) {
	receiptRef := openlibBytes32("1")

	L, tos, host := deployOpenlibContract(t, "openlib/privacy/ConfidentialAllowance.tol")
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetUnoValue(host, openlibUnoFromInt(60))
	invokeOpenlib(t, L, tos, "deposit()")
	openlibSetUnoValue(host, openlibUnoFromInt(0))
	if got := LVAsString(invokeOpenlib(t, L, tos, "balanceOf(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(60)) {
		t.Fatalf("balanceOf alice after deposit: got=%s want=%s", got, LVAsString(openlibUnoFromInt(60)))
	}

	openlibSetTimestamp(host, 100)
	invokeOpenlib(t, L, tos, "approve(agent,uno,u256)", LString(bob), openlibUnoFromInt(25), lu256FromInt(150))
	if got := invokeOpenlib(t, L, tos, "isApproved(agent,agent)", LString(alice), LString(bob)); !LVAsBool(got) {
		t.Fatal("approval should be active before expiry")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "allowanceOf(agent,agent)", LString(alice), LString(bob))); got != LVAsString(openlibUnoFromInt(25)) {
		t.Fatalf("allowanceOf before spend: got=%s want=%s", got, LVAsString(openlibUnoFromInt(25)))
	}

	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "transferFrom(agent,agent,uno,bytes32)", LString(alice), LString(charlie), openlibUnoFromInt(10), LString(receiptRef))
	if got := LVAsString(invokeOpenlib(t, L, tos, "balanceOf(agent)", LString(alice))); got != LVAsString(openlibUnoFromInt(50)) {
		t.Fatalf("balanceOf alice after transferFrom: got=%s want=%s", got, LVAsString(openlibUnoFromInt(50)))
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "balanceOf(agent)", LString(charlie))); got != LVAsString(openlibUnoFromInt(10)) {
		t.Fatalf("balanceOf charlie after transferFrom: got=%s want=%s", got, LVAsString(openlibUnoFromInt(10)))
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "allowanceOf(agent,agent)", LString(alice), LString(bob))); got != LVAsString(openlibUnoFromInt(15)) {
		t.Fatalf("allowanceOf after transferFrom: got=%s want=%s", got, LVAsString(openlibUnoFromInt(15)))
	}

	openlibSetTimestamp(host, 151)
	if got := invokeOpenlib(t, L, tos, "isApproved(agent,agent)", LString(alice), LString(bob)); LVAsBool(got) {
		t.Fatal("approval should be inactive after expiry")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "allowanceOf(agent,agent)", LString(alice), LString(bob))); got != LVAsString(openlibUnoFromInt(0)) {
		t.Fatalf("allowanceOf after expiry: got=%s want=%s", got, LVAsString(openlibUnoFromInt(0)))
	}
	errMsg := invokeOpenlibErr(t, L, tos, "transferFrom(agent,agent,uno,bytes32)", LString(alice), LString(charlie), openlibUnoFromInt(1), LString(receiptRef))
	if !strings.Contains(errMsg, "ALLOWANCE_EXPIRED") {
		t.Fatalf("expected ALLOWANCE_EXPIRED, got %q", errMsg)
	}
}

func TestAuditorDisclosureBookRuntimeExpiryAndSnapshots(t *testing.T) {
	scopeRef := openlibBytes32("1")
	snapshotID := openlibBytes32("2")
	dataRef := openlibBytes32("3")
	proofRefOne := openlibBytes32("4")
	proofRefTwo := openlibBytes32("5")

	L, tos, host := deployOpenlibContract(t, "openlib/privacy/AuditorDisclosureBook.tol", LString(alice))
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	invokeOpenlib(t, L, tos, "authorizeAuditor(agent,bytes32,u256)", LString(bob), LString(scopeRef), lu256FromInt(150))
	if got := invokeOpenlib(t, L, tos, "isAuthorized(agent)", LString(bob)); !LVAsBool(got) {
		t.Fatal("auditor should be authorized before expiry")
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "scopeRefOf(agent)", LString(bob))); got != scopeRef {
		t.Fatalf("scopeRefOf: got=%s want=%s", got, scopeRef)
	}

	openlibSetTimestamp(host, 151)
	if got := invokeOpenlib(t, L, tos, "isAuthorized(agent)", LString(bob)); LVAsBool(got) {
		t.Fatal("auditor should not be authorized after expiry")
	}

	openlibSetTimestamp(host, 200)
	invokeOpenlib(t, L, tos, "publishSnapshot(bytes32,bytes32,bytes32,u256,u256)", LString(snapshotID), LString(dataRef), LString(proofRefOne), lu256FromInt(0), lu256FromInt(100))
	if got := LVAsString(invokeOpenlib(t, L, tos, "snapshotStatusOf(bytes32)", LString(snapshotID))); got != "1" {
		t.Fatalf("snapshotStatus after publish: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "snapshotCount()")); got != "1" {
		t.Fatalf("snapshotCount after publish: got=%s want=1", got)
	}

	invokeOpenlib(t, L, tos, "attachProof(bytes32,bytes32)", LString(snapshotID), LString(proofRefTwo))
	if got := LVAsString(invokeOpenlib(t, L, tos, "proofRefOf(bytes32)", LString(snapshotID))); got != proofRefTwo {
		t.Fatalf("proofRefOf after attachProof: got=%s want=%s", got, proofRefTwo)
	}

	invokeOpenlib(t, L, tos, "finalizeSnapshot(bytes32)", LString(snapshotID))
	if got := LVAsString(invokeOpenlib(t, L, tos, "snapshotStatusOf(bytes32)", LString(snapshotID))); got != "2" {
		t.Fatalf("snapshotStatus after finalize: got=%s want=2", got)
	}
	if got := invokeOpenlib(t, L, tos, "isFinalized(bytes32)", LString(snapshotID)); !LVAsBool(got) {
		t.Fatal("snapshot should be finalized")
	}

	errMsg := invokeOpenlibErr(t, L, tos, "attachProof(bytes32,bytes32)", LString(snapshotID), LString(proofRefOne))
	if !strings.Contains(errMsg, "NOT_DRAFT") {
		t.Fatalf("expected NOT_DRAFT, got %q", errMsg)
	}

	invokeOpenlib(t, L, tos, "revokeAuditor(agent)", LString(bob))
	if got := invokeOpenlib(t, L, tos, "isAuthorized(agent)", LString(bob)); LVAsBool(got) {
		t.Fatal("auditor should be revoked")
	}
}

func TestRecurringPaymentRuntimeLifecyclePauseResumeCancelAndComplete(t *testing.T) {
	subOne := openlibBytes32("1")
	subTwo := openlibBytes32("2")
	agreementOne := openlibBytes32("3")
	agreementTwo := openlibBytes32("4")
	receiptOne := openlibBytes32("5")
	receiptTwo := openlibBytes32("6")
	receiptThree := openlibBytes32("7")
	receiptFour := openlibBytes32("8")
	reasonRef := openlibBytes32("9")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/RecurringPayment.tol", LString(charlie))
	defer L.Close()

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 30)
	invokeOpenlib(t, L, tos, "subscribe(bytes32,agent,u256,u256,u256,bytes32)", LString(subOne), LString(bob), lu256FromInt(10), lu256FromInt(50), lu256FromInt(3), LString(agreementOne))
	openlibSetValue(host, 0)
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(subOne))); got != "1" {
		t.Fatalf("status sub one after subscribe: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingBalance(bytes32)", LString(subOne))); got != "30" {
		t.Fatalf("remainingBalance sub one after subscribe: got=%s want=30", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "nextPaymentAfter(bytes32)", LString(subOne))); got != "150" {
		t.Fatalf("nextPaymentAfter sub one after subscribe: got=%s want=150", got)
	}
	if host.lastEscrowAddr != alice || host.lastEscrowAmt != "30" {
		t.Fatalf("subscribe escrow mismatch: addr=%q amt=%q", host.lastEscrowAddr, host.lastEscrowAmt)
	}

	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 149)
	errMsg := invokeOpenlibErr(t, L, tos, "executePayment(bytes32,bytes32)", LString(subOne), LString(receiptOne))
	if !strings.Contains(errMsg, "INTERVAL_NOT_ELAPSED") {
		t.Fatalf("expected INTERVAL_NOT_ELAPSED, got %q", errMsg)
	}

	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "executePayment(bytes32,bytes32)", LString(subOne), LString(receiptOne))
	if got := LVAsString(invokeOpenlib(t, L, tos, "cyclesCompleted(bytes32)", LString(subOne))); got != "1" {
		t.Fatalf("cyclesCompleted sub one after first payment: got=%s want=1", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingBalance(bytes32)", LString(subOne))); got != "20" {
		t.Fatalf("remainingBalance sub one after first payment: got=%s want=20", got)
	}
	if host.lastReleaseAddr != bob || host.lastReleaseAmt != "10" {
		t.Fatalf("executePayment release mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "pause(bytes32)", LString(subOne))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(subOne))); got != "2" {
		t.Fatalf("status sub one after pause: got=%s want=2", got)
	}

	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 200)
	errMsg = invokeOpenlibErr(t, L, tos, "executePayment(bytes32,bytes32)", LString(subOne), LString(receiptTwo))
	if !strings.Contains(errMsg, "NOT_ACTIVE") {
		t.Fatalf("expected NOT_ACTIVE while paused, got %q", errMsg)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "resume(bytes32)", LString(subOne))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(subOne))); got != "1" {
		t.Fatalf("status sub one after resume: got=%s want=1", got)
	}

	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 200)
	invokeOpenlib(t, L, tos, "executePayment(bytes32,bytes32)", LString(subOne), LString(receiptTwo))
	if got := LVAsString(invokeOpenlib(t, L, tos, "cyclesCompleted(bytes32)", LString(subOne))); got != "2" {
		t.Fatalf("cyclesCompleted sub one after second payment: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingBalance(bytes32)", LString(subOne))); got != "10" {
		t.Fatalf("remainingBalance sub one after second payment: got=%s want=10", got)
	}

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "cancel(bytes32,bytes32)", LString(subOne), LString(reasonRef))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(subOne))); got != "3" {
		t.Fatalf("status sub one after cancel: got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingBalance(bytes32)", LString(subOne))); got != "0" {
		t.Fatalf("remainingBalance sub one after cancel: got=%s want=0", got)
	}
	if host.lastReleaseAddr != alice || host.lastReleaseAmt != "10" {
		t.Fatalf("cancel release mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 300)
	openlibSetValue(host, 10)
	invokeOpenlib(t, L, tos, "subscribe(bytes32,agent,u256,u256,u256,bytes32)", LString(subTwo), LString(bob), lu256FromInt(5), lu256FromInt(10), lu256FromInt(2), LString(agreementTwo))
	openlibSetValue(host, 0)

	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 310)
	invokeOpenlib(t, L, tos, "executePayment(bytes32,bytes32)", LString(subTwo), LString(receiptThree))
	openlibSetTimestamp(host, 320)
	invokeOpenlib(t, L, tos, "executePayment(bytes32,bytes32)", LString(subTwo), LString(receiptFour))
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(bytes32)", LString(subTwo))); got != "4" {
		t.Fatalf("status sub two after completion: got=%s want=4", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "cyclesCompleted(bytes32)", LString(subTwo))); got != "2" {
		t.Fatalf("cyclesCompleted sub two after completion: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "remainingBalance(bytes32)", LString(subTwo))); got != "0" {
		t.Fatalf("remainingBalance sub two after completion: got=%s want=0", got)
	}
	if host.lastReleaseAddr != bob || host.lastReleaseAmt != "5" {
		t.Fatalf("final completion release mismatch: addr=%q amt=%q", host.lastReleaseAddr, host.lastReleaseAmt)
	}
}

// ---------------------------------------------------------------------------
// @requires(caller: Cap) capability annotation tests
// ---------------------------------------------------------------------------

func TestRequiresCapabilityCompileAndABI(t *testing.T) {
	source := []byte(`pragma tolang 0.4.0;
contract TestCap {
    capability AdminCap;
    /// @requires(caller: AdminCap)
    function adminOnly() public view returns (u256 result) {
        return 42;
    }
    function openFunc() public view returns (u256 result) {
        return 1;
    }
}
`)
	artBytes, err := CompileArtifact(source, "TestCap")
	if err != nil {
		t.Fatalf("CompileArtifact: %v", err)
	}
	art, err := DecodeArtifact(artBytes)
	if err != nil {
		t.Fatalf("DecodeArtifact: %v", err)
	}
	meta, err := metadata.ExtractFromABI(art.ABIJSON)
	if err != nil {
		t.Fatalf("ExtractFromABI: %v", err)
	}

	// Find adminOnly — should have RequiresCapability containing "AdminCap".
	var foundAdmin, foundOpen bool
	for _, fn := range meta.Functions {
		switch fn.Name {
		case "adminOnly":
			foundAdmin = true
			if len(fn.RequiresCapability) == 0 {
				t.Fatal("adminOnly: RequiresCapability is empty, expected [AdminCap]")
			}
			if fn.RequiresCapability[0] != "AdminCap" {
				t.Fatalf("adminOnly: RequiresCapability[0]=%q, want AdminCap", fn.RequiresCapability[0])
			}
		case "openFunc":
			foundOpen = true
			if len(fn.RequiresCapability) != 0 {
				t.Fatalf("openFunc: RequiresCapability should be empty, got %v", fn.RequiresCapability)
			}
		}
	}
	if !foundAdmin {
		t.Fatal("adminOnly not found in metadata functions")
	}
	if !foundOpen {
		t.Fatal("openFunc not found in metadata functions")
	}
}

func TestRequiresCapabilityUnknownCapRejected(t *testing.T) {
	source := []byte(`pragma tolang 0.4.0;
contract BadCap {
    /// @requires(caller: UnknownCap)
    function guarded() public view returns (u256 result) {
        return 1;
	}
}
`)
	_, err := CompileArtifact(source, "BadCap")
	if err == nil {
		t.Fatal("expected compilation error for undeclared capability, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "TOL2302") && !strings.Contains(errMsg, "undeclared capability") {
		t.Fatalf("expected error about undeclared capability (TOL2302), got: %s", errMsg)
	}
}

func TestRequiresCapabilityInvalidKeyRejected(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "doc_comment_key",
			source: `pragma tolang 0.4.0;
contract BadCapDoc {
    capability AdminCap;
    /// @requires(foo: AdminCap)
    function guarded() public view returns (u256 result) {
        return 1;
    }
}
`,
		},
		{
			name: "attribute_key",
			source: `pragma tolang 0.4.0;
contract BadCapAttr {
    capability AdminCap;
    @requires(foo: AdminCap)
    function guarded() public view returns (u256 result) {
        return 1;
    }
}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CompileArtifact([]byte(tc.source), "")
			if err == nil {
				t.Fatal("expected compilation error for invalid @requires key, got nil")
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "@requires only supports 'caller: CapName'") {
				t.Fatalf("expected invalid @requires key error, got: %s", errMsg)
			}
		})
	}
}

func TestRequiresCapabilityRuntimePreamble(t *testing.T) {
	source := []byte(`pragma tolang 0.4.0;
contract GuardedContract {
    capability OwnerCap;
    /// @requires(caller: OwnerCap)
    function secret() public view returns (u256 result) {
        return 99;
    }
    function open() public view returns (u256 result) {
        return 1;
    }
}
`)
	L, tos, host := deployOpenlibSourceContract(t, source, "<GuardedContract.tol>")
	runtimeBC, err := CompileBytecode(source, "<GuardedContract.tol>")
	if err != nil {
		t.Fatalf("CompileBytecode: %v", err)
	}

	// open() should succeed without any capability hook.
	if got := LVAsString(invokeOpenlib(t, L, tos, "open()")); got != "1" {
		t.Fatalf("open(): got=%s want=1", got)
	}

	// secret() should fail — no tos.hascapability hook installed.
	errMsg := invokeOpenlibErr(t, L, tos, "secret()")
	if !strings.Contains(errMsg, "CapabilityDenied") {
		t.Fatalf("secret() without hook: expected CapabilityDenied, got: %s", errMsg)
	}

	// Install tos.hascapability that always grants. Without capabilitybit,
	// the check must still fail closed instead of silently mapping to bit 0.
	L.SetField(host.tosTable, "hascapability", L.NewFunction(func(L *LState) int {
		L.Push(LTrue) // always grant
		return 1
	}))

	errMsg = invokeOpenlibErr(t, L, tos, "secret()")
	if !strings.Contains(errMsg, "CapabilityDenied") {
		t.Fatalf("secret() without capabilitybit: expected CapabilityDenied, got: %s", errMsg)
	}

	// Install capabilitybit and ensure bit 0 still works as a valid capability.
	L.SetField(host.tosTable, "capabilitybit", L.NewFunction(func(L *LState) int {
		if LVAsString(L.CheckAny(1)) != "OwnerCap" {
			L.Push(LNil)
			return 1
		}
		L.Push(lu256FromInt(0))
		return 1
	}))
	if err := L.DoBytecode(runtimeBC); err != nil {
		t.Fatalf("reload runtime with capabilitybit: %v", err)
	}
	tos = L.GetGlobal("tos")

	// secret() should succeed now.
	if got := LVAsString(invokeOpenlib(t, L, tos, "secret()")); got != "99" {
		t.Fatalf("secret() with grant hook: got=%s want=99", got)
	}

	// Change hook to deny.
	L.SetField(host.tosTable, "hascapability", L.NewFunction(func(L *LState) int {
		L.Push(LFalse) // deny
		return 1
	}))

	// secret() should fail again.
	errMsg = invokeOpenlibErr(t, L, tos, "secret()")
	if !strings.Contains(errMsg, "CapabilityDenied") {
		t.Fatalf("secret() with deny hook: expected CapabilityDenied, got: %s", errMsg)
	}
}

func TestPolicyAccountDelegateCapsEnforced(t *testing.T) {
	L, tos, host := deployOpenlibContract(
		t,
		"openlib/account/PolicyAccount.tol",
		LString(alice),
		LString(bob),
		lu256FromInt(1000),
		lu256FromInt(400),
	)
	defer L.Close()

	// Set up a callHook so that execute's target.call succeeds.
	host.callHook = func(addr, value, data string) (bool, string, bool) {
		return true, "0x", true
	}

	// Owner authorizes delegate charlie with allowance=500, expiry=5000.
	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	invokeOpenlib(t, L, tos, "authorizeDelegate(agent,u256,u256)",
		LString(charlie), lu256FromInt(500), lu256FromInt(5000))

	// Owner sets delegate caps: daily=300, single=150.
	invokeOpenlib(t, L, tos, "setDelegateCaps(agent,u256,u256)",
		LString(charlie), lu256FromInt(300), lu256FromInt(150))

	// Charlie calls execute with value=200 → should FAIL (exceeds 150 single limit).
	openlibSetSender(host, charlie)
	errMsg := invokeOpenlibErr(t, L, tos, "execute(agent,bytes,u256)",
		LString(openlibMerchant), LString("0x"), lu256FromInt(200))
	if !strings.Contains(errMsg, "OVER_DELEGATE_SINGLE") {
		t.Fatalf("expected OVER_DELEGATE_SINGLE, got %q", errMsg)
	}

	// Charlie calls execute with value=100 → should succeed.
	if got := invokeOpenlib(t, L, tos, "execute(agent,bytes,u256)",
		LString(openlibMerchant), LString("0x"), lu256FromInt(100)); !LVAsBool(got) {
		t.Fatalf("execute(100) should succeed, got %v", got)
	}

	// Check delegateDailyRemaining(charlie) == 200 (300 - 100).
	if got := LVAsString(invokeOpenlib(t, L, tos, "delegateDailyRemaining(agent)",
		LString(charlie))); got != "200" {
		t.Fatalf("delegateDailyRemaining after first spend: got=%s want=200", got)
	}

	// Charlie calls execute with value=100 again → should succeed.
	if got := invokeOpenlib(t, L, tos, "execute(agent,bytes,u256)",
		LString(openlibMerchant), LString("0x"), lu256FromInt(100)); !LVAsBool(got) {
		t.Fatalf("execute(100) second time should succeed, got %v", got)
	}

	// Charlie calls execute with value=150 → should FAIL (would exceed 300 daily).
	errMsg = invokeOpenlibErr(t, L, tos, "execute(agent,bytes,u256)",
		LString(openlibMerchant), LString("0x"), lu256FromInt(150))
	if !strings.Contains(errMsg, "OVER_DELEGATE_DAILY") {
		t.Fatalf("expected OVER_DELEGATE_DAILY, got %q", errMsg)
	}
}

func TestTrustRegistryReputationWritesAffectEligibility(t *testing.T) {
	L, tos, host := deployOpenlibContract(
		t,
		"openlib/trust/TrustRegistry.tol",
		LString(alice),
		lu256FromInt(0),
		lu256FromInt(0),
	)
	defer L.Close()

	reasonRef := openlibBytes32("f")

	// Owner sets trust floor: min_stake=100, min_reputation=50.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "setTrustFloor(u256,i256)",
		lu256FromInt(100), lu256FromInt(50))

	// Ensure bob is known to the agent registry with low stake initially.
	openlibSetAgentProp(host, bob, "stake", lu256FromInt(0))
	openlibSetAgentProp(host, bob, "suspended", lu256FromInt(0))

	// Check isEligible(bob) → false (reputation=0, stake=0).
	if got := invokeOpenlib(t, L, tos, "isEligible(agent)", LString(bob)); LVAsBool(got) {
		t.Fatal("bob should be ineligible initially")
	}

	// Owner updates reputation: delta=+60.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "updateReputation(agent,i256,bytes32)",
		LString(bob), lu256FromInt(60), LString(reasonRef))

	// Check snapshotReputationOf(bob) → 60.
	if got := LVAsString(invokeOpenlib(t, L, tos, "snapshotReputationOf(agent)",
		LString(bob))); got != "60" {
		t.Fatalf("snapshotReputationOf: got=%s want=60", got)
	}

	// Check isEligible(bob) → still false (stake=0 < 100).
	if got := invokeOpenlib(t, L, tos, "isEligible(agent)", LString(bob)); LVAsBool(got) {
		t.Fatal("bob should still be ineligible (stake=0 < 100)")
	}

	// Set bob's agent stake property to 200 and deposit bond.
	openlibSetAgentProp(host, bob, "stake", lu256FromInt(200))
	openlibSetSender(host, bob)
	openlibSetValue(host, 200)
	invokeOpenlib(t, L, tos, "depositBond()")
	openlibSetValue(host, 0)

	// Check isEligible(bob) → true (rep=60 >= 50, stake=200 >= 100).
	if got := invokeOpenlib(t, L, tos, "isEligible(agent)", LString(bob)); !LVAsBool(got) {
		t.Fatal("bob should be eligible (rep=60 >= 50, stake=200 >= 100)")
	}
}

func TestTaskSettlementMilestoneLifecycle(t *testing.T) {
	taskRef := openlibBytes32("1")
	receiptRef := openlibBytes32("2")
	proofRef0 := openlibBytes32("3")
	proofRef1 := openlibBytes32("4")
	proofRef2 := openlibBytes32("5")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/TaskSettlement.tol", LString(alice))
	defer L.Close()

	// Poster (bob) opens milestone task with 3 milestones, value=100.
	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 100)
	invokeOpenlib(t, L, tos, "openMilestoneTask(bytes32,u256,u256,bytes32)",
		LString(taskRef), lu256FromInt(3), lu256FromInt(500), LString(receiptRef))
	openlibSetValue(host, 0)

	// Worker (charlie) accepts the task.
	openlibSetSender(host, charlie)
	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(1))

	// Poster completes milestone 0.
	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "completeMilestone(u256,u256,bytes32)",
		lu256FromInt(1), lu256FromInt(0), LString(proofRef0))

	// Check milestoneStatusOf(task_id=1, milestone_index=0) → 1.
	if got := LVAsString(invokeOpenlib(t, L, tos, "milestoneStatusOf(u256,u256)",
		lu256FromInt(1), lu256FromInt(0))); got != "1" {
		t.Fatalf("milestoneStatusOf(1,0): got=%s want=1", got)
	}

	// Check statusOf(task_id=1) → STATUS_MILESTONE_PARTIAL (9).
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)",
		lu256FromInt(1))); got != "9" {
		t.Fatalf("statusOf after milestone 0: got=%s want=9", got)
	}

	// Release for milestone 0: per_milestone = 100/3 = 33.
	if host.lastReleaseAmt != "33" {
		t.Fatalf("milestone 0 release: got=%s want=33", host.lastReleaseAmt)
	}

	// Poster completes milestone 1.
	invokeOpenlib(t, L, tos, "completeMilestone(u256,u256,bytes32)",
		lu256FromInt(1), lu256FromInt(1), LString(proofRef1))

	if host.lastReleaseAmt != "33" {
		t.Fatalf("milestone 1 release: got=%s want=33", host.lastReleaseAmt)
	}

	// Poster completes milestone 2 (final) → status should be STATUS_APPROVED (4).
	invokeOpenlib(t, L, tos, "completeMilestone(u256,u256,bytes32)",
		lu256FromInt(1), lu256FromInt(2), LString(proofRef2))

	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)",
		lu256FromInt(1))); got != "4" {
		t.Fatalf("statusOf after all milestones: got=%s want=4", got)
	}

	// Final milestone: per_milestone + remainder = 33 + (100 - 33*3) = 33 + 1 = 34.
	if host.lastReleaseAmt != "34" {
		t.Fatalf("milestone 2 (final) release: got=%s want=34", host.lastReleaseAmt)
	}
}

func TestTaskSettlementMilestoneRequiresWorker(t *testing.T) {
	taskRef := openlibBytes32("1")
	receiptRef := openlibBytes32("2")
	proofRef := openlibBytes32("3")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/TaskSettlement.tol", LString(alice))
	defer L.Close()

	// Poster (bob) opens milestone task but does NOT accept.
	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 50)
	invokeOpenlib(t, L, tos, "openMilestoneTask(bytes32,u256,u256,bytes32)",
		LString(taskRef), lu256FromInt(2), lu256FromInt(500), LString(receiptRef))
	openlibSetValue(host, 0)

	// Poster tries completeMilestone → should FAIL with "WRONG_STATUS".
	errMsg := invokeOpenlibErr(t, L, tos, "completeMilestone(u256,u256,bytes32)",
		lu256FromInt(1), lu256FromInt(0), LString(proofRef))
	if !strings.Contains(errMsg, "WRONG_STATUS") {
		t.Fatalf("expected WRONG_STATUS, got %q", errMsg)
	}
}

func TestServiceDirectoryStructuredFields(t *testing.T) {
	manifestRef := openlibBytes32("2")
	capabilityRef := openlibBytes32("3")
	versionRef := openlibBytes32("4")
	quoteRef := openlibBytes32("5")

	L, tos, host := deployOpenlibContract(t, "openlib/discovery/ServiceDirectory.tol")
	defer L.Close()

	// Provider (alice) registers service.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifestRef), LString(capabilityRef), LString(versionRef), LString(quoteRef))

	// Provider sets fee: setServiceFee(1, 500).
	invokeOpenlib(t, L, tos, "setServiceFee(u256,u256)",
		lu256FromInt(1), lu256FromInt(500))

	// Provider sets SLA: setServiceSLA(1, 3600000).
	invokeOpenlib(t, L, tos, "setServiceSLA(u256,u256)",
		lu256FromInt(1), lu256FromInt(3600000))

	// Check feeOf(1) → 500.
	if got := LVAsString(invokeOpenlib(t, L, tos, "feeOf(u256)",
		lu256FromInt(1))); got != "500" {
		t.Fatalf("feeOf(1): got=%s want=500", got)
	}

	// Check slaOf(1) → 3600000.
	if got := LVAsString(invokeOpenlib(t, L, tos, "slaOf(u256)",
		lu256FromInt(1))); got != "3600000" {
		t.Fatalf("slaOf(1): got=%s want=3600000", got)
	}
}

func TestServiceDirectoryTypedSchemaFields(t *testing.T) {
	manifestRef := openlibBytes32("2")
	capabilityRef := openlibBytes32("3")
	versionRef := openlibBytes32("4")
	quoteRef := openlibBytes32("5")
	trustFloorRef := openlibBytes32("6")

	L, tos, host := deployOpenlibContract(t, "openlib/discovery/ServiceDirectory.tol")
	defer L.Close()

	// Provider registers service.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifestRef), LString(capabilityRef), LString(versionRef), LString(quoteRef))

	// Typed fields default to 0 / empty refs.
	if got := LVAsString(invokeOpenlib(t, L, tos, "serviceKindOf(u256)",
		lu256FromInt(1))); got != "0" {
		t.Fatalf("serviceKindOf(1) default: got=%s want=0", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "capabilityTypeOf(u256)",
		lu256FromInt(1))); got != "0" {
		t.Fatalf("capabilityTypeOf(1) default: got=%s want=0", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "pricingKindOf(u256)",
		lu256FromInt(1))); got != "0" {
		t.Fatalf("pricingKindOf(1) default: got=%s want=0", got)
	}

	// Set full typed schema.
	invokeOpenlib(t, L, tos, "setServiceKind(u256,u256)",
		lu256FromInt(1), lu256FromInt(8))
	invokeOpenlib(t, L, tos, "setCapabilityType(u256,u256)",
		lu256FromInt(1), lu256FromInt(4))
	invokeOpenlib(t, L, tos, "setPricingKind(u256,u256)",
		lu256FromInt(1), lu256FromInt(2))
	invokeOpenlib(t, L, tos, "setPrivacyMode(u256,u256)",
		lu256FromInt(1), lu256FromInt(4))
	invokeOpenlib(t, L, tos, "setReceiptMode(u256,u256)",
		lu256FromInt(1), lu256FromInt(3))
	invokeOpenlib(t, L, tos, "setTrustFloorRef(u256,bytes32)",
		lu256FromInt(1), LString(trustFloorRef))

	if got := LVAsString(invokeOpenlib(t, L, tos, "serviceKindOf(u256)",
		lu256FromInt(1))); got != "8" {
		t.Fatalf("serviceKindOf(1): got=%s want=8", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "capabilityTypeOf(u256)",
		lu256FromInt(1))); got != "4" {
		t.Fatalf("capabilityTypeOf(1): got=%s want=4", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "pricingKindOf(u256)",
		lu256FromInt(1))); got != "2" {
		t.Fatalf("pricingKindOf(1): got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "privacyModeOf(u256)",
		lu256FromInt(1))); got != "4" {
		t.Fatalf("privacyModeOf(1): got=%s want=4", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "receiptModeOf(u256)",
		lu256FromInt(1))); got != "3" {
		t.Fatalf("receiptModeOf(1): got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, L, tos, "trustFloorRefOf(u256)",
		lu256FromInt(1))); got != trustFloorRef {
		t.Fatalf("trustFloorRefOf(1): got=%s want=%s", got, trustFloorRef)
	}
}

func TestServiceDirectoryServiceCount(t *testing.T) {
	manifestRef := openlibBytes32("2")
	capabilityRef := openlibBytes32("3")
	versionRef := openlibBytes32("4")
	quoteRef := openlibBytes32("5")

	L, tos, host := deployOpenlibContract(t, "openlib/discovery/ServiceDirectory.tol")
	defer L.Close()

	// Initially no services registered; count should be 0.
	if got := LVAsString(invokeOpenlib(t, L, tos, "serviceCount()")); got != "0" {
		t.Fatalf("serviceCount initial: got=%s want=0", got)
	}

	// Register first service.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifestRef), LString(capabilityRef), LString(versionRef), LString(quoteRef))

	if got := LVAsString(invokeOpenlib(t, L, tos, "serviceCount()")); got != "1" {
		t.Fatalf("serviceCount after 1: got=%s want=1", got)
	}

	// Register second service.
	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifestRef), LString(capabilityRef), LString(versionRef), LString(quoteRef))

	if got := LVAsString(invokeOpenlib(t, L, tos, "serviceCount()")); got != "2" {
		t.Fatalf("serviceCount after 2: got=%s want=2", got)
	}
}

func TestServiceDirectoryTypedSchemaValidation(t *testing.T) {
	manifestRef := openlibBytes32("2")
	capabilityRef := openlibBytes32("3")
	versionRef := openlibBytes32("4")
	quoteRef := openlibBytes32("5")

	L, tos, host := deployOpenlibContract(t, "openlib/discovery/ServiceDirectory.tol")
	defer L.Close()

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifestRef), LString(capabilityRef), LString(versionRef), LString(quoteRef))

	errMsg := invokeOpenlibErr(t, L, tos, "setServiceKind(u256,u256)", lu256FromInt(1), lu256FromInt(99))
	if !strings.Contains(errMsg, "INVALID_SERVICE_KIND") {
		t.Fatalf("expected INVALID_SERVICE_KIND, got %q", errMsg)
	}

	errMsg = invokeOpenlibErr(t, L, tos, "setPricingKind(u256,u256)", lu256FromInt(1), lu256FromInt(99))
	if !strings.Contains(errMsg, "INVALID_PRICING_KIND") {
		t.Fatalf("expected INVALID_PRICING_KIND, got %q", errMsg)
	}

	errMsg = invokeOpenlibErr(t, L, tos, "setPrivacyMode(u256,u256)", lu256FromInt(1), lu256FromInt(99))
	if !strings.Contains(errMsg, "INVALID_PRIVACY_MODE") {
		t.Fatalf("expected INVALID_PRIVACY_MODE, got %q", errMsg)
	}

	errMsg = invokeOpenlibErr(t, L, tos, "setReceiptMode(u256,u256)", lu256FromInt(1), lu256FromInt(99))
	if !strings.Contains(errMsg, "INVALID_RECEIPT_MODE") {
		t.Fatalf("expected INVALID_RECEIPT_MODE, got %q", errMsg)
	}
}

func TestSessionBookTerminalTrustTaxonomy(t *testing.T) {
	sessionID := openlibBytes32("a")
	invalidSessionID := openlibBytes32("c")
	terminalID := openlibBytes32("b")

	L, tos, host := deployOpenlibContract(t, "openlib/session/SessionBook.tol", LString(alice))
	defer L.Close()

	// Grant session with terminal_type=TERMINAL_NFC (2), trust_tier=TRUST_MEDIUM (2).
	openlibSetSender(host, alice)
	invokeOpenlib(
		t,
		L,
		tos,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(sessionID),
		LString(bob),
		LString(terminalID),
		lu256FromInt(2), // TERMINAL_NFC
		lu256FromInt(2), // TRUST_MEDIUM
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(100),
		LFalse,
	)

	// Verify terminalTypeOf returns 2.
	if got := LVAsString(invokeOpenlib(t, L, tos, "terminalTypeOf(bytes32)", LString(sessionID))); got != "2" {
		t.Fatalf("terminalTypeOf: got=%s want=2", got)
	}

	// Verify trustTierOf returns 2.
	if got := LVAsString(invokeOpenlib(t, L, tos, "trustTierOf(bytes32)", LString(sessionID))); got != "2" {
		t.Fatalf("trustTierOf: got=%s want=2", got)
	}

	errMsg := invokeOpenlibErr(
		t,
		L,
		tos,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(invalidSessionID),
		LString(bob),
		LString(terminalID),
		lu256FromInt(99),
		lu256FromInt(2),
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(100),
		LFalse,
	)
	if !strings.Contains(errMsg, "INVALID_TERMINAL") {
		t.Fatalf("expected INVALID_TERMINAL, got %q", errMsg)
	}

	errMsg = invokeOpenlibErr(
		t,
		L,
		tos,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(openlibBytes32("d")),
		LString(bob),
		LString(terminalID),
		lu256FromInt(2),
		lu256FromInt(99),
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(100),
		LFalse,
	)
	if !strings.Contains(errMsg, "INVALID_TRUST") {
		t.Fatalf("expected INVALID_TRUST, got %q", errMsg)
	}
}

func TestSessionBookEnforcedStepUp(t *testing.T) {
	sessionID := openlibBytes32("a")
	terminalID := openlibBytes32("b")

	L, tos, host := deployOpenlibContract(t, "openlib/session/SessionBook.tol", LString(alice))
	defer L.Close()

	// Grant session with step_up_limit=100.
	openlibSetSender(host, alice)
	invokeOpenlib(
		t,
		L,
		tos,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(sessionID),
		LString(bob),
		LString(terminalID),
		lu256FromInt(1),
		lu256FromInt(1),
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(100),
		LFalse,
	)

	// enforceStepUp with amount below threshold -> succeeds (no revert).
	invokeOpenlib(t, L, tos, "enforceStepUp(bytes32,u256)", LString(sessionID), lu256FromInt(50))

	// enforceStepUp with amount above threshold -> reverts "STEP_UP_REQUIRED".
	errMsg := invokeOpenlibErr(t, L, tos, "enforceStepUp(bytes32,u256)", LString(sessionID), lu256FromInt(150))
	if !strings.Contains(errMsg, "STEP_UP_REQUIRED") {
		t.Fatalf("expected STEP_UP_REQUIRED, got %q", errMsg)
	}
}

func TestTaskSettlementSlashDistribution(t *testing.T) {
	taskRef := openlibBytes32("1")
	receiptRef := openlibBytes32("2")
	resultRef := openlibBytes32("3")
	proofRef := openlibBytes32("4")
	disputeProof := openlibBytes32("5")
	settlementRef := openlibBytes32("6")
	reasonRef := openlibBytes32("7")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/TaskSettlement.tol", LString(charlie))
	defer L.Close()

	// 1. Open task with reward=100.
	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 100)
	invokeOpenlib(t, L, tos, "openTask(bytes32,u256,bytes32)",
		LString(taskRef), lu256FromInt(500), LString(receiptRef))
	openlibSetValue(host, 0)

	// 2. Accept task.
	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(1))

	// 2a. Poster precommits slash policy before submission/dispute.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "setSlashPolicy(u256,u256)", lu256FromInt(1), lu256FromInt(30))

	// 3. Submit task.
	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "submitTask(u256,bytes32,bytes32)",
		lu256FromInt(1), LString(resultRef), LString(proofRef))

	// 4. Reject task.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "rejectTask(u256,bytes32)", lu256FromInt(1), LString(reasonRef))

	// 5. Dispute task with bond=20.
	openlibSetSender(host, bob)
	openlibSetValue(host, 20)
	invokeOpenlib(t, L, tos, "disputeTask(u256,bytes32)", lu256FromInt(1), LString(disputeProof))
	openlibSetValue(host, 0)

	// 6. Policy is frozen once the dispute is live.
	openlibSetSender(host, alice)
	errMsg := invokeOpenlibErr(t, L, tos, "setSlashPolicy(u256,u256)", lu256FromInt(1), lu256FromInt(40))
	if !strings.Contains(errMsg, "POLICY_FROZEN") {
		t.Fatalf("expected POLICY_FROZEN, got %q", errMsg)
	}

	// 7. Resolve dispute — worker loses.
	host.releaseCount = 0
	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "resolveDispute(u256,bool,bytes32)",
		lu256FromInt(1), LFalse, LString(settlementRef))

	// Expect two releases: worker gets 30% of 100 = 30, poster gets 70% of 100 + bond 20 = 90.
	if host.releaseCount != 2 {
		t.Fatalf("expected 2 releases, got %d", host.releaseCount)
	}
	// lastRelease is the second (poster) release.
	if host.lastReleaseAddr != alice || host.lastReleaseAmt != "90" {
		t.Fatalf("poster release mismatch: addr=%q amt=%q want addr=%q amt=90", host.lastReleaseAddr, host.lastReleaseAmt, alice)
	}

	// Status should be RESOLVED (7).
	if got := LVAsString(invokeOpenlib(t, L, tos, "statusOf(u256)", lu256FromInt(1))); got != "7" {
		t.Fatalf("status after resolve: got=%s want=7", got)
	}
}

func TestTaskSettlementAutoReceiptBinding(t *testing.T) {
	taskRef := openlibBytes32("1")
	receiptRef := openlibBytes32("2")
	resultRef := openlibBytes32("3")
	proofRef := openlibBytes32("4")
	settlementRef := openlibBytes32("5")
	settlementAddr := openlibBytes32("6")
	receiptBookAddr := openlibBytes32("7")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/TaskSettlement.tol", LString(charlie))
	defer L.Close()
	receiptL, receiptTOS, receiptHost := deployOpenlibContract(t, "openlib/receipt/ReceiptBook.tol", LString(settlementAddr))
	defer receiptL.Close()

	attachActualPackageRouter(t, host, settlementAddr,
		&openlibDeployedPackageContract{name: "ReceiptBook", addr: receiptBookAddr, L: receiptL, tos: receiptTOS, host: receiptHost},
	)

	// Set receipt book via resolver.
	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "setReceiptBook(agent)", LString(receiptBookAddr))

	// Open task with reward=50.
	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 50)
	invokeOpenlib(t, L, tos, "openTask(bytes32,u256,bytes32)",
		LString(taskRef), lu256FromInt(500), LString(receiptRef))
	openlibSetValue(host, 0)

	// Accept.
	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(1))

	// Submit.
	invokeOpenlib(t, L, tos, "submitTask(u256,bytes32,bytes32)",
		lu256FromInt(1), LString(resultRef), LString(proofRef))

	// Approve — should open and finalize the canonical receipt.
	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "approveTask(u256,bytes32)", lu256FromInt(1), LString(settlementRef))

	if host.packageCallCount != 4 {
		t.Fatalf("expected 4 receipt package calls, got %d", host.packageCallCount)
	}
	if got := LVAsString(invokeOpenlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptRef))); got != "2" {
		t.Fatalf("receipt status after approve: got=%s want=2", got)
	}
	if got := LVAsString(invokeOpenlib(t, receiptL, receiptTOS, "proofRefOf(bytes32)", LString(receiptRef))); got != proofRef {
		t.Fatalf("receipt proof ref after approve: got=%s want=%s", got, proofRef)
	}
}

func TestTaskSettlementDisputeFinalizesFailureReceipt(t *testing.T) {
	taskRef := openlibBytes32("1")
	receiptRef := openlibBytes32("2")
	resultRef := openlibBytes32("3")
	proofRef := openlibBytes32("4")
	disputeProof := openlibBytes32("5")
	settlementRef := openlibBytes32("6")
	settlementAddr := openlibBytes32("7")
	receiptBookAddr := openlibBytes32("8")

	L, tos, host := deployOpenlibContract(t, "openlib/settlement/TaskSettlement.tol", LString(charlie))
	defer L.Close()
	receiptL, receiptTOS, receiptHost := deployOpenlibContract(t, "openlib/receipt/ReceiptBook.tol", LString(settlementAddr))
	defer receiptL.Close()

	attachActualPackageRouter(t, host, settlementAddr,
		&openlibDeployedPackageContract{name: "ReceiptBook", addr: receiptBookAddr, L: receiptL, tos: receiptTOS, host: receiptHost},
	)

	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "setReceiptBook(agent)", LString(receiptBookAddr))

	openlibSetSender(host, alice)
	openlibSetTimestamp(host, 100)
	openlibSetValue(host, 90)
	invokeOpenlib(t, L, tos, "openTask(bytes32,u256,bytes32)",
		LString(taskRef), lu256FromInt(500), LString(receiptRef))
	openlibSetValue(host, 0)

	openlibSetSender(host, bob)
	openlibSetTimestamp(host, 150)
	invokeOpenlib(t, L, tos, "acceptTask(u256)", lu256FromInt(1))

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "setSlashPolicy(u256,u256)", lu256FromInt(1), lu256FromInt(25))

	openlibSetSender(host, bob)
	invokeOpenlib(t, L, tos, "submitTask(u256,bytes32,bytes32)",
		lu256FromInt(1), LString(resultRef), LString(proofRef))

	openlibSetSender(host, alice)
	invokeOpenlib(t, L, tos, "rejectTask(u256,bytes32)", lu256FromInt(1), LString(openlibBytes32("9")))
	if got := LVAsString(invokeOpenlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptRef))); got != "1" {
		t.Fatalf("receipt status after reject: got=%s want=1", got)
	}

	openlibSetSender(host, bob)
	openlibSetValue(host, 10)
	invokeOpenlib(t, L, tos, "disputeTask(u256,bytes32)", lu256FromInt(1), LString(disputeProof))
	openlibSetValue(host, 0)

	openlibSetSender(host, charlie)
	invokeOpenlib(t, L, tos, "resolveDispute(u256,bool,bytes32)",
		lu256FromInt(1), LFalse, LString(settlementRef))

	if got := LVAsString(invokeOpenlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptRef))); got != "3" {
		t.Fatalf("receipt status after dispute loss: got=%s want=3", got)
	}
	if got := LVAsString(invokeOpenlib(t, receiptL, receiptTOS, "proofRefOf(bytes32)", LString(receiptRef))); got != proofRef {
		t.Fatalf("receipt proof ref after dispute: got=%s want=%s", got, proofRef)
	}
	foundReceiptEvent := false
	for i := len(host.emittedEvents) - 1; i >= 0; i-- {
		ev := host.emittedEvents[i]
		if ev.Name != "SettlementReceipt" {
			continue
		}
		foundReceiptEvent = true
		if len(ev.Args) != 8 {
			t.Fatalf("SettlementReceipt args len=%d want=8", len(ev.Args))
		}
		if ev.Args[1] != "1" || ev.Args[3] != receiptRef || ev.Args[5] != bob || ev.Args[7] != "22" {
			t.Fatalf("SettlementReceipt args=%v want value slots [1 %s %s 22]", ev.Args, receiptRef, bob)
		}
		break
	}
	if !foundReceiptEvent {
		t.Fatal("expected SettlementReceipt event on dispute resolution")
	}
}

func TestDelegatedPreambleEnforced(t *testing.T) {
	source := []byte(`pragma tolang 0.4.0;
contract DelegatedContract {
    /// @delegated
    function restricted() public view returns (u256 result) {
        return 42;
    }
    function open() public view returns (u256 result) {
        return 1;
    }
}
`)
	L, tos, host := deployOpenlibSourceContract(t, source, "<DelegatedContract.tol>")
	defer L.Close()

	// open() should succeed without any delegation hook.
	if got := LVAsString(invokeOpenlib(t, L, tos, "open()")); got != "1" {
		t.Fatalf("open(): got=%s want=1", got)
	}

	// restricted() should also succeed when no tos.hasdelegation exists
	// (backward-compatible — the function is callable by anyone).
	if got := LVAsString(invokeOpenlib(t, L, tos, "restricted()")); got != "42" {
		t.Fatalf("restricted() without hook: got=%s want=42", got)
	}

	// Install tos.hasdelegation that returns false → restricted() should fail.
	L.SetField(host.tosTable, "hasdelegation", L.NewFunction(func(L *LState) int {
		L.Push(LFalse)
		return 1
	}))

	errMsg := invokeOpenlibErr(t, L, tos, "restricted()")
	if !strings.Contains(errMsg, "DelegationDenied") {
		t.Fatalf("restricted() with deny hook: expected DelegationDenied, got: %s", errMsg)
	}

	// open() should still succeed (no @delegated annotation).
	if got := LVAsString(invokeOpenlib(t, L, tos, "open()")); got != "1" {
		t.Fatalf("open() after deny hook: got=%s want=1", got)
	}

	// Change hook to return true → restricted() should succeed.
	L.SetField(host.tosTable, "hasdelegation", L.NewFunction(func(L *LState) int {
		L.Push(LTrue)
		return 1
	}))

	if got := LVAsString(invokeOpenlib(t, L, tos, "restricted()")); got != "42" {
		t.Fatalf("restricted() with grant hook: got=%s want=42", got)
	}
}
