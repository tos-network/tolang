package lua

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	luast "github.com/tos-network/tolang/ast"
	"github.com/tos-network/tolang/parse"
	tolast "github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/lower"
	"golang.org/x/crypto/sha3"
)

// bootstrapMode controls which entry points are emitted in the bootstrap chunk.
type bootstrapMode int

const (
	// bootstrapModeRuntime is the default: emit tos.oninvoke for call dispatch.
	// The constructor function and tos.oncreate are NOT emitted; the constructor
	// is never callable from a normal Call() — it lives only in the init artifact.
	bootstrapModeRuntime bootstrapMode = iota
	// bootstrapModeInit emits only the constructor entry point.
	// The constructor function is emitted and called directly at module top-level
	// (no tos.oninvoke, no tos.oncreate wrapper). Used for init_code in the
	// init/runtime split (analogous to EVM initcode).
	bootstrapModeInit
)

// buildDirectIRFromLowered converts lowered TOL program into VM IR.
// Current direct-IR bootstrap supports:
// 1) empty contracts
// 2) function/fallback/constructor wrappers with a restricted statement/expression subset
func buildDirectIRFromLowered(p *lower.Program, sourceName string, mode bootstrapMode) (*IRProgram, error) {
	if p == nil {
		return nil, fmt.Errorf("[%s] nil lowered program", diag.CodeLowerNotImplemented)
	}

	if sourceName == "" {
		sourceName = p.ContractName
	}

	if p.HasConstructor || p.HasFallback || len(p.Functions) > 0 {
		chunk, err := buildBootstrapChunkFromLowered(p, mode)
		if err != nil {
			return nil, err
		}
		return buildIRFromChunk(chunk, sourceName)
	}

	// Empty contract: emit a trivial return-only program.
	return buildIRFromChunk([]luast.Stmt{}, sourceName)
}

func buildBootstrapChunkFromLowered(p *lower.Program, mode bootstrapMode) ([]luast.Stmt, error) {
	if p == nil {
		return nil, fmt.Errorf("[%s] nil lowered program", diag.CodeLowerNotImplemented)
	}
	if !p.HasConstructor && !p.HasFallback && len(p.Functions) == 0 {
		return []luast.Stmt{}, nil
	}

	dispatchFns, err := collectDispatchFuncs(p.Functions)
	if err != nil {
		return nil, err
	}
	env, err := buildLoweringEnv(p.ContractName, dispatchFns, p.Functions, p.StorageSlots, p.Libraries, p.UsingDecls, p.Errors, p.Enums, p.Structs, p.Constants, p.Interfaces, p.TypeAliases)
	if err != nil {
		return nil, err
	}
	if len(p.Purposes) > 0 {
		env.purposeNames = make(map[string]int, len(p.Purposes))
		for i, name := range p.Purposes {
			env.purposeNames[name] = i
		}
	}

	chunk := make([]luast.Stmt, 0, len(p.Functions)+16)
	selectorPrelude, err := buildSelectorPrelude()
	if err != nil {
		return nil, err
	}
	chunk = append(chunk, selectorPrelude...)
	abiPrelude, err := buildABIPrelude(p.Structs)
	if err != nil {
		return nil, err
	}
	chunk = append(chunk, abiPrelude...)
	hostPrelude, err := buildHostPrelude()
	if err != nil {
		return nil, err
	}
	chunk = append(chunk, hostPrelude...)
	if len(p.Events) > 0 {
		eventPrelude, err := buildEventPreludeFromLowered(p.Events)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, eventPrelude...)
	}
	if len(p.StorageSlots) > 0 {
		prelude, err := buildStoragePreludeFromLowered(env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, prelude...)
	}
	if len(p.Capabilities) > 0 || len(p.Purposes) > 0 || hasAgentNativeSlots(p.StorageSlots) {
		agentPrelude, err := buildAgentNativePrelude(p)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, agentPrelude...)
	}
	// Emit library functions first so they are available when contract functions call them.
	for _, lib := range p.Libraries {
		for _, fn := range lib.Functions {
			st, err := lowerLibraryFunctionToLua(lib.Name, fn, env)
			if err != nil {
				return nil, err
			}
			chunk = append(chunk, st)
		}
	}

	if mode == bootstrapModeInit {
		// Init artifact: emit only the constructor behind tos.oncreate.
		// No contract functions, no tos.oninvoke.
		// At deploy time, Execute() calls tos.oncreate() (via tolDispatch) with no args;
		// the constructor reads tos.calldata for ABI-decoded arguments.
		// Test paths can still call tos.oncreate(owner, supply, ...) with varargs.
		if p.HasConstructor {
			st, err := lowerConstructorToLua(p.ConstructorParams, p.ConstructorBody, env)
			if err != nil {
				return nil, err
			}
			chunk = append(chunk, st)
			chunk = append(chunk, buildTosInitStmt())
			chunk = append(chunk, buildOnCreateAssignStmt())
		}
		return chunk, nil
	}

	// Runtime artifact (default): emit all contract functions + tos.oninvoke dispatch.
	// The constructor is NOT emitted here — it lives only in the init artifact.
	for _, fn := range p.Functions {
		st, err := lowerFunctionToLua(fn, env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, st)
	}
	if p.HasFallback {
		st, err := lowerFallbackToLua(p.FallbackBody, env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, st)
	}
	if p.HasReceive {
		st, err := lowerReceiveToLua(p.ReceiveBody, env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, st)
	}

	if p.HasFallback || p.HasReceive || len(dispatchFns) > 0 {
		chunk = append(chunk, buildTosInitStmt())
		chunk = append(chunk, buildOnInvokeAssignStmt(dispatchFns, p.HasFallback, p.HasReceive, env))
	}
	return chunk, nil
}

type dispatchFunc struct {
	Name      string            // TOL source name (for error messages)
	LuaName   string            // actual Lua global function name (may be mangled for overloads)
	Signature string
	Params    []tolast.FieldDecl // parameter list (needed for struct calldata decode)
}

type loweringEnv struct {
	contractName       string
	selectorByFunction map[string]string
	functionByName     map[string]struct{}
	// functionsByName maps source name → all overloads (for internal call resolution).
	functionsByName    map[string][]lower.Function
	storageByName      map[string]storageSlotInfo
	// libraryByName maps library name → (function name → arity) for library call lowering.
	libraryByName      map[string]map[string]int
	// usingTypeToLib maps a TOL type string → library name for 'using X for T' expansion.
	usingTypeToLib     map[string]string
	// enumByName maps enum name → (member name → integer value).
	enumByName         map[string]map[string]int
	// errorSigByName maps error name → canonical ABI signature string (e.g. "Unauthorized(address,uint256)").
	errorSigByName     map[string]string
	// structFields maps struct name → ordered list of field names.
	structFields       map[string][]string
	// structFieldTypes maps struct name → ordered list of fields (name+type).
	structFieldTypes   map[string][]tolast.FieldDecl
	// constantByName maps constant name → its AST literal expression for inline substitution.
	constantByName     map[string]*tolast.Expr
	// interfaceByName maps interface name → list of function signatures (for type(I).interfaceId).
	interfaceByName    map[string][]lower.InterfaceFuncSig
	// packageByInterface maps interface local name → origin package path (e.g. "AgentRegistry" → "tos.registry").
	packageByInterface map[string]string
	// contractByInterface maps interface local name → concrete contract name (e.g. "IRegistry" → "AgentRegistry").
	contractByInterface map[string]string
	// typeAliases maps user-defined value type name → underlying type (e.g. "MyInt" → "uint256").
	typeAliases        map[string]string
	// purposeNames maps purpose declaration name → ordinal (0-based), for escrow/release/slash lowering.
	purposeNames       map[string]int
}

type storageSlotKind string

const (
	storageKindScalar  storageSlotKind = "scalar"
	storageKindMapping storageSlotKind = "mapping"
	storageKindArray   storageSlotKind = "array"
)

type storageSlotInfo struct {
	name         string
	kind         storageSlotKind
	typ          string
	mappingDepth int
	baseSlotHash string // compile-time keccak256("tol.slot.<Contract>.<name>")
	luaConstName string // "__tol_s_<name>" - Lua local constant name
	isTransient  bool   // true for EIP-1153 transient storage (TLOAD/TSTORE)
}

// computeBaseSlotHash returns the canonical base slot hash for a named storage
// slot per TOL spec §8.3: keccak256("tol.slot.<contractName>.<slotName>").
func computeBaseSlotHash(contractName, slotName string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("tol.slot." + contractName + "." + slotName))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

// computeImmutableSlotHash returns the canonical slot hash for an immutable variable:
// keccak256("tol.immutable.<contractName>.<name>").
func computeImmutableSlotHash(contractName, name string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("tol.immutable." + contractName + "." + name))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

// computeTransientSlotHash returns the canonical slot hash for a transient storage variable:
// keccak256("tol.transient.<contractName>.<name>").
func computeTransientSlotHash(contractName, name string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("tol.transient." + contractName + "." + name))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

func buildLoweringEnv(contractName string, dispatchFns []dispatchFunc, funcs []lower.Function, storageSlots []lower.StorageSlot, libraries []lower.Library, usingDecls []lower.UsingDecl, errors []lower.ErrorDecl, enums []lower.EnumDecl, structs []lower.StructDecl, constants []lower.ConstantDecl, interfaces []lower.InterfaceDecl, typeAliases []lower.TypeAlias) (*loweringEnv, error) {
	m := make(map[string]string, len(dispatchFns))
	for _, df := range dispatchFns {
		m[df.Name] = df.Signature
	}
	fm := make(map[string]struct{}, len(funcs))
	fnsByName := make(map[string][]lower.Function, len(funcs))
	for _, fn := range funcs {
		name := strings.TrimSpace(fn.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] function name cannot be empty in lowered program", diag.CodeLowerUnsupportedFeature)
		}
		fm[name] = struct{}{}
		fnsByName[name] = append(fnsByName[name], fn)
	}
	sm := make(map[string]storageSlotInfo, len(storageSlots))
	for _, slot := range storageSlots {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] storage slot name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		if _, exists := sm[name]; exists {
			return nil, fmt.Errorf("[%s] duplicate storage slot '%s' in lowered program", diag.CodeLowerUnsupportedFeature, name)
		}
		kind := classifyStorageSlotKind(slot.Type)
		var slotHash string
		if slot.IsImmutable {
			slotHash = computeImmutableSlotHash(contractName, name)
		} else if slot.IsTransient {
			slotHash = computeTransientSlotHash(contractName, name)
		} else {
			slotHash = computeBaseSlotHash(contractName, name)
		}
		sm[name] = storageSlotInfo{
			name:         name,
			kind:         kind,
			typ:          strings.TrimSpace(slot.Type),
			mappingDepth: mappingTypeDepth(slot.Type),
			baseSlotHash: slotHash,
			luaConstName: "__tol_s_" + name,
			isTransient:  slot.IsTransient,
		}
	}
	// Build library lookup: library name → function name → arity.
	lm := make(map[string]map[string]int, len(libraries))
	for _, lib := range libraries {
		libName := strings.TrimSpace(lib.Name)
		if libName == "" {
			continue
		}
		fnMap := make(map[string]int, len(lib.Functions))
		for _, fn := range lib.Functions {
			fnMap[strings.TrimSpace(fn.Name)] = len(fn.Params)
		}
		lm[libName] = fnMap
	}
	// Build using lookup: type → library name.
	um := make(map[string]string, len(usingDecls))
	for _, ud := range usingDecls {
		typeName := strings.TrimSpace(ud.Type)
		libName := strings.TrimSpace(ud.Library)
		if typeName != "" && libName != "" {
			um[typeName] = libName
		}
	}
	// Build enum lookup: enum name → member name → integer value.
	enumByName := make(map[string]map[string]int, len(enums))
	for _, en := range enums {
		eName := strings.TrimSpace(en.Name)
		if eName == "" {
			continue
		}
		memberVals := make(map[string]int, len(en.Members))
		for i, m := range en.Members {
			mname := strings.TrimSpace(m)
			if mname != "" {
				memberVals[mname] = i
			}
		}
		enumByName[eName] = memberVals
	}
	// Build error ABI signature lookup: error name → canonical signature.
	errorSigByName := make(map[string]string, len(errors))
	for _, ed := range errors {
		eName := strings.TrimSpace(ed.Name)
		if eName == "" {
			continue
		}
		types := make([]string, 0, len(ed.Params))
		for _, p := range ed.Params {
			t := normalizeSelectorType(p.Type)
			if t != "" {
				types = append(types, t)
			}
		}
		sig := fmt.Sprintf("%s(%s)", eName, strings.Join(types, ","))
		errorSigByName[eName] = sig
	}
	// Build struct fields lookup: struct name → ordered field names.
	sfm := make(map[string][]string, len(structs))
	sftm := make(map[string][]tolast.FieldDecl, len(structs))
	for _, sd := range structs {
		sname := strings.TrimSpace(sd.Name)
		if sname == "" {
			continue
		}
		fieldNames := make([]string, 0, len(sd.Fields))
		fieldDecls := make([]tolast.FieldDecl, 0, len(sd.Fields))
		for _, f := range sd.Fields {
			fname := strings.TrimSpace(f.Name)
			if fname != "" {
				fieldNames = append(fieldNames, fname)
				fieldDecls = append(fieldDecls, f)
			}
		}
		sfm[sname] = fieldNames
		sftm[sname] = fieldDecls
	}
	// Build constant lookup: constant name → AST literal expression.
	constByName := make(map[string]*tolast.Expr, len(constants))
	for _, cd := range constants {
		cname := strings.TrimSpace(cd.Name)
		if cname != "" && cd.Value != nil {
			constByName[cname] = cd.Value
		}
	}
	// Build interface lookup: interface name → function signature list.
	ifaceByName := make(map[string][]lower.InterfaceFuncSig, len(interfaces))
	pkgByIface := make(map[string]string, len(interfaces))
	ctByIface := make(map[string]string, len(interfaces))
	for _, iface := range interfaces {
		name := strings.TrimSpace(iface.Name)
		if name != "" {
			ifaceByName[name] = iface.Functions
			if iface.PackageName != "" {
				pkgByIface[name] = iface.PackageName
				ctByIface[name] = iface.ContractName
			}
			// Register qualified constant names: "ContractName.CONST_NAME" and "LocalName.CONST_NAME".
			// This enables fully-qualified access: let fee = AgentRegistry.MAX_FEE;
			contractName := strings.TrimSpace(iface.ContractName)
			for _, cd := range iface.Constants {
				cname := strings.TrimSpace(cd.Name)
				if cname != "" && cd.Value != nil {
					// Register under local binding name (e.g. "AgentRegistry.MAX_FEE" or "IRegistry.MAX_FEE")
					constByName[name+"."+cname] = cd.Value
					// Also register under contract name if it differs from local name
					if contractName != "" && contractName != name {
						constByName[contractName+"."+cname] = cd.Value
					}
				}
			}
			// Register qualified enum access: "ContractName.EnumName" → member map.
			// Members accessible as "ContractName.EnumName.Member".
			for _, ed := range iface.Enums {
				eName := strings.TrimSpace(ed.Name)
				if eName == "" {
					continue
				}
				memberVals := make(map[string]int, len(ed.Members))
				for i, m := range ed.Members {
					mname := strings.TrimSpace(m)
					if mname != "" {
						memberVals[mname] = i
					}
				}
				// Register as "LocalName.EnumName" and "ContractName.EnumName"
				enumByName[name+"."+eName] = memberVals
				if contractName != "" && contractName != name {
					enumByName[contractName+"."+eName] = memberVals
				}
			}
		}
	}
	// Build type alias map: user-defined type name → underlying type.
	typeAliasMap := make(map[string]string, len(typeAliases))
	for _, ta := range typeAliases {
		name := strings.TrimSpace(ta.Name)
		if name != "" {
			typeAliasMap[name] = strings.TrimSpace(ta.Underlying)
		}
	}
	return &loweringEnv{
		contractName:       contractName,
		selectorByFunction: m,
		functionByName:     fm,
		functionsByName:    fnsByName,
		storageByName:      sm,
		libraryByName:      lm,
		usingTypeToLib:     um,
		enumByName:         enumByName,
		errorSigByName:     errorSigByName,
		structFields:       sfm,
		structFieldTypes:   sftm,
		constantByName:     constByName,
		interfaceByName:     ifaceByName,
		packageByInterface:  pkgByIface,
		contractByInterface: ctByIface,
		typeAliases:         typeAliasMap,
	}, nil
}

// resolveOverloadedLuaName resolves the Lua function name for a call to `name` with `argCount`
// arguments. Returns the LuaName of the matching overload, or "" if no overloads or no match.
func resolveOverloadedLuaName(env *loweringEnv, name string, argCount int) string {
	if env == nil {
		return ""
	}
	overloads, ok := env.functionsByName[name]
	if !ok || len(overloads) <= 1 {
		return ""
	}
	// Multiple overloads: pick first one whose param count matches.
	for _, fn := range overloads {
		if len(fn.Params) == argCount {
			return fn.LuaName
		}
	}
	// No match by arity – return "" and let the caller fall through.
	return ""
}

func classifyStorageSlotKind(t string) storageSlotKind {
	norm := normalizeSelectorType(t)
	compact := strings.ReplaceAll(norm, " ", "")
	switch {
	case strings.HasPrefix(compact, "mapping("):
		return storageKindMapping
	case strings.HasSuffix(compact, "]"):
		return storageKindArray
	default:
		return storageKindScalar
	}
}

func mappingTypeDepth(t string) int {
	compact := strings.ReplaceAll(normalizeSelectorType(t), " ", "")
	if compact == "" {
		return 0
	}
	return strings.Count(compact, "mapping(")
}

// mappingValueType extracts the value type V from "mapping(K=>V)".
// Returns "" if t is not a mapping type.
func mappingValueType(t string) string {
	compact := strings.ReplaceAll(normalizeSelectorType(t), " ", "")
	if !strings.HasPrefix(compact, "mapping(") {
		return ""
	}
	// The content inside mapping(...).
	inner := compact[len("mapping(") : len(compact)-1]
	// Find "=>" at depth 0.
	depth := 0
	for i := 0; i < len(inner)-1; i++ {
		switch inner[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '=':
			if depth == 0 && i+1 < len(inner) && inner[i+1] == '>' {
				return inner[i+2:]
			}
		}
	}
	return ""
}

// arrayElemType strips the trailing [] or [N] from an array type.
// Returns "" if t is not an array type.
func arrayElemType(t string) string {
	compact := strings.ReplaceAll(normalizeSelectorType(t), " ", "")
	if !strings.HasSuffix(compact, "]") {
		return ""
	}
	// Find the matching '[' for the last ']'.
	for i := len(compact) - 2; i >= 0; i-- {
		if compact[i] == '[' {
			return compact[:i]
		}
	}
	return ""
}

// slotTypeAfterIndex returns the type after applying one index to t.
// For mapping(K=>V) returns V; for T[] returns T; otherwise returns "".
func slotTypeAfterIndex(t string) string {
	compact := strings.ReplaceAll(normalizeSelectorType(t), " ", "")
	if strings.HasPrefix(compact, "mapping(") {
		return mappingValueType(t)
	}
	if strings.HasSuffix(compact, "]") {
		return arrayElemType(t)
	}
	return ""
}

// slotMaxIndexDepth returns the maximum number of indices that can be applied
// before reaching a scalar type.
func slotMaxIndexDepth(t string) int {
	depth := 0
	cur := strings.TrimSpace(t)
	for {
		next := slotTypeAfterIndex(cur)
		if next == "" {
			break
		}
		depth++
		cur = next
	}
	return depth
}

