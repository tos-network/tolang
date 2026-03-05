package lua

const preloadLimit = 128

var preloads [preloadLimit]LValue

func init() {
	for i := 0; i < preloadLimit; i++ {
		preloads[i] = LUint256{lo: uint64(i)}
	}
}

type allocator struct{}

func newAllocator(size int) *allocator {
	return &allocator{}
}

// LUint256ToI converts an LUint256 to an LValue, using preloaded values for
// small non-negative integers to avoid repeated allocations.
func (al *allocator) LUint256ToI(v LUint256) LValue {
	if v.hi == 0 && v.mh == 0 && v.ml == 0 && v.lo < preloadLimit {
		return preloads[v.lo]
	}
	return v
}
