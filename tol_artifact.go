package lua

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	tolast "github.com/tos-network/tolang/tol/ast"
	"golang.org/x/crypto/sha3"
)

// gtomiPerGas is the assumed gas price for total_cost_tomi computation: 10 gtomi per gas unit.
const gtomiPerGas = uint64(10_000_000_000)

var tocMagic = [4]byte{'T', 'O', 'C', 0}

// ArtifactFormatVersion is the binary format version for .toc artifacts.
const ArtifactFormatVersion uint16 = 1

// Artifact is a decoded .toc payload.
type Artifact struct {
	Version           uint16
	Compiler          string
	ContractName      string
	Bytecode          []byte
	ABIJSON           []byte
	StorageLayoutJSON []byte
	SourceHash        string
	BytecodeHash      string
	// Gas metadata (ArtifactFormatVersion >= 2)
	MaxStackSlots         uint32
	BytecodeLen           uint32
	ContainsUnboundedLoop bool
}

// ArtifactOptions controls .artifact bytecode debug metadata emission.
type ArtifactOptions struct {
	// IncludeSourceMap controls whether embedded bytecode contains source map/debug metadata.
	// Default is true for backward compatibility.
	IncludeSourceMap bool
}

// gasModelVersion is the version string embedded in the gas_model ABI field.
// It identifies the cost model used during compilation so that Agents can
// verify that gas_upper values were computed under the same model version as
// the VM they are targeting.
const gasModelVersion = "tolang/0.2.0"

// Gas cost constants used by the static estimator (§7.4 of TOL_EFFECTS.md).
// These mirror the unexported constants in tol/sema/effects.go and must be
// kept in sync if the cost model changes.
const (
	gasModelSload   = uint64(2100)
	gasModelSstore  = uint64(20000)
	gasModelLogBase = uint64(375)
)

type tocABIGasModel struct {
	Version string `json:"version"`
	Sload   uint64 `json:"sload"`
	Sstore  uint64 `json:"sstore"`
	LogBase uint64 `json:"log_base"`
}

