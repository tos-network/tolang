package lua

const defaultArrayCap = 32
const defaultHashCap = 32

type lValueArraySorter struct {
	L      *LState
	Fn     *LFunction
	Values []LValue
}

const (
	// Meter host-side sort work so large table.sort calls cannot bypass gas.
	tableSortCompareGas uint64 = 1
	tableSortSwapGas    uint64 = 1
)

func (lv lValueArraySorter) Len() int {
	return len(lv.Values)
}

func (lv lValueArraySorter) Swap(i, j int) {
	if lv.L != nil {
		lv.L.chargeGas(tableSortSwapGas)
	}
	lv.Values[i], lv.Values[j] = lv.Values[j], lv.Values[i]
}

func (lv lValueArraySorter) Less(i, j int) bool {
	if lv.L != nil {
		lv.L.chargeGas(tableSortCompareGas)
	}
	if lv.Fn != nil {
		lv.L.Push(lv.Fn)
		lv.L.Push(lv.Values[i])
		lv.L.Push(lv.Values[j])
		lv.L.Call(2, 1)
		return LVAsBool(lv.L.reg.Pop())
	}
	return lessThan(lv.L, lv.Values[i], lv.Values[j])
}

func newLTable(acap int, hcap int) *LTable {
	if acap < 0 {
		acap = 0
	}
	if hcap < 0 {
		hcap = 0
	}
	tb := &LTable{}
	tb.Metatable = LNil
	if acap != 0 {
		tb.array = make([]LValue, 0, acap)
	}
	if hcap != 0 {
		tb.strdict = make(map[string]LValue, hcap)
	}
	return tb
}

func (tb *LTable) numericKeyValue(index int) LUint256 {
	return lu256FromInt(index)
}

func (tb *LTable) removeNumericHashKey(index int) {
	tb.RawSetH(tb.numericKeyValue(index), LNil)
}

func (tb *LTable) shouldStoreArrayIndex(index int) bool {
	if index < 1 || index >= MaxArrayIndex {
		return false
	}
	alen := len(tb.array)
	if index <= alen {
		return true
	}
	return index <= alen+1+MaxArrayHoleGrowth
}

func (tb *LTable) migrateNumericHashRangeToArray(start, end int) {
	if tb.array == nil || start > end {
		return
	}
	for index := start; index <= end; index++ {
		if index <= 0 || index > len(tb.array) {
			continue
		}
		key := tb.numericKeyValue(index)
		if v := tb.RawGetH(key); v != LNil {
			tb.array[index-1] = v
			tb.RawSetH(key, LNil)
		}
	}
}

func (tb *LTable) setArrayValueAt(index int, value LValue) {
	if index < 1 || index >= MaxArrayIndex {
		tb.RawSetH(tb.numericKeyValue(index), value)
		return
	}
	if tb.array == nil {
		tb.array = make([]LValue, 0, defaultArrayCap)
	}
	alen := len(tb.array)
	if value == LNil && index > alen {
		tb.RawSetH(tb.numericKeyValue(index), LNil)
		return
	}
	if index > alen {
		grow := index - alen
		for i := 0; i < grow; i++ {
			tb.array = append(tb.array, LNil)
		}
		tb.migrateNumericHashRangeToArray(alen+1, index)
	}
	tb.array[index-1] = value
	tb.removeNumericHashKey(index)
}

func isEmptyArrayValue(v LValue) bool {
	return v == nil || v == LNil
}

// Len returns length of this LTable without using __len.
func (tb *LTable) Len() int {
	n, _ := tb.LenWithCost()
	return n
}

func (tb *LTable) LenWithCost() (int, int) {
	if tb.array == nil {
		return 0, 0
	}
	var prev LValue = LNil
	cost := 0
	for i := len(tb.array) - 1; i >= 0; i-- {
		cost++
		v := tb.array[i]
		if isEmptyArrayValue(prev) && !isEmptyArrayValue(v) {
			return i + 1, cost
		}
		prev = v
	}
	return 0, cost
}

