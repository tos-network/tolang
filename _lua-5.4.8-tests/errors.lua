-- $Id: testes/errors.lua $
-- See Copyright Notice in file all.lua


-- avoid problems with 'strict' module (which may generate other error messages)
local mt = getmetatable(_G) or {}
local oldmm = mt.__index
mt.__index = nil

local function checkerr (msg, f, ...)
  local st, err = pcall(f, ...)
  assert(not st and string.find(err, msg))
end


-- test error message with no extra info

-- test error message with no info


-- test common errors/errors that crashed in the past


-- tests for better error messages


do
  -- non string messages
  local t = {}
  local res, msg = pcall(function () error(t) end)
  assert(not res and msg == t)

  res, msg = pcall(function () error(nil) end)
  assert(not res and msg == nil)

  local function f() error{msg='x'} end
  res, msg = xpcall(f, function (r) return {msg=r.msg..'y'} end)
  assert(msg.msg == 'xy')

  -- 'assert' with extra arguments
  res, msg = pcall(assert, false, "X", t)
  assert(not res and msg == "X")

  -- 'assert' with non-string messages
  res, msg = pcall(assert, false, t)
  assert(not res and msg == t)

  res, msg = pcall(assert, nil, nil)
  assert(not res and msg == nil)

  -- 'assert' without arguments
  res, msg = pcall(assert)
  assert(not res and string.find(msg, "value expected"))
end

-- xpcall with arguments
local a, b, c = xpcall(string.find, error, "alo", "al")
assert(a and b == 1 and c == 2)
a, b, c = xpcall(string.find, function (x) return {} end, true, "al")
assert(not a and type(b) == "table" and c == nil)


-- testing syntax limits


-- testing other limits

-- upvalues


mt.__index = oldmm