func buildStoragePreludeFromLowered(env *loweringEnv) ([]luast.Stmt, error) {
	if env == nil || len(env.storageByName) == 0 {
		return []luast.Stmt{}, nil
	}
	names := make([]string, 0, len(env.storageByName))
	for name := range env.storageByName {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder

	// Emit compile-time base slot hash constants (spec §8.3).
	// Each named slot gets a local constant holding its canonical keccak256 hash.
	for _, name := range names {
		info := env.storageByName[name]
		sb.WriteString(fmt.Sprintf("local %s = %q\n", info.luaConstName, info.baseSlotHash))
	}

	// Flat storage table: storage[bytes32_hex] = value.
	// All key derivation is done before the load/store call.
	// If host storage hooks exist at tos.sload/tos.sstore, prefer those.
	sb.WriteString(`__tol_storage = __tol_storage or {}

-- Read a slot by its final derived hash key.
function __tol_sload(slot_hash)
  if tos ~= nil and type(tos) == "table" and type(tos.sload) == "function" then
    local hv = tos.sload(slot_hash)
    if hv == nil then return 0 end
    return hv
  end
  local v = __tol_storage[slot_hash]
  if v == nil then return 0 end
  return v
end

-- Write a slot by its final derived hash key.
function __tol_sstore(slot_hash, value)
  if tos ~= nil and type(tos) == "table" and type(tos.sstore) == "function" then
    local hv = tos.sstore(slot_hash, value)
    if hv == nil then return value end
    return hv
  end
  __tol_storage[slot_hash] = value
  return value
end

-- Derive a mapping slot key: keccak256(encode(key) ++ base_hash).
-- Matches spec §8.3: h_n = H(encode(k_n) ++ h_{n-1}).
function __tol_mkey(key, base)
  local base_hex = base:sub(3)       -- strip leading "0x"
  local key_hex  = __tol_enc(key)    -- 64 hex chars, no 0x prefix
  return keccak256("0x" .. key_hex .. base_hex)
end

-- Compute element slot for a storage array: H(base_slot) + index.
-- Matches spec §8.4: element i at keccak256(base_slot) + i.
function __tol_arr_elem(base, idx)
  local data_base = keccak256(base)  -- H(base): hash the 32-byte base slot
  return uint256_add_hex(data_base, idx)
end

-- Read array length (stored at the base slot itself).
function __tol_slen(base)
  return __tol_sload(base)
end

-- Push a value onto a storage dynamic array.
function __tol_spush(base, value)
  local n = __tol_slen(base)
  local elem_slot = __tol_arr_elem(base, n)
  __tol_sstore(elem_slot, value)
  __tol_sstore(base, n + 1)
  return n + 1
end

-- EIP-1153 transient storage (cleared after each transaction).
__tol_transient_storage = __tol_transient_storage or {}

function __tol_tsload(slot_hash)
  if tos ~= nil and type(tos) == "table" and type(tos.tload) == "function" then
    return tos.tload(slot_hash) or 0
  end
  return __tol_transient_storage[slot_hash] or 0
end

function __tol_tsstore(slot_hash, value)
  if tos ~= nil and type(tos) == "table" and type(tos.tstore) == "function" then
    tos.tstore(slot_hash, value); return value
  end
  __tol_transient_storage[slot_hash] = value
  return value
end
`)
	chunk, err := parse.Parse(bytes.NewReader([]byte(sb.String())), "<tol-storage-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build storage prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

// hasAgentNativeSlots reports whether any storage slot uses an agent-native type
// (oracle<T>, vote<T>, task<T>, agent).
func hasAgentNativeSlots(slots []lower.StorageSlot) bool {
	for _, s := range slots {
		t := s.Type
		if t == "agent" ||
			strings.HasPrefix(t, "oracle<") ||
			strings.HasPrefix(t, "vote<") ||
			strings.HasPrefix(t, "task<") {
			return true
		}
	}
	return false
}

// agentNativeSlotKind returns "oracle", "vote", "task", or "agent" for agent-native
// storage slot types, or "" for ordinary types.
func agentNativeSlotKind(typ string) string {
	switch {
	case strings.HasPrefix(typ, "oracle<"):
		return "oracle"
	case strings.HasPrefix(typ, "vote<"):
		return "vote"
	case strings.HasPrefix(typ, "task<"):
		return "task"
	case typ == "agent":
		return "agent"
	}
	return ""
}

// computeAgentSlotHash computes keccak256(path) where path is the fully-qualified slot
// key (e.g. "tol.oracle.TaskBoard.priceOracle.value"). Callers build the full path.
func computeAgentSlotHash(path string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte(path))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

// buildAgentNativePrelude emits:
//  1. Capability bit locals: local __tol_cap_X = tos.capabilitybit and tos.capabilitybit("X") or 0
//  2. Purpose ordinal locals: local __tol_pur_Y = N  (compile-time constant)
//  3. Per oracle/vote/task slot sub-slot hash constants
//  4. Oracle, vote, task helper functions (once, if any such slots exist)
func buildAgentNativePrelude(p *lower.Program) ([]luast.Stmt, error) {
	var sb strings.Builder

	// 1. Capability bit locals.
	for _, cap := range p.Capabilities {
		luaName := "__tol_cap_" + cap
		sb.WriteString(fmt.Sprintf("local %s = tos and type(tos.capabilitybit)==\"function\" and tos.capabilitybit(%q) or 0\n", luaName, cap))
	}

	// 2. Purpose ordinal locals.
	for i, pur := range p.Purposes {
		luaName := "__tol_pur_" + pur
		sb.WriteString(fmt.Sprintf("local %s = %d\n", luaName, i))
	}

	// 3. Sub-slot hash constants for oracle/vote/task slots.
	hasOracle := false
	hasVote := false
	hasTask := false
	for _, slot := range p.StorageSlots {
		kind := agentNativeSlotKind(slot.Type)
		if kind == "" {
			continue
		}
		switch kind {
		case "oracle":
			hasOracle = true
			valSlot := computeAgentSlotHash("tol.oracle." + p.ContractName + "." + slot.Name + ".value")
			setSlot := computeAgentSlotHash("tol.oracle." + p.ContractName + "." + slot.Name + ".set")
			sb.WriteString(fmt.Sprintf("local __tol_s_%s_val = %q\n", slot.Name, valSlot))
			sb.WriteString(fmt.Sprintf("local __tol_s_%s_set = %q\n", slot.Name, setSlot))
		case "vote":
			hasVote = true
			for _, sub := range []string{"tally", "eligible", "voted", "threshold", "deadline", "result"} {
				h := computeAgentSlotHash("tol.vote." + p.ContractName + "." + slot.Name + "." + sub)
				sb.WriteString(fmt.Sprintf("local __tol_s_%s_%s = %q\n", slot.Name, sub, h))
			}
		case "task":
			hasTask = true
			h := computeAgentSlotHash("tol.task." + p.ContractName + "." + slot.Name)
			sb.WriteString(fmt.Sprintf("local __tol_s_%s_base = %q\n", slot.Name, h))
		}
	}

	// 4. Oracle helper functions.
	if hasOracle {
		sb.WriteString(`
local __tol_oracle_fulfill = tos and type(tos.oracle_fulfill)=="function" and tos.oracle_fulfill or function(val_slot, set_slot, value)
  if __tol_sload(set_slot) ~= 0 then error("OracleAlreadySet") end
  __tol_sstore(set_slot, 1)
  __tol_sstore(val_slot, value)
end
local __tol_oracle_is_set = function(set_slot) return __tol_sload(set_slot) ~= 0 end
local __tol_oracle_value  = function(val_slot) return __tol_sload(val_slot) end
`)
	}

	// 5. Vote helper functions.
	if hasVote {
		sb.WriteString(`
local __tol_vote_new = function(elig_slot, thresh_slot, ddl_slot, cap_bit, threshold, deadline)
  local elig = tos and type(tos.totaleligible)=="function" and tos.totaleligible(cap_bit) or 0
  __tol_sstore(elig_slot, elig)
  __tol_sstore(thresh_slot, threshold)
  __tol_sstore(ddl_slot, deadline)
end
local __tol_vote_cast = tos and type(tos.vote_cast)=="function" and tos.vote_cast or function(voted_base, tally_slot, voter, choice)
  local key = __tol_mkey(voter, voted_base)
  if __tol_sload(key) ~= 0 then error("AlreadyVoted") end
  __tol_sstore(key, 1)
  __tol_sstore(tally_slot, __tol_sload(tally_slot) + choice)
end
local __tol_vote_tally   = function(tally_slot) return __tol_sload(tally_slot) end
local __tol_vote_decided = function(tally_slot, thresh_slot) return __tol_sload(tally_slot) >= __tol_sload(thresh_slot) end
`)
	}

	// 6. Task helper functions.
	if hasTask {
		sb.WriteString(`
local __tol_task_post = function(task_base, poster)
  local tid = keccak256(tostring(poster) .. tostring(block and block.number or 0))
  local state_slot = __tol_mkey(tid, task_base)
  __tol_sstore(state_slot, 1)
  return tid
end
local __tol_task_transition = tos and type(tos.task_transition)=="function" and tos.task_transition or function(task_base, tid, from_state, to_state, guard_addr)
  local slot = __tol_mkey(tid, task_base)
  local cur = __tol_sload(slot)
  if cur ~= from_state then error("TaskInvalidTransition") end
  if guard_addr ~= nil and guard_addr ~= 0 and guard_addr ~= msg.sender then error("TaskUnauthorized") end
  __tol_sstore(slot, to_state)
end
local __tol_task_state = function(task_base, tid) return __tol_sload(__tol_mkey(tid, task_base)) end
`)
	}

	// 7. Agent cast helper — always emitted when any agent-native content is present.
	// __tol_agent_cast(addr) verifies the address is a registered agent via tos.agentload.
	sb.WriteString(`
local __tol_agent_cast = tos and type(tos.agentload)=="function" and function(a)
  if tos.agentload(a) == 0 then error("AgentNotFound") end
  return a
end or function(a) return a end
`)

	// 8. Escrow/release/slash helpers — emitted when purposes are declared.
	if len(p.Purposes) > 0 {
		sb.WriteString(`
local __tol_escrow  = tos and type(tos.escrow)=="function"  and tos.escrow  or function(...) error("escrow: tos unavailable") end
local __tol_release = tos and type(tos.release)=="function" and tos.release or function(...) error("release: tos unavailable") end
local __tol_slash   = tos and type(tos.slash)=="function"   and tos.slash   or function(...) error("slash: tos unavailable") end
`)
	}

	// 9. Delegation verify helper (emitted if any function is @delegated).
	hasDelegated := false
	for _, fn := range p.Functions {
		if fn.Doc != nil && fn.Doc.Delegated {
			hasDelegated = true
			break
		}
	}
	if hasDelegated {
		sb.WriteString(`
local __tol_delegation_verify = tos and type(tos.delegationused)=="function" and function(nonce, sig)
  if tos.delegationused(nonce) ~= 0 then error("DelegationNonceUsed") end
  if tos and type(tos.verify_sig)=="function" then
    if not tos.verify_sig(msg.sender, nonce, sig) then error("DelegationInvalidSig") end
  end
  if tos and type(tos.setdelegationused)=="function" then tos.setdelegationused(nonce) end
end or function(nonce, sig) error("delegation.verify: tos unavailable") end
`)
	}

	if sb.Len() == 0 {
		return []luast.Stmt{}, nil
	}
	chunk, err := parse.Parse(bytes.NewReader([]byte(sb.String())), "<tol-agent-native-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build agent-native prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

func buildSelectorPrelude() ([]luast.Stmt, error) {
	const src = `
function __tol_selector(sig)
  if type(sig) ~= "string" then
    error("selector(...) argument must be string")
  end
  if string.sub(sig, 1, 2) == "0x" then
    if #sig ~= 10 then
      error("selector(...) hex selector must be 0x followed by 8 hex chars")
    end
    for i = 3, 10 do
      local c = string.sub(sig, i, i)
      local b = string.byte(c)
      local is_digit = (b >= string.byte("0") and b <= string.byte("9"))
      local is_lower = (b >= string.byte("a") and b <= string.byte("f"))
      local is_upper = (b >= string.byte("A") and b <= string.byte("F"))
      if not (is_digit or is_lower or is_upper) then
        error("selector(...) hex selector must be 0x followed by 8 hex chars")
      end
    end
    return string.lower(sig)
  end
  local hex = "0x"
  for i = 1, #sig do
    local b = string.byte(sig, i)
    hex = hex .. string.format("%02x", b)
  end
  local h = keccak256(hex)
  return string.sub(h, 1, 10)
end
`
	chunk, err := parse.Parse(bytes.NewReader([]byte(src)), "<tol-selector-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build selector prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

func buildABIPrelude(structs []lower.StructDecl) ([]luast.Stmt, error) {
	const staticSrc = `
if abi == nil then
  abi = {}
end
if type(abi) ~= "table" then
  error("abi global must be table")
end

local function __tol_hex_encode_text(s)
  local out = ""
  for i = 1, #s do
    out = out .. string.format("%02x", string.byte(s, i))
  end
  return out
end

local function __tol_is_hex_char_byte(b)
  local is_digit = (b >= string.byte("0") and b <= string.byte("9"))
  local is_lower = (b >= string.byte("a") and b <= string.byte("f"))
  local is_upper = (b >= string.byte("A") and b <= string.byte("F"))
  return is_digit or is_lower or is_upper
end

local function __tol_hex_of(v)
  local tv = type(v)
  if tv == "string" then
    if string.sub(v, 1, 2) == "0x" then
      local rest = string.sub(v, 3)
      if (#rest % 2) ~= 0 then
        error("abi hex string must have even length")
      end
      for i = 1, #rest do
        local b = string.byte(rest, i)
        if not __tol_is_hex_char_byte(b) then
          error("abi hex string contains non-hex character")
        end
      end
      return string.lower(rest)
    end
    return __tol_hex_encode_text(v)
  end
  if tv == "boolean" then
    if v then
      return "01"
    end
    return "00"
  end
  if tv == "nil" then
    return ""
  end
  return __tol_hex_encode_text(tostring(v))
end

-- Legacy abi.encode: kept for internal use by abi.encodeWithSelector.
-- TOL source abi.encode(...) calls are lowered to __tol_abi_encode_v2 with type info.
function abi.encode(...)
  local n = select("#", ...)
  local out = ""
  for i = 1, n do
    if i > 1 then
      out = out .. "ff"
    end
    out = out .. __tol_hex_of(select(i, ...))
  end
  return "0x" .. out
end

-- Legacy abi.encodePacked: kept for internal use by abi.encodeWithSelector.
-- TOL source abi.encodePacked(...) calls are lowered to __tol_abi_encode_packed_v2 with type info.
function abi.encodePacked(...)
  local n = select("#", ...)
  local out = ""
  for i = 1, n do
    out = out .. __tol_hex_of(select(i, ...))
  end
  return "0x" .. out
end

function abi.encodeWithSelector(sel, ...)
  if type(sel) ~= "string" then
    error("abi.encodeWithSelector selector must be string")
  end
  sel = string.lower(sel)
  if #sel ~= 10 or string.sub(sel, 1, 2) ~= "0x" then
    error("abi.encodeWithSelector selector must be 0x followed by 8 hex chars")
  end
  for i = 3, 10 do
    local b = string.byte(sel, i)
    if not __tol_is_hex_char_byte(b) then
      error("abi.encodeWithSelector selector must be 0x followed by 8 hex chars")
    end
  end
  local payload = abi.encodePacked(...)
  return "0x" .. string.sub(sel, 3) .. string.sub(payload, 3)
end

function abi.encodeWithSignature(sig, ...)
  if type(sig) ~= "string" then
    error("abi.encodeWithSignature signature must be string")
  end
  local sel = __tol_selector(sig)
  return abi.encodeWithSelector(sel, ...)
end

function abi.decode(data)
  return data
end

local function __tol_hex_payload(data)
  if type(data) ~= "string" then
    error("abi.decode typed input must be string")
  end
  if string.sub(data, 1, 2) ~= "0x" then
    error("abi.decode typed input must be 0x-prefixed even-length hex bytes")
  end
  local rest = string.sub(data, 3)
  if (#rest % 2) ~= 0 then
    error("abi.decode typed input must be 0x-prefixed even-length hex bytes")
  end
  for i = 1, #rest do
    local b = string.byte(rest, i)
    if not __tol_is_hex_char_byte(b) then
      error("abi.decode typed input contains non-hex character")
    end
  end
  return string.lower(rest)
end

local function __tol_all_zero_hex(s)
  for i = 1, #s do
    if string.sub(s, i, i) ~= "0" then
      return false
    end
  end
  return true
end

function __tol_abi_encode_slot(v, typ)
  if typ == "bool" then
    if v then
      return string.rep("0", 63) .. "1"
    end
    return string.rep("0", 64)
  end
  if typ == "address" then
    local hex = "0"
    if type(v) == "string" and string.sub(v, 1, 2) == "0x" then
      hex = string.sub(v, 3)
    elseif type(v) == "string" then
      hex = v
    end
    local pad = 64 - #hex
    if pad < 0 then pad = 0 end
    return string.rep("0", pad) .. string.lower(hex)
  end
  local n = 0
  if type(v) == "uint256" then
    n = v
  elseif type(v) == "string" then
    n = tonumber(v) or 0
  end
  local hex = string.format("%x", math.floor(n))
  local pad = 64 - #hex
  if pad < 0 then pad = 0 end
  return string.rep("0", pad) .. hex
end

function __tol_abi_encode_value(v, typ)
  if __tol_struct_defs and __tol_struct_defs[typ] then
    local fields = __tol_struct_defs[typ]
    local result = ""
    for _, f in ipairs(fields) do
      result = result .. __tol_abi_encode_slot(v[f.name], f.type)
    end
    return "0x" .. result
  end
  -- Dynamic array: typ ends with "[]"
  local base_type_dyn = string.match(typ, "^(.+)%[%]$")
  if base_type_dyn and type(v) == "table" then
    local n = #v
    local len_hex = string.format("%064x", n)
    local elems = ""
    for i = 1, n do
      elems = elems .. __tol_abi_encode_slot(v[i], base_type_dyn)
    end
    return "0x" .. len_hex .. elems
  end
  -- Fixed array: typ matches "T[N]"
  local base_type_fix = string.match(typ, "^(.+)%[%d+%]$")
  if base_type_fix and type(v) == "table" then
    local result = ""
    for i = 1, #v do
      result = result .. __tol_abi_encode_slot(v[i], base_type_fix)
    end
    return "0x" .. result
  end
  return "0x" .. __tol_abi_encode_slot(v, typ)
end

-- =============================================================================
-- Proper Solidity ABI head/tail encoding (EIP-712 layout).
-- =============================================================================

-- Returns true if a TOL/ABI type string is dynamic (requires offset pointer in head).
local function __tol_abi_is_dynamic(typ)
  if typ == "bytes" or typ == "string" then return true end
  -- T[]: dynamic array
  if string.match(typ, "%[%]$") then return true end
  -- Struct with any dynamic field
  if __tol_struct_defs and __tol_struct_defs[typ] then
    local fields = __tol_struct_defs[typ]
    for _, f in ipairs(fields) do
      if __tol_abi_is_dynamic(f.type) then return true end
    end
  end
  return false
end

-- Encode one static value to a 32-byte (64 hex char) ABI slot.
-- Uses __tol_enc (Go host fn) for correct 256-bit big-endian encoding.
local function __tol_abi_slot_static(v, typ)
  if typ == "bool" then
    if v then return string.rep("0", 63) .. "1" end
    return string.rep("0", 64)
  end
  -- bytesN: left-aligned (data in high bytes, right-padded with zeros)
  if string.sub(typ, 1, 5) == "bytes" and #typ > 5 then
    local n = tonumber(string.sub(typ, 6)) or 0
    local hex_val = ""
    if type(v) == "string" and string.sub(v, 1, 2) == "0x" then
      hex_val = string.lower(string.sub(v, 3))
    end
    -- Truncate or pad data to n bytes (n*2 hex chars)
    local data = string.sub(hex_val, 1, n * 2)
    local pad_data = n * 2 - #data
    if pad_data > 0 then data = data .. string.rep("0", pad_data) end
    -- Right-pad to 64 hex chars
    return data .. string.rep("0", 64 - #data)
  end
  -- For boolean values without explicit type
  if type(v) == "boolean" then
    if v then return string.rep("0", 63) .. "1" end
    return string.rep("0", 64)
  end
  -- For string values: if 0x-prefixed hex, right-align; otherwise text-encode
  if type(v) == "string" then
    if string.sub(v, 1, 2) == "0x" then
      -- Hex string (address, bytesN, or bytes-as-value): right-align in 32 bytes
      local hex = string.lower(string.sub(v, 3))
      if #hex > 64 then hex = string.sub(hex, #hex - 63) end
      local pad = 64 - #hex
      return string.rep("0", pad) .. hex
    else
      -- Plain string: text-encode and right-align (should rarely happen here)
      local hex = ""
      for i = 1, #v do
        hex = hex .. string.format("%02x", string.byte(v, i))
      end
      if #hex > 64 then hex = string.sub(hex, #hex - 63) end
      local pad = 64 - #hex
      return string.rep("0", pad) .. hex
    end
  end
  -- address, uN, iN, uint256: right-aligned; use __tol_enc for full 256-bit correctness
  local encoded = __tol_enc(v)  -- 64-char hex string (no 0x prefix)
  return encoded
end

-- Encode the tail data for one dynamic value (no offset pointer, no 0x prefix).
-- For bytes/string: 32-byte length + data + zero-padding to 32-byte boundary.
-- For T[]: 32-byte length + element slots.
local function __tol_abi_tail_dynamic(v, typ)
  if typ == "bytes" then
    local hex = ""
    if type(v) == "string" then
      if string.sub(v, 1, 2) == "0x" then
        hex = string.lower(string.sub(v, 3))
      else
        -- plain string treated as UTF-8 bytes
        for i = 1, #v do
          hex = hex .. string.format("%02x", string.byte(v, i))
        end
      end
    end
    local byte_len = #hex / 2
    local len_slot = string.format("%064x", byte_len)
    -- Zero-pad data to 32-byte boundary
    local pad = (32 - (byte_len % 32)) % 32
    return len_slot .. hex .. string.rep("0", pad * 2)
  end
  if typ == "string" then
    local hex = ""
    if type(v) == "string" then
      if string.sub(v, 1, 2) == "0x" then
        hex = string.lower(string.sub(v, 3))
      else
        for i = 1, #v do
          hex = hex .. string.format("%02x", string.byte(v, i))
        end
      end
    end
    local byte_len = #hex / 2
    local len_slot = string.format("%064x", byte_len)
    local pad = (32 - (byte_len % 32)) % 32
    return len_slot .. hex .. string.rep("0", pad * 2)
  end
  -- T[]: dynamic array
  local base_type = string.match(typ, "^(.+)%[%]$")
  if base_type and type(v) == "table" then
    local n = #v
    local len_slot = string.format("%064x", n)
    local elems = ""
    for i = 1, n do
      elems = elems .. __tol_abi_slot_static(v[i], base_type)
    end
    return len_slot .. elems
  end
  return ""
end

-- __tol_abi_encode_v2(v1, typ1, v2, typ2, ...) -> 0x-prefixed proper ABI encoding.
-- Implements Solidity ABI head/tail layout: static types inline, dynamic via offset pointer.
function __tol_abi_encode_v2(...)
  local args = {...}
  local nargs = #args / 2
  -- Compute head size (nargs * 32 bytes) for initial tail offset.
  local head_bytes = nargs * 32
  local head = ""
  local tail = ""
  local cur_tail_bytes = 0
  for i = 1, nargs do
    local v = args[i * 2 - 1]
    local typ = args[i * 2]
    if __tol_abi_is_dynamic(typ) then
      local offset = head_bytes + cur_tail_bytes
      head = head .. string.format("%064x", offset)
      local td = __tol_abi_tail_dynamic(v, typ)
      tail = tail .. td
      cur_tail_bytes = cur_tail_bytes + #td / 2
    else
      head = head .. __tol_abi_slot_static(v, typ)
    end
  end
  return "0x" .. head .. tail
end

-- __tol_abi_encode_packed_v2(v1, typ1, v2, typ2, ...) -> 0x-prefixed tight-packed encoding.
-- Implements Solidity abi.encodePacked: no offset pointers, minimal padding.
function __tol_abi_encode_packed_v2(...)
  local args = {...}
  local nargs = #args / 2
  local out = ""
  for i = 1, nargs do
    local v = args[i * 2 - 1]
    local typ = args[i * 2]
    if typ == "bytes" then
      -- raw bytes, no length prefix
      if type(v) == "string" and string.sub(v, 1, 2) == "0x" then
        out = out .. string.lower(string.sub(v, 3))
      elseif type(v) == "string" then
        for j = 1, #v do
          out = out .. string.format("%02x", string.byte(v, j))
        end
      end
    elseif typ == "string" then
      -- raw UTF-8 bytes, no length prefix
      if type(v) == "string" then
        if string.sub(v, 1, 2) == "0x" then
          out = out .. string.lower(string.sub(v, 3))
        else
          for j = 1, #v do
            out = out .. string.format("%02x", string.byte(v, j))
          end
        end
      end
    elseif string.sub(typ, 1, 5) == "bytes" and #typ > 5 then
      -- bytesN: exactly N bytes, left-aligned, no right-padding in packed mode
      local n = tonumber(string.sub(typ, 6)) or 0
      local hex_val = ""
      if type(v) == "string" and string.sub(v, 1, 2) == "0x" then
        hex_val = string.lower(string.sub(v, 3))
      end
      local data = string.sub(hex_val, 1, n * 2)
      local pad_data = n * 2 - #data
      if pad_data > 0 then data = data .. string.rep("0", pad_data) end
      out = out .. data
    elseif typ == "bool" then
      if v then out = out .. "01" else out = out .. "00" end
    elseif typ == "address" then
      -- 20 bytes right-aligned (strip leading zeros from full 32-byte slot to 20 bytes)
      local slot = __tol_abi_slot_static(v, "address")
      out = out .. string.sub(slot, 25)  -- last 40 hex chars = 20 bytes
    else
      -- uN, iN: use full 32-byte slot (packed for uint256 is still 32 bytes in Solidity)
      out = out .. __tol_abi_slot_static(v, typ)
    end
  end
  return "0x" .. out
end

-- =============================================================================
-- ABI decode: full offset-pointer resolution for dynamic types.
-- =============================================================================

function __tol_abi_decode_typed(data, typ)
  local payload = __tol_hex_payload(data)
  if typ == "bool" then
    return not __tol_all_zero_hex(payload)
  end
  if typ == "address" then
    if #payload ~= 64 then
      error("abi.decode typed address expects 32-byte hex payload")
    end
    return "0x" .. payload
  end
  -- Dynamic bytes: may be called with either:
  --   (a) Full ABI-encoded bytes: [32-byte offset pointer][length][data] — used
  --       when abi.decode(data) is called on a top-level ABI blob, or
  --   (b) Raw tail data: [length][data] — used when called from tuple/struct decode
  --       after already following the offset pointer.
  -- Heuristic: if the first 32 bytes parse as a byte-offset that points within
  -- the remaining payload, treat it as an offset pointer and follow it.
  -- Otherwise, treat the first 32 bytes as the length (raw tail data).
  if typ == "bytes" then
    local total_hex_len = #payload
    local first_hex = string.sub(payload, 1, 64)
    local first_val = tonumber(first_hex, 16) or 0
    -- first_val is in bytes; convert to hex chars for substring indexing
    local offset_chars = first_val * 2
    -- If first_val is a plausible offset (points within payload, multiple of 32),
    -- follow it to find the tail data (length+bytes).
    if first_val % 32 == 0 and offset_chars >= 0 and offset_chars + 64 <= total_hex_len then
      local tail = string.sub(payload, offset_chars + 1)
      local len_hex = string.sub(tail, 1, 64)
      local byte_len = tonumber(len_hex, 16) or 0
      local data_hex = string.sub(tail, 65, 64 + byte_len * 2)
      return "0x" .. data_hex
    end
    -- Raw tail data: first 32 bytes = length.
    local byte_len = first_val
    local data_hex = string.sub(payload, 65, 64 + byte_len * 2)
    return "0x" .. data_hex
  end
  if __tol_struct_defs and __tol_struct_defs[typ] then
    local fields = __tol_struct_defs[typ]
    local nf = #fields
    local result = {}
    local slot_off = 0
    for i = 1, nf do
      local ftyp = fields[i].type
      if __tol_abi_is_dynamic(ftyp) then
        -- Read offset pointer from static area
        local off_hex = string.sub(payload, slot_off * 64 + 1, (slot_off + 1) * 64)
        local off_bytes = tonumber(off_hex, 16) or 0
        local tail_hex = string.sub(payload, off_bytes * 2 + 1)
        result[fields[i].name] = __tol_abi_decode_typed("0x" .. tail_hex, ftyp)
        slot_off = slot_off + 1
      else
        local slot_hex = string.sub(payload, slot_off * 64 + 1, (slot_off + 1) * 64)
        result[fields[i].name] = __tol_abi_decode_typed("0x" .. slot_hex, ftyp)
        slot_off = slot_off + 1
      end
    end
    return result
  end
  -- Dynamic array: typ ends with "[]"
  local base_type_dyn = string.match(typ, "^(.+)%[%]$")
  if base_type_dyn then
    -- payload starts with: 32-byte length, then N * 32-byte elements
    local len_hex = string.sub(payload, 1, 64)
    local n = tonumber(len_hex, 16) or 0
    local result = {}
    for i = 1, n do
      local slot_hex = string.sub(payload, 64 + (i-1)*64 + 1, 64 + i*64)
      result[i] = __tol_abi_decode_typed("0x" .. slot_hex, base_type_dyn)
    end
    return result
  end
  -- Fixed array: typ matches "T[N]"
  local base_type_fix, n_str = string.match(typ, "^(.+)%[(%d+)%]$")
  if base_type_fix then
    local n = tonumber(n_str) or 0
    local result = {}
    for i = 1, n do
      local slot_hex = string.sub(payload, (i-1)*64 + 1, i*64)
      result[i] = __tol_abi_decode_typed("0x" .. slot_hex, base_type_fix)
    end
    return result
  end
  local bytes_n = nil
  if type(typ) == "string" and string.sub(typ, 1, 5) == "bytes" then
    local ns = string.sub(typ, 6)
    if ns ~= "" then
      bytes_n = tonumber(ns)
    end
  end
  if bytes_n ~= nil and bytes_n >= 1 and bytes_n <= 32 and (bytes_n % 1) == 0 then
    local want_chars = bytes_n * 2
    if #payload == want_chars then
      -- Exact match: payload is exactly the bytesN value.
      return "0x" .. payload
    elseif #payload >= 64 then
      -- 32-byte ABI slot: bytesN is left-aligned; take first bytes_n*2 chars.
      return "0x" .. string.sub(payload, 1, want_chars)
    else
      error("abi.decode typed " .. typ .. " expects " .. tostring(bytes_n) .. "-byte hex payload")
    end
  end
  -- uN (unsigned) integers
  local ubits = nil
  if type(typ) == "string" and string.sub(typ, 1, 1) == "u" then
    ubits = tonumber(string.sub(typ, 2))
  end
  if ubits ~= nil and ubits >= 8 and ubits <= 256 and (ubits % 8) == 0 then
    local digits = ubits / 4
    if #payload > digits then
      local hi = string.sub(payload, 1, #payload - digits)
      if not __tol_all_zero_hex(hi) then
        error("abi.decode value overflows target type '" .. typ .. "'")
      end
    end
    local tail = payload
    if #tail > digits then
      tail = string.sub(payload, #payload - digits + 1)
    end
    if tail == "" then return tonumber("0") end
    local n = tonumber(tail, 16)
    if n == nil then
      error("abi.decode typed integer parse failed for type '" .. typ .. "'")
    end
    return n
  end
  -- iN (signed) integers: stored as two's-complement in 32-byte slot, right-aligned.
  -- Decode: extract the lower N bits (as uint256 bit pattern — the VM represents
  -- signed values as their two's-complement bit pattern in uint256).
  local ibits = nil
  if type(typ) == "string" and string.sub(typ, 1, 1) == "i" then
    ibits = tonumber(string.sub(typ, 2))
  end
  if ibits ~= nil and ibits >= 8 and ibits <= 256 and (ibits % 8) == 0 then
    local digits = ibits / 4
    local val_hex = payload
    if #val_hex > digits then
      val_hex = string.sub(payload, #payload - digits + 1)
    end
    if val_hex == "" then val_hex = "0" end
    local n = tonumber(val_hex, 16)
    if n == nil then
      error("abi.decode typed integer parse failed for type '" .. typ .. "'")
    end
    return n
  end
  error("abi.decode typed target '" .. tostring(typ) .. "' is not supported in current stage")
end

function __tol_struct_slot_count(typ)
  -- Dynamic types (bytes, string, T[]): always occupy 1 static slot (offset pointer).
  if typ == "bytes" or typ == "string" then return 1 end
  if string.match(typ, "%[%]$") then
    return 1
  end
  if __tol_struct_defs and __tol_struct_defs[typ] then
    local fields = __tol_struct_defs[typ]
    local count = 0
    for _, f in ipairs(fields) do
      count = count + __tol_struct_slot_count(f.type)
    end
    return count
  end
  return 1
end

function __tol_abi_decode_tuple(data, ...)
  local types = {...}
  local n = #types
  if n == 0 then
    error("abi.decode tuple: no types specified")
  end
  local payload = __tol_hex_payload(data)
  local results = {}
  local slot_idx = 0
  for i = 1, n do
    local typ = types[i]
    -- Dynamic types: read offset pointer from static area, then follow it to tail.
    local is_dyn = __tol_abi_is_dynamic(typ)
    if is_dyn then
      local offset_hex = string.sub(payload, slot_idx*64+1, (slot_idx+1)*64)
      local offset_bytes = tonumber(offset_hex, 16) or 0
      local offset_chars = offset_bytes * 2
      -- The tail data starts at offset_chars into the payload.
      local tail_payload = string.sub(payload, offset_chars + 1)
      results[i] = __tol_abi_decode_typed("0x" .. tail_payload, typ)
      slot_idx = slot_idx + 1
    elseif __tol_struct_defs and __tol_struct_defs[typ] then
      local fields = __tol_struct_defs[typ]
      local nf = #fields
      local struct_hex = string.sub(payload, slot_idx*64+1, (slot_idx+nf)*64)
      results[i] = __tol_abi_decode_typed("0x" .. struct_hex, typ)
      slot_idx = slot_idx + nf
    else
      local slot_hex = string.sub(payload, slot_idx*64+1, (slot_idx+1)*64)
      results[i] = __tol_abi_decode_typed("0x" .. slot_hex, typ)
      slot_idx = slot_idx + 1
    end
  end
  return unpack(results, 1, n)
end
`
	// Build the dynamic struct defs table assignment.
	var sb strings.Builder
	sb.WriteString(staticSrc)
	sb.WriteString("\n-- Generated struct definitions (field name + type, in declaration order)\n")
	sb.WriteString("__tol_struct_defs = {\n")
	for _, sd := range structs {
		if strings.TrimSpace(sd.Name) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  [%q] = {\n", sd.Name))
		for _, f := range sd.Fields {
			fname := strings.TrimSpace(f.Name)
			ftype := normalizeSelectorType(f.Type)
			if fname == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("    {name=%q, type=%q},\n", fname, ftype))
		}
		sb.WriteString("  },\n")
	}
	sb.WriteString("}\n")

	chunk, err := parse.Parse(bytes.NewReader([]byte(sb.String())), "<tol-abi-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build abi prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

func buildHostPrelude() ([]luast.Stmt, error) {
	const src = `
function __tol_emit(...)
  local n = select("#", ...)
  local name = nil
  if n >= 1 then
    name = select(1, ...)
  end
  local sig = nil
  local indexed = nil
  if type(name) == "string" then
    if type(__tol_event_sig) == "table" then
      sig = __tol_event_sig[name]
    end
    if type(__tol_event_indexed) == "table" then
      indexed = __tol_event_indexed[name]
    end
  end
  local f = nil
  if tos ~= nil and type(tos) == "table" and type(tos.emit) == "function" then
    f = tos.emit
  elseif type(emit) == "function" then
    f = emit
  end
  if f == nil then
    error("emit host function is not available")
  end
  local args = {}
  for i = 1, n do
    args[i] = select(i, ...)
  end
  args[n + 1] = sig
  args[n + 2] = indexed
  return f(unpack(args, 1, n + 2))
end

local __tol_zero_addr = "0x0000000000000000000000000000000000000000000000000000000000000000"

function __tol_host_call(addr, value, data)
  local f = nil
  if tos ~= nil and type(tos) == "table" and type(tos.call) == "function" then
    f = tos.call
  elseif type(call) == "function" then
    f = call
  end
  if f ~= nil then
    local ok, ret = f(addr, value, data)
    if ok == nil then ok = false end
    if ret == nil then ret = "0x" end
    return ok, ret
  end
  error("host function 'call' is not available")
end

function __tol_host_staticcall(addr, data)
  local f = nil
  if tos ~= nil and type(tos) == "table" and type(tos.staticcall) == "function" then
    f = tos.staticcall
  elseif type(staticcall) == "function" then
    f = staticcall
  end
  if f ~= nil then
    local ok, ret = f(addr, data)
    if ok == nil then ok = false end
    if ret == nil then ret = "0x" end
    return ok, ret
  end
  error("host function 'staticcall' is not available")
end

function __tol_host_delegatecall(addr, data)
  local f = nil
  if tos ~= nil and type(tos) == "table" and type(tos.delegatecall) == "function" then
    f = tos.delegatecall
  elseif type(delegatecall) == "function" then
    f = delegatecall
  end
  if f ~= nil then
    local ok, ret = f(addr, data)
    if ok == nil then ok = false end
    if ret == nil then ret = "0x" end
    return ok, ret
  end
  error("host function 'delegatecall' is not available")
end

function __tol_host_create(value, init_code)
  local f = nil
  if tos ~= nil and type(tos) == "table" and type(tos.create) == "function" then
    f = tos.create
  elseif type(create) == "function" then
    f = create
  end
  if f ~= nil then
    local addr = f(value, init_code)
    if addr == nil then addr = __tol_zero_addr end
    return addr
  end
  error("host function 'create' is not available")
end

function __tol_host_create2(value, salt, init_code)
  local f = nil
  if tos ~= nil and type(tos) == "table" and type(tos.create2) == "function" then
    f = tos.create2
  elseif type(create2) == "function" then
    f = create2
  end
  if f ~= nil then
    local addr = f(value, salt, init_code)
    if addr == nil then addr = __tol_zero_addr end
    return addr
  end
  error("host function 'create2' is not available")
end

function __tol_host_transfer(addr, amount)
  if tos ~= nil and type(tos) == "table" and type(tos.transfer) == "function" then
    return tos.transfer(addr, amount)
  end
  if type(transfer) == "function" then
    return transfer(addr, amount)
  end
  error("host function 'transfer' is not available")
end

function __tol_host_send(addr, amount)
  if tos ~= nil and type(tos) == "table" and type(tos.send) == "function" then
    return tos.send(addr, amount)
  end
  if type(send) == "function" then
    return send(addr, amount)
  end
  -- send() returns bool; default to false (failure) if not available
  return false
end

function __tol_bytes_eq(a, b)
  return __tol_keccak256(a) == __tol_keccak256(b)
end

function __tol_env_get(scope, key)
  if tos ~= nil and type(tos) == "table" then
    local scoped = tos[scope]
    if type(scoped) == "table" then
      local v = scoped[key]
      if v ~= nil then
        return v
      end
    end
    local flat = tos[scope .. "." .. key]
    if flat ~= nil then
      return flat
    end
  end
  local g = nil
  if scope == "msg" then
    g = msg
  elseif scope == "tx" then
    g = tx
  elseif scope == "block" then
    g = block
  end
  if type(g) == "table" then
    local v = g[key]
    if v ~= nil then
      return v
    end
  end
  return nil
end

function __tol_gas_left()
  if tos ~= nil and type(tos) == "table" then
    if type(tos.gas_left) == "function" then
      return tos.gas_left()
    end
    if type(tos.gasleft) == "function" then
      return tos.gasleft()
    end
    if type(tos.gas) == "table" then
      local gv = tos.gas.left
      if type(gv) == "function" then
        return gv()
      end
      if gv ~= nil then
        return gv
      end
    end
    local flat = tos["gas.left"]
    if type(flat) == "function" then
      return flat()
    end
    if flat ~= nil then
      return flat
    end
  end
  if type(gas_left) == "function" then
    return gas_left()
  end
  if type(gas) == "table" then
    local gv = gas.left
    if type(gv) == "function" then
      return gv()
    end
    if gv ~= nil then
      return gv
    end
  end
  error("gas.left host function is not available")
end

local function __tol_is_hex_char_byte(b)
  local is_digit = (b >= string.byte("0") and b <= string.byte("9"))
  local is_lower = (b >= string.byte("a") and b <= string.byte("f"))
  local is_upper = (b >= string.byte("A") and b <= string.byte("F"))
  return is_digit or is_lower or is_upper
end

local function __tol_bytes_to_hex(v)
  local s = ""
  if type(v) == "string" then
    s = v
  else
    s = tostring(v)
  end
  if string.sub(s, 1, 2) == "0x" then
    local rest = string.sub(s, 3)
    if (#rest % 2) ~= 0 then
      error("hex bytes must have even length")
    end
    for i = 1, #rest do
      local b = string.byte(rest, i)
      if not __tol_is_hex_char_byte(b) then
        error("hex bytes contains non-hex character")
      end
    end
    return string.lower(s)
  end
  local out = "0x"
  for i = 1, #s do
    out = out .. string.format("%02x", string.byte(s, i))
  end
  return out
end

function __tol_keccak256(data)
  if tos ~= nil and type(tos) == "table" and type(tos.keccak256) == "function" then
    return tos.keccak256(data)
  end
  if type(keccak256) == "function" then
    return keccak256(__tol_bytes_to_hex(data))
  end
  error("host function 'keccak256' is not available")
end

function __tol_host_package_call(addr, contractName, selector, data)
  -- selector: "0xXXXXXXXX" (4-byte selector as 8 hex chars after 0x prefix)
  -- data: "0x..." ABI-encoded args, or "0x" if none
  -- Builds calldata = selector_bytes ++ data_bytes, then delegates to tos.package_call
  -- which prepends the dispatch tag keccak256("pkg:"+contractName)[:4]
  local sel_hex = (selector or "0x"):sub(3)
  local dat_hex = (data or "0x"):sub(3)
  local calldata = "0x" .. sel_hex .. dat_hex
  if tos ~= nil and type(tos) == "table" and type(tos.package_call) == "function" then
    return tos.package_call(addr, contractName, calldata)
  end
  error("host function 'package_call' is not available")
end

function __tol_host_iface_call(addr, selector, data)
  -- selector: "0xXXXXXXXX" (4-byte selector as 8 hex chars after 0x prefix)
  -- data: "0x..." ABI-encoded args, or "0x" if none
  -- Builds calldata = selector_bytes ++ data_bytes and performs an external call
  -- with value=0. Used by compiled interface variable method calls.
  local calldata = "0x" .. (selector or "0x"):sub(3) .. (data or "0x"):sub(3)
  return __tol_host_call(addr, 0, calldata)
end

function __tol_sha256(data)
  if tos ~= nil and type(tos) == "table" and type(tos.sha256) == "function" then
    return tos.sha256(data)
  end
  if type(sha256) == "function" then
    return sha256(__tol_bytes_to_hex(data))
  end
  error("host function 'sha256' is not available")
end

function __tol_ripemd160(data)
  if tos ~= nil and type(tos) == "table" and type(tos.ripemd160) == "function" then
    return tos.ripemd160(data)
  end
  if type(ripemd160) == "function" then
    return ripemd160(__tol_bytes_to_hex(data))
  end
  error("host function 'ripemd160' is not available")
end

function __tol_ecrecover(hash, v, r, s)
  if tos ~= nil and type(tos) == "table" and type(tos.ecrecover) == "function" then
    return tos.ecrecover(hash, v, r, s)
  end
  if type(ecrecover) == "function" then
    return ecrecover(hash, v, r, s)
  end
  error("host function 'ecrecover' is not available")
end

-- __tol_new_array(size, default_val) -> memory array table.
-- Creates a fixed-size ephemeral array with _size=size and elements [0..size-1]
-- all initialised to default_val (nil = use 0).
function __tol_new_array(size, default_val)
  if default_val == nil then default_val = 0 end
  local n = size
  if type(n) ~= "number" then
    -- LUint256 may have been passed; fall back to lo field.
    if type(n) == "table" and n.lo ~= nil then n = n.lo end
  end
  local arr = {}
  arr._size = size
  for i = 0, n - 1 do
    arr[i] = default_val
  end
  return arr
end

-- __tol_array_get(arr, idx) -> element at 0-based index idx (bounds-checked).
function __tol_array_get(arr, idx)
  local sz = arr._size
  local n = sz
  if type(n) ~= "number" then
    if type(n) == "table" and n.lo ~= nil then n = n.lo end
  end
  local i = idx
  if type(i) ~= "number" then
    if type(i) == "table" and i.lo ~= nil then i = i.lo end
  end
  if i < 0 or i >= n then
    error("array index out of bounds: " .. tostring(i) .. " >= " .. tostring(n))
  end
  local v = arr[i]
  if v == nil then return 0 end
  return v
end

-- __tol_array_set(arr, idx, val) -> sets element at 0-based index idx (bounds-checked).
function __tol_array_set(arr, idx, val)
  local sz = arr._size
  local n = sz
  if type(n) ~= "number" then
    if type(n) == "table" and n.lo ~= nil then n = n.lo end
  end
  local i = idx
  if type(i) ~= "number" then
    if type(i) == "table" and i.lo ~= nil then i = i.lo end
  end
  if i < 0 or i >= n then
    error("array index out of bounds: " .. tostring(i) .. " >= " .. tostring(n))
  end
  arr[i] = val
end
`
	chunk, err := parse.Parse(bytes.NewReader([]byte(src)), "<tol-host-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build host prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

func buildEventPreludeFromLowered(events []lower.Event) ([]luast.Stmt, error) {
	if len(events) == 0 {
		return []luast.Stmt{}, nil
	}
	var sb strings.Builder
	sb.WriteString("__tol_event_sig = __tol_event_sig or {}\n")
	sb.WriteString("__tol_event_indexed = __tol_event_indexed or {}\n")
	for _, ev := range events {
		name := strings.TrimSpace(ev.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] event name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		sig, mask, err := eventSignatureAndIndexedMask(ev)
		if err != nil {
			return nil, err
		}
		sb.WriteString(fmt.Sprintf("__tol_event_sig[%q] = %q\n", name, sig))
		sb.WriteString(fmt.Sprintf("__tol_event_indexed[%q] = %q\n", name, mask))
	}
	chunk, err := parse.Parse(bytes.NewReader([]byte(sb.String())), "<tol-event-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build event prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

func eventSignatureAndIndexedMask(ev lower.Event) (string, string, error) {
	name := strings.TrimSpace(ev.Name)
	if name == "" {
		return "", "", fmt.Errorf("[%s] event name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	types := make([]string, 0, len(ev.Params))
	var mask strings.Builder
	for _, p := range ev.Params {
		t := normalizeSelectorType(p.Type)
		if t == "" {
			return "", "", fmt.Errorf("[%s] event parameter type cannot be empty for '%s'", diag.CodeLowerUnsupportedFeature, name)
		}
		types = append(types, t)
		if p.Indexed {
			mask.WriteByte('1')
		} else {
			mask.WriteByte('0')
		}
	}
	return fmt.Sprintf("%s(%s)", name, strings.Join(types, ",")), mask.String(), nil
}

func collectDispatchFuncs(funcs []lower.Function) ([]dispatchFunc, error) {
	out := make([]dispatchFunc, 0, len(funcs))
	for _, fn := range funcs {
		visibility, err := classifyDirectIRFnModifiers(fn.Modifiers)
		if err != nil {
			return nil, err
		}
		if visibility != "public" && visibility != "external" {
			continue
		}
		sig, err := dispatchSelectorForFunction(fn)
		if err != nil {
			return nil, err
		}
		luaName := fn.LuaName
		if luaName == "" {
			luaName = fn.Name
		}
		out = append(out, dispatchFunc{
			Name:      fn.Name,
			LuaName:   luaName,
			Signature: sig,
			Params:    fn.Params,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Signature == out[j].Signature {
			return out[i].Name < out[j].Name
		}
		return out[i].Signature < out[j].Signature
	})
	for i := 1; i < len(out); i++ {
		if out[i-1].Signature == out[i].Signature {
			return nil, fmt.Errorf("[%s] duplicate dispatch selector signature '%s'", diag.CodeLowerUnsupportedFeature, out[i].Signature)
		}
	}
	return out, nil
}

func dispatchSelectorForFunction(fn lower.Function) (string, error) {
	if strings.TrimSpace(fn.SelectorOverride) != "" {
		return strings.ToLower(strings.TrimSpace(fn.SelectorOverride)), nil
	}
	sig, err := selectorSignatureForFunction(fn)
	if err != nil {
		return "", err
	}
	return selectorHexFromSignature(sig), nil
}

func selectorHexFromSignature(sig string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte(sig))
	sum := h.Sum(nil)
	return "0x" + hex.EncodeToString(sum[:4])
}

func isCanonicalSelectorSignature(sig string) bool {
	if strings.TrimSpace(sig) != sig || sig == "" {
		return false
	}
	open := strings.Index(sig, "(")
	close := strings.LastIndex(sig, ")")
	if !(open > 0 && close == len(sig)-1 && open < close) {
		return false
	}
	rawName := sig[:open]
	if strings.TrimSpace(rawName) != rawName || !isValidSelectorFunctionName(rawName) {
		return false
	}
	args := strings.TrimSpace(sig[open+1 : close])
	if args == "" {
		return true
	}
	for _, p := range strings.Split(args, ",") {
		token := strings.TrimSpace(p)
		if token == "" || token != p || strings.ContainsAny(token, " \t\r\n") {
			return false
		}
	}
	return true
}

func isValidSelectorFunctionName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return false
			}
			continue
		}
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func isHexSelectorLiteral(sel string) bool {
	if strings.TrimSpace(sel) != sel {
		return false
	}
	if len(sel) != 10 || !strings.HasPrefix(sel, "0x") {
		return false
	}
	for i := 2; i < len(sel); i++ {
		c := sel[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !(isDigit || isLower || isUpper) {
			return false
		}
	}
	return true
}

func isEvenHexBytesLiteral(s string) bool {
	if strings.TrimSpace(s) != s {
		return false
	}
	if len(s) < 2 || !strings.HasPrefix(s, "0x") {
		return false
	}
	hexPart := s[2:]
	if len(hexPart)%2 != 0 {
		return false
	}
	for i := 0; i < len(hexPart); i++ {
		c := hexPart[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !(isDigit || isLower || isUpper) {
			return false
		}
	}
	return true
}

func buildTosInitStmt() luast.Stmt {
	return withLineStmt(&luast.AssignStmt{
		Lhs: []luast.Expr{
			withLineExpr(&luast.IdentExpr{Value: "tos"}),
		},
		Rhs: []luast.Expr{
			withLineExpr(&luast.LogicalOpExpr{
				Operator: "or",
				Lhs:      withLineExpr(&luast.IdentExpr{Value: "tos"}),
				Rhs:      withLineExpr(&luast.TableExpr{Fields: []*luast.Field{}}),
			}),
		},
	}, 1)
}

func buildOnCreateAssignStmt() luast.Stmt {
	call := withLineExpr(&luast.FuncCallExpr{
		Func: withLineExpr(&luast.IdentExpr{Value: "__tol_constructor"}),
		Args: []luast.Expr{
			withLineExpr(&luast.Comma3Expr{AdjustRet: false}),
		},
		// Keep return arity unchanged when constructor returns values.
		AdjustRet: false,
	})
	fn := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: true,
			Names:    []string{},
		},
		Stmts: []luast.Stmt{
			withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}, 1),
		},
	})
	return withLineStmt(&luast.AssignStmt{
		Lhs: []luast.Expr{
			withLineExpr(&luast.AttrGetExpr{
				Object: withLineExpr(&luast.IdentExpr{Value: "tos"}),
				Key:    withLineExpr(&luast.StringExpr{Value: "oncreate"}),
			}),
		},
		Rhs: []luast.Expr{fn},
	}, 1)
}

// buildDirectConstructorCallStmt emits a top-level call to __tol_constructor() with
// no arguments.  The init_code reads constructor args via tos.calldata (set by Execute()
// before module load), so no varargs forwarding is needed.
func buildDirectConstructorCallStmt() luast.Stmt {
	return withLineStmt(&luast.FuncCallStmt{
		Expr: withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_constructor"}),
			Args:      []luast.Expr{},
			AdjustRet: false,
		}),
	}, 1)
}

// fnParamsHaveStructType returns true if any of the function's parameters is a struct type
// (i.e., its type name exists in structFields).
func fnParamsHaveStructType(params []tolast.FieldDecl, structFields map[string][]string) bool {
	for _, p := range params {
		typ := normalizeSelectorType(p.Type)
		if _, isStruct := structFields[typ]; isStruct {
			return true
		}
		// Array types (dynamic T[] or fixed T[N]) also require calldata decode path.
		if strings.Contains(typ, "[") {
			return true
		}
	}
	return false
}

func buildOnInvokeAssignStmt(dispatchFns []dispatchFunc, hasFallback bool, hasReceive bool, env *loweringEnv) luast.Stmt {
	body := make([]luast.Stmt, 0, len(dispatchFns)+3)
	// If receive() is declared, dispatch to it when selector is nil (empty calldata).
	if hasReceive {
		cond := withLineExpr(&luast.RelationalOpExpr{
			Operator: "==",
			Lhs:      withLineExpr(&luast.IdentExpr{Value: "selector"}),
			Rhs:      withLineExpr(&luast.NilExpr{}),
		})
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_receive"}),
			Args:      []luast.Expr{},
			AdjustRet: false,
		})
		body = append(body, withLineStmt(&luast.IfStmt{
			Condition: cond,
			Then: []luast.Stmt{
				withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}, 1),
			},
			Else: []luast.Stmt{},
		}, 1))
	}
	for _, fn := range dispatchFns {
		cond := withLineExpr(&luast.RelationalOpExpr{
			Operator: "==",
			Lhs:      withLineExpr(&luast.IdentExpr{Value: "selector"}),
			Rhs:      withLineExpr(&luast.StringExpr{Value: fn.Signature}),
		})

		var branchStmts []luast.Stmt
		if len(fn.Params) == 0 {
			// No parameters: call directly with no args.
			call := withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: fn.LuaName}),
				Args:      []luast.Expr{},
				AdjustRet: false,
			})
			branchStmts = []luast.Stmt{
				withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}, 1),
			}
		} else {
			// All functions with parameters decode args from tos.calldata.
			// This is the single deterministic strategy: VM calls oninvoke(selector)
			// only; args are always decoded from the ABI-encoded calldata.
			typeArgs := make([]string, len(fn.Params))
			paramNames := make([]string, len(fn.Params))
			for i, p := range fn.Params {
				typeArgs[i] = fmt.Sprintf("%q", normalizeSelectorType(p.Type))
				paramNames[i] = fmt.Sprintf("__tol_dp_%d", i)
			}
			lhsNames := strings.Join(paramNames, ", ")
			callArgs := "__tol_cd, " + strings.Join(typeArgs, ", ")
			fnCallArgs := strings.Join(paramNames, ", ")

			branchSrc := fmt.Sprintf(`
do
  local __tol_cd = tos.calldata
  if __tol_cd ~= nil and type(__tol_cd) == "string" and #__tol_cd > 10 then
    -- skip "0x" prefix (2 chars) + 4-byte selector (8 hex chars) = 10 chars
    __tol_cd = "0x" .. string.sub(__tol_cd, 11)
    local %s = __tol_abi_decode_tuple(%s)
    return %s(%s)
  end
  return %s(...)
end
`, lhsNames, callArgs, fn.LuaName, fnCallArgs, fn.LuaName)

			var parseErr error
			branchStmts, parseErr = parse.Parse(bytes.NewReader([]byte(branchSrc)), "<tol-dispatch>")
			if parseErr != nil {
				// Fallback: no-params call on parse error (should never happen).
				call := withLineExpr(&luast.FuncCallExpr{
					Func:      withLineExpr(&luast.IdentExpr{Value: fn.LuaName}),
					Args:      []luast.Expr{},
					AdjustRet: false,
				})
				branchStmts = []luast.Stmt{
					withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}, 1),
				}
			}
		}

		body = append(body, withLineStmt(&luast.IfStmt{
			Condition: cond,
			Then:      branchStmts,
			Else:      []luast.Stmt{},
		}, 1))
	}
	if hasFallback {
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_fallback"}),
			Args:      []luast.Expr{},
			AdjustRet: false,
		})
		body = append(body, withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}, 1))
	} else {
		body = append(body, withLineStmt(&luast.FuncCallStmt{
			Expr: withLineExpr(&luast.FuncCallExpr{
				Func: withLineExpr(&luast.IdentExpr{Value: "error"}),
				Args: []luast.Expr{
					withLineExpr(&luast.StringExpr{Value: "UNKNOWN_SELECTOR"}),
				},
				AdjustRet: true,
			}),
		}, 1))
	}
	fn := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: true,
			Names:    []string{"selector"},
		},
		Stmts: body,
	})
	return withLineStmt(&luast.AssignStmt{
		Lhs: []luast.Expr{
			withLineExpr(&luast.AttrGetExpr{
				Object: withLineExpr(&luast.IdentExpr{Value: "tos"}),
				Key:    withLineExpr(&luast.StringExpr{Value: "oninvoke"}),
			}),
		},
		Rhs: []luast.Expr{fn},
	}, 1)
}

