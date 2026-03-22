package lua

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type stdlibPackageContract struct {
	Name string `json:"name"`
	TOC  string `json:"toc"`
	ABI  string `json:"abi"`
}

type stdlibPackageManifest struct {
	Name      string                  `json:"name"`
	Package   string                  `json:"package"`
	Version   string                  `json:"version"`
	Contracts []stdlibPackageContract `json:"contracts"`
}

type stdlibABIFunction struct {
	Name string `json:"name"`
}

type stdlibABIManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type stdlibABI struct {
	Functions []stdlibABIFunction `json:"functions"`
	Manifest  *stdlibABIManifest  `json:"manifest"`
}

func TestAgentStdlibPackagesCompile(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	testCases := []struct {
		relPath       string
		pkgName       string
		contractName  string
		interfaceName string
		functions     []string
	}{
		{
			relPath:       "stdlib/account/PolicyAccount.tol",
			pkgName:       "tolang.stdlib.account",
			contractName:  "PolicyAccount",
			interfaceName: "IPolicyAccount",
			functions:     []string{"validate", "setSpendCaps", "setAllowlistEnabled", "setAllowlisted", "authorizeDelegate", "revokeDelegate", "suspend", "unsuspend", "execute", "remainingDaily", "delegateRemaining", "isSuspended", "setDelegateCaps", "delegateDailyRemaining"},
		},
		{
			relPath:       "stdlib/authority/AuthorityBook.tol",
			pkgName:       "tolang.stdlib.authority",
			contractName:  "AuthorityBook",
			interfaceName: "IAuthorityBook",
			functions:     []string{"grant", "revoke", "consume", "remainingOf", "isActive", "policyHashOf"},
		},
		{
			relPath:       "stdlib/execution_binding/ExecutionBindingBook.tol",
			pkgName:       "tolang.stdlib.execution_binding",
			contractName:  "ExecutionBindingBook",
			interfaceName: "IExecutionBindingBook",
			functions:     []string{"approve", "cancel", "consume", "isConsumable", "executorOf", "policyHashOf", "proofRefOf"},
		},
		{
			relPath:       "stdlib/session/SessionBook.tol",
			pkgName:       "tolang.stdlib.session",
			contractName:  "SessionBook",
			interfaceName: "ISessionBook",
			functions:     []string{"grantSession", "revokeSession", "consume", "isActive", "requiresStepUp", "remainingOf", "requireTerminal", "terminalTypeOf", "trustTierOf", "enforceStepUp"},
		},
		{
			relPath:       "stdlib/recovery/RecoveryController.tol",
			pkgName:       "tolang.stdlib.recovery",
			contractName:  "RecoveryController",
			interfaceName: "IRecoveryController",
			functions:     []string{"addGuardian", "removeGuardian", "startRecovery", "approveRecovery", "executeRecovery", "cancelRecovery", "freeze", "unfreeze", "currentController", "isFrozen", "isRecoveryActive", "approvalCount"},
		},
		{
			relPath:       "stdlib/evidence/EvidenceBook.tol",
			pkgName:       "tolang.stdlib.evidence",
			contractName:  "EvidenceBook",
			interfaceName: "IEvidenceBook",
			functions:     []string{"setAttester", "openEvidence", "fulfill", "challenge", "finalize", "readValue", "statusOf", "proofRefOf", "isFinalized"},
		},
		{
			relPath:       "stdlib/agreement/CommercialAgreement.tol",
			pkgName:       "tolang.stdlib.agreement",
			contractName:  "CommercialAgreement",
			interfaceName: "ICommercialAgreement",
			functions:     []string{"createOffer", "accept", "cancel", "fulfill", "expire", "createInvoice", "statusOf", "agreementTypeOf", "quoteRefOf", "counterpartyOf", "settlementRefOf"},
		},
		{
			relPath:       "stdlib/settlement/TaskSettlement.tol",
			pkgName:       "tolang.stdlib.settlement",
			contractName:  "TaskSettlement",
			interfaceName: "ITaskSettlement",
			functions:     []string{"openTask", "acceptTask", "submitTask", "approveTask", "rejectTask", "disputeTask", "resolveDispute", "cancelTask", "reclaimExpired", "openMilestoneTask", "completeMilestone", "setSlashPolicy", "setReceiptBook", "statusOf", "rewardOf", "receiptRefOf", "proofRefOf", "milestoneStatusOf"},
		},
		{
			relPath:       "stdlib/receipt/ReceiptBook.tol",
			pkgName:       "tolang.stdlib.receipt",
			contractName:  "ReceiptBook",
			interfaceName: "IReceiptBook",
			functions:     []string{"openReceipt", "finalizeSuccess", "finalizeFailure", "statusOf", "bindingRefOf", "proofRefOf", "isFinalized"},
		},
		{
			relPath:       "stdlib/sponsor/SponsorPolicyRelay.tol",
			pkgName:       "tolang.stdlib.sponsor",
			contractName:  "SponsorPolicyRelay",
			interfaceName: "ISponsorPolicyRelay",
			functions:     []string{"deposit", "withdraw", "authorizeRelayer", "revokeRelayer", "relay", "remainingOf", "policyHashOf", "isRelayerActive"},
		},
		{
			relPath:       "stdlib/trust/TrustRegistry.tol",
			pkgName:       "tolang.stdlib.trust",
			contractName:  "TrustRegistry",
			interfaceName: "ITrustRegistry",
			functions:     []string{"setTrustFloor", "depositBond", "withdrawBond", "setOverride", "isEligible", "bondedAmountOf", "snapshotReputationOf", "snapshotStakeOf", "updateReputation", "setScorerCallback", "lockStake", "unlockStake", "lockedStakeOf"},
		},
		{
			relPath:       "stdlib/privacy/ConfidentialVault.tol",
			pkgName:       "tolang.stdlib.privacy",
			contractName:  "ConfidentialVault",
			interfaceName: "IConfidentialVault",
			functions:     []string{"registerPublicKey", "deposit", "withdraw", "authorizeAuditor", "revokeAuditor", "balanceOf", "nativeBalance", "canAudit", "disclosureRefOf"},
		},
		{
			relPath:       "stdlib/privacy/ConfidentialEscrow.tol",
			pkgName:       "tolang.stdlib.privacy",
			contractName:  "ConfidentialEscrow",
			interfaceName: "IConfidentialEscrow",
			functions:     []string{"openEscrow", "releaseEscrow", "refundEscrow", "refundEscrowTo", "reclaimExpired", "statusOf", "amountOf", "payerOf", "payeeOf", "receiptRefOf", "nativeBalance"},
		},
		{
			relPath:       "stdlib/discovery/ServiceDirectory.tol",
			pkgName:       "tolang.stdlib.discovery",
			contractName:  "ServiceDirectory",
			interfaceName: "IServiceDirectory",
			functions:     []string{"registerService", "updateManifest", "updateQuote", "deactivate", "providerOf", "manifestRefOf", "capabilityRefOf", "quoteRefOf", "isActive", "setServiceFee", "setServiceSLA", "feeOf", "slaOf", "setCapabilityType", "setServiceKind", "setPricingKind", "setPrivacyMode", "setReceiptMode", "setTrustFloorRef", "capabilityTypeOf", "serviceKindOf", "pricingKindOf", "privacyModeOf", "receiptModeOf", "trustFloorRefOf", "serviceCount"},
		},
		{
			relPath:       "stdlib/privacy/ConfidentialPayment.tol",
			pkgName:       "tolang.stdlib.privacy",
			contractName:  "ConfidentialPayment",
			interfaceName: "IConfidentialPayment",
			functions:     []string{"pay", "batchPay", "addPayee", "releasePayment", "releaseBatch", "refund", "refundBatch", "statusOf", "batchStatusOf", "payerOf", "payeeOf", "receiptRefOf", "nativeBalance"},
		},
		{
			relPath:       "stdlib/privacy/ConfidentialTreasury.tol",
			pkgName:       "tolang.stdlib.privacy",
			contractName:  "ConfidentialTreasury",
			interfaceName: "IConfidentialTreasury",
			functions:     []string{"deposit", "withdraw", "addSigner", "removeSigner", "authorizeSpend", "executeSpend", "cancelSpend", "authorizeAuditor", "revokeAuditor", "totalBalance", "nativeBalance", "isSigner", "spendStatusOf", "canAudit", "disclosureRefOf"},
		},
		{
			relPath:       "stdlib/privacy/ConfidentialAllowance.tol",
			pkgName:       "tolang.stdlib.privacy",
			contractName:  "ConfidentialAllowance",
			interfaceName: "IConfidentialAllowance",
			functions:     []string{"deposit", "withdraw", "approve", "revokeApproval", "transferFrom", "balanceOf", "allowanceOf", "isApproved", "expiryOf", "nativeBalance"},
		},
		{
			relPath:       "stdlib/privacy/AuditorDisclosureBook.tol",
			pkgName:       "tolang.stdlib.privacy",
			contractName:  "AuditorDisclosureBook",
			interfaceName: "IAuditorDisclosureBook",
			functions:     []string{"authorizeAuditor", "revokeAuditor", "publishSnapshot", "finalizeSnapshot", "attachProof", "isAuthorized", "scopeRefOf", "expiryOf", "snapshotStatusOf", "dataRefOf", "proofRefOf", "isFinalized", "snapshotCount"},
		},
		{
			relPath:       "stdlib/settlement/RecurringPayment.tol",
			pkgName:       "tolang.stdlib.settlement",
			contractName:  "RecurringPayment",
			interfaceName: "IRecurringPayment",
			functions:     []string{"subscribe", "executePayment", "cancel", "pause", "resume", "statusOf", "cyclesCompleted", "nextPaymentAfter", "remainingBalance", "providerOf", "subscriberOf", "agreementRefOf"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.contractName, func(t *testing.T) {
			path := filepath.Join(repoRoot, tc.relPath)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.relPath, err)
			}

			artifactBytes, err := CompileArtifact(source, path)
			if err != nil {
				t.Fatalf("compile artifact %s: %v", tc.relPath, err)
			}
			artifact, err := DecodeArtifact(artifactBytes)
			if err != nil {
				t.Fatalf("decode artifact %s: %v", tc.relPath, err)
			}
			if artifact.ContractName != tc.contractName {
				t.Fatalf("artifact contract name mismatch: got=%q want=%q", artifact.ContractName, tc.contractName)
			}

			pkgBytes, err := CompilePackage(source, path, &PackageOptions{PackageVersion: "1.0.0"})
			if err != nil {
				t.Fatalf("compile package %s: %v", tc.relPath, err)
			}
			pkg, err := DecodePackage(pkgBytes)
			if err != nil {
				t.Fatalf("decode package %s: %v", tc.relPath, err)
			}

			var manifest stdlibPackageManifest
			if err := json.Unmarshal(pkg.ManifestJSON, &manifest); err != nil {
				t.Fatalf("unmarshal manifest %s: %v", tc.relPath, err)
			}
			if manifest.Name != tc.pkgName {
				t.Fatalf("manifest name mismatch: got=%q want=%q", manifest.Name, tc.pkgName)
			}
			if manifest.Package != tc.pkgName {
				t.Fatalf("manifest package mismatch: got=%q want=%q", manifest.Package, tc.pkgName)
			}

			seenNames := make(map[string]stdlibPackageContract, len(manifest.Contracts))
			for _, contract := range manifest.Contracts {
				seenNames[contract.Name] = contract
			}
			if _, ok := seenNames[tc.contractName]; !ok {
				t.Fatalf("manifest missing concrete contract entry %q", tc.contractName)
			}

			contractEntry := seenNames[tc.contractName]
			if contractEntry.TOC == "" {
				t.Fatalf("concrete contract entry %q missing toc path", tc.contractName)
			}
			if contractEntry.ABI == "" {
				t.Fatalf("concrete contract entry %q missing abi path", tc.contractName)
			}
			if filepath.Base(contractEntry.ABI) != tc.interfaceName+".abi" {
				t.Fatalf("unexpected abi path for %q: got=%q want file=%q", tc.contractName, contractEntry.ABI, tc.interfaceName+".abi")
			}

			pkgArtifactBytes, ok := pkg.Files[contractEntry.TOC]
			if !ok {
				t.Fatalf("package missing toc payload at %q", contractEntry.TOC)
			}
			pkgArtifact, err := DecodeArtifact(pkgArtifactBytes)
			if err != nil {
				t.Fatalf("decode package toc %q: %v", contractEntry.TOC, err)
			}
			if pkgArtifact.ContractName != tc.contractName {
				t.Fatalf("package artifact contract mismatch: got=%q want=%q", pkgArtifact.ContractName, tc.contractName)
			}

			abiBytes, ok := pkg.Files[contractEntry.ABI]
			if !ok {
				t.Fatalf("package missing abi payload at %q", contractEntry.ABI)
			}
			if len(abiBytes) == 0 {
				t.Fatalf("package abi %q is empty", contractEntry.ABI)
			}

			var abi stdlibABI
			if err := json.Unmarshal(pkgArtifact.ABIJSON, &abi); err != nil {
				t.Fatalf("unmarshal artifact abi %s: %v", tc.relPath, err)
			}
			if abi.Manifest == nil {
				t.Fatalf("artifact abi manifest missing for %s", tc.relPath)
			}
			if abi.Manifest.Name != tc.contractName {
				t.Fatalf("abi manifest name mismatch: got=%q want=%q", abi.Manifest.Name, tc.contractName)
			}
			if abi.Manifest.Version != "1.0.0" {
				t.Fatalf("abi manifest version mismatch: got=%q want=%q", abi.Manifest.Version, "1.0.0")
			}
			if abi.Manifest.Description == "" {
				t.Fatalf("abi manifest description missing for %s", tc.relPath)
			}

			functions := make(map[string]bool, len(abi.Functions))
			for _, fn := range abi.Functions {
				functions[fn.Name] = true
			}
			for _, want := range tc.functions {
				if !functions[want] {
					t.Fatalf("abi for %s missing function %q", tc.relPath, want)
				}
			}
		})
	}
}

