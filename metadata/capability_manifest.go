package metadata

// CapabilityManifest standardizes how a contract advertises its capabilities
// for discovery clients to consume. It covers capability advertisement,
// verifiable methods, and task-oracle-delegation interfaces.
type CapabilityManifest struct {
	ContractName      string         `json:"contract_name"`
	Version           string         `json:"version"`
	Capabilities      []CapabilityAd `json:"capabilities"`
	VerifiableMethods []VerifiableAd `json:"verifiable_methods"`
	DelegatedMethods  []DelegatedAd  `json:"delegated_methods"`
	TaskInterfaces    []TaskInterface `json:"task_interfaces,omitempty"`
	OracleSlots       []OracleSlot   `json:"oracle_slots,omitempty"`
}

// CapabilityAd describes a single capability that the contract requires or provides.
type CapabilityAd struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`    // must caller have this cap
	Provided    bool   `json:"provided"`    // does contract provide this cap
	Description string `json:"description"`
}

// VerifiableAd describes a function whose output can be independently verified.
type VerifiableAd struct {
	FunctionName string `json:"function_name"`
	Selector     string `json:"selector"`
	ProofType    string `json:"proof_type"` // "state_proof", "execution_proof"
	Description  string `json:"description"`
}

// DelegatedAd describes a function that can be executed on behalf of another agent.
type DelegatedAd struct {
	FunctionName string   `json:"function_name"`
	Selector     string   `json:"selector"`
	ScopeFields  []string `json:"scope_fields"`
	Description  string   `json:"description"`
}

// TaskInterface describes a task lifecycle supported by the contract,
// including the states it can be in and the methods that move it between them.
type TaskInterface struct {
	Name    string   `json:"name"`
	States  []string `json:"states"`
	Methods []string `json:"methods"`
}

// OracleSlot describes a data feed slot that the contract exposes for external
// oracle fulfillment.
type OracleSlot struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Method   string `json:"fulfill_method"`
}

// BuildCapabilityManifest derives a CapabilityManifest from ContractMetadata,
// extracting capability ads, verifiable methods, delegated methods, task
// interfaces, and oracle slots from the function metadata.
func BuildCapabilityManifest(meta *ContractMetadata) *CapabilityManifest {
	cm := &CapabilityManifest{
		ContractName: meta.Contract.Name,
	}
	if meta.Manifest != nil {
		cm.Version = meta.Manifest.Version
	}

	// Build capability ads from the contract-level capabilities.
	// These are capabilities the contract provides.
	for _, cap := range meta.Capabilities {
		cm.Capabilities = append(cm.Capabilities, CapabilityAd{
			Name:        cap,
			Provided:    true,
			Description: "Provided by contract",
		})
	}

	// Scan functions for required capabilities, verifiable and delegated methods.
	reqSeen := make(map[string]bool)
	for _, fn := range meta.Functions {
		if fn.Visibility == "internal" || fn.Visibility == "private" {
			continue
		}

		// Required capabilities from function metadata.
		for _, rc := range fn.RequiresCapability {
			if !reqSeen[rc] {
				reqSeen[rc] = true
				cm.Capabilities = append(cm.Capabilities, CapabilityAd{
					Name:        rc,
					Required:    true,
					Description: "Required by " + fn.Name,
				})
			}
		}

		// Verifiable methods.
		if fn.Verifiable {
			proofType := "execution_proof"
			if fn.Mutability == "view" || fn.Mutability == "pure" {
				proofType = "state_proof"
			}
			cm.VerifiableMethods = append(cm.VerifiableMethods, VerifiableAd{
				FunctionName: fn.Name,
				Selector:     fn.Selector,
				ProofType:    proofType,
				Description:  GenerateFunctionDescription(fn),
			})
		}

		// Delegated methods.
		if fn.Delegated {
			var scopeFields []string
			for _, p := range fn.Params {
				scopeFields = append(scopeFields, p.Name)
			}
			cm.DelegatedMethods = append(cm.DelegatedMethods, DelegatedAd{
				FunctionName: fn.Name,
				Selector:     fn.Selector,
				ScopeFields:  scopeFields,
				Description:  GenerateFunctionDescription(fn),
			})
		}
	}

	// Infer task interfaces from function-name patterns.
	cm.TaskInterfaces = inferTaskInterfaces(meta)

	// Infer oracle slots from function-name patterns.
	cm.OracleSlots = inferOracleSlots(meta)

	return cm
}

// inferTaskInterfaces detects task lifecycle patterns in function names.
func inferTaskInterfaces(meta *ContractMetadata) []TaskInterface {
	// Group task-related methods by detecting common prefixes.
	taskMethods := make(map[string][]string)
	taskStates := make(map[string][]string)

	for _, fn := range meta.Functions {
		if fn.Visibility == "internal" || fn.Visibility == "private" {
			continue
		}
		name := fn.Name
		// Detect task lifecycle methods by naming convention.
		for _, prefix := range []string{"post", "create", "accept", "submit", "approve", "reject", "dispute", "resolve", "cancel", "reclaim", "complete"} {
			if len(name) > len(prefix) && name[:len(prefix)] == prefix && name[len(prefix)] >= 'A' && name[len(prefix)] <= 'Z' {
				taskName := name[len(prefix):]
				taskMethods[taskName] = append(taskMethods[taskName], name)
				// Map method prefixes to states.
				stateMap := map[string]string{
					"post": "posted", "create": "created",
					"accept": "accepted", "submit": "submitted",
					"approve": "approved", "reject": "rejected",
					"dispute": "disputed", "resolve": "resolved",
					"cancel": "cancelled", "reclaim": "reclaimed",
					"complete": "completed",
				}
				if s, ok := stateMap[prefix]; ok {
					taskStates[taskName] = appendUnique(taskStates[taskName], s)
				}
				break
			}
		}
	}

	var interfaces []TaskInterface
	for name, methods := range taskMethods {
		if len(methods) < 2 {
			continue // Need at least two lifecycle methods to be a real task.
		}
		interfaces = append(interfaces, TaskInterface{
			Name:    name,
			States:  taskStates[name],
			Methods: methods,
		})
	}
	return interfaces
}

// inferOracleSlots detects oracle feed patterns in function names.
func inferOracleSlots(meta *ContractMetadata) []OracleSlot {
	var slots []OracleSlot
	for _, fn := range meta.Functions {
		if fn.Visibility == "internal" || fn.Visibility == "private" {
			continue
		}
		name := fn.Name
		// Oracle fulfillment methods: submit*, update*, report*.
		for _, prefix := range []string{"submit", "update", "report"} {
			if len(name) > len(prefix) && name[:len(prefix)] == prefix && name[len(prefix)] >= 'A' && name[len(prefix)] <= 'Z' {
				dataType := "bytes32"
				if len(fn.Params) > 0 {
					dataType = fn.Params[0].Type
				}
				slots = append(slots, OracleSlot{
					Name:     name[len(prefix):],
					DataType: dataType,
					Method:   name,
				})
				break
			}
		}
	}
	return slots
}

// appendUnique appends s to slice only if not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