func selectorSignatureForFunction(fn lower.Function) (string, error) {
	if strings.TrimSpace(fn.Name) == "" {
		return "", fmt.Errorf("[%s] function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	types := make([]string, 0, len(fn.Params))
	for _, p := range fn.Params {
		t := normalizeSelectorType(p.Type)
		if t == "" {
			return "", fmt.Errorf("[%s] function parameter type cannot be empty for '%s'", diag.CodeLowerUnsupportedFeature, fn.Name)
		}
		types = append(types, t)
	}
	return fmt.Sprintf("%s(%s)", fn.Name, strings.Join(types, ",")), nil
}

// normalizeSelectorType canonicalises a type name string by collapsing
// whitespace and removing spaces around punctuation using the same rules as
// the parser's joinTypeTokens: no space before/after [ ( , ].
func normalizeSelectorType(t string) string {
	tokens := strings.Fields(t)
	if len(tokens) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(tokens[0])
	for i := 1; i < len(tokens); i++ {
		prev, cur := tokens[i-1], tokens[i]
		noSpaceBefore := cur == "[" || cur == "]" || cur == "(" || cur == ")" || cur == ","
		noSpaceAfter := prev == "[" || prev == "(" || prev == ","
		if !noSpaceBefore && !noSpaceAfter {
			b.WriteByte(' ')
		}
		b.WriteString(cur)
	}
	result := b.String()
	// "address payable" and "address" have identical ABI and runtime representation.
	if result == "address payable" {
		result = "address"
	}
	return result
}

func defaultValueExprForType(typeName string) (luast.Expr, bool) {
	t := strings.TrimSpace(typeName)
	switch t {
	case "bool":
		return withLineExpr(&luast.FalseExpr{}), true
	case "address":
		return withLineExpr(&luast.StringExpr{Value: "0x" + strings.Repeat("0", 64)}), true
	case "string":
		return withLineExpr(&luast.StringExpr{Value: ""}), true
	case "bytes":
		return withLineExpr(&luast.StringExpr{Value: "0x"}), true
	}
	if strings.HasPrefix(t, "bytes") {
		nStr := t[len("bytes"):]
		if n, err := strconv.Atoi(nStr); err == nil && n >= 1 && n <= 32 {
			return withLineExpr(&luast.StringExpr{Value: "0x" + strings.Repeat("0", n*2)}), true
		}
	}
	if len(t) >= 2 && (t[0] == 'u' || t[0] == 'i') {
		if n, err := strconv.Atoi(t[1:]); err == nil && n >= 8 && n <= 256 && n%8 == 0 {
			return withLineExpr(&luast.NumberExpr{Value: "0"}), true
		}
	}
	return nil, false
}

// defaultValueExprForTypeWithStructs extends defaultValueExprForType with
// support for named struct types.  When typeName matches a known struct, it
// returns a Lua table constructor whose fields are initialised to their own
// zero values (recursively).  The visited set prevents infinite recursion for
// pathological self-referential structs.
func defaultValueExprForTypeWithStructs(typeName string, structFieldTypes map[string][]tolast.FieldDecl, visited map[string]bool) (luast.Expr, bool) {
	// Try the scalar defaults first.
	if expr, ok := defaultValueExprForType(typeName); ok {
		return expr, true
	}
	t := strings.TrimSpace(typeName)
	fields, ok := structFieldTypes[t]
	if !ok {
		return nil, false
	}
	// Guard against self-referential structs (shouldn't occur in well-typed TOL,
	// but be safe anyway).
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[t] {
		// Fallback: return an empty table.
		return withLineExpr(&luast.TableExpr{Fields: []*luast.Field{}}), true
	}
	visited[t] = true
	defer delete(visited, t)

	luaFields := make([]*luast.Field, 0, len(fields))
	for _, f := range fields {
		fname := strings.TrimSpace(f.Name)
		ftype := normalizeSelectorType(f.Type)
		var valExpr luast.Expr
		if ve, ok2 := defaultValueExprForTypeWithStructs(ftype, structFieldTypes, visited); ok2 {
			valExpr = ve
		} else {
			// Unknown field type — use numeric 0 as a safe fallback.
			valExpr = withLineExpr(&luast.NumberExpr{Value: "0"})
		}
		luaFields = append(luaFields, &luast.Field{
			Key:   withLineExpr(&luast.StringExpr{Value: fname}),
			Value: valExpr,
		})
	}
	return withLineExpr(&luast.TableExpr{Fields: luaFields}), true
}

// buildRequiresCapPreamble emits Lua guard statements for @requires(caller: X) annotations.
// For each capability name, emits:
//
//	if not (tos and type(tos.hascapability)=="function" and tos.hascapability(msg.sender, __tol_cap_X)) then
//	  error("CapabilityDenied:X")
//	end
func buildRequiresCapPreamble(caps []string) ([]luast.Stmt, error) {
	var sb strings.Builder
	for _, cap := range caps {
		luaCapVar := "__tol_cap_" + cap
		sb.WriteString(fmt.Sprintf(
			"if not (tos and type(tos.hascapability)==\"function\" and tos.hascapability(msg.sender, %s)) then error(%q) end\n",
			luaCapVar, "CapabilityDenied:"+cap,
		))
	}
	if sb.Len() == 0 {
		return nil, nil
	}
	stmts, err := parse.Parse(bytes.NewReader([]byte(sb.String())), "<tol-requires-cap-preamble>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build @requires preamble: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return stmts, nil
}

func lowerFunctionToLua(fn lower.Function, env *loweringEnv) (luast.Stmt, error) {
	if strings.TrimSpace(fn.Name) == "" {
		return nil, fmt.Errorf("[%s] function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	if _, err := classifyDirectIRFnModifiers(fn.Modifiers); err != nil {
		return nil, err
	}

	parNames := make([]string, 0, len(fn.Params))
	for _, p := range fn.Params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] function parameter name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		parNames = append(parNames, name)
	}

	ctx := newLoweringCtx(env)
	for i, name := range parNames {
		ctx.declareLocalWithType(name, normalizeSelectorType(fn.Params[i].Type))
	}
	body, err := tolStmtsToLuaWithCtx(ctx, fn.Body)
	if err != nil {
		return nil, err
	}

	// Inject agent-native preamble guards based on doc annotations.
	if fn.Doc != nil && len(fn.Doc.RequiresCap) > 0 {
		preamble, pErr := buildRequiresCapPreamble(fn.Doc.RequiresCap)
		if pErr != nil {
			return nil, pErr
		}
		body = append(preamble, body...)
	}

	// Use LuaName if set, otherwise fall back to Name.
	luaFuncName := fn.LuaName
	if luaFuncName == "" {
		luaFuncName = fn.Name
	}
	nameExpr := withLineExpr(&luast.IdentExpr{Value: luaFuncName})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    parNames,
		},
		Stmts: body,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}, 1), nil
}

