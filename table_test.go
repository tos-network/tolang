package lua

import (
	"testing"
)

func TestTableNewLTable(t *testing.T) {
	tbl := newLTable(-1, -2)
	errorIfNotEqual(t, 0, cap(tbl.array))

	tbl = newLTable(10, 9)
	errorIfNotEqual(t, 10, cap(tbl.array))
}

func TestTableLen(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(10, LNil)
	tbl.RawSetInt(9, lu256FromInt(10))
	tbl.RawSetInt(8, LNil)
	tbl.RawSetInt(7, lu256FromInt(10))
	errorIfNotEqual(t, 9, tbl.Len())

	tbl = newLTable(0, 0)
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	errorIfNotEqual(t, 3, tbl.Len())
}

func TestTableLenType(t *testing.T) {
	L := NewState(Options{})
	err := L.DoString(`
        mt = {
            __index = mt,
            __len = function (self)
                return {hello = "world"}
            end
        }

        v = {}
        v.__index = v

	        setmetatable(v, mt)

	        assert(#v ~= 0, "#v should return a table reference in this case")
	    `)
	if err != nil {
		t.Error(err)
	}
}

func TestTableAppend(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(1, lu256FromInt(1))
	tbl.RawSetInt(2, lu256FromInt(2))
	tbl.RawSetInt(3, lu256FromInt(3))
	errorIfNotEqual(t, 3, tbl.Len())

	tbl.RawSetInt(1, LNil)
	tbl.RawSetInt(2, LNil)
	errorIfNotEqual(t, 3, tbl.Len())

	tbl.Append(lu256FromInt(4))
	errorIfNotEqual(t, 4, tbl.Len())

	tbl.RawSetInt(3, LNil)
	tbl.RawSetInt(4, LNil)
	errorIfNotEqual(t, 0, tbl.Len())

	tbl.Append(lu256FromInt(5))
	errorIfNotEqual(t, 1, tbl.Len())
}

func TestTableInsert(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	tbl.Append(LTrue)

	tbl.Insert(5, LFalse)
	errorIfNotEqual(t, LFalse, tbl.RawGetInt(5))
	errorIfNotEqual(t, 5, tbl.Len())

	tbl.Insert(-10, LFalse)
	errorIfNotEqual(t, LFalse, tbl.RawGet(LUint256Zero))
	errorIfNotEqual(t, 5, tbl.Len())

	tbl = newLTable(0, 0)
	tbl.Append(lu256FromInt(1))
	tbl.Append(lu256FromInt(2))
	tbl.Append(lu256FromInt(3))
	tbl.Insert(1, lu256FromInt(10))
	errorIfNotEqual(t, lu256FromInt(10), tbl.RawGetInt(1))
	errorIfNotEqual(t, lu256FromInt(1), tbl.RawGetInt(2))
	errorIfNotEqual(t, lu256FromInt(2), tbl.RawGetInt(3))
	errorIfNotEqual(t, lu256FromInt(3), tbl.RawGetInt(4))
	errorIfNotEqual(t, 4, tbl.Len())

	tbl = newLTable(0, 0)
	tbl.Insert(5, lu256FromInt(10))
	errorIfNotEqual(t, lu256FromInt(10), tbl.RawGetInt(5))

}

func TestTableMaxN(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	errorIfNotEqual(t, 3, tbl.MaxN())

	tbl = newLTable(0, 0)
	errorIfNotEqual(t, 0, tbl.MaxN())

	tbl = newLTable(10, 0)
	errorIfNotEqual(t, 0, tbl.MaxN())
}

func TestTableRemove(t *testing.T) {
	tbl := newLTable(0, 0)
	errorIfNotEqual(t, LNil, tbl.Remove(10))
	tbl.Append(LTrue)
	errorIfNotEqual(t, LNil, tbl.Remove(10))

	tbl.Append(LFalse)
	tbl.Append(LTrue)
	errorIfNotEqual(t, LFalse, tbl.Remove(2))
	errorIfNotEqual(t, 2, tbl.MaxN())
	tbl.Append(LFalse)
	errorIfNotEqual(t, LFalse, tbl.Remove(-1))
	errorIfNotEqual(t, 2, tbl.MaxN())

}

func TestTableRawSetInt(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(MaxArrayIndex+1, LTrue)
	errorIfNotEqual(t, 0, tbl.MaxN())
	errorIfNotEqual(t, LTrue, tbl.RawGet(lu256FromInt(MaxArrayIndex+1)))

	tbl.RawSetInt(1, LTrue)
	tbl.RawSetInt(3, LTrue)
	errorIfNotEqual(t, 3, tbl.MaxN())
	errorIfNotEqual(t, LTrue, tbl.RawGetInt(1))
	errorIfNotEqual(t, LNil, tbl.RawGetInt(2))
	errorIfNotEqual(t, LTrue, tbl.RawGetInt(3))
	tbl.RawSetInt(2, LTrue)
	errorIfNotEqual(t, LTrue, tbl.RawGetInt(1))
	errorIfNotEqual(t, LTrue, tbl.RawGetInt(2))
	errorIfNotEqual(t, LTrue, tbl.RawGetInt(3))
}

func TestTableRawSetH(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetH(LString("key"), LTrue)
	tbl.RawSetH(LString("key"), LNil)
	_, found := tbl.dict[LString("key")]
	errorIfNotEqual(t, false, found)

	tbl.RawSetH(LTrue, LTrue)
	tbl.RawSetH(LTrue, LNil)
	_, foundb := tbl.dict[LTrue]
	errorIfNotEqual(t, false, foundb)
}

