# Tolang — Deterministic Lua VM for TOS Blockchain

Design constraints for this VM. All decisions below are intentional. **Do not revert them.**

---

## Standard libraries

Only these four libraries are loaded (`linit.go`):

```go
var luaLibs = []luaLib{
    {BaseLibName, OpenBase},
    {TabLibName, OpenTable},
    {StringLibName, OpenString},
    {MathLibName, OpenMath},
}
```

Do not add coroutine, io, os, debug, or any other library.

---

## Determinism rules

- `String()` on tables/functions/userdata must return stable type labels, never pointer addresses.
- `RegisterModule` / `SetFuncs` must use sorted key order; never iterate Go maps in contract-visible paths.
- `math.random` / `math.randomseed` are not available.
- `string.dump` is not available.
- `print`, `dofile`, `loadfile`, `load`, `loadstring`, `require`, `collectgarbage` are not available.

---

## LUint256 type

`LUint256` is `type LUint256 struct{lo,ml,mh,hi uint64}` (little-endian 256-bit). Simple operations (add, sub, bitwise, shifts, compare) use `math/bits` natively; complex operations (div, mod, pow, String) bridge to `math/big.Int`. Helpers are in `number_uint256.go`. Do not regress to `float64` or lossy representations.

---

## Gas metering

`LState` has `gasLimit` / `gasUsed` fields with `SetGasLimit()` / `GasUsed()` methods. The VM loop checks gas on every instruction. Consensus termination is gas-driven only — no wall-clock timeouts, no host context cancellation, no host memory watchers.

---

## Architecture

Hard-requires 64-bit (`strconv.IntSize == 64`). Do not remove the startup guard in `config.go`.

---

## Tests

- `TestCoroutineApi1` and `TestContextWithCroutine` in `state_test.go` must stay skipped.
- External Lua script test directories have been removed. VM behaviour is covered by inline Go tests only.