// libraryFuncLuaName returns the Lua global name for a library function.
// Library functions are emitted as global Lua functions with this prefixed name
// to avoid colliding with contract functions.
func libraryFuncLuaName(libName, fnName string) string {
	return "__tol_lib_" + libName + "_" + fnName
}

// lowerLibraryFunctionToLua lowers a single library function to a Lua function definition.
// Library functions have no storage context (__tol_storage is NOT injected).
// The function is emitted under the name "__tol_lib_<LibName>_<fnName>".
func lowerLibraryFunctionToLua(libName string, fn lower.Function, env *loweringEnv) (luast.Stmt, error) {
	if strings.TrimSpace(fn.Name) == "" {
		return nil, fmt.Errorf("[%s] library function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}

	parNames := make([]string, 0, len(fn.Params))
	for _, p := range fn.Params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] library function parameter name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		parNames = append(parNames, name)
	}

	// Library functions use the same lowering environment but without storage writes being
	// meaningful (libraries in TOL spec are stateless). We create a normal ctx.
	ctx := newLoweringCtx(env)
	for i, name := range parNames {
		ctx.declareLocalWithType(name, normalizeSelectorType(fn.Params[i].Type))
	}
	body, err := tolStmtsToLuaWithCtx(ctx, fn.Body)
	if err != nil {
		return nil, err
	}

	luaName := libraryFuncLuaName(libName, fn.Name)
	nameExpr := withLineExpr(&luast.IdentExpr{Value: luaName})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    parNames,
		},
		Stmts: body,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}, 1), nil
}

func lowerConstructorToLua(params []tolast.FieldDecl, body []tolast.Statement, env *loweringEnv) (luast.Stmt, error) {
	parNames := make([]string, 0, len(params))
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] constructor parameter name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		parNames = append(parNames, name)
	}

	ctx := newLoweringCtx(env)
	for i, name := range parNames {
		ctx.declareLocalWithType(name, normalizeSelectorType(params[i].Type))
	}
	stmts, err := tolStmtsToLuaWithCtx(ctx, body)
	if err != nil {
		return nil, err
	}

	// If the constructor has parameters, prepend an ABI calldata decode guard.
	// When tos.calldata is a non-empty 0x-prefixed hex string (the on-chain
	// deploy path), constructor arguments are decoded from ABI-encoded calldata
	// rather than being received as direct Lua function arguments (test path).
	if len(params) > 0 {
		guardStmts, gErr := buildConstructorABIDecodeGuard(parNames, params)
		if gErr != nil {
			return nil, gErr
		}
		stmts = append(guardStmts, stmts...)
	}

	nameExpr := withLineExpr(&luast.IdentExpr{Value: "__tol_constructor"})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    parNames,
		},
		Stmts: stmts,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}, 1), nil
}

// buildConstructorABIDecodeGuard generates the Lua statements that decode
// constructor parameters from tos.calldata when it is available.
//
// The generated code checks whether tos.calldata is a non-empty 0x-prefixed hex
// string. If so, the parameter locals are reassigned by calling
// __tol_abi_decode_tuple, which decodes each parameter (including struct types
// that expand to multiple 32-byte slots) from the ABI-encoded calldata.
// When tos.calldata is absent or empty the parameters retain whatever values
// were passed as direct Lua function arguments (test-runner path).
//
// Example output for constructor(supply: u256, owner: address):
//
//	if tos ~= nil and type(tos.calldata) == "string" and #tos.calldata > 2 then
//	  supply, owner = __tol_abi_decode_tuple(tos.calldata, "u256", "address")
//	end
//
// Example output for constructor(p: Point, n: u256) where Point={x,y}:
//
//	if tos ~= nil and type(tos.calldata) == "string" and #tos.calldata > 2 then
//	  p, n = __tol_abi_decode_tuple(tos.calldata, "Point", "u256")
//	end
func buildConstructorABIDecodeGuard(parNames []string, params []tolast.FieldDecl) ([]luast.Stmt, error) {
	typeArgs := make([]string, len(params))
	for i, p := range params {
		typeArgs[i] = fmt.Sprintf("%q", normalizeSelectorType(p.Type))
	}
	lhsNames := strings.Join(parNames, ", ")
	callArgs := "tos.calldata, " + strings.Join(typeArgs, ", ")

	src := fmt.Sprintf(`
if tos ~= nil and type(tos.calldata) == "string" and #tos.calldata > 2 then
  %s = __tol_abi_decode_tuple(%s)
end
`, lhsNames, callArgs)

	chunk, err := parse.Parse(bytes.NewReader([]byte(src)), "<tol-ctor-abi-guard>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build constructor ABI decode guard: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
}

func classifyDirectIRFnModifiers(mods []string) (string, error) {
	visibility := ""
	for _, m := range mods {
		if !isAllowedDirectIRFnModifier(m) {
			return "", fmt.Errorf("[%s] unsupported function modifier token '%s' in direct IR lowering", diag.CodeLowerUnsupportedFeature, m)
		}
		switch m {
		case "public", "external", "internal", "private":
			if visibility != "" && visibility != m {
				return "", fmt.Errorf("[%s] conflicting function visibility modifiers '%s' and '%s'", diag.CodeLowerUnsupportedFeature, visibility, m)
			}
			visibility = m
		}
	}
	return visibility, nil
}

func isAllowedDirectIRFnModifier(m string) bool {
	switch m {
	case "public", "external", "internal", "private", "view", "pure", "payable":
		return true
	default:
		return false
	}
}

func lowerFallbackToLua(body []tolast.Statement, env *loweringEnv) (luast.Stmt, error) {
	stmts, err := tolStmtsToLuaWithCtx(newLoweringCtx(env), body)
	if err != nil {
		return nil, err
	}
	nameExpr := withLineExpr(&luast.IdentExpr{Value: "__tol_fallback"})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    []string{},
		},
		Stmts: stmts,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}, 1), nil
}

func lowerReceiveToLua(body []tolast.Statement, env *loweringEnv) (luast.Stmt, error) {
	stmts, err := tolStmtsToLuaWithCtx(newLoweringCtx(env), body)
	if err != nil {
		return nil, err
	}
	nameExpr := withLineExpr(&luast.IdentExpr{Value: "__tol_receive"})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    []string{},
		},
		Stmts: stmts,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}, 1), nil
}

type loweringLoop struct {
	continueLabel string
}

type loweringCtx struct {
	labelSeq   int
	trySeq     int
	loops      []loweringLoop
	env        *loweringEnv
	scopes     []map[string]struct{}
	localTypes []map[string]string // per-scope map: local name -> TOL type
	unchecked  bool                // true when inside an unchecked {} block
	ternarySeq int                 // counter for unique ternary temp names
}

func newLoweringCtx(env *loweringEnv) *loweringCtx {
	c := &loweringCtx{
		labelSeq:   0,
		loops:      nil,
		env:        env,
		scopes:     nil,
		localTypes: nil,
	}
	c.pushScope()
	return c
}

func (c *loweringCtx) newLabel(prefix string) string {
	c.labelSeq++
	return fmt.Sprintf("%s_%d", prefix, c.labelSeq)
}

func (c *loweringCtx) pushLoop(continueLabel string) {
	c.loops = append(c.loops, loweringLoop{continueLabel: continueLabel})
}

func (c *loweringCtx) popLoop() {
	if len(c.loops) == 0 {
		return
	}
	c.loops = c.loops[:len(c.loops)-1]
}

func (c *loweringCtx) currentContinueLabel() string {
	if len(c.loops) == 0 {
		return ""
	}
	return c.loops[len(c.loops)-1].continueLabel
}

func (c *loweringCtx) pushScope() {
	c.scopes = append(c.scopes, map[string]struct{}{})
	c.localTypes = append(c.localTypes, map[string]string{})
}

func (c *loweringCtx) popScope() {
	if len(c.scopes) == 0 {
		return
	}
	c.scopes = c.scopes[:len(c.scopes)-1]
	if len(c.localTypes) > 0 {
		c.localTypes = c.localTypes[:len(c.localTypes)-1]
	}
}

func (c *loweringCtx) declareLocal(name string) {
	if len(c.scopes) == 0 {
		c.pushScope()
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	c.scopes[len(c.scopes)-1][name] = struct{}{}
}

// declareLocalWithType declares a local variable and records its TOL type.
// If typName is a user-defined value type alias, it is resolved to its underlying type.
func (c *loweringCtx) declareLocalWithType(name, typName string) {
	c.declareLocal(name)
	name = strings.TrimSpace(name)
	typName = strings.TrimSpace(typName)
	if name == "" || typName == "" {
		return
	}
	if len(c.localTypes) == 0 {
		return
	}
	// Resolve user-defined value type aliases transparently.
	typName = resolveTypeAlias(c, typName)
	c.localTypes[len(c.localTypes)-1][name] = typName
}

// typeOfLocal looks up the recorded TOL type for a local variable.
// Returns "" if not known.
func (c *loweringCtx) typeOfLocal(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for i := len(c.localTypes) - 1; i >= 0; i-- {
		if t, ok := c.localTypes[i][name]; ok {
			return t
		}
	}
	return ""
}

func (c *loweringCtx) isLocalName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if _, ok := c.scopes[i][name]; ok {
			return true
		}
	}
	return false
}

func (c *loweringCtx) storageInfoByName(name string) (storageSlotInfo, bool) {
	if c == nil || c.env == nil || len(c.env.storageByName) == 0 {
		return storageSlotInfo{}, false
	}
	info, ok := c.env.storageByName[name]
	return info, ok
}

func (c *loweringCtx) storagePathFromExpr(e *tolast.Expr) (string, []*tolast.Expr, bool) {
	if c == nil || e == nil {
		return "", nil, false
	}
	switch e.Kind {
	case "paren":
		return c.storagePathFromExpr(e.Left)
	case "ident":
		name := strings.TrimSpace(e.Value)
		if name == "" || c.isLocalName(name) {
			return "", nil, false
		}
		if _, ok := c.storageInfoByName(name); !ok {
			return "", nil, false
		}
		return name, []*tolast.Expr{}, true
	case "index":
		slot, keys, ok := c.storagePathFromExpr(e.Object)
		if !ok {
			return "", nil, false
		}
		out := make([]*tolast.Expr, 0, len(keys)+1)
		out = append(out, keys...)
		out = append(out, e.Index)
		return slot, out, true
	default:
		return "", nil, false
	}
}

func tolStmtToLua(ctx *loweringCtx, stmt tolast.Statement) (luast.Stmt, error) {
	switch stmt.Kind {
	case "let":
		exprs := []luast.Expr{}
		if stmt.Expr != nil {
			if decodeArg, ok := abiDecodeCallDataArg(stmt.Expr); ok && strings.TrimSpace(stmt.Type) != "" {
				typeTag := normalizeSelectorType(stmt.Type)
				if !isSupportedABIDecodeTargetTypeWithStructs(typeTag, ctx.env.structFields) {
					return nil, fmt.Errorf("[%s] abi.decode typed local binding unsupported target type '%s' in current stage", diag.CodeLowerUnsupportedFeature, typeTag)
				}
				dataExpr, err := tolExprToLua(ctx, decodeArg)
				if err != nil {
					return nil, err
				}
				exprs = append(exprs, withLineExpr(&luast.FuncCallExpr{
					Func: withLineExpr(&luast.IdentExpr{Value: "__tol_abi_decode_typed"}),
					Args: []luast.Expr{
						dataExpr,
						withLineExpr(&luast.StringExpr{Value: typeTag}),
					},
					AdjustRet: true,
				}))
			} else {
				ex, err := tolExprToLua(ctx, stmt.Expr)
				if err != nil {
					return nil, err
				}
				exprs = append(exprs, ex)
			}
		} else if strings.TrimSpace(stmt.Type) != "" {
			var structTypes map[string][]tolast.FieldDecl
			if ctx != nil && ctx.env != nil {
				structTypes = ctx.env.structFieldTypes
			}
			defaultExpr, ok := defaultValueExprForTypeWithStructs(normalizeSelectorType(stmt.Type), structTypes, nil)
			if !ok {
				return nil, fmt.Errorf("[%s] local '%s' of type '%s' requires explicit initializer in current stage", diag.CodeLowerUnsupportedFeature, strings.TrimSpace(stmt.Name), normalizeSelectorType(stmt.Type))
			}
			exprs = append(exprs, defaultExpr)
		}
		out := withLineStmt(&luast.LocalAssignStmt{
			Names: []string{stmt.Name},
			Exprs: exprs,
		}, stmt.Line)
		// For memory arrays (new T[](size)), register with a "__mem:" prefix so that
		// lowerMemArrayIndexExpr / lowerMemArrayStoreStmt can distinguish them from
		// ABI-decoded array parameters which share the same TOL type string.
		typeTag := normalizeSelectorType(stmt.Type)
		if stmt.Expr != nil && stmt.Expr.Kind == "new_array" {
			typeTag = "__mem:" + typeTag
		}
		ctx.declareLocalWithType(stmt.Name, typeTag)
		return out, nil
	case "let-tuple":
		return lowerLetTupleStmt(ctx, stmt)
	case "set":
		if storageStmt, ok, err := lowerStorageStoreStmt(ctx, stmt.Target, stmt.Expr); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageStmt, nil
		}
		// Memory array bounds-checked write: set arr[i] = v → __tol_array_set(arr, i, v)
		if memArrStmt, ok, err := lowerMemArrayStoreStmt(ctx, stmt, stmt.Target, stmt.Expr); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return memArrStmt, nil
		}
		lhs, err := tolExprToLua(ctx, stmt.Target)
		if err != nil {
			return nil, err
		}
		rhs, err := tolExprToLua(ctx, stmt.Expr)
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.AssignStmt{
			Lhs: []luast.Expr{lhs},
			Rhs: []luast.Expr{rhs},
		}, stmt.Line), nil
	case "return":
		exprs := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, ex)
		}
		return withLineStmt(&luast.ReturnStmt{Exprs: exprs}, stmt.Line), nil
	case "if":
		cond, err := tolExprToLua(ctx, stmt.Cond)
		if err != nil {
			return nil, err
		}
		thenStmts, err := tolStmtsToLuaWithCtx(ctx, stmt.Then)
		if err != nil {
			return nil, err
		}
		elseStmts, err := tolStmtsToLuaWithCtx(ctx, stmt.Else)
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.IfStmt{
			Condition: cond,
			Then:      thenStmts,
			Else:      elseStmts,
		}, stmt.Line), nil
	case "while":
		cond, err := tolExprToLua(ctx, stmt.Cond)
		if err != nil {
			return nil, err
		}
		continueLabel := ctx.newLabel("tol_continue")
		ctx.pushLoop(continueLabel)
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.popLoop()
		if err != nil {
			return nil, err
		}
		body = append(body, withLineStmt(&luast.LabelStmt{Name: continueLabel}, stmt.Line))
		return withLineStmt(&luast.WhileStmt{
			Condition: cond,
			Stmts:     body,
		}, stmt.Line), nil
	case "dowhile":
		// do { body } while (cond)  →  Lua: repeat body until not(cond)
		// The continue label is placed at the end of the body so that `continue`
		// jumps to the condition re-evaluation (same semantics as while).
		cond, err := tolExprToLua(ctx, stmt.Cond)
		if err != nil {
			return nil, err
		}
		continueLabel := ctx.newLabel("tol_dowhile_continue")
		ctx.pushLoop(continueLabel)
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.popLoop()
		if err != nil {
			return nil, err
		}
		body = append(body, withLineStmt(&luast.LabelStmt{Name: continueLabel}, stmt.Line))
		// Lua repeat...until exits when condition is true, so negate the TOL while-condition.
		notCond := withLineExpr(&luast.UnaryNotOpExpr{Expr: cond})
		return withLineStmt(&luast.RepeatStmt{
			Condition: notCond,
			Stmts:     body,
		}, stmt.Line), nil
	case "break":
		return withLineStmt(&luast.BreakStmt{}, stmt.Line), nil
	case "continue":
		lbl := ctx.currentContinueLabel()
		if lbl == "" {
			return nil, fmt.Errorf("[%s] continue used outside lowered loop context", diag.CodeLowerUnsupportedFeature)
		}
		return withLineStmt(&luast.GotoStmt{Label: lbl}, stmt.Line), nil
	case "for":
		block := make([]luast.Stmt, 0, 2)
		if stmt.Init != nil {
			ctx.pushScope()
			initStmt, err := tolStmtToLua(ctx, *stmt.Init)
			if err != nil {
				ctx.popScope()
				return nil, err
			}
			block = append(block, initStmt)
		} else {
			ctx.pushScope()
		}

		cond := luast.Expr(withLineExpr(&luast.TrueExpr{}))
		if stmt.Cond != nil {
			ce, err := tolExprToLua(ctx, stmt.Cond)
			if err != nil {
				ctx.popScope()
				return nil, err
			}
			cond = ce
		}

		continueLabel := ctx.newLabel("tol_for_continue")
		ctx.pushLoop(continueLabel)
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.popLoop()
		if err != nil {
			ctx.popScope()
			return nil, err
		}
		body = append(body, withLineStmt(&luast.LabelStmt{Name: continueLabel}, stmt.Line))
		if stmt.Post != nil {
			postStmt, err := tolExprStmtToLua(ctx, stmt.Post, stmt.Line)
			if err != nil {
				ctx.popScope()
				return nil, err
			}
			body = append(body, postStmt)
		}
		block = append(block, withLineStmt(&luast.WhileStmt{
			Condition: cond,
			Stmts:     body,
		}, stmt.Line))
		ctx.popScope()

		return withLineStmt(&luast.DoBlockStmt{Stmts: block}, stmt.Line), nil
	case "expr":
		return tolExprStmtToLua(ctx, stmt.Expr, stmt.Line)
	case "emit":
		// emit EventName(arg1, arg2, ...) → emit("EventName", arg1, arg2, ...)
		// Host wiring: prefer tos.emit(...), fallback to global emit(...).
		args := []luast.Expr{}
		if stmt.Expr != nil && stmt.Expr.Kind == "call" && stmt.Expr.Callee != nil {
			// First arg: event name as string literal.
			eventName := ""
			if stmt.Expr.Callee.Kind == "ident" {
				eventName = stmt.Expr.Callee.Value
			}
			args = append(args, withLineExpr(&luast.StringExpr{Value: eventName}))
			for _, a := range stmt.Expr.Args {
				ex, err := tolExprToLua(ctx, a)
				if err != nil {
					return nil, err
				}
				args = append(args, ex)
			}
		} else if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_emit"}),
			Args:      args,
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}, stmt.Line), nil
	case "require", "assert":
		// require(cond, "msg") → assert(cond, "msg")
		// assert(cond, "msg") → assert(cond, "msg")
		// Lua's assert(v, msg) raises an error with msg if v is falsy.
		//
		// Solidity compliance: require(cond) with NO message reverts with Panic(0x01),
		// not Error(""). The parser stores stmt.Text = `""` (empty string literal, 2
		// chars) when no message argument was provided. We detect this case for
		// "require" and emit "Panic(0x01)" to match Solidity reference behaviour.
		args := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		if stmt.Text != "" {
			msg := unquoteIfNeeded(stmt.Text)
			if stmt.Kind == "require" && msg == "" {
				// No message provided to require() → Solidity emits Panic(0x01).
				msg = "Panic(0x01)"
			}
			args = append(args, withLineExpr(&luast.StringExpr{Value: msg}))
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "assert"}),
			Args:      args,
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}, stmt.Line), nil
	case "revert":
		// revert "msg" → error("msg")
		args := []luast.Expr{}
		if stmt.Expr != nil {
			if ex, ok, err := lowerCustomErrorRevertExpr(ctx, stmt.Expr); err != nil {
				return nil, err
			} else if ok {
				args = append(args, ex)
			} else {
				ex, err := tolExprToLua(ctx, stmt.Expr)
				if err != nil {
					return nil, err
				}
				args = append(args, ex)
			}
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "error"}),
			Args:      args,
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}, stmt.Line), nil
	case "delete":
		return lowerDeleteStmt(ctx, stmt)
	case "unchecked":
		prevUnchecked := ctx.unchecked
		ctx.unchecked = true
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.unchecked = prevUnchecked
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.DoBlockStmt{Stmts: body}, stmt.Line), nil
	case "try":
		return lowerTryCatchStmt(ctx, stmt)
	default:
		return nil, fmt.Errorf("[%s] unsupported statement kind '%s'", diag.CodeLowerUnsupportedFeature, stmt.Kind)
	}
}

