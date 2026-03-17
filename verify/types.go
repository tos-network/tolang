package verify

// RPCClient is the interface for fetching on-chain contract data from a GTOS node.
type RPCClient interface {
	// GetCode returns the deployed bytecode at the given contract address.
	// The returned bytes are the raw .tor package or .toc artifact bytes
	// as stored on-chain by the LVM create operation.
	GetCode(address string) ([]byte, error)
}

// VerificationResult holds the outcome of verifying a single contract.
type VerificationResult struct {
	ContractName         string   `json:"contract_name"`
	ContractAddress      string   `json:"contract_address,omitempty"`
	SourcePath           string   `json:"source_path"`
	Match                bool     `json:"match"`
	BytecodeMatch        bool     `json:"bytecode_match"`
	ABIMatch             bool     `json:"abi_match"`
	SourceHash           string   `json:"source_hash"`
	ExpectedBytecodeHash string   `json:"expected_bytecode_hash"`
	ActualBytecodeHash   string   `json:"actual_bytecode_hash"`
	ExpectedABIHash      string   `json:"expected_abi_hash"`
	ActualABIHash        string   `json:"actual_abi_hash"`
	Errors               []string `json:"errors,omitempty"`
}

// VerificationReport aggregates results for multiple contracts.
type VerificationReport struct {
	SchemaVersion string               `json:"schema_version"`
	Timestamp     string               `json:"timestamp"`
	Results       []VerificationResult `json:"results"`
	AllMatch      bool                 `json:"all_match"`
}
