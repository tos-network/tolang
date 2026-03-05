-- $Id: testes/bitwise.lua $
-- See Copyright Notice in file all.lua


local numbits = 64

assert(~0 == -1)


-- basic tests for bitwise operators;
-- use variables to avoid constant folding
local a, b, c, d
a = 0xF0; b = 0xCC; c = 0xAA; d = 0xFD
assert(a | b ~ c & d == 0xF4)


a = 0xF0000000; b = 0xCC000000;
c = 0xAA000000; d = 0xFD000000
assert(a | b ~ c & d == 0xF4000000)
assert(~~a == a and ~a == -1 ~ a and -d == ~d + 1)

a = a << 32
b = b << 32
c = c << 32
d = d << 32
assert(a | b ~ c & d == 0xF4000000 << 32)
assert(~~a == a and ~a == -1 ~ a and -d == ~d + 1)


-- coercion from strings to integers

assert(("7" .. 3) << 1 == 146)
assert(0xffffffff >> (1 .. "9") == 0x1fff)
assert(10 | (1 .. "9") == 27)


-- out of range number
assert(not pcall(function () return "0xffffffffffffffff.0" | 0 end))

-- embedded zeros
assert(not pcall(function () return "0xffffffffffffffff\0" | 0 end))

