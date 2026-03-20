package lua

import (
	"sort"
)

func OpenTable(L *LState) int {
	tabmod := L.RegisterModule(TabLibName, tableFuncs)
	L.Push(tabmod)
	return 1
}

var tableFuncs = map[string]LGFunction{
	"getn":   tableGetN,
	"concat": tableConcat,
	"insert": tableInsert,
	"maxn":   tableMaxN,
	"remove": tableRemove,
	"sort":   tableSort,
}

func tableSort(L *LState) int {
	tbl := L.CheckTable(1)
	sorter := lValueArraySorter{L, nil, tbl.array}
	if L.GetTop() != 1 {
		sorter.Fn = L.CheckFunction(2)
	}
	sort.Sort(sorter)
	return 0
}

func tableGetN(L *LState) int {
	n, cost := L.CheckTable(1).LenWithCost()
	chargeLinearWorkGas(L, cost)
	L.Push(lu256FromInt(n))
	return 1
}

func tableMaxN(L *LState) int {
	n, cost := L.CheckTable(1).MaxNWithCost()
	chargeLinearWorkGas(L, cost)
	L.Push(lu256FromInt(n))
	return 1
}

func tableRemove(L *LState) int {
	tbl := L.CheckTable(1)
	if L.GetTop() == 1 {
		v, cost := tbl.RemoveWithCost(-1)
		chargeLinearWorkGas(L, cost)
		L.Push(v)
	} else {
		pos := L.CheckInt(2)
		size, lenCost := tbl.LenWithCost()
		chargeLinearWorkGas(L, lenCost)
		// Lua 5.4: if pos == size (same as the default), skip bounds check.
		// Otherwise pos must be in [1, size+1].
		if pos != size && (pos < 1 || pos > size+1) {
			L.ArgError(2, "position out of bounds")
		}
		v, cost := tbl.RemoveWithCost(pos)
		chargeLinearWorkGas(L, cost)
		L.Push(v)
	}
	return 1
}

func tableConcat(L *LState) int {
	tbl := L.CheckTable(1)
	sep := LString(L.OptString(2, ""))
	lenVal, lenCost := tbl.LenWithCost()
	chargeLinearWorkGas(L, lenCost)
	i := L.OptInt(3, 1)
	j := L.OptInt(4, lenVal)
	if L.GetTop() == 3 {
		if i > lenVal || i < 1 {
			L.Push(emptyLString)
			return 1
		}
	}
	i = intMax(intMin(i, lenVal), 1)
	j = intMin(intMin(j, lenVal), lenVal)
	if i > j {
		L.Push(emptyLString)
		return 1
	}
	//TODO should flushing?
	retbottom := L.GetTop()
	for ; i <= j; i++ {
		L.chargeGas(1)
		v := tbl.RawGetInt(i)
		if !LVCanConvToString(v) {
			L.RaiseError("invalid value (%s) at index %d in table for concat", v.Type().String(), i)
		}
		L.Push(v)
		if i != j {
			L.Push(sep)
		}
	}
	L.Push(stringConcat(L, L.GetTop()-retbottom, L.reg.Top()-1))
	return 1
}

func tableInsert(L *LState) int {
	tbl := L.CheckTable(1)
	nargs := L.GetTop()
	if nargs == 1 {
		L.RaiseError("wrong number of arguments")
	}

	if nargs == 2 {
		tbl.Append(L.Get(2))
		return 0
	}
	// 3-argument form: table.insert(t, pos, v)
	// Lua 5.4: pos must be in [1, #t+1]
	pos := L.CheckInt(2)
	size, lenCost := tbl.LenWithCost()
	chargeLinearWorkGas(L, lenCost)
	e := size + 1
	if pos < 1 || pos > e {
		L.ArgError(2, "position out of bounds")
	}
	cost := tbl.InsertWithCost(pos, L.CheckAny(3))
	chargeLinearWorkGas(L, cost)
	return 0
}

//