func TestAgentStdlibPackageImportsCompile(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	baseDir := filepath.Dir(repoRoot)

	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "account",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.account.PolicyAccount;
contract Consumer {
  function suspended(agent account_addr) external returns (bool ok) {
    PolicyAccount account = PolicyAccount(account_addr);
    return account.isSuspended();
  }
}
`,
		},
		{
			name: "authority",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.authority.AuthorityBook;
contract Consumer {
  function remaining(agent book, agent operator, bytes32 scope) external returns (u256 rem) {
    AuthorityBook authority = AuthorityBook(book);
    return authority.remainingOf(operator, scope);
  }
}
`,
		},
		{
			name: "execution_binding",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.execution_binding.ExecutionBindingBook;
contract Consumer {
  function ready(agent book, bytes32 binding_id) external returns (bool ok) {
    ExecutionBindingBook bindings = ExecutionBindingBook(book);
    return bindings.isConsumable(binding_id);
  }
}
`,
		},
		{
			name: "session",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.session.SessionBook;
contract Consumer {
  function needsStepUp(agent book, bytes32 session_id, u256 amount) external returns (bool ok) {
    SessionBook sessions = SessionBook(book);
    return sessions.requiresStepUp(session_id, amount);
  }
}
`,
		},
		{
			name: "recovery",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.recovery.RecoveryController;
contract Consumer {
  function frozen(agent book) external returns (bool ok) {
    RecoveryController recovery = RecoveryController(book);
    return recovery.isFrozen();
  }
}
`,
		},
		{
			name: "evidence",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.evidence.EvidenceBook;
contract Consumer {
  function status(agent book, bytes32 evidence_id) external returns (u256 st) {
    EvidenceBook evidence = EvidenceBook(book);
    return evidence.statusOf(evidence_id);
  }
}
`,
		},
		{
			name: "settlement",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.settlement.TaskSettlement;
contract Consumer {
  function reward(agent book, u256 task_id) external returns (u256 value) {
    TaskSettlement settlement = TaskSettlement(book);
    return settlement.rewardOf(task_id);
  }
}
`,
		},
		{
			name: "agreement",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.agreement.CommercialAgreement;
contract Consumer {
  function status(agent book, u256 agreement_id) external returns (u256 st) {
    CommercialAgreement agreements = CommercialAgreement(book);
    return agreements.statusOf(agreement_id);
  }
}
`,
		},
		{
			name: "receipt",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.receipt.ReceiptBook;
contract Consumer {
  function finalized(agent book, bytes32 receipt_id) external returns (bool ok) {
    ReceiptBook receipts = ReceiptBook(book);
    return receipts.isFinalized(receipt_id);
  }
}
`,
		},
		{
			name: "trust",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.trust.TrustRegistry;
contract Consumer {
  function eligible(agent book, agent subject) external returns (bool ok) {
    TrustRegistry trust = TrustRegistry(book);
    return trust.isEligible(subject);
  }
}
`,
		},
		{
			name: "sponsor",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.sponsor.SponsorPolicyRelay;
contract Consumer {
  function remaining(agent relay_addr, agent relayer) external returns (u256 rem) {
    SponsorPolicyRelay relay = SponsorPolicyRelay(relay_addr);
    return relay.remainingOf(relayer);
  }
}
`,
		},
		{
			name: "discovery",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.discovery.ServiceDirectory;
contract Consumer {
  function manifest(agent book, u256 service_id) external returns (bytes32 ref) {
    ServiceDirectory directory = ServiceDirectory(book);
    return directory.manifestRefOf(service_id);
  }
}
`,
		},
		{
			name: "privacy",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.privacy.ConfidentialVault;
contract Consumer {
  function readBalance(agent vault_addr, agent owner) external returns (uno balance) {
    ConfidentialVault vault = ConfidentialVault(vault_addr);
    return vault.balanceOf(owner);
  }
}
`,
		},
		{
			name: "privacy_escrow",
			src: `
pragma tolang 0.4.0;
import tolang.stdlib.privacy.ConfidentialEscrow;
contract Consumer {
  function status(agent escrow_addr, bytes32 escrow_id) external returns (u256 st) {
    ConfidentialEscrow escrow = ConfidentialEscrow(escrow_addr);
    return escrow.statusOf(escrow_id);
  }
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name := filepath.Join(baseDir, "consumer_"+tc.name+".tol")
			if _, err := BuildIRWithResolver([]byte(tc.src), name, NewOSFileResolver(baseDir)); err != nil {
				t.Fatalf("import compile failed for %s: %v", tc.name, err)
			}
		})
	}
}

// TestPackageImportResolvesFromSearchPaths verifies that package-style imports
// resolve correctly when PackageSearchPaths is set, eliminating the need for
// synthetic compile paths.
func TestPackageImportResolvesFromSearchPaths(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	// A contract that imports from tolang.stdlib.account.
	source := []byte(`
pragma tolang 0.4.0;
import tolang.stdlib.account.IPolicyAccount;
contract Importer {
    function check() public view returns (bool ok) {
        return true;
    }
}
`)
	// Compile using the REAL file path (not a synthetic one) with
	// PackageSearchPaths pointing to the stdlib source directory.
	compileName := filepath.Join(repoRoot, "test_importer.tol")
	_, compileErr := CompileArtifactWithOptions(source, compileName, &ArtifactOptions{
		PackageSearchPaths: []string{
			filepath.Join(repoRoot, "stdlib"),
		},
	})
	if compileErr != nil {
		t.Fatalf("compile with PackageSearchPaths failed: %v", compileErr)
	}
}