func TestTableRawGetH(t *testing.T) {
	tbl := newLTable(0, 0)
	errorIfNotEqual(t, LNil, tbl.RawGetH(lu256FromInt(1)))
	errorIfNotEqual(t, LNil, tbl.RawGetH(LString("key0")))
	tbl.RawSetH(LString("key0"), LTrue)
	tbl.RawSetH(LString("key1"), LFalse)
	tbl.RawSetH(lu256FromInt(1), LTrue)
	errorIfNotEqual(t, LTrue, tbl.RawGetH(LString("key0")))
	errorIfNotEqual(t, LTrue, tbl.RawGetH(lu256FromInt(1)))
	errorIfNotEqual(t, LNil, tbl.RawGetH(LString("notexist")))
	errorIfNotEqual(t, LNil, tbl.RawGetH(LTrue))
}

func TestTableForEach(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.Append(lu256FromInt(1))
	tbl.Append(lu256FromInt(2))
	tbl.Append(lu256FromInt(3))
	tbl.Append(LNil)
	tbl.Append(lu256FromInt(5))

	tbl.RawSetH(LString("a"), LString("a"))
	tbl.RawSetH(LString("b"), LString("b"))
	tbl.RawSetH(LString("c"), LString("c"))

	tbl.RawSetH(LTrue, LString("true"))
	tbl.RawSetH(LFalse, LString("false"))

	tbl.ForEach(func(key, value LValue) {
		switch k := key.(type) {
		case LBool:
			switch bool(k) {
			case true:
				errorIfNotEqual(t, LString("true"), value)
			case false:
				errorIfNotEqual(t, LString("false"), value)
			default:
				t.Fail()
			}
		case LUint256:
			ik, ok := lu256ToInt(k)
			if !ok {
				t.Fail()
				return
			}
			switch ik {
			case 1:
				errorIfNotEqual(t, lu256FromInt(1), value)
			case 2:
				errorIfNotEqual(t, lu256FromInt(2), value)
			case 3:
				errorIfNotEqual(t, lu256FromInt(3), value)
			case 4:
				errorIfNotEqual(t, lu256FromInt(5), value)
			default:
				t.Fail()
			}
		case LString:
			switch string(k) {
			case "a":
				errorIfNotEqual(t, LString("a"), value)
			case "b":
				errorIfNotEqual(t, LString("b"), value)
			case "c":
				errorIfNotEqual(t, LString("c"), value)
			default:
				t.Fail()
			}
		}
	})
}

func TestTableValidNextKeyAllowsStaleKeys(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetInt(1, lu256FromInt(10))
	tbl.RawSetInt(2, lu256FromInt(20))
	tbl.RawSetH(LString("a"), LTrue)
	tbl.RawSetH(LString("b"), LFalse)

	hashKey, _ := tbl.Next(lu256FromInt(2))
	errorIfNotEqual(t, LString("a"), hashKey)

	tbl.RawSetInt(1, LNil)
	tbl.RawSetH(hashKey, LNil)

	errorIfFalse(t, tbl.isValidNextKey(lu256FromInt(1)), "expected stale array key to remain valid for next()")
	errorIfFalse(t, tbl.isValidNextKey(hashKey), "expected iterated stale hash key to remain valid for next()")
	errorIfFalse(t, !tbl.isValidNextKey(lu256FromInt(3)), "unexpectedly accepted missing array key")
	errorIfFalse(t, !tbl.isValidNextKey(LString("missing")), "unexpectedly accepted missing hash key")
}

func TestTableNextFromStaleHashKey(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetH(LString("a"), lu256FromInt(1))
	tbl.RawSetH(LString("b"), lu256FromInt(2))
	staleKey, _ := tbl.Next(LNil)
	errorIfNotEqual(t, LString("a"), staleKey)
	tbl.RawSetH(staleKey, LNil)

	key, value := tbl.Next(staleKey)
	errorIfNotEqual(t, LString("b"), key)
	errorIfNotEqual(t, lu256FromInt(2), value)
}

func TestTableDeleteWithoutIterationDoesNotRetainTombstones(t *testing.T) {
	tbl := newLTable(0, 0)
	for i := 0; i < 1000; i++ {
		key := LString("k" + lu256FromInt(i).String())
		tbl.RawSetH(key, LTrue)
		tbl.RawSetH(key, LNil)
	}
	errorIfNotEqual(t, 0, len(tbl.keys))
	errorIfNotEqual(t, 0, len(tbl.k2i))
}

func TestTableTraversalCompactsStaleHashTombstonesAtEnd(t *testing.T) {
	tbl := newLTable(0, 0)
	tbl.RawSetH(LString("a"), lu256FromInt(1))
	tbl.RawSetH(LString("b"), lu256FromInt(2))

	k1, _ := tbl.Next(LNil)
	tbl.RawSetH(k1, LNil)
	k2, _ := tbl.Next(k1)
	tbl.RawSetH(k2, LNil)
	k3, v3 := tbl.Next(k2)

	errorIfNotEqual(t, LNil, k3)
	errorIfNotEqual(t, LNil, v3)
	errorIfNotEqual(t, 0, len(tbl.keys))
	errorIfNotEqual(t, 0, len(tbl.k2i))
	errorIfNotEqual(t, 0, len(tbl.nextIterated))
}
