package metadata

import (
	"testing"
)

func TestBuildCapabilityManifest_Basic(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		Contract: ContractInfo{
			Name: "TaskEscrow",
		},
		Functions: []FunctionMeta{
			{
				Name:               "postTask",
				Selector:           "0xaabbccdd",
				Visibility:         "external",
				Mutability:         "payable",
				Verifiable:         true,
				RequiresCapability: []string{"escrow"},
				Params:             []ParamMeta{{Name: "reward", Type: "u256"}, {Name: "deadline", Type: "u256"}},
			},
			{
				Name:       "acceptTask",
				Selector:   "0x11223344",
				Visibility: "external",
				Mutability: "nonpayable",
				Delegated:  true,
				Params:     []ParamMeta{{Name: "taskId", Type: "u256"}},
			},
			{
				Name:       "submitTask",
				Selector:   "0x55667788",
				Visibility: "external",
				Mutability: "nonpayable",
			},
			{
				Name:       "approveTask",
				Selector:   "0x99001122",
				Visibility: "external",
				Mutability: "nonpayable",
			},
			{
				Name:       "internalHelper",
				Selector:   "0xdeadbeef",
				Visibility: "internal",
			},
		},
		Manifest: &ManifestMeta{
			Version: "1.0.0",
		},
		Capabilities: []string{"task_management"},
	}

	cm := BuildCapabilityManifest(meta)

	if cm.ContractName != "TaskEscrow" {
		t.Errorf("ContractName = %q, want TaskEscrow", cm.ContractName)
	}
	if cm.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", cm.Version)
	}

	// Check capabilities: 1 provided + 1 required.
	if len(cm.Capabilities) != 2 {
		t.Fatalf("len(Capabilities) = %d, want 2", len(cm.Capabilities))
	}
	foundProvided := false
	foundRequired := false
	for _, cap := range cm.Capabilities {
		if cap.Name == "task_management" && cap.Provided {
			foundProvided = true
		}
		if cap.Name == "escrow" && cap.Required {
			foundRequired = true
		}
	}
	if !foundProvided {
		t.Error("expected provided capability 'task_management'")
	}
	if !foundRequired {
		t.Error("expected required capability 'escrow'")
	}

	// Verifiable methods: postTask only.
	if len(cm.VerifiableMethods) != 1 {
		t.Fatalf("len(VerifiableMethods) = %d, want 1", len(cm.VerifiableMethods))
	}
	if cm.VerifiableMethods[0].FunctionName != "postTask" {
		t.Errorf("VerifiableMethods[0].FunctionName = %q, want postTask", cm.VerifiableMethods[0].FunctionName)
	}
	if cm.VerifiableMethods[0].ProofType != "execution_proof" {
		t.Errorf("ProofType = %q, want execution_proof", cm.VerifiableMethods[0].ProofType)
	}

	// Delegated methods: acceptTask only.
	if len(cm.DelegatedMethods) != 1 {
		t.Fatalf("len(DelegatedMethods) = %d, want 1", len(cm.DelegatedMethods))
	}
	if cm.DelegatedMethods[0].FunctionName != "acceptTask" {
		t.Errorf("DelegatedMethods[0].FunctionName = %q, want acceptTask", cm.DelegatedMethods[0].FunctionName)
	}
	if len(cm.DelegatedMethods[0].ScopeFields) != 1 || cm.DelegatedMethods[0].ScopeFields[0] != "taskId" {
		t.Errorf("ScopeFields = %v, want [taskId]", cm.DelegatedMethods[0].ScopeFields)
	}

	// Task interfaces: should detect "Task" lifecycle.
	if len(cm.TaskInterfaces) != 1 {
		t.Fatalf("len(TaskInterfaces) = %d, want 1", len(cm.TaskInterfaces))
	}
	if cm.TaskInterfaces[0].Name != "Task" {
		t.Errorf("TaskInterfaces[0].Name = %q, want Task", cm.TaskInterfaces[0].Name)
	}
	if len(cm.TaskInterfaces[0].Methods) < 2 {
		t.Errorf("expected at least 2 task methods, got %d", len(cm.TaskInterfaces[0].Methods))
	}
}

