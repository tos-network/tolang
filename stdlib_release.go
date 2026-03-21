package lua

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tolast "github.com/tos-network/tolang/tol/ast"
)

// StdlibReleaseEntry describes one published stdlib contract artifact set.
type StdlibReleaseEntry struct {
	Family             string
	Contract           string
	SourcePath         string
	SourcePackagePath  string
	ReleasePackageName string
}

// StdlibReleaseArtifacts is the deterministic release payload generated from a
// stdlib source file.
type StdlibReleaseArtifacts struct {
	ArtifactPath  string
	ArtifactTOC   []byte
	InterfacePath string
	InterfaceABI  []byte
	InitPath      string
	InitTOC       []byte
	PackageTOR    []byte
}

// StdlibReleaseCatalog returns the published stdlib contract set.
func StdlibReleaseCatalog() []StdlibReleaseEntry {
	return []StdlibReleaseEntry{
		{
			Family:             "account",
			Contract:           "PolicyAccount",
			SourcePath:         "stdlib/account/PolicyAccount.tol",
			SourcePackagePath:  "tolang.stdlib.account",
			ReleasePackageName: "tolang.stdlib.account",
		},
		{
			Family:             "agreement",
			Contract:           "CommercialAgreement",
			SourcePath:         "stdlib/agreement/CommercialAgreement.tol",
			SourcePackagePath:  "tolang.stdlib.agreement",
			ReleasePackageName: "tolang.stdlib.agreement",
		},
		{
			Family:             "authority",
			Contract:           "AuthorityBook",
			SourcePath:         "stdlib/authority/AuthorityBook.tol",
			SourcePackagePath:  "tolang.stdlib.authority",
			ReleasePackageName: "tolang.stdlib.authority",
		},
		{
			Family:             "discovery",
			Contract:           "ServiceDirectory",
			SourcePath:         "stdlib/discovery/ServiceDirectory.tol",
			SourcePackagePath:  "tolang.stdlib.discovery",
			ReleasePackageName: "tolang.stdlib.discovery",
		},
		{
			Family:             "evidence",
			Contract:           "EvidenceBook",
			SourcePath:         "stdlib/evidence/EvidenceBook.tol",
			SourcePackagePath:  "tolang.stdlib.evidence",
			ReleasePackageName: "tolang.stdlib.evidence",
		},
		{
			Family:             "execution_binding",
			Contract:           "ExecutionBindingBook",
			SourcePath:         "stdlib/execution_binding/ExecutionBindingBook.tol",
			SourcePackagePath:  "tolang.stdlib.execution_binding",
			ReleasePackageName: "tolang.stdlib.execution_binding",
		},
		{
			Family:             "privacy",
			Contract:           "ConfidentialVault",
			SourcePath:         "stdlib/privacy/ConfidentialVault.tol",
			SourcePackagePath:  "tolang.stdlib.privacy",
			ReleasePackageName: "tolang.stdlib.privacy.confidential_vault",
		},
		{
			Family:             "privacy",
			Contract:           "ConfidentialEscrow",
			SourcePath:         "stdlib/privacy/ConfidentialEscrow.tol",
			SourcePackagePath:  "tolang.stdlib.privacy",
			ReleasePackageName: "tolang.stdlib.privacy.confidential_escrow",
		},
		{
			Family:             "receipt",
			Contract:           "ReceiptBook",
			SourcePath:         "stdlib/receipt/ReceiptBook.tol",
			SourcePackagePath:  "tolang.stdlib.receipt",
			ReleasePackageName: "tolang.stdlib.receipt",
		},
		{
			Family:             "recovery",
			Contract:           "RecoveryController",
			SourcePath:         "stdlib/recovery/RecoveryController.tol",
			SourcePackagePath:  "tolang.stdlib.recovery",
			ReleasePackageName: "tolang.stdlib.recovery",
		},
		{
			Family:             "session",
			Contract:           "SessionBook",
			SourcePath:         "stdlib/session/SessionBook.tol",
			SourcePackagePath:  "tolang.stdlib.session",
			ReleasePackageName: "tolang.stdlib.session",
		},
		{
			Family:             "settlement",
			Contract:           "TaskSettlement",
			SourcePath:         "stdlib/settlement/TaskSettlement.tol",
			SourcePackagePath:  "tolang.stdlib.settlement",
			ReleasePackageName: "tolang.stdlib.settlement",
		},
		{
			Family:             "sponsor",
			Contract:           "SponsorPolicyRelay",
			SourcePath:         "stdlib/sponsor/SponsorPolicyRelay.tol",
			SourcePackagePath:  "tolang.stdlib.sponsor",
			ReleasePackageName: "tolang.stdlib.sponsor",
		},
		{
			Family:             "trust",
			Contract:           "TrustRegistry",
			SourcePath:         "stdlib/trust/TrustRegistry.tol",
			SourcePackagePath:  "tolang.stdlib.trust",
			ReleasePackageName: "tolang.stdlib.trust",
		},
	}
}