// tocABIManifest holds the optional manifest section of the .toc ABI JSON.
type tocABIManifest struct {
	Name        string            `json:"name,omitempty"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type tocABI struct {
	GasModel        tocABIGasModel   `json:"gas_model"`
	Functions       []tocABIFunction `json:"functions"`
	Events          []tocABIEvent    `json:"events"`
	Manifest        *tocABIManifest  `json:"manifest,omitempty"`
	AccountContract bool             `json:"account_contract,omitempty"`
}

// tocABIParam holds a named parameter with its type, emitted alongside the
// type-only params list for backward compatibility.
type tocABIParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type tocABIFunction struct {
	Name               string        `json:"name"`
	Visibility         string        `json:"visibility"`
	Mutability         string        `json:"mutability,omitempty"` // "pure", "view", "payable", "nonpayable"
	Selector           string        `json:"selector"`
	Params             []string      `json:"params,omitempty"`
	Returns            []string      `json:"returns,omitempty"`
	NamedParams        []tocABIParam `json:"named_params,omitempty"`
	NamedReturns       []tocABIParam `json:"named_returns,omitempty"`
	Doc                *tocABIDoc    `json:"doc,omitempty"`
	// Agent-native ABI extensions
	RequiresCapability string     `json:"requires_capability,omitempty"`
	PayAmountTomi      string     `json:"pay_amount_tomi,omitempty"`
	PayRecipient       string     `json:"pay_recipient,omitempty"`
	TotalCostTomi      string     `json:"total_cost_tomi,omitempty"`
	Verifiable         bool       `json:"verifiable,omitempty"`
	Delegated          bool       `json:"delegated,omitempty"`
	VerifiableStub     bool       `json:"verifiable_stub,omitempty"`
	// @quota annotation
	QuotaCalls string `json:"quota_calls,omitempty"`
	QuotaPrice string `json:"quota_price,omitempty"`
	// @total_cost annotation (declared max, separate from computed TotalCostTomi)
	DeclaredTotalCostMax string `json:"declared_total_cost_max,omitempty"`
}

type tocABIEvent struct {
	Name        string        `json:"name"`
	Params      []string      `json:"params,omitempty"`
	NamedParams []tocABIParam `json:"named_params,omitempty"`
}

// tocABIDoc is the optional doc metadata emitted per function in the ABI JSON.
type tocABIDoc struct {
	Notice        string         `json:"notice,omitempty"`
	Effects       *tocABIEffects `json:"effects,omitempty"`
	Bounds        []string       `json:"bounds,omitempty"`
	GasUpper      uint64         `json:"gas_upper,omitempty"`
	NonComposable bool           `json:"non_composable,omitempty"`
}

// tocABIEffects holds the @effects sub-object in ABI JSON.
type tocABIEffects struct {
	Reads  []string    `json:"reads,omitempty"`
	Writes []string    `json:"writes,omitempty"`
	Emits  []string    `json:"emits,omitempty"`
	Calls  interface{} `json:"calls,omitempty"` // []tocABICallRef or nil
}

// tocABICallRef is one call entry in the ABI JSON calls array.
type tocABICallRef struct {
	Cap      string `json:"cap,omitempty"`
	Iface    string `json:"iface,omitempty"`
	Selector string `json:"selector,omitempty"`
	MaxGas   uint64 `json:"max_gas,omitempty"`
	MaxCalls uint32 `json:"max_calls,omitempty"`
	MaxDepth uint32 `json:"max_depth,omitempty"`
	Wildcard bool   `json:"wildcard,omitempty"`
}

type tocStorageLayout struct {
	Slots []tocStorageSlot `json:"slots"`
}

type tocStorageSlot struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	CanonicalHash string `json:"canonical_hash"`
}

// IsArtifact reports whether the input starts with .toc magic bytes.
func IsArtifact(data []byte) bool {
	if len(data) < len(tocMagic) {
		return false
	}
	for i := range tocMagic {
		if data[i] != tocMagic[i] {
			return false
		}
	}
	return true
}

// CompileArtifact compiles TOL source into a .toc artifact.
func CompileArtifact(source []byte, name string) ([]byte, error) {
	return CompileArtifactWithOptions(source, name, nil)
}

// CompileArtifactWithOptions compiles TOL source into a .toc artifact with
// configurable bytecode debug metadata emission.
func CompileArtifactWithOptions(source []byte, name string, opts *ArtifactOptions) ([]byte, error) {
	includeSourceMap := true
	if opts != nil {
		includeSourceMap = opts.IncludeSourceMap
	}
	bytecode, err := CompileBytecodeWithOptions(source, name, &CompileOptions{
		IncludeSourceMap: includeSourceMap,
	})
	if err != nil {
		return nil, err
	}
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	contractName, abiJSON, storageJSON, err := buildArtifactMetadata(mod)
	if err != nil {
		return nil, err
	}
	maxSlots, bcLen, unbounded, err := analyzeBytecodeMetadata(bytecode)
	if err != nil {
		return nil, fmt.Errorf("artifact gas metadata analysis: %w", err)
	}
	return EncodeArtifact(&Artifact{
		Version:               ArtifactFormatVersion,
		Compiler:              "tolang/" + PackageVersion,
		ContractName:          contractName,
		Bytecode:              bytecode,
		ABIJSON:               abiJSON,
		StorageLayoutJSON:     storageJSON,
		SourceHash:            keccak256Hex(source),
		BytecodeHash:          keccak256Hex(bytecode),
		MaxStackSlots:         maxSlots,
		BytecodeLen:           bcLen,
		ContainsUnboundedLoop: unbounded,
	})
}

// EncodeArtifact serializes a compiled artifact into deterministic binary bytes.
func EncodeArtifact(a *Artifact) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("nil artifact")
	}
	if strings.TrimSpace(a.ContractName) == "" {
		return nil, fmt.Errorf("artifact contract name is required")
	}
	if len(a.Bytecode) == 0 {
		return nil, fmt.Errorf("artifact bytecode is required")
	}
	version := a.Version
	if version == 0 {
		version = ArtifactFormatVersion
	}
	if a.Compiler == "" {
		a.Compiler = "tolang/" + PackageVersion
	}
	sourceHash, err := decodeHashHex(a.SourceHash)
	if err != nil {
		return nil, fmt.Errorf("invalid source hash: %w", err)
	}
	bytecodeHash, err := decodeHashHex(a.BytecodeHash)
	if err != nil {
		return nil, fmt.Errorf("invalid bytecode hash: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(tocMagic[:])
	if err := writeU16(&buf, version); err != nil {
		return nil, err
	}
	if err := writeString(&buf, a.Compiler); err != nil {
		return nil, err
	}
	if err := writeString(&buf, strings.TrimSpace(a.ContractName)); err != nil {
		return nil, err
	}
	if err := writeLenBytes(&buf, a.Bytecode); err != nil {
		return nil, err
	}
	if err := writeLenBytes(&buf, a.ABIJSON); err != nil {
		return nil, err
	}
	if err := writeLenBytes(&buf, a.StorageLayoutJSON); err != nil {
		return nil, err
	}
	if _, err := buf.Write(sourceHash); err != nil {
		return nil, err
	}
	if _, err := buf.Write(bytecodeHash); err != nil {
		return nil, err
	}
	if err := writeU32(&buf, a.MaxStackSlots); err != nil {
		return nil, err
	}
	if err := writeU32(&buf, a.BytecodeLen); err != nil {
		return nil, err
	}
	ubyte := uint8(0)
	if a.ContainsUnboundedLoop {
		ubyte = 1
	}
	buf.WriteByte(ubyte)
	return buf.Bytes(), nil
}

// DecodeArtifact deserializes a .toc payload into a structured artifact.
func DecodeArtifact(data []byte) (*Artifact, error) {
	r := &byteReader{b: data}
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("invalid artifact header: %w", err)
	}
	if magic != tocMagic {
		return nil, fmt.Errorf("invalid artifact magic")
	}
	version, err := readU16(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact version: %w", err)
	}
	if version != ArtifactFormatVersion {
		return nil, fmt.Errorf("unsupported artifact version: got=%d want=%d", version, ArtifactFormatVersion)
	}
	compiler, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact compiler: %w", err)
	}
	contractName, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact contract name: %w", err)
	}
	bytecode, err := readLenBytes(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact bytecode payload: %w", err)
	}
	abiJSON, err := readLenBytes(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact abi payload: %w", err)
	}
	storageJSON, err := readLenBytes(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact storage payload: %w", err)
	}
	sourceHash, err := readFixedBytes(r, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact source hash: %w", err)
	}
	bytecodeHash, err := readFixedBytes(r, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact bytecode hash: %w", err)
	}
	maxStackSlots, err := readU32(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact max_stack_slots: %w", err)
	}
	bytecodeLen, err := readU32(r)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact bytecode_len: %w", err)
	}
	unboundedByte, err := readFixedBytes(r, 1)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact contains_unbounded_loop: %w", err)
	}
	if r.n != len(data) {
		return nil, fmt.Errorf("trailing bytes in artifact payload")
	}
	if strings.TrimSpace(contractName) == "" {
		return nil, fmt.Errorf("artifact contract name is empty")
	}
	if len(bytecode) == 0 {
		return nil, fmt.Errorf("artifact bytecode payload is empty")
	}
	gotBytecodeHash := keccak256Bytes(bytecode)
	if !bytes.Equal(gotBytecodeHash, bytecodeHash) {
		return nil, fmt.Errorf("artifact bytecode hash mismatch")
	}
	if _, err := DecodeFunctionProto(bytecode); err != nil {
		return nil, fmt.Errorf("artifact embedded bytecode decode failed: %w", err)
	}
	if len(abiJSON) > 0 && !json.Valid(abiJSON) {
		return nil, fmt.Errorf("artifact abi payload is not valid json")
	}
	if len(storageJSON) > 0 && !json.Valid(storageJSON) {
		return nil, fmt.Errorf("artifact storage payload is not valid json")
	}
	return &Artifact{
		Version:               version,
		Compiler:              compiler,
		ContractName:          contractName,
		Bytecode:              bytecode,
		ABIJSON:               abiJSON,
		StorageLayoutJSON:     storageJSON,
		SourceHash:            "0x" + hex.EncodeToString(sourceHash),
		BytecodeHash:          "0x" + hex.EncodeToString(bytecodeHash),
		MaxStackSlots:         maxStackSlots,
		BytecodeLen:           bytecodeLen,
		ContainsUnboundedLoop: unboundedByte[0] != 0,
	}, nil
}

// VerifySourceHash checks whether a decoded artifact matches the given source bytes.
func VerifySourceHash(art *Artifact, source []byte) error {
	if art == nil {
		return fmt.Errorf("nil artifact")
	}
	want := keccak256Hex(source)
	got := strings.ToLower(strings.TrimSpace(art.SourceHash))
	if got != want {
		return fmt.Errorf("artifact source hash mismatch: got=%s want=%s", art.SourceHash, want)
	}
	return nil
}

func analyzeBytecodeMetadata(bytecode []byte) (maxSlots uint32, bcLen uint32, unbounded bool, err error) {
	proto, err := DecodeFunctionProto(bytecode)
	if err != nil {
		return 0, 0, false, err
	}
	bcLen = uint32(len(bytecode))
	maxSlots, unbounded = walkProtoMetadata(proto)
	return maxSlots, bcLen, unbounded, nil
}

func walkProtoMetadata(p *FunctionProto) (maxSlots uint32, hasUnboundedLoop bool) {
	if p == nil {
		return 0, false
	}
	maxSlots = uint32(p.NumUsedRegisters)
	for _, inst := range p.Code {
		if opGetOpCode(inst) == OP_JMP && opGetArgSbx(inst) < 0 {
			hasUnboundedLoop = true
		}
	}
	for _, child := range p.FunctionPrototypes {
		childSlots, childUnbounded := walkProtoMetadata(child)
		if childSlots > maxSlots {
			maxSlots = childSlots
		}
		if childUnbounded {
			hasUnboundedLoop = true
		}
	}
	return maxSlots, hasUnboundedLoop
}

// docMetaToABI converts an AST DocMeta to its ABI JSON representation.
// Returns nil if meta is nil or contains no exportable data.
func docMetaToABI(meta *tolast.DocMeta) *tocABIDoc {
	if meta == nil {
		return nil
	}
	doc := &tocABIDoc{}
	hasData := false

	if meta.Notice != "" {
		doc.Notice = meta.Notice
		hasData = true
	}

	if meta.Effects != nil {
		eff := &tocABIEffects{}
		if len(meta.Effects.Reads) > 0 {
			eff.Reads = meta.Effects.Reads
			hasData = true
		}
		if len(meta.Effects.Writes) > 0 {
			eff.Writes = meta.Effects.Writes
			hasData = true
		}
		if len(meta.Effects.Emits) > 0 {
			eff.Emits = meta.Effects.Emits
			hasData = true
		}
		if meta.Effects.Calls != nil {
			calls := make([]tocABICallRef, 0, len(meta.Effects.Calls))
			nonComposable := false
			for _, cr := range meta.Effects.Calls {
				calls = append(calls, tocABICallRef{
					Cap:      cr.Cap,
					Iface:    cr.Iface,
					Selector: cr.Selector,
					MaxGas:   cr.MaxGas,
					MaxCalls: cr.MaxCalls,
					MaxDepth: cr.MaxDepth,
					Wildcard: cr.Wildcard,
				})
				if cr.Wildcard {
					nonComposable = true
				}
			}
			eff.Calls = calls
			doc.NonComposable = nonComposable
			hasData = true
		}
		doc.Effects = eff
	}

	if meta.Bounds != nil && len(meta.Bounds.Constraints) > 0 {
		bs := make([]string, 0, len(meta.Bounds.Constraints))
		for _, bc := range meta.Bounds.Constraints {
			bs = append(bs, fmt.Sprintf("%s%s%d", bc.Ident, bc.Op, bc.Value))
		}
		doc.Bounds = bs
		hasData = true
	}

	if meta.Gas != nil && meta.Gas.Upper > 0 {
		doc.GasUpper = meta.Gas.Upper
		hasData = true
	}

	if !hasData {
		return nil
	}
	return doc
}

// buildArtifactMetadataForContract builds ABI and storage-layout JSON for a specific contract.
func buildArtifactMetadataForContract(c *tolast.ContractDecl) (string, []byte, []byte, error) {
	if c == nil {
		return "", nil, nil, fmt.Errorf("artifact metadata requires a contract")
	}
	contractName := strings.TrimSpace(c.Name)
	if contractName == "" {
		return "", nil, nil, fmt.Errorf("artifact metadata requires contract name")
	}

	abi := tocABI{
		GasModel: tocABIGasModel{
			Version: gasModelVersion,
			Sload:   gasModelSload,
			Sstore:  gasModelSstore,
			LogBase: gasModelLogBase,
		},
		Functions:       make([]tocABIFunction, 0, len(c.Functions)),
		Events:          make([]tocABIEvent, 0, len(c.Events)),
		AccountContract: c.IsAccount,
	}
	for _, fn := range c.Functions {
		vis := functionVisibilityFromModifiers(fn.Modifiers)
		if vis != "public" && vis != "external" {
			continue
		}
		paramTypes := make([]string, 0, len(fn.Params))
		namedParams := make([]tocABIParam, 0, len(fn.Params))
		for _, p := range fn.Params {
			t := normalizeABIType(p.Type)
			paramTypes = append(paramTypes, t)
			namedParams = append(namedParams, tocABIParam{
				Name: p.Name,
				Type: t,
			})
		}
		returnTypes := make([]string, 0, len(fn.Returns))
		namedReturns := make([]tocABIParam, 0, len(fn.Returns))
		for _, r := range fn.Returns {
			t := normalizeABIType(r.Type)
			returnTypes = append(returnTypes, t)
			namedReturns = append(namedReturns, tocABIParam{
				Name: r.Name,
				Type: t,
			})
		}
		selector := strings.ToLower(strings.TrimSpace(fn.SelectorOverride))
		if selector == "" {
			selector = abiSelectorHex(fn.Name, paramTypes)
		}
		abiFn := tocABIFunction{
			Name:         fn.Name,
			Visibility:   vis,
			Mutability:   deriveFunctionMutability(fn),
			Selector:     selector,
			Params:       paramTypes,
			Returns:      returnTypes,
			NamedParams:  namedParams,
			NamedReturns: namedReturns,
			Doc:          docMetaToABI(fn.Doc),
		}
		if fn.Doc != nil {
			if len(fn.Doc.RequiresCap) > 0 {
				abiFn.RequiresCapability = strings.Join(fn.Doc.RequiresCap, ",")
			}
			if fn.Doc.PayAmount != "" {
				abiFn.PayAmountTomi = fn.Doc.PayAmount
			}
			if fn.Doc.PayRecipient != "" {
				abiFn.PayRecipient = fn.Doc.PayRecipient
			}
			// total_cost_tomi = pay_amount_tomi + gas_upper × 10gtomi (when both are known).
			if fn.Doc.HasPay && fn.Doc.PayAmount != "" && fn.Doc.Gas != nil && fn.Doc.Gas.Upper > 0 {
				if payInt, err := strconv.ParseUint(fn.Doc.PayAmount, 10, 64); err == nil {
					gasCost := fn.Doc.Gas.Upper * gtomiPerGas
					total := payInt + gasCost
					if total >= payInt { // overflow guard
						abiFn.TotalCostTomi = strconv.FormatUint(total, 10)
					}
				}
			}
			abiFn.Verifiable = fn.Doc.Verifiable
			abiFn.Delegated = fn.Doc.Delegated
			// @quota annotation
			if fn.Doc.QuotaCalls != "" {
				abiFn.QuotaCalls = fn.Doc.QuotaCalls
			}
			if fn.Doc.QuotaPrice != "" {
				abiFn.QuotaPrice = fn.Doc.QuotaPrice
			}
			// @total_cost declared max
			if fn.Doc.TotalCostMax != "" {
				abiFn.DeclaredTotalCostMax = fn.Doc.TotalCostMax
			}
		}
		abi.Functions = append(abi.Functions, abiFn)
	}
	// Synthesize verify_* stub ABI entries for @verifiable functions.
	for _, fn := range c.Functions {
		if fn.Doc == nil || !fn.Doc.Verifiable {
			continue
		}
		vis := functionVisibilityFromModifiers(fn.Modifiers)
		if vis != "public" && vis != "external" {
			continue
		}
		// Stub params: bytes proof, then original params, then expected_<ret> for each return.
		stubParams := []string{"bytes"}
		for _, p := range fn.Params {
			stubParams = append(stubParams, normalizeABIType(p.Type))
		}
		for _, r := range fn.Returns {
			stubParams = append(stubParams, normalizeABIType(r.Type))
		}
		stubSelector := abiSelectorHex("verify_"+fn.Name, stubParams)
		abi.Functions = append(abi.Functions, tocABIFunction{
			Name:           "verify_" + fn.Name,
			Visibility:     "external",
			Selector:       stubSelector,
			Params:         stubParams,
			Returns:        []string{"bool"},
			VerifiableStub: true,
		})
	}
	for _, ev := range c.Events {
		paramTypes := make([]string, 0, len(ev.Params))
		namedEvParams := make([]tocABIParam, 0, len(ev.Params))
		for _, p := range ev.Params {
			t := normalizeABIType(p.Type)
			paramTypes = append(paramTypes, t)
			namedEvParams = append(namedEvParams, tocABIParam{
				Name: p.Name,
				Type: t,
			})
		}
		abi.Events = append(abi.Events, tocABIEvent{
			Name:        ev.Name,
			Params:      paramTypes,
			NamedParams: namedEvParams,
		})
	}
	storage := tocStorageLayout{
		Slots: make([]tocStorageSlot, 0),
	}
	if c.Storage != nil {
		storage.Slots = make([]tocStorageSlot, 0, len(c.Storage.Slots))
		for _, s := range c.Storage.Slots {
			name := strings.TrimSpace(s.Name)
			typ := normalizeABIType(s.Type)
			storage.Slots = append(storage.Slots, tocStorageSlot{
				Name:          name,
				Type:          typ,
				CanonicalHash: keccak256Hex([]byte(fmt.Sprintf("tol.slot.%s.%s", contractName, name))),
			})
		}
	}

	// Populate manifest section if the contract declares one.
	if c.Manifest != nil && len(c.Manifest.Fields) > 0 {
		manifest := &tocABIManifest{}
		extra := make(map[string]string)
		for _, f := range c.Manifest.Fields {
			var val string
			if f.IsArray {
				// Serialize array as JSON: ["A","B"]
				var sb strings.Builder
				sb.WriteString("[")
				for i, elem := range f.Array {
					if i > 0 {
						sb.WriteString(",")
					}
					// Strip quotes if element is a string literal
					if len(elem) >= 2 && elem[0] == '"' && elem[len(elem)-1] == '"' {
						sb.WriteString(elem) // already quoted
					} else {
						sb.WriteString(`"`)
						sb.WriteString(elem)
						sb.WriteString(`"`)
					}
				}
				sb.WriteString("]")
				val = sb.String()
			} else {
				// Strip surrounding quotes from string literal values stored by the parser.
				val = f.Value
				if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
					val = val[1 : len(val)-1]
				}
			}
			switch f.Key {
			case "name":
				manifest.Name = val
			case "version":
				manifest.Version = val
			case "description":
				manifest.Description = val
			default:
				extra[f.Key] = val
			}
		}
		if len(extra) > 0 {
			manifest.Extra = extra
		}
		abi.Manifest = manifest
	}

	abiJSON, err := json.Marshal(abi)
	if err != nil {
		return "", nil, nil, err
	}
	storageJSON, err := json.Marshal(storage)
	if err != nil {
		return "", nil, nil, err
	}
	return contractName, abiJSON, storageJSON, nil
}

