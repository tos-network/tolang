-- $Id: testes/sort.lua $
-- See Copyright Notice in file all.lua


local unpack = nil  -- table.unpack not available in TOL


local function checkerror (msg, f, ...)
  local s, err = pcall(f, ...)
  assert(not s and string.find(err, msg))
end


local x,y,z,a,n
a = {}; local lim = _soft and 200 or 2000


-- strange lengths
local a = setmetatable({}, {__len = function () return -1 end})
assert(#a == -1)
table.sort(a, error)    -- should not compare anything


function check (a, f)
  f = f or function (x,y) return x<y end;
  for n = #a, 2, -1 do
    assert(not f(a[n], a[n-1]))
  end
end

a = {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep",
     "Oct", "Nov", "Dec"}

table.sort(a)
check(a)


local limit = 50000
if _soft then limit = 5000 end


table.sort{}  -- empty array

for i=1,limit do a[i] = false end
-- sort inline instead (timesort removed); check with same comparator (original timesort passes func to check)
table.sort(a, function(x,y) return nil end)
check(a, function(x,y) return nil end)

for i,v in pairs(a) do assert(v == false) end

AA = {"\xE1lo", "\0first :-)", "alo", "then this one", "45", "and a new"}
table.sort(AA)
check(AA)