func tolStmtsToLua(in []tolast.Statement) ([]luast.Stmt, error) {
	return tolStmtsToLuaWithCtx(newLoweringCtx(nil), in)
}

func tolStmtsToLuaWithCtx(ctx *loweringCtx, in []tolast.Statement) ([]luast.Stmt, error) {
	ctx.pushScope()
	defer ctx.popScope()
	if len(in) == 0 {
		return []luast.Stmt{}, nil
	}
	out := make([]luast.Stmt, 0, len(in))
	for _, s := range in {
		ls, err := tolStmtToLua(ctx, s)
		if err != nil {
			return nil, err
		}
		out = append(out, ls)
	}
	return out, nil
}

// lowerTryCatchStmt lowers a try/catch statement to a Lua pcall pattern:
//
//	local __tol_try_ok_N, __tol_try_result_N = pcall(function()
//	    return <try_expr>
//	end)
//	if __tol_try_ok_N then
//	    <success_body>
//	else
//	    <catch_body>  -- with param bound to __tol_try_result_N if applicable
//	end
func lowerTryCatchStmt(ctx *loweringCtx, stmt tolast.Statement) (luast.Stmt, error) {
	ctx.trySeq++
	n := ctx.trySeq
	okVar := fmt.Sprintf("__tol_try_ok_%d", n)
	resultVar := fmt.Sprintf("__tol_try_result_%d", n)

	// Build the pcall wrapper function body: return <try_expr>
	var innerStmts []luast.Stmt
	if stmt.Expr != nil {
		tryExpr, err := tolExprToLua(ctx, stmt.Expr)
		if err != nil {
			return nil, err
		}
		innerStmts = []luast.Stmt{
			withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{tryExpr}}, stmt.Line),
		}
	} else {
		innerStmts = []luast.Stmt{}
	}

	// Build the anonymous function: function() return <try_expr> end
	anonFn := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{HasVargs: false, Names: []string{}},
		Stmts:   innerStmts,
	})

	// Build: local okVar, resultVar = pcall(anonFn)
	pcallExpr := withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "pcall"}),
		Args:      []luast.Expr{anonFn},
		AdjustRet: false,
	})
	pcallAssign := withLineStmt(&luast.LocalAssignStmt{
		Names: []string{okVar, resultVar},
		Exprs: []luast.Expr{pcallExpr},
	}, stmt.Line)

	// Lower the success body.
	successStmts, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
	if err != nil {
		return nil, err
	}

	// Build the else (catch) body.
	elseStmts, err := lowerCatchClauses(ctx, stmt.Catches, resultVar, stmt.Line)
	if err != nil {
		return nil, err
	}

	// Build: if okVar then <success> else <catch> end
	ifStmt := withLineStmt(&luast.IfStmt{
		Condition: withLineExpr(&luast.IdentExpr{Value: okVar}),
		Then:      successStmts,
		Else:      elseStmts,
	}, stmt.Line)

	// Wrap both statements in a do-block for scoping.
	return withLineStmt(&luast.DoBlockStmt{
		Stmts: []luast.Stmt{pcallAssign, ifStmt},
	}, stmt.Line), nil
}

// lowerCatchClauses builds the else-body for a try/catch: the catch dispatch logic.
// If there are no catch clauses, returns an empty slice.
// If there is only one clause, the body is lowered directly (with optional param binding).
// If there are multiple clauses, they are chained with type-checks.
func lowerCatchClauses(ctx *loweringCtx, catches []tolast.CatchClause, resultVar string, line int) ([]luast.Stmt, error) {
	if len(catches) == 0 {
		return []luast.Stmt{}, nil
	}

	if len(catches) == 1 {
		return lowerSingleCatchClause(ctx, catches[0], resultVar, line)
	}

	// Multiple clauses: separate Error/bytes clauses from bare clause.
	// Build chained if/else based on type check.
	// Strategy: check type of resultVar to dispatch.
	// For now, handle up to one typed clause + one bare clause.
	var typedClauses []tolast.CatchClause
	var bareClauses []tolast.CatchClause
	for _, c := range catches {
		if c.Kind == "" {
			bareClauses = append(bareClauses, c)
		} else {
			typedClauses = append(typedClauses, c)
		}
	}

	// Build the else chain from back to front.
	// Start with the bare clause body (or empty).
	currentElse := []luast.Stmt{}
	if len(bareClauses) > 0 {
		var err error
		currentElse, err = lowerSingleCatchClause(ctx, bareClauses[0], resultVar, line)
		if err != nil {
			return nil, err
		}
	}

	// Wrap each typed clause in a type check.
	for i := len(typedClauses) - 1; i >= 0; i-- {
		clause := typedClauses[i]
		clauseBody, err := lowerSingleCatchClause(ctx, clause, resultVar, line)
		if err != nil {
			return nil, err
		}
		// Panic clauses match numeric errors: type(resultVar) == "number".
		// Error/bytes clauses match string errors: type(resultVar) == "string".
		luaTypeName := "string"
		if clause.Kind == "Panic" {
			luaTypeName = "uint256"
		}
		typeCheckExpr := withLineExpr(&luast.RelationalOpExpr{
			Operator: "==",
			Lhs: withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: "type"}),
				Args:      []luast.Expr{withLineExpr(&luast.IdentExpr{Value: resultVar})},
				AdjustRet: true,
			}),
			Rhs: withLineExpr(&luast.StringExpr{Value: luaTypeName}),
		})
		currentElse = []luast.Stmt{
			withLineStmt(&luast.IfStmt{
				Condition: typeCheckExpr,
				Then:      clauseBody,
				Else:      currentElse,
			}, line),
		}
	}

	return currentElse, nil
}

// lowerSingleCatchClause lowers a single catch clause body, optionally binding the param.
func lowerSingleCatchClause(ctx *loweringCtx, clause tolast.CatchClause, resultVar string, line int) ([]luast.Stmt, error) {
	stmts := []luast.Stmt{}

	// If the clause has a parameter, bind it as a local.
	if clause.ParamName != "" {
		ctx.pushScope()
		defer ctx.popScope()
		ctx.declareLocalWithType(clause.ParamName, clause.ParamType)
		stmts = append(stmts, withLineStmt(&luast.LocalAssignStmt{
			Names: []string{clause.ParamName},
			Exprs: []luast.Expr{withLineExpr(&luast.IdentExpr{Value: resultVar})},
		}, line))
	}

	bodyStmts, err := tolStmtsToLuaWithCtx(ctx, clause.Body)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, bodyStmts...)
	return stmts, nil
}

func tolExprStmtToLua(ctx *loweringCtx, e *tolast.Expr, line int) (luast.Stmt, error) {
	if e == nil {
		return nil, fmt.Errorf("[%s] nil expression statement", diag.CodeLowerUnsupportedFeature)
	}
	// Postfix i++ / i-- used as for post-step expression or standalone statement:
	// rewrite to an assign expression and re-lower through the normal path so
	// that u256 checked arithmetic is applied correctly.
	if e.Kind == "unary" && (e.Op == "post++" || e.Op == "post--") {
		binOp := "+"
		if e.Op == "post--" {
			binOp = "-"
		}
		one := &tolast.Expr{Kind: "number", Value: "1"}
		rhs := &tolast.Expr{Kind: "binary", Op: binOp, Left: e.Right, Right: one}
		assignExpr := &tolast.Expr{Kind: "assign", Left: e.Right, Right: rhs}
		return tolExprStmtToLua(ctx, assignExpr, line)
	}
	if e.Kind == "assign" {
		if storageStmt, ok, err := lowerStorageStoreStmt(ctx, e.Left, e.Right); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageStmt, nil
		}
		lhs, err := tolExprToLua(ctx, e.Left)
		if err != nil {
			return nil, err
		}
		rhs, err := tolExprToLua(ctx, e.Right)
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.AssignStmt{
			Lhs: []luast.Expr{lhs},
			Rhs: []luast.Expr{rhs},
		}, line), nil
	}
	ex, err := tolExprToLua(ctx, e)
	if err != nil {
		return nil, err
	}
	call, ok := ex.(*luast.FuncCallExpr)
	if !ok {
		return nil, fmt.Errorf("[%s] expression statement must be a function call or assignment", diag.CodeLowerUnsupportedFeature)
	}
	return withLineStmt(&luast.FuncCallStmt{Expr: call}, line), nil
}

// lowerDeleteStmt lowers a "delete x" statement.
// - Local variable: emit `x = <zero_for_type>`.
// - Storage mapping key (m[k]): emit `__tol_sstore(slotExpr, nil)`.
// - Storage array element (arr[i]): emit `__tol_sstore(slotExpr, 0)`.
// - Scalar storage slot (s): emit `__tol_sstore(slotExpr, 0)`.
func lowerDeleteStmt(ctx *loweringCtx, stmt tolast.Statement) (luast.Stmt, error) {
	target := stmt.Expr
	if target == nil {
		return nil, fmt.Errorf("[%s] delete statement missing target expression", diag.CodeLowerUnsupportedFeature)
	}

	// Strip parens.
	for target.Kind == "paren" {
		target = target.Left
	}

	// Check if target is a storage path.
	if slotName, keys, ok := ctx.storagePathFromExpr(target); ok {
		info, _ := ctx.storageInfoByName(slotName)
		// Build the slot hash expression.
		slotExpr, err := buildHashSlotExpr(ctx, info, keys)
		if err != nil {
			return nil, err
		}
		// For mapping keys: store nil (deletion). For arrays/scalars: store 0.
		var valueExpr luast.Expr
		if info.kind == storageKindMapping {
			valueExpr = withLineExpr(&luast.NilExpr{})
		} else {
			valueExpr = withLineExpr(&luast.NumberExpr{Value: "0"})
		}
		deleteFn := "__tol_sstore"
		if info.isTransient {
			deleteFn = "__tol_tsstore"
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: deleteFn}),
			Args:      []luast.Expr{slotExpr, valueExpr},
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}, stmt.Line), nil
	}

	// Local variable: assign zero value based on its declared type.
	if target.Kind == "ident" {
		varName := strings.TrimSpace(target.Value)
		typeName := ctx.typeOfLocal(varName)
		var zeroExpr luast.Expr
		if typeName != "" {
			var ok bool
			zeroExpr, ok = defaultValueExprForType(typeName)
			if !ok {
				// Fallback to numeric 0.
				zeroExpr = withLineExpr(&luast.NumberExpr{Value: "0"})
			}
		} else {
			zeroExpr = withLineExpr(&luast.NumberExpr{Value: "0"})
		}
		lhs := withLineExpr(&luast.IdentExpr{Value: varName})
		return withLineStmt(&luast.AssignStmt{
			Lhs: []luast.Expr{lhs},
			Rhs: []luast.Expr{zeroExpr},
		}, stmt.Line), nil
	}

	return nil, fmt.Errorf("[%s] unsupported delete target kind '%s'", diag.CodeLowerUnsupportedFeature, target.Kind)
}

func tolExprToLua(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, error) {
	if e == nil {
		return nil, fmt.Errorf("[%s] nil expression", diag.CodeLowerUnsupportedFeature)
	}
	switch e.Kind {
	case "ident":
		if slotName, keys, ok := ctx.storagePathFromExpr(e); ok {
			return lowerStorageLoadExpr(ctx, slotName, keys)
		}
		// Inline constant references: if the ident matches a constant name and is not
		// a local variable, substitute its compile-time literal value.
		if !ctx.isLocalName(e.Value) {
			if constVal, ok := ctx.env.constantByName[strings.TrimSpace(e.Value)]; ok && constVal != nil {
				return tolExprToLua(ctx, constVal)
			}
		}
		switch e.Value {
		case "true":
			return withLineExpr(&luast.TrueExpr{}), nil
		case "false":
			return withLineExpr(&luast.FalseExpr{}), nil
		case "nil":
			return withLineExpr(&luast.NilExpr{}), nil
		default:
			return withLineExpr(&luast.IdentExpr{Value: e.Value}), nil
		}
	case "number":
		return withLineExpr(&luast.NumberExpr{Value: e.Value}), nil
	case "string":
		return withLineExpr(&luast.StringExpr{Value: unquoteIfNeeded(e.Value)}), nil
	case "hex_lit":
		// e.Value is a lowercase hex string (no "0x" prefix, no underscores, even length).
		// Runtime representation for bytesN / bytes values is "0x<hex>" string.
		return withLineExpr(&luast.StringExpr{Value: "0x" + e.Value}), nil
	case "paren":
		return tolExprToLua(ctx, e.Left)
	case "unary":
		inner, err := tolExprToLua(ctx, e.Right)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "-":
			// Check for signed unary negation.
			if signedExpr, ok, err := lowerSignedUnaryNeg(ctx, e, inner); ok || err != nil {
				if err != nil {
					return nil, err
				}
				return signedExpr, nil
			}
			return withLineExpr(&luast.UnaryMinusOpExpr{Expr: inner}), nil
		case "!":
			return withLineExpr(&luast.UnaryNotOpExpr{Expr: inner}), nil
		case "~":
			return withLineExpr(&luast.UnaryBitNotOpExpr{Expr: inner}), nil
		case "+":
			return inner, nil
		default:
			return nil, fmt.Errorf("[%s] unsupported unary operator '%s'", diag.CodeLowerUnsupportedFeature, e.Op)
		}
	case "binary":
		lhs, err := tolExprToLua(ctx, e.Left)
		if err != nil {
			return nil, err
		}
		rhs, err := tolExprToLua(ctx, e.Right)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "&&":
			return withLineExpr(&luast.LogicalOpExpr{Operator: "and", Lhs: lhs, Rhs: rhs}), nil
		case "||":
			return withLineExpr(&luast.LogicalOpExpr{Operator: "or", Lhs: lhs, Rhs: rhs}), nil
		case "==", "!=":
			// Equality/inequality is the same for signed and unsigned (same bit patterns).
			op := e.Op
			if op == "!=" {
				op = "~="
			}
			return withLineExpr(&luast.RelationalOpExpr{Operator: op, Lhs: lhs, Rhs: rhs}), nil
		case "<", "<=", ">", ">=":
			// Signed comparison: route to signed helpers if operand types are iN.
			if signedExpr, ok, err := lowerSignedBinaryExpr(ctx, e, lhs, rhs); ok || err != nil {
				if err != nil {
					return nil, err
				}
				return signedExpr, nil
			}
			return withLineExpr(&luast.RelationalOpExpr{Operator: e.Op, Lhs: lhs, Rhs: rhs}), nil
		case "+", "-", "*", "/", "%":
			// Signed arithmetic: route to signed helpers if operand types are iN.
			if signedExpr, ok, err := lowerSignedBinaryExpr(ctx, e, lhs, rhs); ok || err != nil {
				if err != nil {
					return nil, err
				}
				return signedExpr, nil
			}
			// Checked unsigned arithmetic for +, -, * when not inside unchecked {}.
			if ctx != nil && !ctx.unchecked && (e.Op == "+" || e.Op == "-" || e.Op == "*") {
				if checkedExpr, ok, err := lowerCheckedUintBinaryExpr(ctx, e, lhs, rhs); ok || err != nil {
					if err != nil {
						return nil, err
					}
					return checkedExpr, nil
				}
			}
			return withLineExpr(&luast.ArithmeticOpExpr{Operator: e.Op, Lhs: lhs, Rhs: rhs}), nil
		case "**":
			// Power operator: lower to Lua "^" which compiles to OP_POW → lu256Pow.
			return withLineExpr(&luast.ArithmeticOpExpr{Operator: "^", Lhs: lhs, Rhs: rhs}), nil
		case "&", "|", "^", "<<":
			return withLineExpr(&luast.ArithmeticOpExpr{Operator: e.Op, Lhs: lhs, Rhs: rhs}), nil
		case ">>":
			// Solidity semantics: >> is arithmetic (SAR) for signed types, logical (SHR) for unsigned.
			// Infer the type from the left operand.
			typStr := inferExprType(ctx, e.Left)
			bits := signedIntBits(typStr)
			if bits != 0 {
				// Signed type: use arithmetic right shift (__tol_signed_sar preserves sign bit).
				bitsExpr := makeSignedBitsExpr(bits)
				return withLineExpr(&luast.FuncCallExpr{
					Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_sar"}),
					Args:      []luast.Expr{lhs, rhs, bitsExpr},
					AdjustRet: true,
				}), nil
			}
			// Unsigned type (or unknown): logical shift (zero-fill). The Lua VM's >> is already logical.
			return withLineExpr(&luast.ArithmeticOpExpr{Operator: ">>", Lhs: lhs, Rhs: rhs}), nil
		case ">>>":
			// Logical (unsigned) right shift: always zero-fills high bits.
			// The underlying Lua VM's >> is already a logical shift, so we lower
			// >>> directly to >> in the Lua arithmetic op layer.
			return withLineExpr(&luast.ArithmeticOpExpr{Operator: ">>", Lhs: lhs, Rhs: rhs}), nil
		default:
			return nil, fmt.Errorf("[%s] unsupported binary operator '%s'", diag.CodeLowerUnsupportedFeature, e.Op)
		}
	case "assign":
		return nil, fmt.Errorf("[%s] assignment expressions are not supported in expression lowering", diag.CodeLowerUnsupportedFeature)
	case "call":
		// Intercept call options block: expr{gas: G, value: V}(args).
		if len(e.Options) > 0 {
			if optExpr, ok, err := lowerCallWithOptionsExpr(ctx, e); ok || err != nil {
				if err != nil {
					return nil, err
				}
				return optExpr, nil
			}
		}
		// Intercept overloaded direct contract function calls: fn(args) where fn has overloads.
		if overloadExpr, ok, err := lowerOverloadedDirectCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return overloadExpr, nil
		}
		// Intercept signed type casts: iN(expr) → __tol_signed_trunc(expr, N).
		if signedExpr, ok, err := lowerSignedTypeCastExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return signedExpr, nil
		}
		// Intercept unsigned type casts: uN(expr) → truncation to N bits.
		if unsignedExpr, ok, err := lowerUintTypeCastExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return unsignedExpr, nil
		}
		// Intercept payable(expr): identity cast — just return the argument as-is.
		if e.Callee != nil && e.Callee.Kind == "ident" && strings.TrimSpace(e.Callee.Value) == "payable" {
			if len(e.Args) == 1 {
				return tolExprToLua(ctx, e.Args[0])
			}
		}
		// Intercept agent-native calls: agent(expr), escrow/release/slash, delegation.verify().
		if agentExpr, ok, err := lowerAgentNativeCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return agentExpr, nil
		}
		if gasExpr, ok, err := lowerGasLeftBuiltinExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return gasExpr, nil
		}
		if cryptoExpr, ok, err := lowerCryptoBuiltinCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return cryptoExpr, nil
		}
		if transferExpr, ok, err := lowerAddressTransferSendCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return transferExpr, nil
		}
		if hostExpr, ok, err := lowerHostBuiltinCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return hostExpr, nil
		}
		if superCallExpr, ok, err := lowerSuperCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return superCallExpr, nil
		}
		if udvtExpr, ok, err := lowerUDVTWrapUnwrapCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return udvtExpr, nil
		}
		if scopedCallExpr, ok, err := lowerContractScopedCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return scopedCallExpr, nil
		}
		if concatExpr, ok, err := lowerBytesStringConcatCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return concatExpr, nil
		}
		if abiExpr, ok, err := lowerABIBuiltinExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return abiExpr, nil
		}
		if selExpr, ok, err := lowerSelectorBuiltinExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return selExpr, nil
		}
		if storageExpr, ok, err := lowerStoragePushCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageExpr, nil
		}
		if libExpr, ok, err := lowerLibraryCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return libExpr, nil
		}
		callee, err := tolExprToLua(ctx, e.Callee)
		if err != nil {
			return nil, err
		}
		args := make([]luast.Expr, 0, len(e.Args))
		for _, a := range e.Args {
			ex, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      callee,
			Args:      args,
			AdjustRet: true,
		}), nil
	case "member":
		if typeMinMaxExpr, ok, err := lowerTypeMinMaxExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return typeMinMaxExpr, nil
		}
		if ifaceIdExpr, ok, err := lowerTypeInterfaceIdExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return ifaceIdExpr, nil
		}
		if qualExpr, ok, err := lowerQualifiedMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return qualExpr, nil
		}
		if enumExpr, ok, err := lowerEnumMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return enumExpr, nil
		}
		if envExpr, ok, err := lowerEnvMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return envExpr, nil
		}
		if sel, ok, err := lowerSelectorMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return sel, nil
		}
		if storageExpr, ok, err := lowerStorageLengthMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageExpr, nil
		}
		if dynLenExpr, ok, err := lowerDynLengthMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return dynLenExpr, nil
		}
		obj, err := tolExprToLua(ctx, e.Object)
		if err != nil {
			return nil, err
		}
		return withLineExpr(&luast.AttrGetExpr{
			Object: obj,
			Key:    withLineExpr(&luast.StringExpr{Value: e.Member}),
		}), nil
	case "slice":
		if sliceExpr, ok, err := lowerBytesSliceExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return sliceExpr, nil
		}
		return nil, fmt.Errorf("[%s] slice expression lowering failed", diag.CodeLowerUnsupportedFeature)
	case "index":
		if slotName, keys, ok := ctx.storagePathFromExpr(e); ok {
			return lowerStorageLoadExpr(ctx, slotName, keys)
		}
		// Byte-level indexing on bytes/string locals: x[i] → __tol_bytes_index(x, i)
		if bytesIndexExpr, ok, err := lowerBytesIndexExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return bytesIndexExpr, nil
		}
		// Memory array indexing (bounds-checked): arr[i] → __tol_array_get(arr, i)
		if arrIndexExpr, ok, err := lowerMemArrayIndexExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return arrIndexExpr, nil
		}
		obj, err := tolExprToLua(ctx, e.Object)
		if err != nil {
			return nil, err
		}
		idx, err := tolExprToLua(ctx, e.Index)
		if err != nil {
			return nil, err
		}
		return withLineExpr(&luast.AttrGetExpr{
			Object: obj,
			Key:    idx,
		}), nil
	case "ternary":
		if len(e.Args) != 3 {
			return nil, fmt.Errorf("[%s] ternary expression requires exactly 3 operands", diag.CodeLowerUnsupportedFeature)
		}
		cond, err := tolExprToLua(ctx, e.Args[0])
		if err != nil {
			return nil, err
		}
		thenExpr, err := tolExprToLua(ctx, e.Args[1])
		if err != nil {
			return nil, err
		}
		elseExpr, err := tolExprToLua(ctx, e.Args[2])
		if err != nil {
			return nil, err
		}
		// Emit: (function() if cond then return then_val else return else_val end end)()
		fnBody := []luast.Stmt{
			withLineStmt(&luast.IfStmt{
				Condition: cond,
				Then:      []luast.Stmt{withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{thenExpr}}, 1)},
				Else:      []luast.Stmt{withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{elseExpr}}, 1)},
			}, 1),
		}
		fn := withLineExpr(&luast.FunctionExpr{
			ParList: &luast.ParList{HasVargs: false, Names: []string{}},
			Stmts:   fnBody,
		})
		return withLineExpr(&luast.FuncCallExpr{
			Func:      fn,
			Args:      []luast.Expr{},
			AdjustRet: true,
		}), nil
	case "struct_lit":
		// Struct literal: Foo { field: val, ... } → Lua table { field = val_expr, ... }
		// Field order follows declaration order in the struct.
		var luaFields []*luast.Field
		// Collect provided field expressions by name.
		fieldMap := make(map[string]luast.Expr, len(e.StructFields))
		for _, sf := range e.StructFields {
			valExpr, err := tolExprToLua(ctx, sf.Expr)
			if err != nil {
				return nil, err
			}
			fieldMap[sf.Name] = valExpr
		}
		// Emit fields in declaration order if known, otherwise in provided order.
		if ctx != nil && ctx.env != nil {
			if orderedNames, ok := ctx.env.structFields[e.Value]; ok {
				for _, fname := range orderedNames {
					keyExpr := withLineExpr(&luast.StringExpr{Value: fname})
					valExpr, hasVal := fieldMap[fname]
					if !hasVal {
						valExpr = withLineExpr(&luast.NilExpr{})
					}
					luaFields = append(luaFields, &luast.Field{Key: keyExpr, Value: valExpr})
				}
			} else {
				for _, sf := range e.StructFields {
					valExpr, err := tolExprToLua(ctx, sf.Expr)
					if err != nil {
						return nil, err
					}
					luaFields = append(luaFields, &luast.Field{
						Key:   withLineExpr(&luast.StringExpr{Value: sf.Name}),
						Value: valExpr,
					})
				}
			}
		} else {
			for _, sf := range e.StructFields {
				valExpr, err := tolExprToLua(ctx, sf.Expr)
				if err != nil {
					return nil, err
				}
				luaFields = append(luaFields, &luast.Field{
					Key:   withLineExpr(&luast.StringExpr{Value: sf.Name}),
					Value: valExpr,
				})
			}
		}
		return withLineExpr(&luast.TableExpr{Fields: luaFields}), nil
	case "new":
		// new ContractName(args...) — lower as a call to __tol_new_<ContractName>.
		// The constructor function must be registered at runtime by the host environment.
		// When used inside try/catch, any error thrown during construction is caught by pcall.
		constructorFn := fmt.Sprintf("__tol_new_%s", e.Value)
		args := make([]luast.Expr, 0, len(e.Args))
		for _, a := range e.Args {
			if a == nil {
				return nil, fmt.Errorf("[%s] 'new %s' argument cannot be nil", diag.CodeLowerUnsupportedFeature, e.Value)
			}
			ex, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: constructorFn}),
			Args:      args,
			AdjustRet: true,
		}), nil
	case "new_array":
		// new T[](size) — ephemeral memory array.
		// Lower to __tol_new_array(size, default_val) where default_val is the zero value for T.
		if len(e.Args) != 1 || e.Args[0] == nil {
			return nil, fmt.Errorf("[%s] 'new %s[]' requires exactly one size argument", diag.CodeLowerUnsupportedFeature, e.Value)
		}
		sizeExpr, err := tolExprToLua(ctx, e.Args[0])
		if err != nil {
			return nil, err
		}
		// Determine zero value for the element type.
		elemType := normalizeSelectorType(e.Value)
		var defaultExpr luast.Expr
		if elemType == "bool" {
			defaultExpr = withLineExpr(&luast.FalseExpr{})
		} else if elemType == "address" || strings.HasPrefix(elemType, "bytes") {
			defaultExpr = withLineExpr(&luast.StringExpr{Value: "0x"})
		} else {
			// uN, iN, u256: default is 0.
			defaultExpr = withLineExpr(&luast.NumberExpr{Value: "0"})
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_new_array"}),
			Args:      []luast.Expr{sizeExpr, defaultExpr},
			AdjustRet: true,
		}), nil
	case "array_lit":
		// Inline array literal: [1, 2, 3] → Lua sequential table {1, 2, 3}
		var fields []*luast.Field
		for _, elem := range e.Args {
			val, err := tolExprToLua(ctx, elem)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &luast.Field{Value: val})
		}
		return withLineExpr(&luast.TableExpr{Fields: fields}), nil
	case "tuple":
		// Tuple expression: (a, b) → Lua sequential table {a, b}
		var fields []*luast.Field
		for _, elem := range e.Args {
			if elem == nil {
				fields = append(fields, &luast.Field{Value: withLineExpr(&luast.NilExpr{})})
				continue
			}
			val, err := tolExprToLua(ctx, elem)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &luast.Field{Value: val})
		}
		return withLineExpr(&luast.TableExpr{Fields: fields}), nil
	case "named_call":
		// Named call arguments: f({to: alice, amount: 100}) → f({to = alice, amount = 100})
		calleeExpr, err := tolExprToLua(ctx, e.Callee)
		if err != nil {
			return nil, err
		}
		var argFields []*luast.Field
		for _, na := range e.NamedArgs {
			val, err := tolExprToLua(ctx, na.Expr)
			if err != nil {
				return nil, err
			}
			argFields = append(argFields, &luast.Field{
				Key:   withLineExpr(&luast.StringExpr{Value: na.Name}),
				Value: val,
			})
		}
		tableArg := withLineExpr(&luast.TableExpr{Fields: argFields})
		return withLineExpr(&luast.FuncCallExpr{
			Func:      calleeExpr,
			Args:      []luast.Expr{tableArg},
			AdjustRet: true,
		}), nil
	default:
		return nil, fmt.Errorf("[%s] unsupported expression kind '%s'", diag.CodeLowerUnsupportedFeature, e.Kind)
	}
}

func lowerCryptoBuiltinCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)
	if ctx != nil && ctx.env != nil && len(ctx.env.functionByName) > 0 {
		if _, exists := ctx.env.functionByName[name]; exists {
			// Contract function names shadow crypto builtin aliases in direct calls.
			return nil, false, nil
		}
	}
	hostFn := ""
	wantArity := -1
	switch name {
	case "bytes_eq", "string_eq":
		hostFn = "__tol_bytes_eq"
		wantArity = 2
	case "keccak256":
		hostFn = "__tol_keccak256"
		wantArity = 1
	case "sha256":
		hostFn = "__tol_sha256"
		wantArity = 1
	case "ripemd160":
		hostFn = "__tol_ripemd160"
		wantArity = 1
	case "ecrecover":
		hostFn = "__tol_ecrecover"
		wantArity = 4
	default:
		return nil, false, nil
	}
	if len(e.Args) != wantArity {
		return nil, true, fmt.Errorf("[%s] %s(...) requires exactly %d argument(s)", diag.CodeLowerUnsupportedFeature, name, wantArity)
	}
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] %s(...) argument cannot be nil", diag.CodeLowerUnsupportedFeature, name)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: hostFn}),
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

func lowerGasLeftBuiltinExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	_ = ctx
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	obj := stripTolParens(callee.Object)
	if obj == nil || obj.Kind != "ident" || strings.TrimSpace(obj.Value) != "gas" || strings.TrimSpace(callee.Member) != "left" {
		return nil, false, nil
	}
	if len(e.Args) != 0 {
		return nil, true, fmt.Errorf("[%s] gas.left() requires no arguments", diag.CodeLowerUnsupportedFeature)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_gas_left"}),
		Args:      []luast.Expr{},
		AdjustRet: true,
	}), true, nil
}

// lowerCallWithOptionsExpr handles call expressions with a {gas: G, value: V} options block.
// Syntax: expr{gas: G, value: V}(args)
//
// Supported patterns:
//   - addr.call{gas: G, value: V}(data) → __tol_host_call(addr, V, data)
//     (gas option is accepted but currently ignored by the host function)
//   - addr.staticcall{gas: G}(data)     → __tol_host_staticcall(addr, data)
//   - addr.delegatecall{gas: G}(data)   → __tol_host_delegatecall(addr, data)
//   - addr.transfer{value: V}(to)       → __tol_host_transfer(addr, V)  [value from options]
//
// For unrecognized callee patterns the function returns (nil, false, nil) and
// the call falls through to the normal lowering pipeline.
func lowerCallWithOptionsExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" || len(e.Options) == 0 {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		// Options block on non-member calls (e.g. direct call) is unsupported.
		return nil, false, nil
	}
	member := strings.TrimSpace(callee.Member)

	// Extract gas and value from options.
	var gasOpt, valueOpt *tolast.Expr
	for i := range e.Options {
		switch e.Options[i].Key {
		case "gas":
			gasOpt = e.Options[i].Value
		case "value":
			valueOpt = e.Options[i].Value
		}
	}
	_ = gasOpt // gas option accepted but currently passed through; host handles it

	addrExpr, err := tolExprToLua(ctx, callee.Object)
	if err != nil {
		return nil, true, err
	}

	switch member {
	case "call":
		// addr.call{value: V}(data) → __tol_host_call(addr, V, data)
		// The value defaults to 0 if not provided.
		var valueExpr luast.Expr
		if valueOpt != nil {
			valueExpr, err = tolExprToLua(ctx, valueOpt)
			if err != nil {
				return nil, true, err
			}
		} else {
			valueExpr = withLineExpr(&luast.NumberExpr{Value: "0"})
		}
		// data argument: must provide exactly 1 arg
		if len(e.Args) != 1 {
			return nil, true, fmt.Errorf("[%s] addr.call{...}(data) requires exactly 1 data argument", diag.CodeLowerUnsupportedFeature)
		}
		dataExpr, err := tolExprToLua(ctx, e.Args[0])
		if err != nil {
			return nil, true, err
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_host_call"}),
			Args:      []luast.Expr{addrExpr, valueExpr, dataExpr},
			AdjustRet: true,
		}), true, nil

	case "staticcall":
		// addr.staticcall{gas: G}(data) → __tol_host_staticcall(addr, data)
		if len(e.Args) != 1 {
			return nil, true, fmt.Errorf("[%s] addr.staticcall{...}(data) requires exactly 1 data argument", diag.CodeLowerUnsupportedFeature)
		}
		dataExpr, err := tolExprToLua(ctx, e.Args[0])
		if err != nil {
			return nil, true, err
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_host_staticcall"}),
			Args:      []luast.Expr{addrExpr, dataExpr},
			AdjustRet: true,
		}), true, nil

	case "delegatecall":
		// addr.delegatecall{gas: G}(data) → __tol_host_delegatecall(addr, data)
		if len(e.Args) != 1 {
			return nil, true, fmt.Errorf("[%s] addr.delegatecall{...}(data) requires exactly 1 data argument", diag.CodeLowerUnsupportedFeature)
		}
		dataExpr, err := tolExprToLua(ctx, e.Args[0])
		if err != nil {
			return nil, true, err
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_host_delegatecall"}),
			Args:      []luast.Expr{addrExpr, dataExpr},
			AdjustRet: true,
		}), true, nil

	case "transfer":
		// addr.transfer{value: V}(to): value comes from options (not args) if given.
		// If no value option, fall through to normal transfer lowering.
		if valueOpt == nil {
			return nil, false, nil
		}
		// transfer with explicit value option: ignore args (transfer has no args in Solidity)
		valueExpr, err := tolExprToLua(ctx, valueOpt)
		if err != nil {
			return nil, true, err
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_host_transfer"}),
			Args:      []luast.Expr{addrExpr, valueExpr},
			AdjustRet: true,
		}), true, nil
	}

	// Unrecognized member: fall through to normal lowering.
	return nil, false, nil
}

// lowerAddressTransferSendCallExpr handles addr.transfer(amount) and addr.send(amount),
// lowering them to __tol_host_transfer(addr, amount) and __tol_host_send(addr, amount).
// The sema pass (TOL2085) already guarantees addr is address payable; this handles lowering.
func lowerAddressTransferSendCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	member := strings.TrimSpace(callee.Member)
	if member != "transfer" && member != "send" {
		return nil, false, nil
	}
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] .%s() requires exactly 1 argument", diag.CodeLowerUnsupportedFeature, member)
	}
	hostFn := "__tol_host_transfer"
	if member == "send" {
		hostFn = "__tol_host_send"
	}
	addrExpr, err := tolExprToLua(ctx, callee.Object)
	if err != nil {
		return nil, true, err
	}
	amountExpr, err := tolExprToLua(ctx, e.Args[0])
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: hostFn}),
		Args:      []luast.Expr{addrExpr, amountExpr},
		AdjustRet: true,
	}), true, nil
}

func lowerHostBuiltinCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)
	if ctx != nil && ctx.env != nil && len(ctx.env.functionByName) > 0 {
		if _, exists := ctx.env.functionByName[name]; exists {
			// Contract function names shadow host builtin aliases in direct calls.
			return nil, false, nil
		}
	}
	hostFn := ""
	wantArity := -1
	switch name {
	case "call":
		hostFn = "__tol_host_call"
		wantArity = 3
	case "staticcall":
		hostFn = "__tol_host_staticcall"
		wantArity = 2
	case "delegatecall":
		hostFn = "__tol_host_delegatecall"
		wantArity = 2
	case "create":
		hostFn = "__tol_host_create"
		wantArity = 2
	case "create2":
		hostFn = "__tol_host_create2"
		wantArity = 3
	case "transfer":
		hostFn = "__tol_host_transfer"
		wantArity = 2
	default:
		return nil, false, nil
	}
	if len(e.Args) != wantArity {
		return nil, true, fmt.Errorf("[%s] %s(...) requires exactly %d argument(s)", diag.CodeLowerUnsupportedFeature, name, wantArity)
	}
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] %s(...) argument cannot be nil", diag.CodeLowerUnsupportedFeature, name)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: hostFn}),
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

// lowerAgentNativeCallExpr handles agent-native call expressions:
//   - agent(expr)                            → __tol_agent_cast(expr)
//   - escrow(agent, amount, purpose)         → __tol_escrow(agent, amount, __tol_pur_purpose)
//   - release(agent, amount, purpose)        → __tol_release(agent, amount, __tol_pur_purpose)
//   - slash(agent, amount, recipient, purp)  → __tol_slash(agent, amount, recipient, __tol_pur_purpose)
//   - delegation.verify(nonce, sig)          → __tol_delegation_verify(nonce, sig)
//
// Returns (expr, true, nil) on match, (nil, false, nil) if not an agent-native call.
func lowerAgentNativeCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil {
		return nil, false, nil
	}

	// delegation.verify(nonce, sig) — member call on "delegation" object.
	if callee.Kind == "member" {
		obj := stripTolParens(callee.Object)
		if obj != nil && obj.Kind == "ident" &&
			strings.TrimSpace(obj.Value) == "delegation" &&
			strings.TrimSpace(callee.Member) == "verify" {
			if len(e.Args) != 2 {
				return nil, true, fmt.Errorf("[%s] delegation.verify(nonce, sig) requires exactly 2 arguments", diag.CodeLowerUnsupportedFeature)
			}
			nonce, err := tolExprToLua(ctx, e.Args[0])
			if err != nil {
				return nil, true, err
			}
			sig, err := tolExprToLua(ctx, e.Args[1])
			if err != nil {
				return nil, true, err
			}
			return withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_delegation_verify"}),
				Args:      []luast.Expr{nonce, sig},
				AdjustRet: true,
			}), true, nil
		}
		return nil, false, nil
	}

	if callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)

	// agent(expr) cast — inline agentload guard.
	if name == "agent" {
		if len(e.Args) != 1 {
			return nil, true, fmt.Errorf("[%s] agent(expr) requires exactly 1 argument", diag.CodeLowerUnsupportedFeature)
		}
		inner, err := tolExprToLua(ctx, e.Args[0])
		if err != nil {
			return nil, true, err
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_agent_cast"}),
			Args:      []luast.Expr{inner},
			AdjustRet: true,
		}), true, nil
	}

	// escrow / release / slash builtins.
	type escrowSpec struct {
		helperName  string
		arity       int
		purposeIdx  int // index of the purpose argument
	}
	var spec *escrowSpec
	switch name {
	case "escrow":
		spec = &escrowSpec{helperName: "__tol_escrow", arity: 3, purposeIdx: 2}
	case "release":
		spec = &escrowSpec{helperName: "__tol_release", arity: 3, purposeIdx: 2}
	case "slash":
		spec = &escrowSpec{helperName: "__tol_slash", arity: 4, purposeIdx: 3}
	}
	if spec != nil {
		if len(e.Args) != spec.arity {
			return nil, true, fmt.Errorf("[%s] %s(...) requires exactly %d arguments", diag.CodeLowerUnsupportedFeature, name, spec.arity)
		}
		args := make([]luast.Expr, spec.arity)
		for i, a := range e.Args {
			if i == spec.purposeIdx {
				// Purpose argument: transform ident "PurposeName" → "__tol_pur_PurposeName".
				purIdent := stripTolParens(a)
				if purIdent != nil && purIdent.Kind == "ident" {
					purName := strings.TrimSpace(purIdent.Value)
					args[i] = withLineExpr(&luast.IdentExpr{Value: "__tol_pur_" + purName})
					continue
				}
			}
			x, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, true, err
			}
			args[i] = x
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: spec.helperName}),
			Args:      args,
			AdjustRet: true,
		}), true, nil
	}

	return nil, false, nil
}

// lowerQualifiedMemberExpr handles package-qualified constant and enum access:
//
//	ContractName.CONST_NAME          → inline constant (from constByName["ContractName.CONST_NAME"])
//	ContractName.EnumName.Member     → inline enum integer (from enumByName["ContractName.EnumName"]["Member"])
//
// Returns (expr, true, nil) on success, (nil, false, nil) if not a qualified access.
func lowerQualifiedMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil {
		return nil, false, nil
	}
	obj := stripTolParens(e.Object)
	if obj == nil {
		return nil, false, nil
	}
	member := strings.TrimSpace(e.Member)

	// Case 1: ContractName.CONST_NAME — obj is a simple ident.
	if obj.Kind == "ident" {
		qualKey := strings.TrimSpace(obj.Value) + "." + member
		if constVal, ok := ctx.env.constantByName[qualKey]; ok && constVal != nil {
			expr, err := tolExprToLua(ctx, constVal)
			if err != nil {
				return nil, true, err
			}
			return expr, true, nil
		}
	}

	// Case 2: ContractName.EnumName.Member — obj is itself a member expr (ContractName.EnumName).
	if obj.Kind == "member" {
		innerObj := stripTolParens(obj.Object)
		if innerObj != nil && innerObj.Kind == "ident" {
			enumKey := strings.TrimSpace(innerObj.Value) + "." + strings.TrimSpace(obj.Member)
			if members, ok := ctx.env.enumByName[enumKey]; ok {
				val, ok := members[member]
				if !ok {
					return nil, true, fmt.Errorf("[%s] enum '%s' has no member '%s'",
						diag.CodeLowerUnsupportedFeature, enumKey, member)
				}
				return withLineExpr(&luast.NumberExpr{Value: strconv.Itoa(val)}), true, nil
			}
		}
	}

	return nil, false, nil
}

// lowerEnumMemberExpr lowers an enum member access (e.g. State.Active) to an integer constant.
// Returns (expr, true, nil) if the object is a known enum name and the member is valid.
func lowerEnumMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil || len(ctx.env.enumByName) == 0 {
		return nil, false, nil
	}
	obj := stripTolParens(e.Object)
	if obj == nil || obj.Kind != "ident" {
		return nil, false, nil
	}
	enumName := strings.TrimSpace(obj.Value)
	members, isEnum := ctx.env.enumByName[enumName]
	if !isEnum {
		return nil, false, nil
	}
	memberName := strings.TrimSpace(e.Member)
	val, ok := members[memberName]
	if !ok {
		return nil, true, fmt.Errorf("[%s] enum '%s' has no member '%s'", diag.CodeLowerUnsupportedFeature, enumName, memberName)
	}
	return withLineExpr(&luast.NumberExpr{Value: strconv.Itoa(val)}), true, nil
}

func lowerEnvMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	_ = ctx
	if e == nil || e.Kind != "member" {
		return nil, false, nil
	}
	obj := stripTolParens(e.Object)
	if obj == nil || obj.Kind != "ident" {
		return nil, false, nil
	}
	scope := strings.TrimSpace(obj.Value)
	switch scope {
	case "msg", "tx", "block":
		// supported
	default:
		return nil, false, nil
	}
	key := strings.TrimSpace(e.Member)
	if key == "" {
		return nil, true, fmt.Errorf("[%s] %s.<field> requires non-empty field name", diag.CodeLowerUnsupportedFeature, scope)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func: withLineExpr(&luast.IdentExpr{Value: "__tol_env_get"}),
		Args: []luast.Expr{
			withLineExpr(&luast.StringExpr{Value: scope}),
			withLineExpr(&luast.StringExpr{Value: key}),
		},
		AdjustRet: true,
	}), true, nil
}

// lowerSuperCallExpr lowers super.fn(args) → fn(args).
// In the current single-file model a super call dispatches to the function by name,
// allowing the child's implementation (which calls super) to delegate upward.
// This is a best-effort lowering; full MRO-based dispatch would require cross-file IR.
func lowerSuperCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	scopeExpr := stripTolParens(callee.Object)
	if scopeExpr == nil || scopeExpr.Kind != "ident" || strings.TrimSpace(scopeExpr.Value) != "super" {
		return nil, false, nil
	}
	fnName := strings.TrimSpace(callee.Member)
	if fnName == "" {
		return nil, true, fmt.Errorf("[%s] super call target function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] super call argument cannot be nil", diag.CodeLowerUnsupportedFeature)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: fnName}),
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

// lowerUDVTWrapUnwrapCallExpr handles Solidity user-defined value type wrap/unwrap:
//
//	TypeName.wrap(value)   → identity: returns value as-is (both are same underlying type)
//	value.unwrap()         → identity: returns value as-is
//
// Solidity `type X is BaseType` creates a distinct type. The only conversions are:
//   - X.wrap(v)   — BaseType → X (identity at runtime)
//   - x.unwrap()  — X → BaseType (identity at runtime)
//
// At the IR level both are no-ops: the underlying representation is identical.
func lowerUDVTWrapUnwrapCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil || len(ctx.env.typeAliases) == 0 {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	obj := stripTolParens(callee.Object)
	if obj == nil {
		return nil, false, nil
	}
	member := strings.TrimSpace(callee.Member)

	// Pattern 1: TypeName.wrap(value) — obj is ident matching a UDVT name.
	if obj.Kind == "ident" {
		typName := strings.TrimSpace(obj.Value)
		if _, isAlias := ctx.env.typeAliases[typName]; isAlias && member == "wrap" {
			if len(e.Args) != 1 || e.Args[0] == nil {
				return nil, true, fmt.Errorf("[%s] %s.wrap() requires exactly 1 argument", diag.CodeLowerUnsupportedFeature, typName)
			}
			// Identity cast: return the argument as-is.
			result, err := tolExprToLua(ctx, e.Args[0])
			if err != nil {
				return nil, true, err
			}
			return result, true, nil
		}
	}

	// Pattern 2: value.unwrap() — member is "unwrap", no arguments.
	// We accept unwrap() on any expression when the expression's inferred type is a UDVT.
	// Since we can't always determine the exact type statically, we accept unwrap() on any
	// expression when there are no arguments and the method name is "unwrap".
	// This is safe because unwrap() is an identity operation at runtime.
	if member == "unwrap" && len(e.Args) == 0 {
		// Check that obj resolves to something whose type might be a UDVT.
		// For local variables, try to infer the type; otherwise accept unconditionally
		// when at least one UDVT is declared (since unwrap() is unambiguous).
		result, err := tolExprToLua(ctx, obj)
		if err != nil {
			return nil, true, err
		}
		return result, true, nil
	}

	return nil, false, nil
}

func lowerContractScopedCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	scopeExpr := stripTolParens(callee.Object)
	if scopeExpr == nil || scopeExpr.Kind != "ident" {
		return nil, false, nil
	}
	scope := strings.TrimSpace(scopeExpr.Value)
	fnName := strings.TrimSpace(callee.Member)
	if fnName == "" {
		return nil, true, fmt.Errorf("[%s] contract-scoped call target function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}

	// Check if this is a same-contract call (this.method or ContractName.method).
	isSameContract := scope == "this" ||
		(ctx != nil && ctx.env != nil && strings.TrimSpace(ctx.env.contractName) != "" && scope == ctx.env.contractName)

	if isSameContract {
		args := make([]luast.Expr, 0, len(e.Args))
		for _, a := range e.Args {
			if a == nil {
				return nil, true, fmt.Errorf("[%s] contract-scoped call argument cannot be nil", diag.CodeLowerUnsupportedFeature)
			}
			ex, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, true, err
			}
			args = append(args, ex)
		}
		luaFnName := fnName
		if resolved := resolveOverloadedLuaName(ctx.env, fnName, len(e.Args)); resolved != "" {
			luaFnName = resolved
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: luaFnName}),
			Args:      args,
			AdjustRet: true,
		}), true, nil
	}

	// Check if scope is an interface-typed variable.
	if ctx != nil && ctx.env != nil {
		localType := ctx.typeOfLocal(scope)
		if localType != "" {
			if _, isIface := ctx.env.interfaceByName[localType]; isIface {
				return lowerInterfaceVarCall(ctx, scope, localType, fnName, e)
			}
			// Step 9a: handle fully-qualified type like "tos.registry.AgentRegistry".
			// Fall back to looking up the last dot-segment as an interface name.
			if strings.Contains(localType, ".") {
				if idx := strings.LastIndex(localType, "."); idx > 0 {
					contractSeg := localType[idx+1:]
					if _, isIface := ctx.env.interfaceByName[contractSeg]; isIface {
						return lowerInterfaceVarCall(ctx, scope, contractSeg, fnName, e)
					}
				}
			}
		}
	}

	return nil, false, nil
}

// lowerInterfaceVarCall lowers a call of the form ifaceVar.method(args) where ifaceVar
// has a known interface type. It computes the ABI selector from the interface declaration
// and emits __tol_host_iface_call(addr, selector, abi_encoded_args).
func lowerInterfaceVarCall(ctx *loweringCtx, varName, ifaceName, fnName string, e *tolast.Expr) (luast.Expr, bool, error) {
	fns := ctx.env.interfaceByName[ifaceName]
	var sig *lower.InterfaceFuncSig
	for i := range fns {
		if fns[i].Name == fnName {
			sig = &fns[i]
			break
		}
	}
	if sig == nil {
		return nil, true, fmt.Errorf("[%s] function %q not found in interface %q", diag.CodeLowerUnsupportedFeature, fnName, ifaceName)
	}

	// Compute the 4-byte ABI selector from the interface function signature.
	types := make([]string, 0, len(sig.Params))
	for _, p := range sig.Params {
		types = append(types, normalizeSelectorType(p.Type))
	}
	sigStr := fmt.Sprintf("%s(%s)", fnName, strings.Join(types, ","))
	selHex := selectorHexFromSignature(sigStr) // "0xAABBCCDD"

	selExpr := withLineExpr(&luast.StringExpr{Value: selHex})

	// Build ABI-encoded arguments.
	// If no args: data is "0x". Otherwise: __tol_abi_encode_v2(v1, "type1", v2, "type2", ...)
	var dataExpr luast.Expr
	if len(e.Args) == 0 {
		dataExpr = withLineExpr(&luast.StringExpr{Value: "0x"})
	} else {
		if len(e.Args) != len(sig.Params) {
			return nil, true, fmt.Errorf("[%s] interface call %s.%s: expected %d args, got %d",
				diag.CodeLowerUnsupportedFeature, ifaceName, fnName, len(sig.Params), len(e.Args))
		}
		encArgs := make([]luast.Expr, 0, len(e.Args)*2)
		for i, a := range e.Args {
			if a == nil {
				return nil, true, fmt.Errorf("[%s] interface call argument cannot be nil", diag.CodeLowerUnsupportedFeature)
			}
			argExpr, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, true, err
			}
			encArgs = append(encArgs, argExpr)
			encArgs = append(encArgs, withLineExpr(&luast.StringExpr{Value: normalizeSelectorType(sig.Params[i].Type)}))
		}
		dataExpr = withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_abi_encode_v2"}),
			Args:      encArgs,
			AdjustRet: false,
		})
	}

	addrExpr := withLineExpr(&luast.IdentExpr{Value: varName})
	// Package-aware routing: use __tol_host_package_call if contractName is known.
	if contractName, ok := ctx.env.contractByInterface[ifaceName]; ok && contractName != "" {
		return withLineExpr(&luast.FuncCallExpr{
			Func: withLineExpr(&luast.IdentExpr{Value: "__tol_host_package_call"}),
			Args: []luast.Expr{
				addrExpr,
				withLineExpr(&luast.StringExpr{Value: contractName}),
				selExpr,
				dataExpr,
			},
			AdjustRet: true,
		}), true, nil
	}
	// Fallback: file-based import without package info → generic external call.
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_host_iface_call"}),
		Args:      []luast.Expr{addrExpr, selExpr, dataExpr},
		AdjustRet: true,
	}), true, nil
}

// lowerLibraryCallExpr lowers a library function call of the form LibName.fn(args)
// to __tol_lib_LibName_fn(args). It only applies when LibName is a known library.
func lowerLibraryCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil || len(ctx.env.libraryByName) == 0 {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	objExpr := stripTolParens(callee.Object)
	if objExpr == nil || objExpr.Kind != "ident" {
		return nil, false, nil
	}
	libName := strings.TrimSpace(objExpr.Value)
	if _, isLib := ctx.env.libraryByName[libName]; !isLib {
		return nil, false, nil
	}
	fnName := strings.TrimSpace(callee.Member)
	if fnName == "" {
		return nil, true, fmt.Errorf("[%s] library call target function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	luaFuncName := libraryFuncLuaName(libName, fnName)
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] library call argument cannot be nil", diag.CodeLowerUnsupportedFeature)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: luaFuncName}),
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

func abiDecodeCallDataArg(e *tolast.Expr) (*tolast.Expr, bool) {
	root := stripTolParens(e)
	if root == nil || root.Kind != "call" {
		return nil, false
	}
	callee := stripTolParens(root.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false
	}
	obj := stripTolParens(callee.Object)
	if obj == nil || obj.Kind != "ident" || strings.TrimSpace(obj.Value) != "abi" {
		return nil, false
	}
	if strings.TrimSpace(callee.Member) != "decode" {
		return nil, false
	}
	if len(root.Args) != 1 || root.Args[0] == nil {
		return nil, false
	}
	return root.Args[0], true
}

// lowerLetTupleStmt lowers: let (a, b, ...) : (T1, T2, ...) = expr;
//
// When the RHS is a 1-arg abi.decode call AND type annotations are provided,
// it expands to:
//
//	local a, b, ... = __tol_abi_decode_tuple(data, "T1", "T2", ...)
//
// For general function calls it expands to:
//
//	local a, b, ... = fn(args)
func lowerLetTupleStmt(ctx *loweringCtx, stmt tolast.Statement) (luast.Stmt, error) {
	if len(stmt.Names) == 0 {
		return nil, fmt.Errorf("[%s] let-tuple binding has no variable names", diag.CodeLowerUnsupportedFeature)
	}
	if stmt.Expr == nil {
		return nil, fmt.Errorf("[%s] let-tuple binding requires an initializer expression", diag.CodeLowerUnsupportedFeature)
	}

	var rhsExpr luast.Expr

	// Check if the RHS is a 1-arg abi.decode call with tuple type annotations.
	if dataArg, ok := abiDecodeCallDataArg(stmt.Expr); ok && len(stmt.Types) > 0 {
		// Validate counts match.
		if len(stmt.Types) != len(stmt.Names) {
			return nil, fmt.Errorf("[%s] abi.decode tuple binding: %d variable(s) but %d type(s)", diag.CodeLowerUnsupportedFeature, len(stmt.Names), len(stmt.Types))
		}
		// Lower: __tol_abi_decode_tuple(data, "T1", "T2", ...)
		dataLuaExpr, err := tolExprToLua(ctx, dataArg)
		if err != nil {
			return nil, err
		}
		decodeArgs := make([]luast.Expr, 0, 1+len(stmt.Types))
		decodeArgs = append(decodeArgs, dataLuaExpr)
		for _, typ := range stmt.Types {
			typeTag := normalizeSelectorType(typ)
			if !isSupportedABIDecodeTargetTypeWithStructs(typeTag, ctx.env.structFields) {
				return nil, fmt.Errorf("[%s] abi.decode tuple binding unsupported target type '%s' in current stage", diag.CodeLowerUnsupportedFeature, typeTag)
			}
			decodeArgs = append(decodeArgs, withLineExpr(&luast.StringExpr{Value: typeTag}))
		}
		rhsExpr = withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_abi_decode_tuple"}),
			Args:      decodeArgs,
			AdjustRet: false, // Preserve all return values
		})
	} else {
		// General multi-return call: local a, b = fn(args)
		ex, err := tolExprToLua(ctx, stmt.Expr)
		if err != nil {
			return nil, err
		}
		// Ensure the call returns all values without adjustment.
		if fc, ok := ex.(*luast.FuncCallExpr); ok {
			fc.AdjustRet = false
		}
		rhsExpr = ex
	}

	// Build local names list.
	names := make([]string, 0, len(stmt.Names))
	for _, n := range stmt.Names {
		names = append(names, strings.TrimSpace(n))
	}

	out := withLineStmt(&luast.LocalAssignStmt{
		Names: names,
		Exprs: []luast.Expr{rhsExpr},
	}, stmt.Line)

	// Register all names in the local scope with their types.
	for i, name := range names {
		typ := ""
		if i < len(stmt.Types) {
			typ = normalizeSelectorType(stmt.Types[i])
		}
		ctx.declareLocalWithType(name, typ)
	}

	return out, nil
}

func isSupportedABIDecodeTargetType(typeName string) bool {
	return isSupportedABIDecodeTargetTypeWithStructs(typeName, nil)
}

func isSupportedABIDecodeTargetTypeWithStructs(typeName string, structFields map[string][]string) bool {
	switch typeName {
	case "bool", "address", "bytes":
		return true
	}
	if strings.HasPrefix(typeName, "bytes") {
		nStr := typeName[len("bytes"):]
		if nStr != "" {
			n, err := strconv.Atoi(nStr)
			return err == nil && n >= 1 && n <= 32
		}
	}
	// uN (unsigned integers)
	if len(typeName) >= 2 && typeName[0] == 'u' {
		n, err := strconv.Atoi(typeName[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	// iN (signed integers)
	if len(typeName) >= 2 && typeName[0] == 'i' {
		n, err := strconv.Atoi(typeName[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	// T[] or T[N] arrays
	if strings.HasSuffix(typeName, "]") {
		return true
	}
	if len(structFields) > 0 {
		if _, isStruct := structFields[typeName]; isStruct {
			return true
		}
	}
	return false
}

// lowerBytesStringConcatCallExpr lowers bytes.concat(a, b, ...) and string.concat(a, b, ...)
// to __tol_bytes_concat(a, b, ...) and __tol_str_concat(a, b, ...) respectively.
func lowerBytesStringConcatCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	obj := stripTolParens(callee.Object)
	if obj == nil || obj.Kind != "ident" {
		return nil, false, nil
	}
	ns := strings.TrimSpace(obj.Value)
	var hostFn string
	switch ns {
	case "bytes":
		if strings.TrimSpace(callee.Member) != "concat" {
			return nil, true, fmt.Errorf("[%s] unsupported bytes builtin '%s.%s'; only bytes.concat is supported", diag.CodeLowerUnsupportedFeature, ns, callee.Member)
		}
		hostFn = "__tol_bytes_concat"
	case "string":
		if strings.TrimSpace(callee.Member) != "concat" {
			return nil, true, fmt.Errorf("[%s] unsupported string builtin '%s.%s'; only string.concat is supported", diag.CodeLowerUnsupportedFeature, ns, callee.Member)
		}
		hostFn = "__tol_str_concat"
	default:
		return nil, false, nil
	}
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] %s.concat argument cannot be nil", diag.CodeLowerUnsupportedFeature, ns)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: hostFn}),
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

// lowerDynLengthMemberExpr lowers expr.length on non-storage bytes/string expressions
// to __tol_dyn_length(expr). It only fires when the object is not a storage path.
func lowerDynLengthMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" || strings.TrimSpace(e.Member) != "length" {
		return nil, false, nil
	}
	// Only handle non-storage expressions here; storage arrays are handled by lowerStorageLengthMemberExpr.
	if _, _, ok := ctx.storagePathFromExpr(e.Object); ok {
		return nil, false, nil
	}
	obj, err := tolExprToLua(ctx, e.Object)
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_dyn_length"}),
		Args:      []luast.Expr{obj},
		AdjustRet: true,
	}), true, nil
}

// lowerTypeMinMaxExpr handles `type(T).min` and `type(T).max` compile-time integer
// bound expressions. The AST pattern is member(call(ident("type"), [ident(T)]), "min"|"max").
//
// All integer values in TOL are stored as LUint256 (two's-complement 256-bit).
// Signed types use two's complement encoding in [0, 2^N):
//   - type(uN).max = 2^N - 1
//   - type(uN).min = 0
//   - type(iN).max = 2^(N-1) - 1
//   - type(iN).min = 2^(N-1)  (= -2^(N-1) in two's complement; stored unsigned)
//
// Returns (expr, true, nil) if matched and valid, (nil, false, nil) if not matched,
// or (nil, true, err) on error.
func lowerTypeMinMaxExpr(_ *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" {
		return nil, false, nil
	}
	bound := strings.TrimSpace(e.Member)
	if bound != "min" && bound != "max" {
		return nil, false, nil
	}
	obj := stripTolParens(e.Object)
	if obj == nil || obj.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(obj.Callee)
	if callee == nil || callee.Kind != "ident" || strings.TrimSpace(callee.Value) != "type" {
		return nil, false, nil
	}
	if len(obj.Args) != 1 || obj.Args[0] == nil {
		return nil, false, nil
	}
	arg := stripTolParens(obj.Args[0])
	if arg == nil || arg.Kind != "ident" {
		return nil, false, nil
	}
	typName := strings.TrimSpace(arg.Value)
	if len(typName) < 2 || (typName[0] != 'u' && typName[0] != 'i') {
		return nil, false, nil
	}
	bits, err := strconv.Atoi(typName[1:])
	if err != nil || bits < 8 || bits > 256 || bits%8 != 0 {
		return nil, false, nil
	}
	var val LUint256
	switch {
	case typName[0] == 'u' && bound == "max":
		// 2^N - 1
		val = lu256Sub(lu256Shl(LUint256One, uint(bits)), LUint256One)
	case typName[0] == 'u' && bound == "min":
		// 0
		val = LUint256Zero
	case typName[0] == 'i' && bound == "max":
		// 2^(N-1) - 1
		val = lu256Sub(lu256Shl(LUint256One, uint(bits-1)), LUint256One)
	default: // iN.min: -2^(N-1) stored as two's complement = 2^(N-1)
		val = lu256Shl(LUint256One, uint(bits-1))
	}
	return withLineExpr(&luast.NumberExpr{Value: val.String()}), true, nil
}

// lowerTypeInterfaceIdExpr handles `type(I).interfaceId` compile-time EIP-165 interface ID
// computation. The AST pattern is member(call(ident("type"), [ident(I)]), "interfaceId").
//
// The interface ID is computed as the XOR of all 4-byte function selectors in the interface.
// Each selector is computed as keccak256(canonical_signature)[0:4] using TOL type names.
//
// Returns (expr, true, nil) if matched and valid, (nil, false, nil) if not matched,
// or (nil, true, err) on error.
func lowerTypeInterfaceIdExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" {
		return nil, false, nil
	}
	if strings.TrimSpace(e.Member) != "interfaceId" {
		return nil, false, nil
	}
	obj := stripTolParens(e.Object)
	if obj == nil || obj.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(obj.Callee)
	if callee == nil || callee.Kind != "ident" || strings.TrimSpace(callee.Value) != "type" {
		return nil, false, nil
	}
	if len(obj.Args) != 1 || obj.Args[0] == nil {
		return nil, false, nil
	}
	arg := stripTolParens(obj.Args[0])
	if arg == nil || arg.Kind != "ident" {
		return nil, false, nil
	}
	ifaceName := strings.TrimSpace(arg.Value)
	if ifaceName == "" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil {
		return nil, true, fmt.Errorf("[%s] type(%s).interfaceId: no lowering context", diag.CodeLowerUnsupportedFeature, ifaceName)
	}
	fns, ok := ctx.env.interfaceByName[ifaceName]
	if !ok {
		return nil, true, fmt.Errorf("[%s] type(%s).interfaceId: unknown interface '%s'", diag.CodeLowerUnsupportedFeature, ifaceName, ifaceName)
	}
	// XOR all 4-byte selectors.
	var xor [4]byte
	for _, fn := range fns {
		types := make([]string, 0, len(fn.Params))
		for _, p := range fn.Params {
			t := normalizeSelectorType(p.Type)
			if t == "" {
				return nil, true, fmt.Errorf("[%s] type(%s).interfaceId: parameter type cannot be empty for '%s'", diag.CodeLowerUnsupportedFeature, ifaceName, fn.Name)
			}
			types = append(types, t)
		}
		sig := fmt.Sprintf("%s(%s)", fn.Name, strings.Join(types, ","))
		sel := selectorHexFromSignature(sig)
		// sel is "0x" + 8 hex chars — decode 4 bytes.
		selBytes, err := hex.DecodeString(sel[2:])
		if err != nil {
			return nil, true, fmt.Errorf("[%s] type(%s).interfaceId: invalid selector hex for '%s': %w", diag.CodeLowerUnsupportedFeature, ifaceName, fn.Name, err)
		}
		xor[0] ^= selBytes[0]
		xor[1] ^= selBytes[1]
		xor[2] ^= selBytes[2]
		xor[3] ^= selBytes[3]
	}
	result := "0x" + hex.EncodeToString(xor[:])
	return withLineExpr(&luast.StringExpr{Value: result}), true, nil
}

// lowerBytesSliceExpr lowers expr[start:end] (slice) to __tol_bytes_slice(expr, start, end).
func lowerBytesSliceExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "slice" {
		return nil, false, nil
	}
	if len(e.Args) != 2 {
		return nil, true, fmt.Errorf("[%s] slice expression requires exactly 2 bound arguments", diag.CodeLowerUnsupportedFeature)
	}
	obj, err := tolExprToLua(ctx, e.Object)
	if err != nil {
		return nil, true, err
	}
	start, err := tolExprToLua(ctx, e.Args[0])
	if err != nil {
		return nil, true, err
	}
	end, err := tolExprToLua(ctx, e.Args[1])
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_bytes_slice"}),
		Args:      []luast.Expr{obj, start, end},
		AdjustRet: true,
	}), true, nil
}

// lowerMemArrayIndexExpr lowers memory array index read: arr[i] → __tol_array_get(arr, i).
// Fires only when the object is a local declared with `new T[](n)` (type tag starts with "__mem:").
// Returns (expr, true, nil) if handled, (nil, false, nil) if not a memory array index.
func lowerMemArrayIndexExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "index" || e.Object == nil || e.Index == nil {
		return nil, false, nil
	}
	// Only handle when the object type is a memory array (tagged with "__mem:" prefix).
	objType := inferExprType(ctx, e.Object)
	if !strings.HasPrefix(objType, "__mem:") {
		return nil, false, nil
	}
	obj, err := tolExprToLua(ctx, e.Object)
	if err != nil {
		return nil, true, err
	}
	idx, err := tolExprToLua(ctx, e.Index)
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_array_get"}),
		Args:      []luast.Expr{obj, idx},
		AdjustRet: true,
	}), true, nil
}

// lowerMemArrayStoreStmt lowers a bounds-checked memory array write:
//   set arr[i] = v  →  __tol_array_set(arr, i, v)  (FuncCallStmt)
// Fires only when stmt.Target is an index expression on a memory array local (type "__mem:*").
// Returns (stmt, true, nil) if handled, (nil, false, nil) if not a memory array store.
func lowerMemArrayStoreStmt(ctx *loweringCtx, s tolast.Statement, target *tolast.Expr, valExpr *tolast.Expr) (luast.Stmt, bool, error) {
	if target == nil || target.Kind != "index" || target.Object == nil || target.Index == nil {
		return nil, false, nil
	}
	objType := inferExprType(ctx, target.Object)
	if !strings.HasPrefix(objType, "__mem:") {
		return nil, false, nil
	}
	obj, err := tolExprToLua(ctx, target.Object)
	if err != nil {
		return nil, true, err
	}
	idx, err := tolExprToLua(ctx, target.Index)
	if err != nil {
		return nil, true, err
	}
	val, err := tolExprToLua(ctx, valExpr)
	if err != nil {
		return nil, true, err
	}
	callExpr := &luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_array_set"}),
		Args:      []luast.Expr{obj, idx, val},
		AdjustRet: true,
	}
	return withLineStmt(&luast.FuncCallStmt{Expr: callExpr}, s.Line), true, nil
}

// lowerBytesIndexExpr lowers bytes/string byte-level indexing: expr[i] → __tol_bytes_index(expr, i).
// Fires only when the object is a local of type "bytes" or "string".
// Returns (expr, true, nil) if handled, (nil, false, nil) if not a bytes/string index.
func lowerBytesIndexExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "index" || e.Object == nil || e.Index == nil {
		return nil, false, nil
	}
	// Only handle when the object type is bytes or string.
	objType := inferExprType(ctx, e.Object)
	if objType != "bytes" && objType != "string" {
		return nil, false, nil
	}
	obj, err := tolExprToLua(ctx, e.Object)
	if err != nil {
		return nil, true, err
	}
	idx, err := tolExprToLua(ctx, e.Index)
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_bytes_index"}),
		Args:      []luast.Expr{obj, idx},
		AdjustRet: true,
	}), true, nil
}

func lowerABIBuiltinExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "member" {
		return nil, false, nil
	}
	obj := stripTolParens(callee.Object)
	if obj == nil || obj.Kind != "ident" || strings.TrimSpace(obj.Value) != "abi" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Member)
	rewriteEncodeWithSelector := false
	rewriteSelectorValue := ""
	switch name {
	case "encode", "encodePacked":
		// Rewritten below to typed variants (__tol_abi_encode_v2 / __tol_abi_encode_packed_v2).
	case "encodeWithSelector":
		if len(e.Args) < 1 {
			return nil, true, fmt.Errorf("[%s] abi.encodeWithSelector requires at least selector argument", diag.CodeLowerUnsupportedFeature)
		}
		if e.Args[0] == nil {
			return nil, true, fmt.Errorf("[%s] abi.encodeWithSelector selector argument cannot be nil", diag.CodeLowerUnsupportedFeature)
		}
		if literal := stripTolParens(e.Args[0]); literal != nil && literal.Kind == "string" {
			if !isHexSelectorLiteral(unquoteIfNeeded(literal.Value)) {
				return nil, true, fmt.Errorf("[%s] abi.encodeWithSelector selector string literal must be 0x followed by 8 hex chars", diag.CodeLowerUnsupportedFeature)
			}
		}
	case "encodeWithSignature":
		if len(e.Args) < 1 {
			return nil, true, fmt.Errorf("[%s] abi.encodeWithSignature requires signature string literal first argument", diag.CodeLowerUnsupportedFeature)
		}
		first := stripTolParens(e.Args[0])
		if first == nil || first.Kind != "string" {
			return nil, true, fmt.Errorf("[%s] abi.encodeWithSignature requires signature string literal first argument", diag.CodeLowerUnsupportedFeature)
		}
		sig := unquoteIfNeeded(first.Value)
		if !isCanonicalSelectorSignature(sig) {
			return nil, true, fmt.Errorf("[%s] abi.encodeWithSignature requires canonical signature literal 'name(type1,type2,...)'", diag.CodeLowerUnsupportedFeature)
		}
		rewriteEncodeWithSelector = true
		rewriteSelectorValue = selectorHexFromSignature(sig)
	case "decode":
		if len(e.Args) != 1 {
			return nil, true, fmt.Errorf("[%s] abi.decode requires exactly one argument in current stage", diag.CodeLowerUnsupportedFeature)
		}
		if e.Args[0] == nil {
			return nil, true, fmt.Errorf("[%s] abi.decode argument cannot be nil", diag.CodeLowerUnsupportedFeature)
		}
		if literal := stripTolParens(e.Args[0]); literal != nil && literal.Kind == "string" {
			if !isEvenHexBytesLiteral(unquoteIfNeeded(literal.Value)) {
				return nil, true, fmt.Errorf("[%s] abi.decode string literal must be 0x-prefixed even-length hex bytes", diag.CodeLowerUnsupportedFeature)
			}
		}
	default:
		return nil, true, fmt.Errorf("[%s] unsupported abi builtin '%s' in current stage", diag.CodeLowerUnsupportedFeature, name)
	}

	if rewriteEncodeWithSelector {
		args := make([]luast.Expr, 0, len(e.Args))
		args = append(args, withLineExpr(&luast.StringExpr{Value: rewriteSelectorValue}))
		for i := 1; i < len(e.Args); i++ {
			a := e.Args[i]
			if a == nil {
				return nil, true, fmt.Errorf("[%s] abi builtin argument cannot be nil", diag.CodeLowerUnsupportedFeature)
			}
			ex, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, true, err
			}
			args = append(args, ex)
		}
		luaCallee := withLineExpr(&luast.AttrGetExpr{
			Object: withLineExpr(&luast.IdentExpr{Value: "abi"}),
			Key:    withLineExpr(&luast.StringExpr{Value: "encodeWithSelector"}),
		})
		return withLineExpr(&luast.FuncCallExpr{
			Func:      luaCallee,
			Args:      args,
			AdjustRet: true,
		}), true, nil
	}

	// abi.encode and abi.encodePacked: rewrite to typed variants with type info interleaved.
	if name == "encode" || name == "encodePacked" {
		luaFnName := "__tol_abi_encode_v2"
		if name == "encodePacked" {
			luaFnName = "__tol_abi_encode_packed_v2"
		}
		args := make([]luast.Expr, 0, len(e.Args)*2)
		for _, a := range e.Args {
			if a == nil {
				return nil, true, fmt.Errorf("[%s] abi builtin argument cannot be nil", diag.CodeLowerUnsupportedFeature)
			}
			ex, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, true, err
			}
			args = append(args, ex)
			// Append the TOL type of this argument (best-effort; "" for unknown).
			typ := inferExprType(ctx, a)
			typ = normalizeSelectorType(typ)
			args = append(args, withLineExpr(&luast.StringExpr{Value: typ}))
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: luaFnName}),
			Args:      args,
			AdjustRet: true,
		}), true, nil
	}

	luaCallee, err := tolExprToLua(ctx, e.Callee)
	if err != nil {
		return nil, true, err
	}
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] abi builtin argument cannot be nil", diag.CodeLowerUnsupportedFeature)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      luaCallee,
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

func lowerSelectorBuiltinExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" || strings.TrimSpace(callee.Value) != "selector" {
		return nil, false, nil
	}
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] selector(...) requires exactly one argument", diag.CodeLowerUnsupportedFeature)
	}
	arg := e.Args[0]
	if arg == nil {
		return nil, true, fmt.Errorf("[%s] selector(...) requires a non-nil argument", diag.CodeLowerUnsupportedFeature)
	}
	if arg.Kind == "string" {
		sig := unquoteIfNeeded(arg.Value)
		return withLineExpr(&luast.StringExpr{Value: selectorHexFromSignature(sig)}), true, nil
	}
	ex, err := tolExprToLua(ctx, arg)
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_selector"}),
		Args:      []luast.Expr{ex},
		AdjustRet: true,
	}), true, nil
}

func lowerCustomErrorRevertExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	root := stripTolParens(e)
	if root == nil || root.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(root.Callee)
	if callee == nil || callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)
	if name == "" || name == "selector" {
		return nil, false, nil
	}

	// Lower args.
	args := make([]luast.Expr, 0, len(root.Args))
	for _, a := range root.Args {
		arg, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, arg)
	}

	// If we have the error ABI signature, emit abi.encodeWithSignature(sig, args...).
	// This produces the standard EVM custom error encoding: 4-byte selector + ABI args.
	if ctx != nil && ctx.env != nil {
		if sig, ok := ctx.env.errorSigByName[name]; ok {
			callArgs := make([]luast.Expr, 0, 1+len(args))
			callArgs = append(callArgs, withLineExpr(&luast.StringExpr{Value: sig}))
			callArgs = append(callArgs, args...)
			return withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: "abi.encodeWithSignature"}),
				Args:      callArgs,
				AdjustRet: true,
			}), true, nil
		}
	}

	// Fallback: format as human-readable string (when error is not declared or no env).
	msg := luast.Expr(withLineExpr(&luast.StringExpr{Value: name + "("}))
	for i, arg := range args {
		if i > 0 {
			msg = withLineExpr(&luast.StringConcatOpExpr{
				Lhs: msg,
				Rhs: withLineExpr(&luast.StringExpr{Value: ","}),
			})
		}
		asString := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "tostring"}),
			Args:      []luast.Expr{arg},
			AdjustRet: true,
		})
		msg = withLineExpr(&luast.StringConcatOpExpr{
			Lhs: msg,
			Rhs: asString,
		})
	}
	msg = withLineExpr(&luast.StringConcatOpExpr{
		Lhs: msg,
		Rhs: withLineExpr(&luast.StringExpr{Value: ")"}),
	})
	return msg, true, nil
}

func stripTolParens(e *tolast.Expr) *tolast.Expr {
	cur := e
	for cur != nil && cur.Kind == "paren" {
		cur = cur.Left
	}
	return cur
}

func lowerSelectorMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" || e.Member != "selector" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil {
		return nil, true, fmt.Errorf("[%s] selector member expression requires contract lowering context", diag.CodeLowerUnsupportedFeature)
	}
	fnRef := e.Object
	if fnRef == nil || fnRef.Kind != "member" || fnRef.Object == nil || fnRef.Object.Kind != "ident" {
		return nil, true, fmt.Errorf("[%s] selector member expression must be 'this.fn.selector' or 'Contract.fn.selector'", diag.CodeLowerUnsupportedFeature)
	}
	scope := strings.TrimSpace(fnRef.Object.Value)
	switch scope {
	case "this":
		// ok
	case ctx.env.contractName:
		// ok
	default:
		return nil, true, fmt.Errorf("[%s] selector scope '%s' is unsupported (expected 'this' or '%s')", diag.CodeLowerUnsupportedFeature, scope, ctx.env.contractName)
	}

	fnName := strings.TrimSpace(fnRef.Member)
	if fnName == "" {
		return nil, true, fmt.Errorf("[%s] selector target function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	sel, ok := ctx.env.selectorByFunction[fnName]
	if !ok {
		return nil, true, fmt.Errorf("[%s] selector target '%s' is not externally dispatchable in current stage", diag.CodeLowerUnsupportedFeature, fnName)
	}
	return withLineExpr(&luast.StringExpr{Value: sel}), true, nil
}

// buildHashSlotExpr builds the final Lua expression for a storage slot access.
// For scalars: the compile-time constant ident (__tol_s_<name>).
// For mappings: a chain of __tol_mkey calls, one per key in order.
// For arrays with one index: __tol_arr_elem(base, idx).
// For nested types (arr[][], mapping=>arr[], etc.): walks the type chain,
// applying __tol_mkey for mapping levels and __tol_arr_elem for array levels.
func buildHashSlotExpr(ctx *loweringCtx, info storageSlotInfo, keys []*tolast.Expr) (luast.Expr, error) {
	base := luast.Expr(withLineExpr(&luast.IdentExpr{Value: info.luaConstName}))
	if len(keys) == 0 {
		return base, nil
	}
	// Walk the type chain applying each key in sequence.
	cur := base
	curType := info.typ
	for i, k := range keys {
		compact := strings.ReplaceAll(normalizeSelectorType(curType), " ", "")
		kExpr, err := tolExprToLua(ctx, k)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(compact, "mapping(") {
			// mapping level: __tol_mkey(key, cur_base)
			cur = withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_mkey"}),
				Args:      []luast.Expr{kExpr, cur},
				AdjustRet: true,
			})
			curType = mappingValueType(curType)
		} else if strings.HasSuffix(compact, "]") {
			// array level: __tol_arr_elem(cur_base, idx)
			cur = withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_arr_elem"}),
				Args:      []luast.Expr{cur, kExpr},
				AdjustRet: true,
			})
			curType = arrayElemType(curType)
		} else {
			return nil, fmt.Errorf("[%s] storage slot '%s' indexed too deeply at index %d (type '%s' is scalar)", diag.CodeLowerUnsupportedFeature, info.name, i, curType)
		}
	}
	return cur, nil
}

func lowerStorageStoreStmt(ctx *loweringCtx, target *tolast.Expr, valueExpr *tolast.Expr) (luast.Stmt, bool, error) {
	slotName, keys, ok := ctx.storagePathFromExpr(target)
	if !ok {
		return nil, false, nil
	}
	info, _ := ctx.storageInfoByName(slotName)
	if err := validateStorageKeyShape(info, keys, "set"); err != nil {
		return nil, true, err
	}
	value, err := tolExprToLua(ctx, valueExpr)
	if err != nil {
		return nil, true, err
	}
	slotExpr, err := buildHashSlotExpr(ctx, info, keys)
	if err != nil {
		return nil, true, err
	}
	storeFn := "__tol_sstore"
	if info.isTransient {
		storeFn = "__tol_tsstore"
	}
	call := withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: storeFn}),
		Args:      []luast.Expr{slotExpr, value},
		AdjustRet: true,
	})
	return withLineStmt(&luast.FuncCallStmt{Expr: call}, 1), true, nil
}

func lowerStorageLoadExpr(ctx *loweringCtx, slotName string, keys []*tolast.Expr) (luast.Expr, error) {
	info, _ := ctx.storageInfoByName(slotName)
	if err := validateStorageKeyShape(info, keys, "read"); err != nil {
		return nil, err
	}
	slotExpr, err := buildHashSlotExpr(ctx, info, keys)
	if err != nil {
		return nil, err
	}
	loadFn := "__tol_sload"
	if info.isTransient {
		loadFn = "__tol_tsload"
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: loadFn}),
		Args:      []luast.Expr{slotExpr},
		AdjustRet: true,
	}), nil
}

func lowerStorageLengthMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" || e.Member != "length" {
		return nil, false, nil
	}
	slotName, keys, ok := ctx.storagePathFromExpr(e.Object)
	if !ok {
		return nil, false, nil
	}
	info, _ := ctx.storageInfoByName(slotName)
	// Determine the type after applying any collected keys.
	curType := info.typ
	for range keys {
		curType = slotTypeAfterIndex(curType)
		if curType == "" {
			return nil, true, fmt.Errorf("[%s] '.length' applied to non-indexable type on slot '%s'", diag.CodeLowerUnsupportedFeature, info.name)
		}
	}
	// The resulting type must be an array.
	compact := strings.ReplaceAll(normalizeSelectorType(curType), " ", "")
	if !strings.HasSuffix(compact, "]") {
		return nil, true, fmt.Errorf("[%s] '.length' is only supported on storage arrays (slot '%s')", diag.CodeLowerUnsupportedFeature, info.name)
	}
	// Compute the base address for the (possibly nested) array.
	baseExpr, err := buildHashSlotExpr(ctx, info, keys)
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_slen"}),
		Args:      []luast.Expr{baseExpr},
		AdjustRet: true,
	}), true, nil
}

func lowerStoragePushCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" || e.Callee == nil || e.Callee.Kind != "member" || e.Callee.Member != "push" {
		return nil, false, nil
	}
	slotName, keys, ok := ctx.storagePathFromExpr(e.Callee.Object)
	if !ok {
		return nil, false, nil
	}
	info, _ := ctx.storageInfoByName(slotName)
	// Determine the type after applying any collected keys.
	curType := info.typ
	for range keys {
		curType = slotTypeAfterIndex(curType)
		if curType == "" {
			return nil, true, fmt.Errorf("[%s] '.push(v)' applied to non-indexable type on slot '%s'", diag.CodeLowerUnsupportedFeature, info.name)
		}
	}
	// The resulting type must be an array.
	compact := strings.ReplaceAll(normalizeSelectorType(curType), " ", "")
	if !strings.HasSuffix(compact, "]") {
		return nil, true, fmt.Errorf("[%s] '.push(v)' is only supported on storage arrays (slot '%s')", diag.CodeLowerUnsupportedFeature, info.name)
	}
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] storage array push requires exactly one argument", diag.CodeLowerUnsupportedFeature)
	}
	val, err := tolExprToLua(ctx, e.Args[0])
	if err != nil {
		return nil, true, err
	}
	// Compute the base address for the (possibly nested) array.
	baseExpr, err := buildHashSlotExpr(ctx, info, keys)
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func: withLineExpr(&luast.IdentExpr{Value: "__tol_spush"}),
		Args: []luast.Expr{
			baseExpr,
			val,
		},
		AdjustRet: true,
	}), true, nil
}

func validateStorageKeyShape(info storageSlotInfo, keys []*tolast.Expr, action string) error {
	maxDepth := slotMaxIndexDepth(info.typ)
	switch info.kind {
	case storageKindScalar:
		if len(keys) > 0 {
			return fmt.Errorf("[%s] storage slot '%s' of type '%s' does not support indexed %s", diag.CodeLowerUnsupportedFeature, info.name, info.typ, action)
		}
		return nil
	case storageKindMapping:
		if maxDepth <= 0 {
			maxDepth = 1
		}
		if len(keys) != maxDepth {
			return fmt.Errorf("[%s] storage mapping slot '%s' requires exactly %d index key(s), got %d", diag.CodeLowerUnsupportedFeature, info.name, maxDepth, len(keys))
		}
		return nil
	case storageKindArray:
		if len(keys) == 0 || len(keys) > maxDepth {
			return fmt.Errorf("[%s] storage array slot '%s' %s requires 1..%d index key(s), got %d", diag.CodeLowerUnsupportedFeature, info.name, action, maxDepth, len(keys))
		}
		return nil
	default:
		return fmt.Errorf("[%s] unsupported storage slot kind for '%s'", diag.CodeLowerUnsupportedFeature, info.name)
	}
}

// =============================================================================
// Signed integer type helpers (TOL M2)
// =============================================================================

// isSignedIntType returns true if typName is an iN type (i8..i256).
func isSignedIntType(typName string) bool {
	t := strings.TrimSpace(typName)
	if len(t) < 2 || t[0] != 'i' {
		return false
	}
	n, err := strconv.Atoi(t[1:])
	return err == nil && n >= 8 && n <= 256 && n%8 == 0
}

// signedIntBits returns the bit width N for an iN type. Returns 0 for non-iN types.
func signedIntBits(typName string) int {
	t := strings.TrimSpace(typName)
	if len(t) < 2 || t[0] != 'i' {
		return 0
	}
	n, err := strconv.Atoi(t[1:])
	if err != nil || n < 8 || n > 256 || n%8 != 0 {
		return 0
	}
	return n
}

// unsignedIntBits returns the bit width N for a uN type. Returns 0 for non-uN types.
// Returns 256 for "u256".
func unsignedIntBits(typName string) int {
	t := strings.TrimSpace(typName)
	if len(t) < 2 || t[0] != 'u' {
		return 0
	}
	n, err := strconv.Atoi(t[1:])
	if err != nil || n < 8 || n > 256 || n%8 != 0 {
		return 0
	}
	return n
}

// isTypeCastCallName returns true if name is a TOL type cast like u256, i8, bool, address, etc.
func isTypeCastCallName(name string) bool {
	n := strings.TrimSpace(name)
	switch n {
	case "bool", "address", "bytes", "string":
		return true
	}
	if strings.HasPrefix(n, "bytes") {
		bits, err := strconv.Atoi(n[5:])
		return err == nil && bits >= 1 && bits <= 32
	}
	if len(n) >= 2 && (n[0] == 'u' || n[0] == 'i') {
		bits, err := strconv.Atoi(n[1:])
		return err == nil && bits >= 8 && bits <= 256 && bits%8 == 0
	}
	return false
}

// resolveTypeAlias resolves a user-defined value type name to its underlying
// primitive type. If typName is not a known alias, it is returned unchanged.
// Performs one level of alias resolution (aliases of aliases are not supported
// in the current implementation).
func resolveTypeAlias(ctx *loweringCtx, typName string) string {
	if ctx == nil || ctx.env == nil || len(ctx.env.typeAliases) == 0 {
		return typName
	}
	if underlying, ok := ctx.env.typeAliases[typName]; ok {
		return underlying
	}
	return typName
}

// inferExprType infers the TOL type of an expression based on context.
// Returns "" if the type cannot be determined.
// This is a best-effort type inference used only for signed arithmetic routing.
func inferExprType(ctx *loweringCtx, e *tolast.Expr) string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case "paren":
		return inferExprType(ctx, e.Left)
	case "ident":
		return resolveTypeAlias(ctx, ctx.typeOfLocal(e.Value))
	case "call":
		// Check if it's a type cast: iN(expr) or uN(expr)
		callee := stripTolParens(e.Callee)
		if callee != nil && callee.Kind == "ident" {
			name := strings.TrimSpace(callee.Value)
			if isTypeCastCallName(name) {
				return name
			}
			// Also handle user-defined type casts: MyInt(expr) → underlying type.
			return resolveTypeAlias(ctx, name)
		}
		return ""
	case "member":
		// type(T).min / type(T).max: infer type as T.
		bound := strings.TrimSpace(e.Member)
		if bound == "min" || bound == "max" {
			obj := stripTolParens(e.Object)
			if obj != nil && obj.Kind == "call" {
				callee := stripTolParens(obj.Callee)
				if callee != nil && callee.Kind == "ident" && strings.TrimSpace(callee.Value) == "type" {
					if len(obj.Args) == 1 && obj.Args[0] != nil {
						arg := stripTolParens(obj.Args[0])
						if arg != nil && arg.Kind == "ident" {
							return strings.TrimSpace(arg.Value)
						}
					}
				}
			}
		}
		return ""
	case "binary":
		// For binary ops, infer from left operand (both must be same type by sema rules).
		lt := inferExprType(ctx, e.Left)
		if lt != "" {
			return lt
		}
		return inferExprType(ctx, e.Right)
	case "unary":
		return inferExprType(ctx, e.Right)
	case "number":
		// Untyped literal - could be either signed or unsigned, treat as untyped.
		return ""
	default:
		return ""
	}
}

// makeSignedBitsExpr builds a Lua number literal expression for bit width N.
func makeSignedBitsExpr(bits int) luast.Expr {
	return withLineExpr(&luast.NumberExpr{Value: strconv.Itoa(bits)})
}

// lowerSignedTypeCastExpr handles explicit signed type cast calls: iN(expr).
// lowerOverloadedDirectCallExpr handles a direct call fn(args) where `fn` is a contract function
// name with multiple overloads. It resolves the correct mangled Lua name by argument count.
// Returns (expr, true, nil) if handled (i.e. overloads exist), (nil, false, nil) if not overloaded.
func lowerOverloadedDirectCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)
	if name == "" {
		return nil, false, nil
	}
	resolved := resolveOverloadedLuaName(ctx.env, name, len(e.Args))
	if resolved == "" {
		return nil, false, nil
	}
	args := make([]luast.Expr, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			return nil, true, fmt.Errorf("[%s] overloaded call argument cannot be nil", diag.CodeLowerUnsupportedFeature)
		}
		ex, err := tolExprToLua(ctx, a)
		if err != nil {
			return nil, true, err
		}
		args = append(args, ex)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: resolved}),
		Args:      args,
		AdjustRet: true,
	}), true, nil
}

// Returns (expr, true, nil) if handled, (nil, false, nil) if not a signed cast,
// or (nil, true, err) on error.
func lowerSignedTypeCastExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)
	bits := signedIntBits(name)
	if bits == 0 {
		return nil, false, nil
	}
	// Contract-defined functions shadow type-cast names.
	if ctx != nil && ctx.env != nil {
		if _, exists := ctx.env.functionByName[name]; exists {
			return nil, false, nil
		}
	}
	// It's a signed type cast: iN(expr)
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] type cast %s(...) requires exactly 1 argument", diag.CodeLowerUnsupportedFeature, name)
	}
	inner, err := tolExprToLua(ctx, e.Args[0])
	if err != nil {
		return nil, true, err
	}
	// Emit: __tol_signed_trunc(inner, bits)
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_trunc"}),
		Args:      []luast.Expr{inner, makeSignedBitsExpr(bits)},
		AdjustRet: true,
	}), true, nil
}

// lowerSignedBinaryExpr handles binary operations on signed integer types.
// Returns (expr, true, nil) if handled as signed, (nil, false, nil) if not signed,
// or (nil, true, err) on error.
func lowerSignedBinaryExpr(ctx *loweringCtx, e *tolast.Expr, lhs, rhs luast.Expr) (luast.Expr, bool, error) {
	if e == nil {
		return nil, false, nil
	}
	// Infer the signed type from the operands.
	typStr := inferExprType(ctx, e.Left)
	if typStr == "" {
		typStr = inferExprType(ctx, e.Right)
	}
	bits := signedIntBits(typStr)
	if bits == 0 {
		// Not a known signed type - fall through to default (unsigned) lowering.
		return nil, false, nil
	}
	bitsExpr := makeSignedBitsExpr(bits)

	// Use checked variants for +, -, * unless inside unchecked {}.
	checked := ctx == nil || !ctx.unchecked
	switch e.Op {
	case "+":
		fnName := "__tol_signed_add"
		if checked {
			fnName = "__tol_signed_checked_add"
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: fnName}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case "-":
		fnName := "__tol_signed_sub"
		if checked {
			fnName = "__tol_signed_checked_sub"
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: fnName}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case "*":
		fnName := "__tol_signed_mul"
		if checked {
			fnName = "__tol_signed_checked_mul"
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: fnName}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case "/":
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_div"}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case "%":
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_mod"}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case "<":
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_lt"}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case ">":
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_gt"}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case "<=":
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_le"}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	case ">=":
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_ge"}),
			Args:      []luast.Expr{lhs, rhs, bitsExpr},
			AdjustRet: true,
		}), true, nil
	default:
		// Other ops (bitwise, ==, !=) fall through to default lowering.
		return nil, false, nil
	}
}

// lowerCheckedUintBinaryExpr handles +, -, * for unsigned integer types with overflow checks.
// Returns (expr, true, nil) when the expression type is a known uN type,
// (nil, false, nil) when type is unknown (caller falls back to unchecked).
func lowerCheckedUintBinaryExpr(ctx *loweringCtx, e *tolast.Expr, lhs, rhs luast.Expr) (luast.Expr, bool, error) {
	if e == nil {
		return nil, false, nil
	}
	typStr := inferExprType(ctx, e.Left)
	if typStr == "" {
		typStr = inferExprType(ctx, e.Right)
	}
	bits := unsignedIntBits(typStr)
	if bits == 0 {
		return nil, false, nil
	}
	bitsExpr := withLineExpr(&luast.NumberExpr{Value: strconv.Itoa(bits)})
	var fnName string
	switch e.Op {
	case "+":
		fnName = "__tol_uint_checked_add"
	case "-":
		fnName = "__tol_uint_checked_sub"
	case "*":
		fnName = "__tol_uint_checked_mul"
	default:
		return nil, false, nil
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: fnName}),
		Args:      []luast.Expr{lhs, rhs, bitsExpr},
		AdjustRet: true,
	}), true, nil
}

// lowerUintTypeCastExpr handles explicit unsigned integer type cast calls: uN(expr).
// Truncates the value to N bits (mod 2^N). u256(expr) is a no-op for well-formed values.
// Returns (expr, true, nil) if handled, (nil, false, nil) if not an unsigned cast.
func lowerUintTypeCastExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" {
		return nil, false, nil
	}
	name := strings.TrimSpace(callee.Value)
	if len(name) < 2 || name[0] != 'u' {
		return nil, false, nil
	}
	bits, err := strconv.Atoi(name[1:])
	if err != nil || bits < 8 || bits > 256 || bits%8 != 0 {
		return nil, false, nil
	}
	// Contract-defined functions shadow type-cast names.
	if ctx != nil && ctx.env != nil {
		if _, exists := ctx.env.functionByName[name]; exists {
			return nil, false, nil
		}
	}
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] type cast %s(...) requires exactly 1 argument", diag.CodeLowerUnsupportedFeature, name)
	}
	inner, err2 := tolExprToLua(ctx, e.Args[0])
	if err2 != nil {
		return nil, true, err2
	}
	if bits == 256 {
		// u256(expr) is a no-op truncation (values are already in u256 range).
		return inner, true, nil
	}
	// For sub-256 types, truncate: use __tol_signed_trunc but interpret as unsigned.
	// __tol_signed_trunc(a, bits) returns a mod 2^bits in [0, 2^bits) which is correct for uN.
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_trunc"}),
		Args:      []luast.Expr{inner, makeSignedBitsExpr(bits)},
		AdjustRet: true,
	}), true, nil
}

// lowerSignedUnaryNeg handles unary negation on a signed integer type: -expr.
// Returns (expr, true, nil) if handled, (nil, false, nil) if not signed.
func lowerSignedUnaryNeg(ctx *loweringCtx, e *tolast.Expr, inner luast.Expr) (luast.Expr, bool, error) {
	if e == nil {
		return nil, false, nil
	}
	typStr := inferExprType(ctx, e.Right)
	bits := signedIntBits(typStr)
	if bits == 0 {
		return nil, false, nil
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_signed_neg"}),
		Args:      []luast.Expr{inner, makeSignedBitsExpr(bits)},
		AdjustRet: true,
	}), true, nil
}

func withLineExpr[T luast.Expr](e T) T {
	e.SetLine(1)
	e.SetLastLine(1)
	return e
}

func withLineStmt[T luast.Stmt](s T, line int) T {
	if line <= 0 {
		line = 1
	}
	s.SetLine(line)
	s.SetLastLine(line)
	return s
}

func unquoteIfNeeded(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		if uq, err := strconv.Unquote(s); err == nil {
			return uq
		}
		return s[1 : len(s)-1]
	}
	return s
}
