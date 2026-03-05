package lua

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/tos-network/tolang/parse"
)

func TestIRPipelineAndBytecodeRoundTrip(t *testing.T) {
	src := `
local x = (2 ^ 8) + (0xF0 | 0x0F)
local t = {3, 4}
_result = x + t[1]
`
	chunk, err := parse.Parse(strings.NewReader(src), "<ir>")
	if err != nil {
		t.Fatal(err)
	}

	irp, err := BuildIRFromChunk(chunk, "<ir>")
	if err != nil {
		t.Fatal(err)
	}
	if irp == nil || irp.Root == nil || len(irp.Root.Instructions) == 0 {
		t.Fatal("expected non-empty IR")
	}

	proto, err := CompileIR(irp)
	if err != nil {
		t.Fatal(err)
	}
	if len(proto.Code) == 0 {
		t.Fatal("expected non-empty bytecode")
	}

	bc, err := EncodeFunctionProto(proto)
	if err != nil {
		t.Fatal(err)
	}
	if len(bc) == 0 {
		t.Fatal("expected non-empty encoded bytecode")
	}

	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	got := LVAsString(L.GetGlobal("_result"))
	if got != "514" {
		t.Fatalf("unexpected result: got=%s want=514", got)
	}
}

func TestCompileSourceToBytecodeAndExecute(t *testing.T) {
	src := []byte(`_result = (100 // 3) + (7 % 5)`)
	bc, err := CompileSourceToBytecode(src, "<src>")
	if err != nil {
		t.Fatal(err)
	}

	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatal(err)
	}

	if got := LVAsString(L.GetGlobal("_result")); got != "35" {
		t.Fatalf("unexpected result: got=%s want=35", got)
	}
}

func TestDecodeFunctionProtoRejectsInvalidMagic(t *testing.T) {
	if _, err := DecodeFunctionProto([]byte("not-bytecode")); err == nil {
		t.Fatal("expected decode error for invalid magic")
	}
}

func TestLoadAutoDetectsBytecode(t *testing.T) {
	src := []byte(`_result = (5 << 8) + 7`)
	bc, err := CompileSourceToBytecode(src, "<bc>")
	if err != nil {
		t.Fatal(err)
	}
	if !IsBytecode(bc) {
		t.Fatal("expected IsBytecode=true")
	}

	L := NewState()
	defer L.Close()
	fn, err := L.Load(bytes.NewReader(bc), "<bc>")
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, MultRet, nil); err != nil {
		t.Fatal(err)
	}
	if got := LVAsString(L.GetGlobal("_result")); got != "1287" {
		t.Fatalf("unexpected result: got=%s want=1287", got)
	}
}

func TestLoadSourceStripsShebang(t *testing.T) {
	src := []byte("#!/usr/bin/env tolang\n_result = 9 + 1\n")
	L := NewState()
	defer L.Close()
	fn, err := L.Load(bytes.NewReader(src), "<src>")
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, MultRet, nil); err != nil {
		t.Fatal(err)
	}
	if got := LVAsString(L.GetGlobal("_result")); got != "10" {
		t.Fatalf("unexpected result: got=%s want=10", got)
	}
}

func TestDecodeRejectsTamperedChecksum(t *testing.T) {
	bc, err := CompileSourceToBytecode([]byte(`_result = 1 + 2`), "<sum>")
	if err != nil {
		t.Fatal(err)
	}
	bc[len(bc)-1] ^= 0xFF
	if _, err := DecodeFunctionProto(bc); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestDecodeRejectsMismatchedVMID(t *testing.T) {
	bc, err := CompileSourceToBytecode([]byte(`_result = 11`), "<vmid>")
	if err != nil {
		t.Fatal(err)
	}
	// Layout: magic(4) + version(2) + vmidLen(4) + vmid + payloadLen(4) + payload + hash(32)
	const header = 4 + 2
	vmLen := int(binary.BigEndian.Uint32(bc[header : header+4]))
	vmStart := header + 4
	if vmLen == 0 || vmStart+vmLen > len(bc) {
		t.Fatal("unexpected bytecode vmid layout")
	}
	bc[vmStart] ^= 0x01 // alter vmid only (payload hash unchanged)
	if _, err := DecodeFunctionProto(bc); err == nil {
		t.Fatal("expected vm mismatch error")
	}
}

func TestDecodeRejectsLegacyHeaderLayout(t *testing.T) {
	src := []byte(`_result = 1`)
	bc, err := CompileSourceToBytecode(src, "<legacy>")
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the version to v1 and drop v3 header fields to mimic old layout.
	var legacy bytes.Buffer
	legacy.Write([]byte{'T', 'O', 'L', 'B', 0x01, 0x00})
	// append only payload section (skip new header and checksum)
	const hdr = 4 + 2
	vmLen := int(binary.BigEndian.Uint32(bc[hdr : hdr+4]))
	payloadLenOff := hdr + 4 + vmLen
	payloadLen := int(binary.BigEndian.Uint32(bc[payloadLenOff : payloadLenOff+4]))
	payloadOff := payloadLenOff + 4
	legacy.Write(bc[payloadOff : payloadOff+payloadLen])
	if _, err := DecodeFunctionProto(legacy.Bytes()); err == nil {
		t.Fatal("expected legacy format rejection")
	}
}

func TestIRPreservesSetListExtraWord(t *testing.T) {
	p := newFunctionProto("<setlist-extra>")
	p.NumUsedRegisters = 2
	p.Code = []uint32{
		opCreateABC(OP_NEWTABLE, 0, 0, 0),
		opCreateABC(OP_SETLIST, 0, 1, 0), // C == 0 => next word is raw block index
		777,
		opCreateABC(OP_RETURN, 0, 1, 0),
	}
	p.DbgSourcePositions = []int{1, 1, 1, 1}

	irp := BuildIRFromProto(p, "<setlist-extra>")
	if irp == nil || irp.Root == nil {
		t.Fatal("expected non-nil IR")
	}
	if len(irp.Root.Instructions) != 4 {
		t.Fatalf("unexpected IR length: got=%d want=4", len(irp.Root.Instructions))
	}
	if irp.Root.Instructions[2].Op >= 0 || irp.Root.Instructions[2].Raw != 777 {
		t.Fatalf("expected raw extra word 777 in IR, got op=%d raw=%d", irp.Root.Instructions[2].Op, irp.Root.Instructions[2].Raw)
	}

	out, err := CompileIR(irp)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Code) != 4 || out.Code[2] != 777 {
		t.Fatalf("expected proto extra word 777 preserved, got code=%v", out.Code)
	}

	bc, err := EncodeFunctionProto(out)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := DecodeFunctionProto(bc)
	if err != nil {
		t.Fatal(err)
	}
	if len(dec.Code) != 4 || dec.Code[2] != 777 {
		t.Fatalf("expected decode to preserve extra word 777, got code=%v", dec.Code)
	}
}
