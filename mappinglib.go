package lua

import (
	"fmt"
	"strings"
)

const MappingLibName = "mapping"

const mappingTypeName = "__tos_mapping"

const (
	zeroAgent = "0x0000000000000000000000000000000000000000000000000000000000000000"
	zeroBytes32 = "0x0000000000000000000000000000000000000000000000000000000000000000"
)

type mappingKind uint8

const (
	mappingKindU256 mappingKind = iota
	mappingKindBool
	mappingKindString
	mappingKindAgent
	mappingKindBytes32
)

type solidityMapping struct {
	keyKind   mappingKind
	valueKind mappingKind
	entries   map[string]LValue
}

var mappingFuncs = map[string]LGFunction{
	"new":      mappingNew,
	"get":      mappingGet,
	"set":      mappingSet,
	"delete":   mappingDelete,
	"has":      mappingHas,
	"key_type": mappingKeyType,
	"val_type": mappingValType,
}

func openMapping(L *LState) {
	registerMappingMetatable(L)
	L.RegisterModule(MappingLibName, mappingFuncs)
}

func registerMappingMetatable(L *LState) {
	mt := L.NewTypeMetatable(mappingTypeName)
	L.SetField(mt, "__index", L.NewFunction(mappingMetaIndex))
	L.SetField(mt, "__newindex", L.NewFunction(mappingMetaNewIndex))
	L.SetField(mt, "__tostring", L.NewFunction(mappingMetaToString))
	L.SetField(mt, "__metatable", LString("protected"))
}

func mappingNew(L *LState) int {
	top := L.GetTop()
	if top > 2 {
		L.RaiseError("wrong number of arguments")
	}

	keyKind := mappingKindU256
	valKind := mappingKindU256
	if top >= 1 {
		k, err := parseMappingKind(L.CheckString(1), true)
		if err != nil {
			L.RaiseError(err.Error())
		}
		keyKind = k
	}
	if top >= 2 {
		v, err := parseMappingKind(L.CheckString(2), false)
		if err != nil {
			L.RaiseError(err.Error())
		}
		valKind = v
	}

	ud := L.NewUserData()
	ud.Value = &solidityMapping{
		keyKind:   keyKind,
		valueKind: valKind,
		entries:   make(map[string]LValue),
	}
	L.SetMetatable(ud, L.GetTypeMetatable(mappingTypeName))
	L.Push(ud)
	return 1
}

func mappingGet(L *LState) int {
	m := checkMapping(L, 1)
	key, err := normalizeMappingKey(L.CheckAny(2), m.keyKind)
	if err != nil {
		L.ArgError(2, err.Error())
	}
	if value, ok := m.entries[key]; ok {
		L.Push(value)
	} else {
		L.Push(defaultMappingValue(m.valueKind))
	}
	return 1
}

func mappingSet(L *LState) int {
	m := checkMapping(L, 1)
	key, err := normalizeMappingKey(L.CheckAny(2), m.keyKind)
	if err != nil {
		L.ArgError(2, err.Error())
	}
	value, err := normalizeMappingValue(L.CheckAny(3), m.valueKind)
	if err != nil {
		L.ArgError(3, err.Error())
	}
	m.entries[key] = value
	return 0
}

func mappingDelete(L *LState) int {
	m := checkMapping(L, 1)
	key, err := normalizeMappingKey(L.CheckAny(2), m.keyKind)
	if err != nil {
		L.ArgError(2, err.Error())
	}
	delete(m.entries, key)
	return 0
}

func mappingHas(L *LState) int {
	m := checkMapping(L, 1)
	key, err := normalizeMappingKey(L.CheckAny(2), m.keyKind)
	if err != nil {
		L.ArgError(2, err.Error())
	}
	_, ok := m.entries[key]
	L.Push(LBool(ok))
	return 1
}

func mappingKeyType(L *LState) int {
	m := checkMapping(L, 1)
	L.Push(LString(mappingKindName(m.keyKind)))
	return 1
}

func mappingValType(L *LState) int {
	m := checkMapping(L, 1)
	L.Push(LString(mappingKindName(m.valueKind)))
	return 1
}

func mappingMetaIndex(L *LState) int {
	m := checkMapping(L, 1)
	key, err := normalizeMappingKey(L.CheckAny(2), m.keyKind)
	if err != nil {
		L.RaiseError("mapping key type error: %s", err.Error())
	}
	if value, ok := m.entries[key]; ok {
		L.Push(value)
	} else {
		L.Push(defaultMappingValue(m.valueKind))
	}
	return 1
}

func mappingMetaNewIndex(L *LState) int {
	m := checkMapping(L, 1)
	key, err := normalizeMappingKey(L.CheckAny(2), m.keyKind)
	if err != nil {
		L.RaiseError("mapping key type error: %s", err.Error())
	}
	value, err := normalizeMappingValue(L.CheckAny(3), m.valueKind)
	if err != nil {
		L.RaiseError("mapping value type error: %s", err.Error())
	}
	m.entries[key] = value
	return 0
}