// Append appends a given LValue to this LTable.
func (tb *LTable) Append(value LValue) {
	if value == LNil {
		return
	}
	if tb.array == nil {
		tb.array = make([]LValue, 0, defaultArrayCap)
	}
	if len(tb.array) == 0 || !isEmptyArrayValue(tb.array[len(tb.array)-1]) {
		tb.setArrayValueAt(len(tb.array)+1, value)
	} else {
		i := len(tb.array) - 2
		for ; i >= 0; i-- {
			if !isEmptyArrayValue(tb.array[i]) {
				break
			}
		}
		tb.array[i+1] = value
		tb.removeNumericHashKey(i + 2)
	}
}

// Insert inserts a given LValue at position `i` in this table.
func (tb *LTable) Insert(i int, value LValue) {
	tb.InsertWithCost(i, value)
}

func (tb *LTable) InsertWithCost(i int, value LValue) int {
	if tb.array == nil {
		tb.array = make([]LValue, 0, defaultArrayCap)
	}
	if i > len(tb.array) {
		tb.RawSetInt(i, value)
		return 0
	}
	if i <= 0 {
		tb.RawSet(lu256FromInt(i), value)
		return 0
	}
	i -= 1
	oldLen := len(tb.array)
	tb.array = append(tb.array, LNil)
	copy(tb.array[i+1:], tb.array[i:])
	tb.array[i] = value
	tb.removeNumericHashKey(i + 1)
	return oldLen - i
}

// MaxN returns a maximum number key that nil value does not exist before it.
func (tb *LTable) MaxN() int {
	n, _ := tb.MaxNWithCost()
	return n
}

func (tb *LTable) MaxNWithCost() (int, int) {
	if tb.array == nil {
		return 0, 0
	}
	cost := 0
	for i := len(tb.array) - 1; i >= 0; i-- {
		cost++
		if !isEmptyArrayValue(tb.array[i]) {
			return i + 1, cost
		}
	}
	return 0, cost
}

// Remove removes from this table the element at a given position.
func (tb *LTable) Remove(pos int) LValue {
	value, _ := tb.RemoveWithCost(pos)
	return value
}

func (tb *LTable) RemoveWithCost(pos int) (LValue, int) {
	if tb.array == nil {
		return LNil, 0
	}
	larray := len(tb.array)
	if larray == 0 {
		return LNil, 0
	}
	i := pos - 1
	oldval := LNil
	switch {
	case i >= larray:
		// nothing to do
		return LNil, 0
	case i == larray-1 || i < 0:
		oldval = tb.array[larray-1]
		tb.array[larray-1] = nil
		tb.array = tb.array[:larray-1]
	default:
		oldval = tb.array[i]
		copy(tb.array[i:], tb.array[i+1:])
		tb.array[larray-1] = nil
		tb.array = tb.array[:larray-1]
		return oldval, larray - i - 1
	}
	return oldval, 0
}

// RawSet sets a given LValue to a given index without the __newindex metamethod.
// It is recommended to use `RawSetString` or `RawSetInt` for performance
// if you already know the given LValue is a string or number.
func (tb *LTable) RawSet(key LValue, value LValue) {
	switch v := key.(type) {
	case LUint256:
		if isArrayKey(v) {
			intv, ok := lu256ToInt(v)
			if !ok {
				tb.RawSetH(key, value)
				return
			}
			if tb.shouldStoreArrayIndex(intv) {
				tb.setArrayValueAt(intv, value)
			} else {
				tb.RawSetH(key, value)
			}
			return
		}
	case LString:
		tb.RawSetString(string(v), value)
		return
	}

	tb.RawSetH(key, value)
}

// RawSetInt sets a given LValue at a position `key` without the __newindex metamethod.
func (tb *LTable) RawSetInt(key int, value LValue) {
	if !tb.shouldStoreArrayIndex(key) {
		tb.RawSetH(lu256FromInt(key), value)
		return
	}
	tb.setArrayValueAt(key, value)
}

// RawSetString sets a given LValue to a given string index without the __newindex metamethod.
func (tb *LTable) RawSetString(key string, value LValue) {
	lkey := LString(key)
	if value == LNil {
		if tb.strdict != nil {
			delete(tb.strdict, key)
			if len(tb.strdict) == 0 {
				tb.strdict = nil
			}
		}
		if !tb.shouldKeepStaleNextKey(lkey) {
			tb.removeHashKeyMetadata(lkey)
		}
		return
	}
	if tb.strdict == nil {
		tb.strdict = make(map[string]LValue, defaultHashCap)
	}
	if tb.keys == nil {
		tb.keys = []LValue{}
		tb.k2i = map[LValue]int{}
	}
	tb.strdict[key] = value
	if _, ok := tb.k2i[lkey]; !ok {
		tb.k2i[lkey] = len(tb.keys)
		tb.keys = append(tb.keys, lkey)
	}
}