func TestBuildCapabilityManifest_ViewVerifiable(t *testing.T) {
	meta := &ContractMetadata{
		Contract: ContractInfo{Name: "PriceOracle"},
		Functions: []FunctionMeta{
			{
				Name:       "getPrice",
				Selector:   "0xaabb0001",
				Visibility: "public",
				Mutability: "view",
				Verifiable: true,
			},
		},
	}

	cm := BuildCapabilityManifest(meta)

	if len(cm.VerifiableMethods) != 1 {
		t.Fatalf("len(VerifiableMethods) = %d, want 1", len(cm.VerifiableMethods))
	}
	if cm.VerifiableMethods[0].ProofType != "state_proof" {
		t.Errorf("ProofType = %q, want state_proof for view function", cm.VerifiableMethods[0].ProofType)
	}
}

func TestBuildCapabilityManifest_OracleSlots(t *testing.T) {
	meta := &ContractMetadata{
		Contract: ContractInfo{Name: "OracleResolver"},
		Functions: []FunctionMeta{
			{
				Name:       "submitPrice",
				Selector:   "0xcc001122",
				Visibility: "external",
				Mutability: "nonpayable",
				Params:     []ParamMeta{{Name: "price", Type: "u256"}},
			},
			{
				Name:       "updateFeed",
				Selector:   "0xdd001122",
				Visibility: "external",
				Mutability: "nonpayable",
				Params:     []ParamMeta{{Name: "data", Type: "bytes32"}},
			},
		},
	}

	cm := BuildCapabilityManifest(meta)

	if len(cm.OracleSlots) != 2 {
		t.Fatalf("len(OracleSlots) = %d, want 2", len(cm.OracleSlots))
	}

	// Find the Price slot.
	found := false
	for _, slot := range cm.OracleSlots {
		if slot.Name == "Price" && slot.DataType == "u256" && slot.Method == "submitPrice" {
			found = true
		}
	}
	if !found {
		t.Error("expected OracleSlot for submitPrice")
	}
}

func TestBuildCapabilityManifest_NoManifest(t *testing.T) {
	meta := &ContractMetadata{
		Contract: ContractInfo{Name: "Simple"},
	}

	cm := BuildCapabilityManifest(meta)

	if cm.ContractName != "Simple" {
		t.Errorf("ContractName = %q, want Simple", cm.ContractName)
	}
	if cm.Version != "" {
		t.Errorf("Version = %q, want empty", cm.Version)
	}
	if len(cm.Capabilities) != 0 {
		t.Errorf("expected no capabilities, got %d", len(cm.Capabilities))
	}
}

func TestBuildAgentPackageInfo(t *testing.T) {
	meta := &ContractMetadata{
		SchemaVersion: SchemaVersion,
		Contract: ContractInfo{
			Name: "TaskEscrow",
		},
		ArtifactRef: ArtifactRef{
			PackageHash:  "0xaabb",
			BytecodeHash: "0xccdd",
			ABIHash:      "0xeeff",
		},
		Functions: []FunctionMeta{
			{
				Name:       "postTask",
				Selector:   "0xaabbccdd",
				Visibility: "external",
				Mutability: "payable",
				Verifiable: true,
			},
			{
				Name:       "acceptTask",
				Selector:   "0x11223344",
				Visibility: "external",
				Mutability: "nonpayable",
			},
		},
		Manifest: &ManifestMeta{
			Version: "2.0.0",
		},
		Capabilities: []string{"task_management"},
	}

	pkg := BuildAgentPackageInfo(meta, "TaskEscrow")

	if pkg.PackageName != "TaskEscrow" {
		t.Errorf("PackageName = %q, want TaskEscrow", pkg.PackageName)
	}
	if pkg.PackageVersion != "2.0.0" {
		t.Errorf("PackageVersion = %q, want 2.0.0", pkg.PackageVersion)
	}
	if pkg.Capabilities == nil {
		t.Fatal("Capabilities is nil")
	}
	if pkg.Discovery == nil {
		t.Fatal("Discovery is nil")
	}
	if pkg.ArtifactRef.PackageHash != "0xaabb" {
		t.Errorf("ArtifactRef.PackageHash = %q, want 0xaabb", pkg.ArtifactRef.PackageHash)
	}
}

func TestAppendUnique(t *testing.T) {
	s := []string{"a", "b"}
	s = appendUnique(s, "b")
	if len(s) != 2 {
		t.Errorf("expected no duplicate, got %v", s)
	}
	s = appendUnique(s, "c")
	if len(s) != 3 {
		t.Errorf("expected new element, got %v", s)
	}
}