func mappingMetaToString(L *LState) int {
	m := checkMapping(L, 1)
	L.Push(LString(fmt.Sprintf("mapping<%s=>%s>", mappingKindName(m.keyKind), mappingKindName(m.valueKind))))
	return 1
}

func checkMapping(L *LState, n int) *solidityMapping {
	ud := L.CheckUserData(n)
	m, ok := ud.Value.(*solidityMapping)
	if !ok {
		L.ArgError(n, "mapping expected")
	}
	return m
}

func parseMappingKind(name string, allowStringKey bool) (mappingKind, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "u256", "uint256":
		return mappingKindU256, nil
	case "bool":
		return mappingKindBool, nil
	case "string":
		if !allowStringKey {
			return mappingKindString, nil
		}
		return mappingKindString, nil
	case "agent":
		return mappingKindAgent, nil
	case "bytes32":
		return mappingKindBytes32, nil
	default:
		if allowStringKey {
			return mappingKindU256, fmt.Errorf("unsupported mapping key type: %s", name)
		}
		return mappingKindU256, fmt.Errorf("unsupported mapping value type: %s", name)
	}
}

func mappingKindName(k mappingKind) string {
	switch k {
	case mappingKindU256:
		return "u256"
	case mappingKindBool:
		return "bool"
	case mappingKindString:
		return "string"
	case mappingKindAgent:
		return "agent"
	case mappingKindBytes32:
		return "bytes32"
	default:
		return "unknown"
	}
}

func defaultMappingValue(kind mappingKind) LValue {
	switch kind {
	case mappingKindU256:
		return LUint256Zero
	case mappingKindBool:
		return LFalse
	case mappingKindAgent:
		return LAgent(zeroAgent)
	case mappingKindBytes32:
		return LString(zeroBytes32)
	case mappingKindString:
		return LString("")
	default:
		return LNil
	}
}

func normalizeMappingKey(v LValue, kind mappingKind) (string, error) {
	switch kind {
	case mappingKindU256:
		return normalizeU256(v)
	case mappingKindBool:
		if b, ok := v.(LBool); ok {
			if bool(b) {
				return "1", nil
			}
			return "0", nil
		}
		return "", fmt.Errorf("expected bool key")
	case mappingKindString:
		if s, ok := v.(LString); ok {
			return string(s), nil
		}
		return "", fmt.Errorf("expected string key")
	case mappingKindAgent:
		addr, err := parseAgentValue(v)
		if err != nil {
			return "", err
		}
		return string(addr), nil
	case mappingKindBytes32:
		return normalizeHexString(v, 32, "bytes32")
	default:
		return "", fmt.Errorf("unsupported key kind")
	}
}

func normalizeMappingValue(v LValue, kind mappingKind) (LValue, error) {
	switch kind {
	case mappingKindU256:
		num, err := normalizeU256(v)
		if err != nil {
			return LNil, err
		}
		n, err := parseUint256(num)
		if err != nil {
			return LNil, fmt.Errorf("expected u256 value")
		}
		return n, nil
	case mappingKindBool:
		if b, ok := v.(LBool); ok {
			return b, nil
		}
		return LNil, fmt.Errorf("expected bool value")
	case mappingKindString:
		if s, ok := v.(LString); ok {
			return s, nil
		}
		return LNil, fmt.Errorf("expected string value")
	case mappingKindAgent:
		addr, err := parseAgentValue(v)
		if err != nil {
			return LNil, err
		}
		return addr, nil
	case mappingKindBytes32:
		s, err := normalizeHexString(v, 32, "bytes32")
		if err != nil {
			return LNil, err
		}
		return LString(s), nil
	default:
		return LNil, fmt.Errorf("unsupported value kind")
	}
}

func normalizeU256(v LValue) (string, error) {
	switch lv := v.(type) {
	case LUint256:
		return lv.String(), nil
	case LString:
		n, err := parseUint256(string(lv))
		if err != nil {
			return "", fmt.Errorf("expected u256 value")
		}
		return n.String(), nil
	default:
		return "", fmt.Errorf("expected u256 value")
	}
}

func normalizeHexString(v LValue, nbytes int, kindName string) (string, error) {
	s, ok := v.(LString)
	if !ok {
		return "", fmt.Errorf("expected %s string", kindName)
	}
	raw := strings.ToLower(strings.TrimSpace(string(s)))
	if !strings.HasPrefix(raw, "0x") {
		return "", fmt.Errorf("expected %s with 0x prefix", kindName)
	}
	hex := raw[2:]
	if len(hex) != nbytes*2 {
		return "", fmt.Errorf("expected %s with %d hex chars", kindName, nbytes*2)
	}
	for _, ch := range hex {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", fmt.Errorf("invalid %s hex string", kindName)
		}
	}
	return "0x" + hex, nil
}
