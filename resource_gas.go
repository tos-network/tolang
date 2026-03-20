package lua

const (
	// Charge one unit of gas per slot scanned or shifted in host-side table work.
	tableLinearWorkGasPerUnit uint64 = 1

	// Charge one gas unit per 32 bytes of hash / ABI input processed.
	cryptoLinearWorkGasChunkBytes = 32
)

func chargeLinearWorkGas(L *LState, units int) {
	if units <= 0 {
		return
	}
	L.chargeGas(uint64(units) * tableLinearWorkGasPerUnit)
}

func chargeChunkedWorkGas(L *LState, size int, chunkSize int) {
	if size <= 0 || chunkSize <= 0 {
		return
	}
	units := (size + chunkSize - 1) / chunkSize
	L.chargeGas(uint64(units))
}
