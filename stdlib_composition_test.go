package lua

import (
	"os"
	"path/filepath"
	"testing"
)

func compileStdlibComposition(t *testing.T, baseDir string, name string, src string) {
	t.Helper()
	irp, err := BuildIRWithResolver([]byte(src), name, NewOSFileResolver(baseDir))
	if err != nil {
		t.Fatalf("build IR for %s: %v", filepath.Base(name), err)
	}
	proto, err := CompileIR(irp)
	if err != nil {
		t.Fatalf("compile IR for %s: %v", filepath.Base(name), err)
	}
	if proto == nil || len(proto.Code) == 0 {
		t.Fatalf("expected non-empty bytecode for %s", filepath.Base(name))
	}
}

func TestAgentStdlibControlPlaneCompositionCompiles(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	baseDir := filepath.Dir(repoRoot)
	src := `
pragma tolang 0.4.0;
import tolang.stdlib.account.PolicyAccount;
import tolang.stdlib.authority.AuthorityBook;
import tolang.stdlib.execution_binding.ExecutionBindingBook;
import tolang.stdlib.session.SessionBook;
import tolang.stdlib.receipt.ReceiptBook;
import tolang.stdlib.recovery.RecoveryController;

contract ControlPlaneCoordinator {
  function preflight(
    agent account_addr,
    agent authority_addr,
    agent binding_addr,
    agent session_addr,
    agent receipt_addr,
    agent recovery_addr,
    agent operator,
    bytes32 scope,
    bytes32 binding_id,
    bytes32 session_id,
    bytes32 receipt_id,
    u256 amount
  ) external returns (bool ok) {
    PolicyAccount account = PolicyAccount(account_addr);
    AuthorityBook authority = AuthorityBook(authority_addr);
    ExecutionBindingBook binding = ExecutionBindingBook(binding_addr);
    SessionBook session_book = SessionBook(session_addr);
    ReceiptBook receipt = ReceiptBook(receipt_addr);
    RecoveryController recovery = RecoveryController(recovery_addr);

    require(account.isSuspended() != true, "ACCOUNT_SUSPENDED");
    require(authority.isActive(operator, scope) == true, "AUTHORITY_INACTIVE");
    require(binding.isConsumable(binding_id) == true, "BINDING_INACTIVE");
    require(session_book.isActive(session_id) == true, "SESSION_INACTIVE");
    require(session_book.requiresStepUp(session_id, amount) != true, "STEP_UP_REQUIRED");
    require(receipt.isFinalized(receipt_id) != true, "RECEIPT_FINAL");
    require(recovery.isFrozen() != true, "RECOVERY_FROZEN");
    return true;
  }
}
`
	compileStdlibComposition(t, baseDir, filepath.Join(baseDir, "control_plane_composition.tol"), src)
}

func TestAgentStdlibExecutionPlaneCompositionCompiles(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	baseDir := filepath.Dir(repoRoot)
	src := `
pragma tolang 0.4.0;
import tolang.stdlib.agreement.CommercialAgreement;
import tolang.stdlib.settlement.TaskSettlement;
import tolang.stdlib.evidence.EvidenceBook;
import tolang.stdlib.sponsor.SponsorPolicyRelay;
import tolang.stdlib.receipt.ReceiptBook;

contract ExecutionPlaneCoordinator {
  function routeReady(
    agent agreement_addr,
    agent settlement_addr,
    agent evidence_addr,
    agent sponsor_addr,
    agent receipt_addr,
    u256 agreement_id,
    u256 task_id,
    bytes32 evidence_id,
    agent relayer
  ) external returns (bool ok) {
    CommercialAgreement agreement = CommercialAgreement(agreement_addr);
    TaskSettlement settlement = TaskSettlement(settlement_addr);
    EvidenceBook evidence = EvidenceBook(evidence_addr);
    SponsorPolicyRelay sponsor = SponsorPolicyRelay(sponsor_addr);
    ReceiptBook receipt = ReceiptBook(receipt_addr);

    require(agreement.statusOf(agreement_id) >= 1, "AGREEMENT_UNKNOWN");
    require(settlement.statusOf(task_id) >= 1, "TASK_UNKNOWN");
    require(evidence.statusOf(evidence_id) >= 1, "EVIDENCE_UNKNOWN");
    require(sponsor.isRelayerActive(relayer) == true, "RELAYER_INACTIVE");
    require(receipt.statusOf(evidence.proofRefOf(evidence_id)) >= 0, "RECEIPT_LOOKUP");
    return true;
  }
}
`
	compileStdlibComposition(t, baseDir, filepath.Join(baseDir, "execution_plane_composition.tol"), src)
}

func TestAgentStdlibMarketPlaneCompositionCompiles(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	baseDir := filepath.Dir(repoRoot)
	src := `
pragma tolang 0.4.0;
import tolang.stdlib.trust.TrustRegistry;
import tolang.stdlib.discovery.ServiceDirectory;
import tolang.stdlib.privacy.ConfidentialVault;

contract MarketPlaneCoordinator {
  function discoverAndAudit(
    agent trust_addr,
    agent directory_addr,
    agent vault_addr,
    agent provider,
    u256 service_id,
    agent owner,
    agent auditor
  ) external returns (bool ok) {
    TrustRegistry trust = TrustRegistry(trust_addr);
    ServiceDirectory directory = ServiceDirectory(directory_addr);
    ConfidentialVault vault = ConfidentialVault(vault_addr);

    require(trust.isEligible(provider) == true, "PROVIDER_NOT_ELIGIBLE");
    require(directory.isActive(service_id) == true, "SERVICE_INACTIVE");
    require(directory.providerOf(service_id) == provider, "PROVIDER_MISMATCH");
    require(vault.canAudit(owner, auditor) == true, "AUDIT_NOT_ALLOWED");
    return true;
  }
}
`
	compileStdlibComposition(t, baseDir, filepath.Join(baseDir, "market_plane_composition.tol"), src)
}
