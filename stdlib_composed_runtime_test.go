package lua

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const stdlibCallTargetSource = `
pragma tolang 0.4.0;

contract CallTargetRecorder {
    bytes32 last_order_id;
    u256 last_amount;
    u256 call_count;
    bool fail_next;

    function record(bytes32 order_id, u256 amount) public returns (bool ok) {
        require(fail_next != true, "FAIL_NEXT");
        set last_order_id = order_id;
        set last_amount = amount;
        set call_count = call_count + 1;
        return true;
    }

    function setFailNext(bool fail) public {
        set fail_next = fail;
    }

    function lastOrderId() public view returns (bytes32 order_id) {
        return last_order_id;
    }

    function lastAmount() public view returns (u256 amount) {
        return last_amount;
    }

    function callCount() public view returns (u256 count) {
        return call_count;
    }
}
`

func deployStdlibExampleContract(t *testing.T, relPath string, ctorArgs ...LValue) (*LState, LValue, *stdlibRuntimeHost) {
	t.Helper()

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	compileName := filepath.Join(filepath.Dir(repoRoot), filepath.Base(relPath))
	return deployStdlibContractWithCompileName(t, relPath, compileName, ctorArgs...)
}

func stdlibSelectorFromCalldata(calldata string) string {
	calldata = strings.ToLower(strings.TrimSpace(calldata))
	if len(calldata) < 10 {
		return calldata
	}
	return calldata[:10]
}

type stdlibDeployedPackageContract struct {
	name string
	addr string
	L    *LState
	tos  LValue
	host *stdlibRuntimeHost
}

func invokePackageContractCalldata(t *testing.T, dep *stdlibDeployedPackageContract, caller, calldata string) []LValue {
	t.Helper()

	oninvoke := dep.L.GetField(dep.tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("%s missing tos.oninvoke", dep.name)
	}

	base := dep.L.GetTop()
	stdlibSetSender(dep.host, caller)
	dep.host.tosTable.RawSetString("calldata", LString(calldata))
	dep.L.Push(oninvoke)
	dep.L.Push(LString(stdlibSelectorFromCalldata(calldata)))
	if err := dep.L.PCall(1, MultRet, nil); err != nil {
		t.Fatalf("package call %s failed: %v", dep.name, err)
	}

	n := dep.L.GetTop() - base
	rets := make([]LValue, 0, n)
	for i := 0; i < n; i++ {
		rets = append(rets, dep.L.Get(base+1+i))
	}
	dep.L.SetTop(base)
	return rets
}

func attachActualPackageRouter(t *testing.T, coordinatorHost *stdlibRuntimeHost, caller string, deps ...*stdlibDeployedPackageContract) {
	t.Helper()

	byAddr := make(map[string]*stdlibDeployedPackageContract, len(deps))
	for _, dep := range deps {
		byAddr[dep.addr] = dep
	}
	coordinatorHost.packageCallHook = func(addr, contractName, calldata string) []LValue {
		dep := byAddr[addr]
		if dep == nil {
			t.Fatalf("package_call to unknown addr=%s contract=%s calldata=%s", addr, contractName, calldata)
		}
		if dep.name != contractName {
			t.Fatalf("package_call contract mismatch: addr=%s got=%s want=%s", addr, contractName, dep.name)
		}
		return invokePackageContractCalldata(t, dep, caller, calldata)
	}
}

func invokeCallContractCalldata(t *testing.T, dep *stdlibDeployedPackageContract, caller, calldata string) (bool, string) {
	t.Helper()

	oninvoke := dep.L.GetField(dep.tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("%s missing tos.oninvoke", dep.name)
	}

	base := dep.L.GetTop()
	stdlibSetSender(dep.host, caller)
	dep.host.tosTable.RawSetString("calldata", LString(calldata))
	dep.L.Push(oninvoke)
	dep.L.Push(LString(stdlibSelectorFromCalldata(calldata)))
	if err := dep.L.PCall(1, MultRet, nil); err != nil {
		dep.L.SetTop(base)
		return false, err.Error()
	}
	dep.L.SetTop(base)
	return true, "0x"
}

func attachActualCallRouter(t *testing.T, callerHost *stdlibRuntimeHost, caller string, deps ...*stdlibDeployedPackageContract) {
	t.Helper()

	byAddr := make(map[string]*stdlibDeployedPackageContract, len(deps))
	for _, dep := range deps {
		byAddr[dep.addr] = dep
	}
	callerHost.callHook = func(addr, value, data string) (bool, string, bool) {
		dep := byAddr[addr]
		if dep == nil {
			t.Fatalf("call to unknown addr=%s data=%s", addr, data)
		}
		ok, ret := invokeCallContractCalldata(t, dep, caller, data)
		return ok, ret, true
	}
}

func TestPolicySponsoredCheckoutRuntimeComposedFlow(t *testing.T) {
	accountAddr := stdlibBytes32("1")
	authorityAddr := stdlibBytes32("2")
	bindingAddr := stdlibBytes32("3")
	sessionAddr := stdlibBytes32("4")
	sponsorAddr := stdlibBytes32("5")
	receiptAddr := stdlibBytes32("6")
	scope := stdlibBytes32("7")
	bindingID := stdlibBytes32("8")
	sessionID := stdlibBytes32("9")
	receiptID := stdlibBytes32("a")

	L, tos, host := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/PolicySponsoredCheckout.tol",
		LString(alice),
		LString(accountAddr),
		LString(authorityAddr),
		LString(bindingAddr),
		LString(sessionAddr),
		LString(sponsorAddr),
		LString(receiptAddr),
	)
	defer L.Close()

	expectedAddr := map[string]string{
		"PolicyAccount":        accountAddr,
		"AuthorityBook":        authorityAddr,
		"ExecutionBindingBook": bindingAddr,
		"SessionBook":          sessionAddr,
		"SponsorPolicyRelay":   sponsorAddr,
		"ReceiptBook":          receiptAddr,
	}

	var trace []string
	requiresStepUp := false
	receiptFinal := false
	host.packageCallHook = func(addr, contractName, calldata string) []LValue {
		sel := stdlibSelectorFromCalldata(calldata)
		trace = append(trace, contractName+":"+sel)
		if want := expectedAddr[contractName]; want == "" || addr != want {
			t.Fatalf("unexpected package call routing: contract=%s addr=%s want=%s", contractName, addr, want)
		}
		switch contractName {
		case "PolicyAccount":
			switch sel {
			case selectorHexFromSignature("isSuspended()"):
				return []LValue{LFalse}
			case selectorHexFromSignature("remainingDaily()"):
				return []LValue{lu256FromInt(800)}
			}
		case "AuthorityBook":
			switch sel {
			case selectorHexFromSignature("isActive(agent,bytes32)"):
				return []LValue{LTrue}
			case selectorHexFromSignature("remainingOf(agent,bytes32)"):
				return []LValue{lu256FromInt(500)}
			}
		case "ExecutionBindingBook":
			if sel == selectorHexFromSignature("isConsumable(bytes32)") {
				return []LValue{LTrue}
			}
		case "SessionBook":
			switch sel {
			case selectorHexFromSignature("isActive(bytes32)"):
				return []LValue{LTrue}
			case selectorHexFromSignature("requiresStepUp(bytes32,u256)"):
				return []LValue{LBool(requiresStepUp)}
			}
		case "SponsorPolicyRelay":
			if sel == selectorHexFromSignature("isRelayerActive(agent)") {
				return []LValue{LTrue}
			}
		case "ReceiptBook":
			switch sel {
			case selectorHexFromSignature("isFinalized(bytes32)"):
				return []LValue{LBool(receiptFinal)}
			case selectorHexFromSignature("statusOf(bytes32)"):
				return []LValue{lu256FromInt(2)}
			}
		}
		t.Fatalf("unexpected package call: contract=%s selector=%s calldata=%s", contractName, sel, calldata)
		return nil
	}

	got := invokeStdlib(
		t,
		L,
		tos,
		"preauthorize(agent,agent,bytes32,bytes32,bytes32,bytes32,u256)",
		LString(bob),
		LString(charlie),
		LString(scope),
		LString(bindingID),
		LString(sessionID),
		LString(receiptID),
		lu256FromInt(200),
	)
	if !LVAsBool(got) {
		t.Fatalf("preauthorize should return true, got %v", got)
	}
	if host.packageCallCount != 8 {
		t.Fatalf("package_call count after preauthorize: got=%d want=8", host.packageCallCount)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "dailyRemaining()")); got != "800" {
		t.Fatalf("dailyRemaining: got=%s want=800", got)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "receiptStatus(bytes32)", LString(receiptID))); got != "2" {
		t.Fatalf("receiptStatus: got=%s want=2", got)
	}

	requiresStepUp = true
	errMsg := invokeStdlibErr(
		t,
		L,
		tos,
		"preauthorize(agent,agent,bytes32,bytes32,bytes32,bytes32,u256)",
		LString(bob),
		LString(charlie),
		LString(scope),
		LString(bindingID),
		LString(sessionID),
		LString(receiptID),
		lu256FromInt(200),
	)
	if !strings.Contains(errMsg, "STEP_UP_REQUIRED") {
		t.Fatalf("expected STEP_UP_REQUIRED, got %q", errMsg)
	}
	if len(trace) < 8 {
		t.Fatalf("expected composed package call trace, got %v", trace)
	}
}

