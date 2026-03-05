-- $Id: testes/math.lua $
-- See Copyright Notice in file all.lua


local function checkerror (msg, f, ...)
  local s, err = pcall(f, ...)
  assert(not s and string.find(err, msg))
end


-- testing numeric strings

assert("2" + 1 == 3)
assert("2 " + 1 == 3)
assert(not pcall(function() return " -2 " + 1 end))
assert(not pcall(function() return " -0xa " + 1 end))


-- testing 'tonumber'

assert(tonumber(3) == 3)

-- 'tonumber' with strings
assert(tonumber("0") == 0)
assert(not tonumber(""))
assert(not tonumber("  "))
assert(not tonumber("-"))
assert(not tonumber("  -0x "))
assert(not tonumber{})
assert(tonumber('-012') == nil)

assert(tonumber("0xffffffffffff") == (1 << (4*12)) - 1)

-- testing 'tonumber' with base
assert(tonumber('  001010  ', 2) == 10)
assert(tonumber('  001010  ', 10) == 1010)
assert(tonumber('  -1010  ', 2) == nil)
assert(tonumber('10', 36) == 36)
assert(tonumber('  -10  ', 36) == nil)
assert(tonumber('  +1Z  ', 36) == nil)
assert(tonumber('  -1z  ', 36) == nil)
assert(tonumber('-fFfa', 16) == nil)
assert(tonumber('ffffFFFF', 16)+1 == (1 << 32))
assert(tonumber('0ffffFFFF', 16)+1 == (1 << 32))
assert(tonumber('-0ffffffFFFF', 16) == nil)
for i = 2,36 do
  local i2 = i * i
  local i10 = i2 * i2 * i2 * i2 * i2      -- i^10
  assert(tonumber('\t10000000000\t', i) == i10)
end

-- testing 'tonumber' for invalid formats

local function f (...)
  if select('#', ...) == 1 then
    return (...)
  else
    return "***"
  end
end

assert(not f(tonumber('fFfa', 15)))
assert(not f(tonumber('099', 8)))
assert(not f(tonumber('1\0', 2)))
assert(not f(tonumber('', 8)))
assert(not f(tonumber('  ', 9)))
assert(not f(tonumber('  ', 9)))
assert(not f(tonumber('0xf', 10)))

assert(not f(tonumber('inf')))
assert(not f(tonumber(' INF ')))
assert(not f(tonumber('Nan')))
assert(not f(tonumber('nan')))

assert(not f(tonumber('  ')))
assert(not f(tonumber('')))
assert(not f(tonumber('1  a')))
assert(not f(tonumber('1  a', 2)))
assert(not f(tonumber('1\0')))
assert(not f(tonumber('1 \0')))
assert(not f(tonumber('1\0 ')))
assert(not f(tonumber('e1')))
assert(not f(tonumber('e  1')))
assert(not f(tonumber(' 3.4.5 ')))


-- testing 'tonumber' for invalid hexadecimal formats

assert(not tonumber('0x'))
assert(not tonumber('x'))
assert(not tonumber('x3'))
assert(not tonumber('0x3.3.3'))   -- two decimal points
assert(not tonumber('00x2'))
assert(not tonumber('0x 2'))
assert(not tonumber('0 x2'))
assert(not tonumber('23x'))
assert(not tonumber('- 0xaa'))
assert(not tonumber('-0xaaP '))
assert(not tonumber('0x0.51p'))
assert(not tonumber('0x5p+-2'))


-- testing hexadecimal numerals

assert(0x10 == 16 and 0xfff == 4095 and 0XFB == 251)
assert(0xFFFFFFFF == (1 << 32) - 1)
assert(tonumber('+0x2') == nil)
assert(tonumber('-0xaA') == nil)
assert(tonumber('-0xffFFFfff') == nil)


-- testing order operators
assert(not(1<1) and (1<2) and not(2<1))
assert(not('a'<'a') and ('a'<'b') and not('b'<'a'))
assert((1<=1) and (1<=2) and not(2<=1))
assert(('a'<='a') and ('a'<='b') and not('b'<='a'))
assert(not(1>1) and not(1>2) and (2>1))
assert(not('a'>'a') and not('a'>'b') and ('b'>'a'))
assert((1>=1) and not(1>=2) and (2>=1))
assert(('a'>='a') and not('a'>='b') and ('b'>='a'))


-- testing constant limits
-- 2^23 = 8388608
assert(8388609 + -8388609 == 0)
assert(8388608 + -8388608 == 0)
assert(8388607 + -8388607 == 0)