// buildArtifactMetadata builds ABI and storage-layout JSON from the module's primary contract.
func buildArtifactMetadata(mod *tolast.Module) (string, []byte, []byte, error) {
	if mod == nil {
		return "", nil, nil, fmt.Errorf("artifact metadata requires a contract module")
	}
	c := mod.PrimaryContract()
	if c == nil {
		return "", nil, nil, fmt.Errorf("artifact metadata requires contract module with at least one contract")
	}
	return buildArtifactMetadataForContract(c)
}

// deriveFunctionMutability computes the Solidity-style mutability string from
// the AST function declaration. It uses the doc metadata (effects and pay
// annotations) to determine the correct value at compile time, so that
// downstream consumers do not need to re-derive it from effects heuristics.
func deriveFunctionMutability(fn tolast.FunctionDecl) string {
	if fn.Doc != nil && fn.Doc.PayAmount != "" {
		return "payable"
	}
	if fn.Doc == nil || fn.Doc.Effects == nil {
		return "view"
	}
	eff := fn.Doc.Effects
	if len(eff.Writes) > 0 || len(eff.Emits) > 0 {
		return "nonpayable"
	}
	if len(eff.Reads) > 0 {
		return "view"
	}
	return "pure"
}

func functionVisibilityFromModifiers(modifiers []string) string {
	vis := ""
	for _, m := range modifiers {
		switch m {
		case "public", "external", "internal", "private":
			vis = m
		}
	}
	return vis
}