func TestPrivateServiceOrderRuntimeComposedFlow(t *testing.T) {
	agreementAddr := stdlibBytes32("1")
	settlementAddr := stdlibBytes32("2")
	evidenceAddr := stdlibBytes32("3")
	trustAddr := stdlibBytes32("4")
	discoveryAddr := stdlibBytes32("5")
	vaultAddr := stdlibBytes32("6")
	evidenceID := stdlibBytes32("7")
	manifestRef := stdlibBytes32("8")

	L, tos, host := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/PrivateServiceOrder.tol",
		LString(alice),
		LString(agreementAddr),
		LString(settlementAddr),
		LString(evidenceAddr),
		LString(trustAddr),
		LString(discoveryAddr),
		LString(vaultAddr),
	)
	defer L.Close()

	expectedAddr := map[string]string{
		"CommercialAgreement": agreementAddr,
		"TaskSettlement":      settlementAddr,
		"EvidenceBook":        evidenceAddr,
		"TrustRegistry":       trustAddr,
		"ServiceDirectory":    discoveryAddr,
		"ConfidentialVault":   vaultAddr,
	}

	auditAllowed := true
	var trace []string
	host.packageCallHook = func(addr, contractName, calldata string) []LValue {
		sel := stdlibSelectorFromCalldata(calldata)
		trace = append(trace, contractName+":"+sel)
		if want := expectedAddr[contractName]; want == "" || addr != want {
			t.Fatalf("unexpected package call routing: contract=%s addr=%s want=%s", contractName, addr, want)
		}
		switch contractName {
		case "TrustRegistry":
			if sel == selectorHexFromSignature("isEligible(agent)") {
				return []LValue{LTrue}
			}
		case "ServiceDirectory":
			switch sel {
			case selectorHexFromSignature("isActive(u256)"):
				return []LValue{LTrue}
			case selectorHexFromSignature("providerOf(u256)"):
				return []LValue{LString(bob)}
			case selectorHexFromSignature("manifestRefOf(u256)"):
				return []LValue{LString(manifestRef)}
			}
		case "CommercialAgreement":
			if sel == selectorHexFromSignature("statusOf(u256)") {
				return []LValue{lu256FromInt(2)}
			}
		case "TaskSettlement":
			if sel == selectorHexFromSignature("statusOf(u256)") {
				return []LValue{lu256FromInt(3)}
			}
		case "EvidenceBook":
			if sel == selectorHexFromSignature("statusOf(bytes32)") {
				return []LValue{lu256FromInt(4)}
			}
		case "ConfidentialVault":
			switch sel {
			case selectorHexFromSignature("canAudit(agent,agent)"):
				return []LValue{LBool(auditAllowed)}
			case selectorHexFromSignature("balanceOf(agent)"):
				return []LValue{stdlibUnoFromInt(77)}
			}
		}
		t.Fatalf("unexpected package call: contract=%s selector=%s calldata=%s", contractName, sel, calldata)
		return nil
	}

	got := invokeStdlib(
		t,
		L,
		tos,
		"ready(agent,agent,agent,u256,u256,u256,bytes32)",
		LString(bob),
		LString(alice),
		LString(charlie),
		lu256FromInt(1),
		lu256FromInt(2),
		lu256FromInt(3),
		LString(evidenceID),
	)
	if !LVAsBool(got) {
		t.Fatalf("ready should return true, got %v", got)
	}
	if host.packageCallCount != 7 {
		t.Fatalf("package_call count after ready: got=%d want=7", host.packageCallCount)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "customerVaultBalance(agent)", LString(alice))); got != LVAsString(stdlibUnoFromInt(77)) {
		t.Fatalf("customerVaultBalance: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(77)))
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "serviceManifest(u256)", lu256FromInt(1))); got != manifestRef {
		t.Fatalf("serviceManifest: got=%s want=%s", got, manifestRef)
	}

	auditAllowed = false
	errMsg := invokeStdlibErr(
		t,
		L,
		tos,
		"ready(agent,agent,agent,u256,u256,u256,bytes32)",
		LString(bob),
		LString(alice),
		LString(charlie),
		lu256FromInt(1),
		lu256FromInt(2),
		lu256FromInt(3),
		LString(evidenceID),
	)
	if !strings.Contains(errMsg, "AUDIT_NOT_ALLOWED") {
		t.Fatalf("expected AUDIT_NOT_ALLOWED, got %q", errMsg)
	}
	if len(trace) < 7 {
		t.Fatalf("expected composed package call trace, got %v", trace)
	}
}