// BuildStdlibReleaseArtifacts compiles a stdlib source file into deterministic
// release artifacts (.toc/.abi/init .toc if needed/.tor).
func BuildStdlibReleaseArtifacts(source []byte, sourcePath string, entry StdlibReleaseEntry) (*StdlibReleaseArtifacts, error) {
	if strings.TrimSpace(entry.Contract) == "" {
		return nil, fmt.Errorf("stdlib release: empty contract name")
	}
	if strings.TrimSpace(entry.ReleasePackageName) == "" {
		return nil, fmt.Errorf("stdlib release: empty release package name for %s", entry.Contract)
	}
	if strings.TrimSpace(sourcePath) == "" {
		return nil, fmt.Errorf("stdlib release: empty source path for %s", entry.Contract)
	}

	artifactBytes, err := CompileArtifact(source, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("compile artifact %s: %w", entry.Contract, err)
	}
	ifaceBytes, err := CompileInterfaceWithOptions(source, sourcePath, &InterfaceOptions{
		InterfaceName: "I" + entry.Contract,
		ContractName:  entry.Contract,
	})
	if err != nil {
		return nil, fmt.Errorf("compile interface %s: %w", entry.Contract, err)
	}

	artifactPath := fmt.Sprintf("bytecode/%s.toc", entry.Contract)
	interfacePath := fmt.Sprintf("interfaces/I%s.abi", entry.Contract)
	files := map[string][]byte{
		artifactPath:  artifactBytes,
		interfacePath: ifaceBytes,
	}

	mod, err := ParseModule(source, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("parse module %s: %w", entry.Contract, err)
	}
	contractDecl, needsInit, err := stdlibContractLookup(mod, entry.Contract)
	if err != nil {
		return nil, err
	}

	initPath := ""
	var initBytes []byte
	if needsInit {
		initPath = fmt.Sprintf("init/%s_init.toc", entry.Contract)
		initBytes, err = compileContractToArtifact(source, sourcePath, contractDecl, false, true)
		if err != nil {
			return nil, fmt.Errorf("compile init %s: %w", entry.Contract, err)
		}
		files[initPath] = initBytes
	}

	manifest := struct {
		Name         string `json:"name"`
		Package      string `json:"package,omitempty"`
		Version      string `json:"version"`
		MainContract string `json:"main_contract,omitempty"`
		InitCode     string `json:"init_code,omitempty"`
		Contracts    []struct {
			Name      string `json:"name"`
			Artifact  string `json:"toc"`
			Interface string `json:"abi"`
		} `json:"contracts"`
	}{
		Name:         entry.ReleasePackageName,
		Package:      entry.SourcePackagePath,
		Version:      "1.0.0",
		MainContract: entry.Contract,
		InitCode:     initPath,
		Contracts: []struct {
			Name      string `json:"name"`
			Artifact  string `json:"toc"`
			Interface string `json:"abi"`
		}{
			{Name: entry.Contract, Artifact: artifactPath, Interface: interfacePath},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest %s: %w", entry.Contract, err)
	}
	packageBytes, err := EncodePackage(manifestJSON, files)
	if err != nil {
		return nil, fmt.Errorf("encode package %s: %w", entry.Contract, err)
	}

	return &StdlibReleaseArtifacts{
		ArtifactPath:  artifactPath,
		ArtifactTOC:   artifactBytes,
		InterfacePath: interfacePath,
		InterfaceABI:  ifaceBytes,
		InitPath:      initPath,
		InitTOC:       initBytes,
		PackageTOR:    packageBytes,
	}, nil
}

func stdlibContractLookup(mod *tolast.Module, contractName string) (*tolast.ContractDecl, bool, error) {
	if mod == nil {
		return nil, false, fmt.Errorf("stdlib release: nil module for %s", contractName)
	}
	for i := range mod.Contracts {
		c := &mod.Contracts[i]
		if strings.TrimSpace(c.Name) != strings.TrimSpace(contractName) {
			continue
		}
		if c.Constructor != nil {
			return c, true, nil
		}
		if c.Storage != nil {
			for _, slot := range c.Storage.Slots {
				if slot.InitExpr != nil {
					return c, true, nil
				}
			}
		}
		return c, false, nil
	}
	return nil, false, fmt.Errorf("stdlib release: contract %s not found", contractName)
}

// BuildStdlibFamilyBundlePackage builds a multi-contract family bundle .tor for
// families that contain more than one published contract release.
func BuildStdlibFamilyBundlePackage(entries []StdlibReleaseEntry, sources map[string][]byte) ([]byte, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("stdlib family bundle: empty entry set")
	}
	ordered := append([]StdlibReleaseEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Family != ordered[j].Family {
			return ordered[i].Family < ordered[j].Family
		}
		return ordered[i].Contract < ordered[j].Contract
	})

	family := strings.TrimSpace(ordered[0].Family)
	packagePath := strings.TrimSpace(ordered[0].SourcePackagePath)
	if family == "" || packagePath == "" {
		return nil, fmt.Errorf("stdlib family bundle: incomplete family metadata")
	}

	files := map[string][]byte{}
	contracts := make([]struct {
		Name      string `json:"name"`
		Artifact  string `json:"toc"`
		Interface string `json:"abi"`
	}, 0, len(ordered))

	for _, entry := range ordered {
		if entry.Family != family {
			return nil, fmt.Errorf("stdlib family bundle: mixed families %q and %q", family, entry.Family)
		}
		if strings.TrimSpace(entry.SourcePackagePath) != packagePath {
			return nil, fmt.Errorf("stdlib family bundle: mixed package paths %q and %q", packagePath, entry.SourcePackagePath)
		}
		source := sources[entry.SourcePath]
		if len(source) == 0 {
			return nil, fmt.Errorf("stdlib family bundle: missing source for %s", entry.SourcePath)
		}
		built, err := BuildStdlibReleaseArtifacts(source, entry.SourcePath, entry)
		if err != nil {
			return nil, err
		}
		files[built.ArtifactPath] = built.ArtifactTOC
		files[built.InterfacePath] = built.InterfaceABI
		if built.InitPath != "" {
			files[built.InitPath] = built.InitTOC
		}
		contracts = append(contracts, struct {
			Name      string `json:"name"`
			Artifact  string `json:"toc"`
			Interface string `json:"abi"`
		}{
			Name: entry.Contract, Artifact: built.ArtifactPath, Interface: built.InterfacePath,
		})
	}

	manifest := struct {
		Name      string `json:"name"`
		Package   string `json:"package,omitempty"`
		Version   string `json:"version"`
		Contracts []struct {
			Name      string `json:"name"`
			Artifact  string `json:"toc"`
			Interface string `json:"abi"`
		} `json:"contracts"`
	}{
		Name:      packagePath,
		Package:   packagePath,
		Version:   "1.0.0",
		Contracts: contracts,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("stdlib family bundle: marshal manifest: %w", err)
	}
	return EncodePackage(manifestJSON, files)
}
