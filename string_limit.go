package lua

// maxStringResultBytes is the maximum byte length of any runtime-produced string.
// This caps memory amplification from low-gas operations such as concat/format.
const maxStringResultBytes = 1 << 20 // 1 MiB

func addStringResultBytes(L *LState, op string, total *int64, delta int) {
	if delta < 0 {
		L.RaiseError("%s: invalid negative size delta %d", op, delta)
		return
	}
	next := *total + int64(delta)
	if next > maxStringResultBytes {
		L.RaiseError("%s: output too large (%d bytes, limit %d)", op, next, maxStringResultBytes)
		return
	}
	*total = next
}

func checkStringResultBytes(L *LState, op string, size int) {
	total := int64(0)
	addStringResultBytes(L, op, &total, size)
}