func TestPrivateEscrowCheckoutRuntimeComposedFlow(t *testing.T) {
	escrowAddr := stdlibBytes32("1")
	receiptAddr := stdlibBytes32("2")
	escrowID := stdlibBytes32("3")
	receiptID := stdlibBytes32("4")
	bindingRef := stdlibBytes32("5")
	proofRef := stdlibBytes32("6")

	L, tos, host := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/PrivateEscrowCheckout.tol",
		LString(alice),
		LString(escrowAddr),
		LString(receiptAddr),
	)
	defer L.Close()

	expectedAddr := map[string]string{
		"ConfidentialEscrow": escrowAddr,
		"ReceiptBook":        receiptAddr,
	}

	proofMatches := true
	var trace []string
	host.packageCallHook = func(addr, contractName, calldata string) []LValue {
		sel := stdlibSelectorFromCalldata(calldata)
		trace = append(trace, contractName+":"+sel)
		if want := expectedAddr[contractName]; want == "" || addr != want {
			t.Fatalf("unexpected package call routing: contract=%s addr=%s want=%s", contractName, addr, want)
		}
		switch contractName {
		case "ConfidentialEscrow":
			switch sel {
			case selectorHexFromSignature("statusOf(bytes32)"):
				return []LValue{lu256FromInt(1)}
			case selectorHexFromSignature("payerOf(bytes32)"):
				return []LValue{LString(alice)}
			case selectorHexFromSignature("payeeOf(bytes32)"):
				return []LValue{LString(bob)}
			case selectorHexFromSignature("receiptRefOf(bytes32)"):
				return []LValue{LString(receiptID)}
			case selectorHexFromSignature("nativeBalance(agent)"):
				return []LValue{stdlibUnoFromInt(60)}
			}
		case "ReceiptBook":
			switch sel {
			case selectorHexFromSignature("statusOf(bytes32)"):
				return []LValue{lu256FromInt(1)}
			case selectorHexFromSignature("bindingRefOf(bytes32)"):
				return []LValue{LString(bindingRef)}
			case selectorHexFromSignature("proofRefOf(bytes32)"):
				if proofMatches {
					return []LValue{LString(proofRef)}
				}
				return []LValue{LString(stdlibBytes32("f"))}
			}
		}
		t.Fatalf("unexpected package call: contract=%s selector=%s calldata=%s", contractName, sel, calldata)
		return nil
	}

	if got := invokeStdlib(
		t,
		L,
		tos,
		"prepare(agent,agent,bytes32,bytes32,bytes32,bytes32)",
		LString(alice),
		LString(bob),
		LString(escrowID),
		LString(receiptID),
		LString(bindingRef),
		LString(proofRef),
	); !LVAsBool(got) {
		t.Fatalf("prepare should return true, got %v", got)
	}
	if host.packageCallCount != 7 {
		t.Fatalf("package_call count after prepare: got=%d want=7", host.packageCallCount)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "confidentialBalance(agent)", LString(bob))); got != LVAsString(stdlibUnoFromInt(60)) {
		t.Fatalf("confidentialBalance: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(60)))
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "receiptState(bytes32)", LString(receiptID))); got != "1" {
		t.Fatalf("receiptState: got=%s want=1", got)
	}

	proofMatches = false
	errMsg := invokeStdlibErr(
		t,
		L,
		tos,
		"prepare(agent,agent,bytes32,bytes32,bytes32,bytes32)",
		LString(alice),
		LString(bob),
		LString(escrowID),
		LString(receiptID),
		LString(bindingRef),
		LString(proofRef),
	)
	if !strings.Contains(errMsg, "PROOF_MISMATCH") {
		t.Fatalf("expected PROOF_MISMATCH, got %q", errMsg)
	}
	if len(trace) < 7 {
		t.Fatalf("expected composed package call trace, got %v", trace)
	}
}