func (tb *LTable) shouldKeepStaleNextKey(key LValue) bool {
	if tb.nextIterated == nil {
		return false
	}
	_, ok := tb.nextIterated[key]
	return ok
}

func (tb *LTable) removeHashKeyMetadata(key LValue) {
	if tb.k2i == nil {
		return
	}
	idx, ok := tb.k2i[key]
	if !ok {
		return
	}
	delete(tb.k2i, key)
	if tb.nextIterated != nil {
		delete(tb.nextIterated, key)
	}
	copy(tb.keys[idx:], tb.keys[idx+1:])
	tb.keys[len(tb.keys)-1] = nil
	tb.keys = tb.keys[:len(tb.keys)-1]
	for i := idx; i < len(tb.keys); i++ {
		tb.k2i[tb.keys[i]] = i
	}
	if len(tb.keys) == 0 {
		tb.keys = nil
		tb.k2i = nil
	}
}

func (tb *LTable) markNextHashKey(key LValue) {
	if tb.nextIterated == nil {
		tb.nextIterated = make(map[LValue]struct{}, 4)
	}
	tb.nextIterated[key] = struct{}{}
}

func (tb *LTable) compactNextIterationState() int {
	if len(tb.nextIterated) == 0 {
		return 0
	}
	cost := len(tb.keys)
	if len(tb.keys) == 0 {
		tb.keys = nil
		tb.k2i = nil
		tb.nextIterated = nil
		return 0
	}
	newKeys := make([]LValue, 0, len(tb.keys))
	newK2I := make(map[LValue]int, len(tb.keys))
	for _, key := range tb.keys {
		if tb.RawGetH(key) == LNil {
			continue
		}
		newK2I[key] = len(newKeys)
		newKeys = append(newKeys, key)
	}
	if len(newKeys) == 0 {
		tb.keys = nil
		tb.k2i = nil
	} else {
		tb.keys = newKeys
		tb.k2i = newK2I
	}
	tb.nextIterated = nil
	return cost
}

// RawSetH sets a given LValue to a given index without the __newindex metamethod.
func (tb *LTable) RawSetH(key LValue, value LValue) {
	if s, ok := key.(LString); ok {
		tb.RawSetString(string(s), value)
		return
	}
	if value == LNil {
		if tb.dict != nil {
			delete(tb.dict, key)
			if len(tb.dict) == 0 {
				tb.dict = nil
			}
		}
		if !tb.shouldKeepStaleNextKey(key) {
			tb.removeHashKeyMetadata(key)
		}
		return
	}
	if tb.dict == nil {
		tb.dict = make(map[LValue]LValue, len(tb.strdict))
	}
	if tb.keys == nil {
		tb.keys = []LValue{}
		tb.k2i = map[LValue]int{}
	}

	tb.dict[key] = value
	if _, ok := tb.k2i[key]; !ok {
		tb.k2i[key] = len(tb.keys)
		tb.keys = append(tb.keys, key)
	}
}

// RawGet returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGet(key LValue) LValue {
	switch v := key.(type) {
	case LUint256:
		if isArrayKey(v) {
			intv, ok := lu256ToInt(v)
			if !ok {
				return LNil
			}
			index := intv - 1
			if tb.array != nil && index >= 0 && index < len(tb.array) {
				if ret := tb.array[index]; !isEmptyArrayValue(ret) {
					return ret
				}
			}
			return tb.RawGetH(key)
		}
	case LString:
		if tb.strdict == nil {
			return LNil
		}
		if ret, ok := tb.strdict[string(v)]; ok {
			return ret
		}
		return LNil
	}
	if tb.dict == nil {
		return LNil
	}
	if v, ok := tb.dict[key]; ok {
		return v
	}
	return LNil
}

// RawGetInt returns an LValue at position `key` without __index metamethod.
func (tb *LTable) RawGetInt(key int) LValue {
	index := int(key) - 1
	if tb.array != nil && index >= 0 && index < len(tb.array) {
		if ret := tb.array[index]; !isEmptyArrayValue(ret) {
			return ret
		}
	}
	return tb.RawGetH(lu256FromInt(key))
}