func normalizeABIType(t string) string {
	s := strings.Join(strings.Fields(t), " ")
	// "agent payable" and bare "agent" normalize to "agent" — agent is TOL's
	// canonical identity type; .toc is a TOL-native format, not EVM ABI.
	if s == "agent payable" || s == "agent" {
		s = "agent"
	}
	repl := strings.NewReplacer(
		"( ", "(",
		" )", ")",
		"[ ", "[",
		" ]", "]",
		" ,", ",",
		", ", ",",
		" => ", "=>",
		" =>", "=>",
		"=> ", "=>",
	)
	return repl.Replace(s)
}

func abiSelectorHex(name string, paramTypes []string) string {
	sig := fmt.Sprintf("%s(%s)", strings.TrimSpace(name), strings.Join(paramTypes, ","))
	sum := keccak256(sig)
	return "0x" + hex.EncodeToString(sum[:4])
}

func keccak256Hex(data []byte) string {
	sum := keccak256Bytes(data)
	return "0x" + hex.EncodeToString(sum)
}

func keccak256(s string) []byte {
	return keccak256Bytes([]byte(s))
}

func keccak256Bytes(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func writeLenBytes(w io.Writer, b []byte) error {
	if err := writeU32(w, uint32(len(b))); err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	_, err := w.Write(b)
	return err
}

func readLenBytes(r *byteReader) ([]byte, error) {
	n, err := readU32(r)
	if err != nil {
		return nil, err
	}
	if int(n) < 0 || int(n) > len(r.b)-r.n {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, int(n))
	copy(out, r.b[r.n:r.n+int(n)])
	r.n += int(n)
	return out, nil
}

func readFixedBytes(r *byteReader, n int) ([]byte, error) {
	if n < 0 || r.n+n > len(r.b) {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, n)
	copy(out, r.b[r.n:r.n+n])
	r.n += n
	return out, nil
}

func decodeHashHex(v string) ([]byte, error) {
	s := strings.TrimSpace(strings.ToLower(v))
	if s == "" {
		return nil, fmt.Errorf("empty hash")
	}
	if !strings.HasPrefix(s, "0x") {
		return nil, fmt.Errorf("hash must start with 0x")
	}
	raw := s[2:]
	if len(raw) != 64 {
		return nil, fmt.Errorf("hash must be 32 bytes")
	}
	out, err := hex.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	return out, nil
}