func TestPolicySponsoredCheckoutRuntimeStatefulPackageFlow(t *testing.T) {
	accountAddr := stdlibBytes32("1")
	authorityAddr := stdlibBytes32("2")
	bindingAddr := stdlibBytes32("3")
	sessionAddr := stdlibBytes32("4")
	sponsorAddr := stdlibBytes32("5")
	receiptAddr := stdlibBytes32("6")
	coordinatorAddr := stdlibBytes32("7")

	scope := stdlibBytes32("8")
	policyHash := stdlibBytes32("9")
	sponsorPolicy := stdlibBytes32("a")
	proofRef := stdlibBytes32("b")
	intentRef := stdlibBytes32("c")
	bindingID := stdlibBytes32("d")
	sessionID := stdlibBytes32("e")
	sessionStepUpID := stdlibBytes32("f")
	terminalID := stdlibBytes32("1")
	receiptID := stdlibBytes32("2")
	externalRef := stdlibBytes32("3")
	executeReceiptID := stdlibBytes32("4")
	executeExternalRef := stdlibBytes32("5")
	resultRef := stdlibBytes32("6")
	settlementRef := stdlibBytes32("7")
	orderID := stdlibBytes32("8")

	accountL, accountTOS, accountHost := deployStdlibContract(
		t,
		"stdlib/account/PolicyAccount.tol",
		LString(alice),
		LString(charlie),
		lu256FromInt(1000),
		lu256FromInt(400),
	)
	defer accountL.Close()
	authorityL, authorityTOS, authorityHost := deployStdlibContract(t, "stdlib/authority/AuthorityBook.tol", LString(coordinatorAddr))
	defer authorityL.Close()
	bindingL, bindingTOS, bindingHost := deployStdlibContract(t, "stdlib/execution_binding/ExecutionBindingBook.tol", LString(coordinatorAddr))
	defer bindingL.Close()
	sessionL, sessionTOS, sessionHost := deployStdlibContract(t, "stdlib/session_book/SessionBook.tol", LString(coordinatorAddr))
	defer sessionL.Close()
	sponsorL, sponsorTOS, sponsorHost := deployStdlibContract(t, "stdlib/sponsor/SponsorPolicyRelay.tol", LString(alice))
	defer sponsorL.Close()
	receiptL, receiptTOS, receiptHost := deployStdlibContract(t, "stdlib/receipt/ReceiptBook.tol", LString(coordinatorAddr))
	defer receiptL.Close()
	targetL, targetTOS, targetHost := deployStdlibSourceContract(t, []byte(stdlibCallTargetSource), "<checkout-target>")
	defer targetL.Close()

	stdlibSetSender(accountHost, alice)
	invokeStdlib(t, accountL, accountTOS, "setAllowlistEnabled(bool)", LTrue)
	invokeStdlib(t, accountL, accountTOS, "setAllowlisted(agent,bool)", LString(stdlibMerchant), LTrue)
	invokeStdlib(t, accountL, accountTOS, "authorizeDelegate(agent,u256,u256)", LString(bob), lu256FromInt(300), lu256FromInt(1000))
	invokeStdlib(t, accountL, accountTOS, "authorizeDelegate(agent,u256,u256)", LString(coordinatorAddr), lu256FromInt(400), lu256FromInt(1000))
	stdlibSetSender(accountHost, bob)
	stdlibSetTimestamp(accountHost, 100)
	invokeStdlib(t, accountL, accountTOS, "execute(agent,bytes,u256)", LString(stdlibMerchant), LString("0x1234"), lu256FromInt(200))

	stdlibSetSender(authorityHost, coordinatorAddr)
	stdlibSetTimestamp(authorityHost, 100)
	invokeStdlib(t, authorityL, authorityTOS, "grant(agent,bytes32,u256,u256,bytes32,u256)", LString(bob), LString(scope), lu256FromInt(500), lu256FromInt(1000), LString(policyHash), lu256FromInt(10))

	stdlibSetSender(bindingHost, coordinatorAddr)
	stdlibSetTimestamp(bindingHost, 100)
	invokeStdlib(
		t,
		bindingL,
		bindingTOS,
		"approve(bytes32,agent,agent,u256,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(bindingID),
		LString(bob),
		LString(stdlibMerchant),
		lu256FromInt(250),
		lu256FromInt(1000),
		LString(policyHash),
		LString(sponsorPolicy),
		LString(proofRef),
		LString(intentRef),
	)

	stdlibSetSender(sessionHost, coordinatorAddr)
	invokeStdlib(
		t,
		sessionL,
		sessionTOS,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(sessionID),
		LString(bob),
		LString(terminalID),
		lu256FromInt(1),
		lu256FromInt(2),
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(250),
		LFalse,
	)
	invokeStdlib(
		t,
		sessionL,
		sessionTOS,
		"grantSession(bytes32,agent,bytes32,u256,u256,u256,u256,u256,bool)",
		LString(sessionStepUpID),
		LString(bob),
		LString(terminalID),
		lu256FromInt(1),
		lu256FromInt(2),
		lu256FromInt(1000),
		lu256FromInt(500),
		lu256FromInt(100),
		LFalse,
	)
	stdlibSetTimestamp(sessionHost, 150)

	stdlibSetSender(sponsorHost, alice)
	invokeStdlib(t, sponsorL, sponsorTOS, "authorizeRelayer(agent,u256,u256,bytes32)", LString(charlie), lu256FromInt(400), lu256FromInt(1000), LString(policyHash))

	stdlibSetSender(receiptHost, coordinatorAddr)
	stdlibSetTimestamp(receiptHost, 150)
	invokeStdlib(
		t,
		receiptL,
		receiptTOS,
		"openReceipt(bytes32,agent,agent,agent,agent,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(receiptID),
		LString(alice),
		LString(bob),
		LString(charlie),
		LString(stdlibMerchant),
		lu256FromInt(200),
		LString(policyHash),
		LString(bindingID),
		LString(proofRef),
		LString(externalRef),
	)

	coordL, coordTOS, coordHost := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/PolicySponsoredCheckout.tol",
		LString(alice),
		LString(accountAddr),
		LString(authorityAddr),
		LString(bindingAddr),
		LString(sessionAddr),
		LString(sponsorAddr),
		LString(receiptAddr),
	)
	defer coordL.Close()

	attachActualPackageRouter(t, coordHost, coordinatorAddr,
		&stdlibDeployedPackageContract{name: "PolicyAccount", addr: accountAddr, L: accountL, tos: accountTOS, host: accountHost},
		&stdlibDeployedPackageContract{name: "AuthorityBook", addr: authorityAddr, L: authorityL, tos: authorityTOS, host: authorityHost},
		&stdlibDeployedPackageContract{name: "ExecutionBindingBook", addr: bindingAddr, L: bindingL, tos: bindingTOS, host: bindingHost},
		&stdlibDeployedPackageContract{name: "SessionBook", addr: sessionAddr, L: sessionL, tos: sessionTOS, host: sessionHost},
		&stdlibDeployedPackageContract{name: "SponsorPolicyRelay", addr: sponsorAddr, L: sponsorL, tos: sponsorTOS, host: sponsorHost},
		&stdlibDeployedPackageContract{name: "ReceiptBook", addr: receiptAddr, L: receiptL, tos: receiptTOS, host: receiptHost},
	)
	attachActualCallRouter(t, accountHost, accountAddr,
		&stdlibDeployedPackageContract{name: "CallTargetRecorder", addr: stdlibMerchant, L: targetL, tos: targetTOS, host: targetHost},
	)

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"preauthorize(agent,agent,bytes32,bytes32,bytes32,bytes32,u256)",
		LString(bob),
		LString(charlie),
		LString(scope),
		LString(bindingID),
		LString(sessionID),
		LString(receiptID),
		lu256FromInt(200),
	); !LVAsBool(got) {
		t.Fatalf("preauthorize should return true, got %v", got)
	}
	if coordHost.packageCallCount != 8 {
		t.Fatalf("package_call count after preauthorize: got=%d want=8", coordHost.packageCallCount)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "dailyRemaining()")); got != "800" {
		t.Fatalf("dailyRemaining: got=%s want=800", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "receiptStatus(bytes32)", LString(receiptID))); got != "1" {
		t.Fatalf("receiptStatus: got=%s want=1", got)
	}

	errMsg := invokeStdlibErr(
		t,
		coordL,
		coordTOS,
		"preauthorize(agent,agent,bytes32,bytes32,bytes32,bytes32,u256)",
		LString(bob),
		LString(charlie),
		LString(scope),
		LString(bindingID),
		LString(sessionStepUpID),
		LString(receiptID),
		lu256FromInt(200),
	)
	if !strings.Contains(errMsg, "STEP_UP_REQUIRED") {
		t.Fatalf("expected STEP_UP_REQUIRED, got %q", errMsg)
	}

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"executeCheckout(agent,agent,agent,bytes,bytes32,bytes32,bytes32,bytes32,bytes32,u256,u256,bool,bytes32,bytes32)",
		LString(bob),
		LString(charlie),
		LString(stdlibMerchant),
		LString(stdlibEncodeStaticCalldata("record(bytes32,u256)", orderID, "c8")),
		LString(scope),
		LString(bindingID),
		LString(sessionID),
		LString(executeReceiptID),
		LString(executeExternalRef),
		lu256FromInt(200),
		lu256FromInt(11),
		LFalse,
		LString(resultRef),
		LString(settlementRef),
	); !LVAsBool(got) {
		t.Fatalf("executeCheckout should return true, got %v", got)
	}
	if got := LVAsString(invokeStdlib(t, accountL, accountTOS, "remainingDaily()")); got != "600" {
		t.Fatalf("remainingDaily after executeCheckout: got=%s want=600", got)
	}
	if got := LVAsString(invokeStdlib(t, authorityL, authorityTOS, "remainingOf(agent,bytes32)", LString(bob), LString(scope))); got != "300" {
		t.Fatalf("authority remaining after executeCheckout: got=%s want=300", got)
	}
	if got := LVAsString(invokeStdlib(t, sessionL, sessionTOS, "remainingOf(bytes32)", LString(sessionID))); got != "300" {
		t.Fatalf("session remaining after executeCheckout: got=%s want=300", got)
	}
	if got := LVAsBool(invokeStdlib(t, bindingL, bindingTOS, "isConsumable(bytes32)", LString(bindingID))); got {
		t.Fatal("binding should be consumed after executeCheckout")
	}
	if got := LVAsString(invokeStdlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(executeReceiptID))); got != "2" {
		t.Fatalf("execute receipt status: got=%s want=2", got)
	}
	if got := LVAsString(invokeStdlib(t, receiptL, receiptTOS, "bindingRefOf(bytes32)", LString(executeReceiptID))); got != bindingID {
		t.Fatalf("execute receipt binding ref: got=%s want=%s", got, bindingID)
	}
	if accountHost.callCount != 2 {
		t.Fatalf("account call count after executeCheckout: got=%d want=2", accountHost.callCount)
	}
	if accountHost.lastCallAddr != stdlibMerchant {
		t.Fatalf("account last call addr: got=%s want=%s", accountHost.lastCallAddr, stdlibMerchant)
	}
	if accountHost.lastCallData != stdlibEncodeStaticCalldata("record(bytes32,u256)", orderID, "c8") {
		t.Fatalf("account last call data: got=%s want=%s", accountHost.lastCallData, stdlibEncodeStaticCalldata("record(bytes32,u256)", orderID, "c8"))
	}
	if got := LVAsString(invokeStdlib(t, targetL, targetTOS, "lastOrderId()")); got != orderID {
		t.Fatalf("target lastOrderId after executeCheckout: got=%s want=%s", got, orderID)
	}
	if got := LVAsString(invokeStdlib(t, targetL, targetTOS, "lastAmount()")); got != "200" {
		t.Fatalf("target lastAmount after executeCheckout: got=%s want=200", got)
	}
	if got := LVAsString(invokeStdlib(t, targetL, targetTOS, "callCount()")); got != "1" {
		t.Fatalf("target callCount after executeCheckout: got=%s want=1", got)
	}
}