// RawGet returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGetH(key LValue) LValue {
	if s, sok := key.(LString); sok {
		if tb.strdict == nil {
			return LNil
		}
		if v, vok := tb.strdict[string(s)]; vok {
			return v
		}
		return LNil
	}
	if tb.dict == nil {
		return LNil
	}
	if v, ok := tb.dict[key]; ok {
		return v
	}
	return LNil
}

// RawGetString returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGetString(key string) LValue {
	if tb.strdict == nil {
		return LNil
	}
	if v, vok := tb.strdict[string(key)]; vok {
		return v
	}
	return LNil
}

// ForEach iterates over this table of elements, yielding each in turn to a given function.
func (tb *LTable) ForEach(cb func(LValue, LValue)) {
	if tb.array != nil {
		for i, v := range tb.array {
			if !isEmptyArrayValue(v) {
				cb(lu256FromInt(i+1), v)
			}
		}
	}

	// Keep hash-part traversal deterministic: iterate insertion-order keys.
	for _, key := range tb.keys {
		if v := tb.RawGetH(key); v != LNil {
			cb(key, v)
		}
	}
}

// isValidNextKey reports whether key is a valid traversal cursor for next/pairs.
// It intentionally accepts stale keys that were previously returned by iteration
// and later deleted, matching Lua's behavior for continuing traversal after
// erasing the current element.
func (tb *LTable) isValidNextKey(key LValue) bool {
	if key == LNil {
		return true
	}
	if kv, ok := key.(LUint256); ok && isArrayKey(kv) {
		index, ok := lu256ToInt(kv)
		if ok && index >= 1 && index <= len(tb.array) {
			return true
		}
	}
	if tb.k2i == nil {
		return false
	}
	_, ok := tb.k2i[key]
	return ok
}

func (tb *LTable) firstHashNext() (LValue, LValue, int) {
	cost := 0
	for _, key := range tb.keys {
		cost++
		if v := tb.RawGetH(key); v != LNil {
			tb.markNextHashKey(key)
			return key, v, cost
		}
	}
	return LNil, LNil, cost
}

func (tb *LTable) nextHashAfter(key LValue) (LValue, LValue, int) {
	if tb.k2i == nil {
		return LNil, LNil, 0
	}
	cost := 0
	for i := tb.k2i[key] + 1; i < len(tb.keys); i++ {
		cost++
		nextKey := tb.keys[i]
		if v := tb.RawGetH(nextKey); v != LNil {
			tb.markNextHashKey(nextKey)
			return nextKey, v, cost
		}
	}
	return LNil, LNil, cost
}

func (tb *LTable) nextAfterArrayIndex(index int) (LValue, LValue, int) {
	cost := 0
	if tb.array != nil {
		for ; index < len(tb.array); index++ {
			cost++
			if v := tb.array[index]; !isEmptyArrayValue(v) {
				return lu256FromInt(index + 1), v, cost
			}
		}
	}
	key, value, hashCost := tb.firstHashNext()
	return key, value, cost + hashCost
}

// This function is equivalent to lua_next ( http://www.lua.org/manual/5.1/manual.html#lua_next ).
func (tb *LTable) Next(key LValue) (LValue, LValue) {
	nextKey, nextValue, _ := tb.NextWithCost(key)
	return nextKey, nextValue
}

func (tb *LTable) NextWithCost(key LValue) (LValue, LValue, int) {
	returnNil := func(cost int) (LValue, LValue, int) {
		cost += tb.compactNextIterationState()
		return LNil, LNil, cost
	}
	if key == LNil {
		nextKey, nextValue, cost := tb.nextAfterArrayIndex(0)
		if nextKey == LNil {
			return returnNil(cost)
		}
		return nextKey, nextValue, cost
	}

	if kv, ok := key.(LUint256); ok && isArrayKey(kv) {
		if index, ok := lu256ToInt(kv); ok && index >= 1 && index <= len(tb.array) {
			nextKey, nextValue, cost := tb.nextAfterArrayIndex(index)
			if nextKey == LNil {
				return returnNil(cost)
			}
			return nextKey, nextValue, cost
		}
	}

	nextKey, nextValue, cost := tb.nextHashAfter(key)
	if nextKey == LNil {
		return returnNil(cost)
	}
	return nextKey, nextValue, cost
}