func TestPrivateEscrowCheckoutRuntimeStatefulPackageFlow(t *testing.T) {
	escrowAddr := stdlibBytes32("1")
	receiptAddr := stdlibBytes32("2")
	coordinatorAddr := stdlibBytes32("3")

	escrowOne := stdlibBytes32("4")
	receiptOne := stdlibBytes32("5")
	bindingRefOne := stdlibBytes32("6")
	proofRefOne := stdlibBytes32("7")
	externalRefOne := stdlibBytes32("8")
	policyHashOne := stdlibBytes32("9")
	resultRefOne := stdlibBytes32("a")
	settlementRefOne := stdlibBytes32("b")

	escrowTwo := stdlibBytes32("c")
	receiptTwo := stdlibBytes32("d")
	bindingRefTwo := stdlibBytes32("e")
	proofRefTwo := stdlibBytes32("f")
	externalRefTwo := stdlibBytes32("1")
	policyHashTwo := stdlibBytes32("2")
	resultRefTwo := stdlibBytes32("3")
	reasonRefTwo := stdlibBytes32("4")

	escrowL, escrowTOS, escrowHost := deployStdlibContract(t, "stdlib/privacy/ConfidentialEscrow.tol", LString(coordinatorAddr))
	defer escrowL.Close()
	receiptL, receiptTOS, receiptHost := deployStdlibContract(t, "stdlib/receipt/ReceiptBook.tol", LString(coordinatorAddr))
	defer receiptL.Close()

	stdlibSetSender(escrowHost, alice)
	stdlibSetTimestamp(escrowHost, 100)
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(60))
	invokeStdlib(t, escrowL, escrowTOS, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowOne), LString(bob), lu256FromInt(500), LString(receiptOne))
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(0))

	stdlibSetSender(receiptHost, coordinatorAddr)
	stdlibSetTimestamp(receiptHost, 100)
	invokeStdlib(
		t,
		receiptL,
		receiptTOS,
		"openReceipt(bytes32,agent,agent,agent,agent,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(receiptOne),
		LString(alice),
		LString(coordinatorAddr),
		LString(charlie),
		LString(stdlibService),
		lu256FromInt(60),
		LString(policyHashOne),
		LString(bindingRefOne),
		LString(proofRefOne),
		LString(externalRefOne),
	)

	coordL, coordTOS, coordHost := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/PrivateEscrowCheckout.tol",
		LString(alice),
		LString(escrowAddr),
		LString(receiptAddr),
	)
	defer coordL.Close()

	attachActualPackageRouter(t, coordHost, coordinatorAddr,
		&stdlibDeployedPackageContract{name: "ConfidentialEscrow", addr: escrowAddr, L: escrowL, tos: escrowTOS, host: escrowHost},
		&stdlibDeployedPackageContract{name: "ReceiptBook", addr: receiptAddr, L: receiptL, tos: receiptTOS, host: receiptHost},
	)

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"prepare(agent,agent,bytes32,bytes32,bytes32,bytes32)",
		LString(alice),
		LString(bob),
		LString(escrowOne),
		LString(receiptOne),
		LString(bindingRefOne),
		LString(proofRefOne),
	); !LVAsBool(got) {
		t.Fatalf("prepare should return true, got %v", got)
	}
	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"settleAndRelease(bytes32,bytes32,bytes32,bytes32)",
		LString(escrowOne),
		LString(receiptOne),
		LString(resultRefOne),
		LString(settlementRefOne),
	); !LVAsBool(got) {
		t.Fatalf("settleAndRelease should return true, got %v", got)
	}
	if got := LVAsString(invokeStdlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptOne))); got != "2" {
		t.Fatalf("receipt one status after settle: got=%s want=2", got)
	}
	if got := LVAsString(invokeStdlib(t, escrowL, escrowTOS, "statusOf(bytes32)", LString(escrowOne))); got != "2" {
		t.Fatalf("escrow one status after release: got=%s want=2", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "receiptState(bytes32)", LString(receiptOne))); got != "2" {
		t.Fatalf("coordinator receiptState one: got=%s want=2", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "confidentialBalance(agent)", LString(bob))); got != LVAsString(stdlibUnoFromInt(60)) {
		t.Fatalf("confidentialBalance bob after release: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(60)))
	}

	stdlibSetSender(escrowHost, alice)
	stdlibSetTimestamp(escrowHost, 150)
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(25))
	invokeStdlib(t, escrowL, escrowTOS, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowTwo), LString(bob), lu256FromInt(550), LString(receiptTwo))
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(0))

	stdlibSetSender(receiptHost, coordinatorAddr)
	stdlibSetTimestamp(receiptHost, 150)
	invokeStdlib(
		t,
		receiptL,
		receiptTOS,
		"openReceipt(bytes32,agent,agent,agent,agent,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(receiptTwo),
		LString(alice),
		LString(coordinatorAddr),
		LString(charlie),
		LString(stdlibService),
		lu256FromInt(25),
		LString(policyHashTwo),
		LString(bindingRefTwo),
		LString(proofRefTwo),
		LString(externalRefTwo),
	)

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"failAndRefund(bytes32,bytes32,bytes32,bytes32)",
		LString(escrowTwo),
		LString(receiptTwo),
		LString(resultRefTwo),
		LString(reasonRefTwo),
	); !LVAsBool(got) {
		t.Fatalf("failAndRefund should return true, got %v", got)
	}
	if got := LVAsString(invokeStdlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptTwo))); got != "3" {
		t.Fatalf("receipt two status after failure: got=%s want=3", got)
	}
	if got := LVAsString(invokeStdlib(t, escrowL, escrowTOS, "statusOf(bytes32)", LString(escrowTwo))); got != "3" {
		t.Fatalf("escrow two status after refund: got=%s want=3", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "confidentialBalance(agent)", LString(alice))); got != LVAsString(stdlibUnoFromInt(25)) {
		t.Fatalf("confidentialBalance alice after refund: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(25)))
	}
}

func TestSponsoredPrivateEscrowCheckoutRuntimeComposedFlow(t *testing.T) {
	bindingAddr := stdlibBytes32("1")
	sponsorAddr := stdlibBytes32("2")
	receiptAddr := stdlibBytes32("3")
	escrowAddr := stdlibBytes32("4")
	relayActor := stdlibBytes32("5")
	bindingID := stdlibBytes32("6")
	receiptID := stdlibBytes32("7")
	escrowID := stdlibBytes32("8")

	L, tos, host := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/SponsoredPrivateEscrowCheckout.tol",
		LString(alice),
		LString(relayActor),
		LString(charlie),
		LString(bindingAddr),
		LString(sponsorAddr),
		LString(receiptAddr),
		LString(escrowAddr),
	)
	defer L.Close()

	expectedAddr := map[string]string{
		"ExecutionBindingBook": bindingAddr,
		"SponsorPolicyRelay":   sponsorAddr,
		"ReceiptBook":          receiptAddr,
		"ConfidentialEscrow":   escrowAddr,
	}

	relayerActive := true
	var trace []string
	host.packageCallHook = func(addr, contractName, calldata string) []LValue {
		sel := stdlibSelectorFromCalldata(calldata)
		trace = append(trace, contractName+":"+sel)
		if want := expectedAddr[contractName]; want == "" || addr != want {
			t.Fatalf("unexpected package call routing: contract=%s addr=%s want=%s", contractName, addr, want)
		}
		switch contractName {
		case "ExecutionBindingBook":
			if sel == selectorHexFromSignature("isConsumable(bytes32)") {
				return []LValue{LTrue}
			}
		case "SponsorPolicyRelay":
			switch sel {
			case selectorHexFromSignature("isRelayerActive(agent)"):
				return []LValue{LBool(relayerActive)}
			case selectorHexFromSignature("remainingOf(agent)"):
				return []LValue{lu256FromInt(90)}
			}
		case "ReceiptBook":
			if sel == selectorHexFromSignature("statusOf(bytes32)") {
				return []LValue{lu256FromInt(0)}
			}
		case "ConfidentialEscrow":
			switch sel {
			case selectorHexFromSignature("statusOf(bytes32)"):
				return []LValue{lu256FromInt(1)}
			case selectorHexFromSignature("payerOf(bytes32)"):
				return []LValue{LString(alice)}
			case selectorHexFromSignature("payeeOf(bytes32)"):
				return []LValue{LString(bob)}
			case selectorHexFromSignature("receiptRefOf(bytes32)"):
				return []LValue{LString(receiptID)}
			case selectorHexFromSignature("nativeBalance(agent)"):
				return []LValue{stdlibUnoFromInt(44)}
			}
		}
		t.Fatalf("unexpected package call: contract=%s selector=%s calldata=%s", contractName, sel, calldata)
		return nil
	}

	if got := invokeStdlib(
		t,
		L,
		tos,
		"preflight(agent,agent,bytes32,bytes32,bytes32,u256)",
		LString(alice),
		LString(bob),
		LString(bindingID),
		LString(receiptID),
		LString(escrowID),
		lu256FromInt(44),
	); !LVAsBool(got) {
		t.Fatalf("preflight should return true, got %v", got)
	}
	if host.packageCallCount != 7 {
		t.Fatalf("package_call count after preflight: got=%d want=7", host.packageCallCount)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "sponsorRemaining()")); got != "90" {
		t.Fatalf("sponsorRemaining: got=%s want=90", got)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "receiptStatus(bytes32)", LString(receiptID))); got != "0" {
		t.Fatalf("receiptStatus: got=%s want=0", got)
	}
	if got := LVAsString(invokeStdlib(t, L, tos, "confidentialBalance(agent)", LString(bob))); got != LVAsString(stdlibUnoFromInt(44)) {
		t.Fatalf("confidentialBalance: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(44)))
	}

	relayerActive = false
	errMsg := invokeStdlibErr(
		t,
		L,
		tos,
		"preflight(agent,agent,bytes32,bytes32,bytes32,u256)",
		LString(alice),
		LString(bob),
		LString(bindingID),
		LString(receiptID),
		LString(escrowID),
		lu256FromInt(44),
	)
	if !strings.Contains(errMsg, "RELAYER_INACTIVE") {
		t.Fatalf("expected RELAYER_INACTIVE, got %q", errMsg)
	}
	if len(trace) < 7 {
		t.Fatalf("expected composed package call trace, got %v", trace)
	}
}

func TestSponsoredPrivateEscrowCheckoutRuntimeStatefulPackageFlow(t *testing.T) {
	bindingAddr := stdlibBytes32("1")
	sponsorAddr := stdlibBytes32("2")
	receiptAddr := stdlibBytes32("3")
	escrowAddr := stdlibBytes32("4")
	coordinatorAddr := stdlibBytes32("5")
	relayActor := coordinatorAddr

	bindingOne := stdlibBytes32("6")
	receiptOne := stdlibBytes32("7")
	escrowOne := stdlibBytes32("8")
	policyHashOne := stdlibBytes32("9")
	proofRefOne := stdlibBytes32("a")
	intentRefOne := stdlibBytes32("b")
	externalRefOne := stdlibBytes32("c")
	resultRefOne := stdlibBytes32("d")
	settlementRefOne := stdlibBytes32("e")
	orderOne := stdlibBytes32("f")

	bindingTwo := stdlibBytes32("1")
	receiptTwo := stdlibBytes32("2")
	escrowTwo := stdlibBytes32("3")
	policyHashTwo := stdlibBytes32("4")
	proofRefTwo := stdlibBytes32("5")
	intentRefTwo := stdlibBytes32("6")
	externalRefTwo := stdlibBytes32("7")
	resultRefTwo := stdlibBytes32("8")
	reasonRefTwo := stdlibBytes32("9")

	bindingL, bindingTOS, bindingHost := deployStdlibContract(t, "stdlib/execution_binding/ExecutionBindingBook.tol", LString(coordinatorAddr))
	defer bindingL.Close()
	sponsorL, sponsorTOS, sponsorHost := deployStdlibContract(t, "stdlib/sponsor/SponsorPolicyRelay.tol", LString(alice))
	defer sponsorL.Close()
	receiptL, receiptTOS, receiptHost := deployStdlibContract(t, "stdlib/receipt/ReceiptBook.tol", LString(coordinatorAddr))
	defer receiptL.Close()
	escrowL, escrowTOS, escrowHost := deployStdlibContract(t, "stdlib/privacy/ConfidentialEscrow.tol", LString(coordinatorAddr))
	defer escrowL.Close()
	targetL, targetTOS, targetHost := deployStdlibSourceContract(t, []byte(stdlibCallTargetSource), "<sponsored-private-target>")
	defer targetL.Close()

	stdlibSetSender(sponsorHost, alice)
	stdlibSetValue(sponsorHost, 200)
	invokeStdlib(t, sponsorL, sponsorTOS, "deposit()")
	stdlibSetValue(sponsorHost, 0)
	stdlibSetTimestamp(sponsorHost, 100)
	invokeStdlib(t, sponsorL, sponsorTOS, "authorizeRelayer(agent,u256,u256,bytes32)", LString(relayActor), lu256FromInt(150), lu256FromInt(1000), LString(policyHashOne))

	stdlibSetSender(bindingHost, coordinatorAddr)
	stdlibSetTimestamp(bindingHost, 100)
	invokeStdlib(
		t,
		bindingL,
		bindingTOS,
		"approve(bytes32,agent,agent,u256,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(bindingOne),
		LString(relayActor),
		LString(stdlibMerchant),
		lu256FromInt(70),
		lu256FromInt(1000),
		LString(policyHashOne),
		LString(policyHashOne),
		LString(proofRefOne),
		LString(intentRefOne),
	)
	invokeStdlib(
		t,
		bindingL,
		bindingTOS,
		"approve(bytes32,agent,agent,u256,u256,bytes32,bytes32,bytes32,bytes32)",
		LString(bindingTwo),
		LString(relayActor),
		LString(stdlibMerchant),
		lu256FromInt(30),
		lu256FromInt(1000),
		LString(policyHashTwo),
		LString(policyHashTwo),
		LString(proofRefTwo),
		LString(intentRefTwo),
	)

	stdlibSetSender(escrowHost, alice)
	stdlibSetTimestamp(escrowHost, 100)
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(70))
	invokeStdlib(t, escrowL, escrowTOS, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowOne), LString(bob), lu256FromInt(500), LString(receiptOne))
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(0))

	stdlibSetTimestamp(escrowHost, 150)
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(30))
	invokeStdlib(t, escrowL, escrowTOS, "openEscrow(bytes32,agent,u256,bytes32)", LString(escrowTwo), LString(bob), lu256FromInt(550), LString(receiptTwo))
	stdlibSetUnoValue(escrowHost, stdlibUnoFromInt(0))

	coordL, coordTOS, coordHost := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/SponsoredPrivateEscrowCheckout.tol",
		LString(alice),
		LString(relayActor),
		LString(alice),
		LString(bindingAddr),
		LString(sponsorAddr),
		LString(receiptAddr),
		LString(escrowAddr),
	)
	defer coordL.Close()

	attachActualPackageRouter(t, coordHost, coordinatorAddr,
		&stdlibDeployedPackageContract{name: "ExecutionBindingBook", addr: bindingAddr, L: bindingL, tos: bindingTOS, host: bindingHost},
		&stdlibDeployedPackageContract{name: "SponsorPolicyRelay", addr: sponsorAddr, L: sponsorL, tos: sponsorTOS, host: sponsorHost},
		&stdlibDeployedPackageContract{name: "ReceiptBook", addr: receiptAddr, L: receiptL, tos: receiptTOS, host: receiptHost},
		&stdlibDeployedPackageContract{name: "ConfidentialEscrow", addr: escrowAddr, L: escrowL, tos: escrowTOS, host: escrowHost},
	)
	attachActualCallRouter(t, sponsorHost, sponsorAddr,
		&stdlibDeployedPackageContract{name: "CallTargetRecorder", addr: stdlibMerchant, L: targetL, tos: targetTOS, host: targetHost},
	)

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"preflight(agent,agent,bytes32,bytes32,bytes32,u256)",
		LString(alice),
		LString(bob),
		LString(bindingOne),
		LString(receiptOne),
		LString(escrowOne),
		lu256FromInt(70),
	); !LVAsBool(got) {
		t.Fatalf("preflight should return true, got %v", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "sponsorRemaining()")); got != "150" {
		t.Fatalf("sponsorRemaining before execute: got=%s want=150", got)
	}

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"executeSponsoredRelease(agent,agent,agent,bytes,bytes32,bytes32,bytes32,u256,bytes32,bytes32,bytes32)",
		LString(alice),
		LString(bob),
		LString(stdlibMerchant),
		LString(stdlibEncodeStaticCalldata("record(bytes32,u256)", orderOne, "46")),
		LString(bindingOne),
		LString(receiptOne),
		LString(escrowOne),
		lu256FromInt(70),
		LString(externalRefOne),
		LString(resultRefOne),
		LString(settlementRefOne),
	); !LVAsBool(got) {
		t.Fatalf("executeSponsoredRelease should return true, got %v", got)
	}
	if got := LVAsBool(invokeStdlib(t, bindingL, bindingTOS, "isConsumable(bytes32)", LString(bindingOne))); got {
		t.Fatal("binding one should be consumed after executeSponsoredRelease")
	}
	if got := LVAsString(invokeStdlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptOne))); got != "2" {
		t.Fatalf("receipt one status: got=%s want=2", got)
	}
	if got := LVAsString(invokeStdlib(t, escrowL, escrowTOS, "statusOf(bytes32)", LString(escrowOne))); got != "2" {
		t.Fatalf("escrow one status: got=%s want=2", got)
	}
	if sponsorHost.lastCallAddr != stdlibMerchant {
		t.Fatalf("sponsor last call addr: got=%s want=%s", sponsorHost.lastCallAddr, stdlibMerchant)
	}
	if sponsorHost.lastCallData != stdlibEncodeStaticCalldata("record(bytes32,u256)", orderOne, "46") {
		t.Fatalf("sponsor last call data: got=%s want=%s", sponsorHost.lastCallData, stdlibEncodeStaticCalldata("record(bytes32,u256)", orderOne, "46"))
	}
	if got := LVAsString(invokeStdlib(t, targetL, targetTOS, "callCount()")); got != "1" {
		t.Fatalf("target callCount after executeSponsoredRelease: got=%s want=1", got)
	}
	if got := LVAsString(invokeStdlib(t, targetL, targetTOS, "lastAmount()")); got != "70" {
		t.Fatalf("target lastAmount after executeSponsoredRelease: got=%s want=70", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "confidentialBalance(agent)", LString(bob))); got != LVAsString(stdlibUnoFromInt(70)) {
		t.Fatalf("confidentialBalance bob after release: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(70)))
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "sponsorRemaining()")); got != "80" {
		t.Fatalf("sponsorRemaining after execute: got=%s want=80", got)
	}

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"abortSponsoredRefund(agent,agent,bytes32,bytes32,bytes32,u256,bytes32,bytes32,bytes32)",
		LString(alice),
		LString(bob),
		LString(bindingTwo),
		LString(receiptTwo),
		LString(escrowTwo),
		lu256FromInt(30),
		LString(externalRefTwo),
		LString(resultRefTwo),
		LString(reasonRefTwo),
	); !LVAsBool(got) {
		t.Fatalf("abortSponsoredRefund should return true, got %v", got)
	}
	if got := LVAsBool(invokeStdlib(t, bindingL, bindingTOS, "isConsumable(bytes32)", LString(bindingTwo))); got {
		t.Fatal("binding two should be inactive after abortSponsoredRefund")
	}
	if got := LVAsString(invokeStdlib(t, receiptL, receiptTOS, "statusOf(bytes32)", LString(receiptTwo))); got != "3" {
		t.Fatalf("receipt two status: got=%s want=3", got)
	}
	if got := LVAsString(invokeStdlib(t, escrowL, escrowTOS, "statusOf(bytes32)", LString(escrowTwo))); got != "3" {
		t.Fatalf("escrow two status: got=%s want=3", got)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "confidentialBalance(agent)", LString(alice))); got != LVAsString(stdlibUnoFromInt(30)) {
		t.Fatalf("confidentialBalance alice after refund: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(30)))
	}
}

func TestPrivateServiceOrderRuntimeStatefulPackageFlow(t *testing.T) {
	agreementAddr := stdlibBytes32("1")
	settlementAddr := stdlibBytes32("2")
	evidenceAddr := stdlibBytes32("3")
	trustAddr := stdlibBytes32("4")
	discoveryAddr := stdlibBytes32("5")
	vaultAddr := stdlibBytes32("6")
	coordinatorAddr := stdlibBytes32("7")

	manifestRef := stdlibBytes32("8")
	capabilityRef := stdlibBytes32("9")
	versionRef := stdlibBytes32("a")
	quoteRef := stdlibBytes32("b")
	termsRef := stdlibBytes32("c")
	acceptanceRef := stdlibBytes32("d")
	taskRef := stdlibBytes32("e")
	taskReceiptRef := stdlibBytes32("f")
	resultRef := stdlibBytes32("1")
	taskProofRef := stdlibBytes32("2")
	evidenceID := stdlibBytes32("3")
	claimRef := stdlibBytes32("4")
	evidenceProofRef := stdlibBytes32("5")
	disclosureRef := stdlibBytes32("6")
	settlementRef := stdlibBytes32("7")

	agreementL, agreementTOS, agreementHost := deployStdlibContract(t, "stdlib/agreement/CommercialAgreement.tol")
	defer agreementL.Close()
	settlementL, settlementTOS, settlementHost := deployStdlibContract(t, "stdlib/settlement/TaskSettlement.tol", LString(charlie))
	defer settlementL.Close()
	evidenceL, evidenceTOS, evidenceHost := deployStdlibContract(t, "stdlib/evidence/EvidenceBook.tol", LString(coordinatorAddr))
	defer evidenceL.Close()
	trustL, trustTOS, trustHost := deployStdlibContract(t, "stdlib/trust/TrustRegistry.tol", LString(alice), lu256FromInt(100), lu256FromInt(5))
	defer trustL.Close()
	discoveryL, discoveryTOS, discoveryHost := deployStdlibContract(t, "stdlib/discovery/ServiceDirectory.tol")
	defer discoveryL.Close()
	vaultL, vaultTOS, vaultHost := deployStdlibContract(t, "stdlib/privacy/ConfidentialVault.tol")
	defer vaultL.Close()

	stdlibSetTimestamp(agreementHost, 100)
	stdlibSetSender(agreementHost, coordinatorAddr)
	invokeStdlib(
		t,
		agreementL,
		agreementTOS,
		"createOffer(agent,u256,u256,bytes32,bytes32)",
		LString(bob),
		lu256FromInt(250),
		lu256FromInt(500),
		LString(quoteRef),
		LString(termsRef),
	)
	stdlibSetTimestamp(agreementHost, 150)
	stdlibSetSender(agreementHost, bob)
	invokeStdlib(t, agreementL, agreementTOS, "accept(u256,bytes32)", lu256FromInt(1), LString(acceptanceRef))

	stdlibSetSender(settlementHost, coordinatorAddr)
	stdlibSetTimestamp(settlementHost, 100)
	stdlibSetValue(settlementHost, 70)
	invokeStdlib(
		t,
		settlementL,
		settlementTOS,
		"openTask(bytes32,u256,bytes32)",
		LString(taskRef),
		lu256FromInt(700),
		LString(taskReceiptRef),
	)
	stdlibSetValue(settlementHost, 0)
	stdlibSetSender(settlementHost, bob)
	stdlibSetTimestamp(settlementHost, 150)
	invokeStdlib(t, settlementL, settlementTOS, "acceptTask(u256)", lu256FromInt(1))
	invokeStdlib(t, settlementL, settlementTOS, "submitTask(u256,bytes32,bytes32)", lu256FromInt(1), LString(resultRef), LString(taskProofRef))

	stdlibSetSender(evidenceHost, coordinatorAddr)
	invokeStdlib(t, evidenceL, evidenceTOS, "setAttester(agent,bool)", LString(bob), LTrue)
	stdlibSetTimestamp(evidenceHost, 100)
	invokeStdlib(t, evidenceL, evidenceTOS, "openEvidence(bytes32,bytes32,agent,u256,u256)", LString(evidenceID), LString(claimRef), LString(bob), lu256FromInt(1000), lu256FromInt(50))
	stdlibSetSender(evidenceHost, bob)
	stdlibSetTimestamp(evidenceHost, 120)
	invokeStdlib(t, evidenceL, evidenceTOS, "fulfill(bytes32,u256,bytes32)", LString(evidenceID), lu256FromInt(42), LString(evidenceProofRef))
	stdlibSetSender(evidenceHost, charlie)
	stdlibSetTimestamp(evidenceHost, 171)
	invokeStdlib(t, evidenceL, evidenceTOS, "finalize(bytes32)", LString(evidenceID))

	stdlibSetAgentProp(trustHost, bob, "stake", lu256FromInt(150))
	stdlibSetAgentProp(trustHost, bob, "reputation", lu256FromInt(10))
	stdlibSetAgentProp(trustHost, bob, "suspended", lu256FromInt(0))

	stdlibSetSender(discoveryHost, bob)
	invokeStdlib(
		t,
		discoveryL,
		discoveryTOS,
		"registerService(bytes32,bytes32,bytes32,bytes32)",
		LString(manifestRef),
		LString(capabilityRef),
		LString(versionRef),
		LString(quoteRef),
	)

	stdlibSetSender(vaultHost, alice)
	stdlibSetUnoValue(vaultHost, stdlibUnoFromInt(77))
	invokeStdlib(t, vaultL, vaultTOS, "deposit()")
	stdlibSetUnoValue(vaultHost, stdlibUnoFromInt(0))
	invokeStdlib(t, vaultL, vaultTOS, "authorizeAuditor(agent,bytes32)", LString(charlie), LString(disclosureRef))

	coordL, coordTOS, coordHost := deployStdlibExampleContract(
		t,
		"examples/stdlib_composed/PrivateServiceOrder.tol",
		LString(alice),
		LString(agreementAddr),
		LString(settlementAddr),
		LString(evidenceAddr),
		LString(trustAddr),
		LString(discoveryAddr),
		LString(vaultAddr),
	)
	defer coordL.Close()

	attachActualPackageRouter(t, coordHost, coordinatorAddr,
		&stdlibDeployedPackageContract{name: "CommercialAgreement", addr: agreementAddr, L: agreementL, tos: agreementTOS, host: agreementHost},
		&stdlibDeployedPackageContract{name: "TaskSettlement", addr: settlementAddr, L: settlementL, tos: settlementTOS, host: settlementHost},
		&stdlibDeployedPackageContract{name: "EvidenceBook", addr: evidenceAddr, L: evidenceL, tos: evidenceTOS, host: evidenceHost},
		&stdlibDeployedPackageContract{name: "TrustRegistry", addr: trustAddr, L: trustL, tos: trustTOS, host: trustHost},
		&stdlibDeployedPackageContract{name: "ServiceDirectory", addr: discoveryAddr, L: discoveryL, tos: discoveryTOS, host: discoveryHost},
		&stdlibDeployedPackageContract{name: "ConfidentialVault", addr: vaultAddr, L: vaultL, tos: vaultTOS, host: vaultHost},
	)

	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"ready(agent,agent,agent,u256,u256,u256,bytes32)",
		LString(bob),
		LString(alice),
		LString(charlie),
		lu256FromInt(1),
		lu256FromInt(1),
		lu256FromInt(1),
		LString(evidenceID),
	); !LVAsBool(got) {
		t.Fatalf("ready should return true, got %v", got)
	}
	if coordHost.packageCallCount != 7 {
		t.Fatalf("package_call count after ready: got=%d want=7", coordHost.packageCallCount)
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "customerVaultBalance(agent)", LString(alice))); got != LVAsString(stdlibUnoFromInt(77)) {
		t.Fatalf("customerVaultBalance: got=%s want=%s", got, LVAsString(stdlibUnoFromInt(77)))
	}
	if got := LVAsString(invokeStdlib(t, coordL, coordTOS, "serviceManifest(u256)", lu256FromInt(1))); got != manifestRef {
		t.Fatalf("serviceManifest: got=%s want=%s", got, manifestRef)
	}
	if got := invokeStdlib(
		t,
		coordL,
		coordTOS,
		"settleReadyOrder(agent,agent,agent,u256,u256,u256,bytes32,bytes32)",
		LString(bob),
		LString(alice),
		LString(charlie),
		lu256FromInt(1),
		lu256FromInt(1),
		lu256FromInt(1),
		LString(evidenceID),
		LString(settlementRef),
	); !LVAsBool(got) {
		t.Fatalf("settleReadyOrder should return true, got %v", got)
	}
	if got := LVAsString(invokeStdlib(t, settlementL, settlementTOS, "statusOf(u256)", lu256FromInt(1))); got != "4" {
		t.Fatalf("settlement status after settleReadyOrder: got=%s want=4", got)
	}
	if got := LVAsString(invokeStdlib(t, agreementL, agreementTOS, "statusOf(u256)", lu256FromInt(1))); got != "4" {
		t.Fatalf("agreement status after settleReadyOrder: got=%s want=4", got)
	}
	if got := LVAsString(invokeStdlib(t, agreementL, agreementTOS, "settlementRefOf(u256)", lu256FromInt(1))); got != settlementRef {
		t.Fatalf("agreement settlement ref after settleReadyOrder: got=%s want=%s", got, settlementRef)
	}

	stdlibSetSender(discoveryHost, bob)
	invokeStdlib(t, discoveryL, discoveryTOS, "deactivate(u256)", lu256FromInt(1))
	errMsg := invokeStdlibErr(
		t,
		coordL,
		coordTOS,
		"ready(agent,agent,agent,u256,u256,u256,bytes32)",
		LString(bob),
		LString(alice),
		LString(charlie),
		lu256FromInt(1),
		lu256FromInt(1),
		lu256FromInt(1),
		LString(evidenceID),
	)
	if !strings.Contains(errMsg, "SERVICE_INACTIVE") {
		t.Fatalf("expected SERVICE_INACTIVE, got %q", errMsg)
	}
}
