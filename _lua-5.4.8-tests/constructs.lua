-- $Id: testes/constructs.lua $
-- See Copyright Notice in file all.lua


-- testing semicollons
local a
do ;;; end
; do ; a = 3; assert(a == 3) end;
;


-- invalid operations should not raise errors when not executed
if false then a = 3 // 0; a = 0 % 0 end


-- testing priorities

assert(2^3^2 == 2^(3^2));
assert(2^3*4 == (2^3)*4);
assert(not nil and 2 and not(2>3 or 3<2));
assert(-3-1-5 == 0+0-9);
assert(not(2+1 > 3*1) and "a".."b" > "a");

assert(0xF0 | 0xCC ~ 0xAA & 0xFD == 0xF4)
assert(0xFD & 0xAA ~ 0xCC | 0xF0 == 0xF4)
assert(0xF0 & 0x0F + 1 == 0x10)


assert(not ((true or false) and nil))
assert(      true or false  and nil)

-- old bug
assert((((1 or false) and true) or false) == true)
assert((((nil and true) or false) and true) == false)

local a,b = 1,nil;
local x = ((b or a)+1 == 2 and (10 or a)+1 == 11); assert(x);
x = (((2<3) or 1) == true and (2<3 and 4) == 4); assert(x);

local x, y = 1, 2;
assert((x>y) and x or y == 2);
x,y=2,1;
assert((x>y) and x or y == 2);

assert(1234567890 == tonumber('1234567890') and 1234567890+1 == 1234567891)


-- silly loops
repeat until 1; repeat until true;
while false do end; while nil do end;

do  -- test old bug (first name could not be an `upvalue')
 local a; local function f(x) x={a=1}; x={x=1}; x={G=1} end
end


local f = function (i)
  if i < 10 then return 'a';
  elseif i < 20 then return 'b';
  elseif i < 30 then return 'c';
  end;
end

assert(f(3) == 'a' and f(12) == 'b' and f(26) == 'c' and f(100) == nil)

for i=1,1000 do break; end;
local n=100;
local i=3;
local t = {};
local a=nil
while not a do
  a=0; for i=1,n do for i=i,1,-1 do a=a+1; t[i]=1; end; end;
end
assert(a == n*(n+1)/2 and i==3);
assert(t[1] and t[n] and not t[0] and not t[n+1])


local f = function (i)
  if i < 10 then return 'a'
  elseif i < 20 then return 'b'
  elseif i < 30 then return 'c'
  else return 8
  end
end

assert(f(3) == 'a' and f(12) == 'b' and f(26) == 'c' and f(100) == 8)

local a, b = nil, 23
x = {f(100)*2+3 or a, a or b+2}
assert(x[1] == 19 and x[2] == 25)
x = {f=2+3 or a, a = b+2}
assert(x.f == 5 and x.a == 25)

a={y=1}
x = {a.y}
assert(x[1] == 1)

local function f (i)
  while 1 do
    if i>0 then i=i-1;
    else return; end;
  end;
end;

local function g(i)
  while 1 do
    if i>0 then i=i-1
    else return end
  end
end

f(10); g(10);

do
  function f () return 1,2,3; end
  local a, b, c = f();
  assert(a==1 and b==2 and c==3)
  a, b, c = (f());
  assert(a==1 and b==nil and c==nil)
end

local a,b = 3 and f();
assert(a==1 and b==nil)

function g() f(); return; end;
assert(g() == nil)
function g() return nil or f() end
a,b = g()
assert(a==1 and b==nil)


do   -- testing constants


end


function g (a,b,c,d,e)
  if not (a>=b or c or d and e or nil) then return 0; else return 1; end;
end

local function h (a,b,c,d,e)
  while (a>=b or c or (d and e) or nil) do return 1; end;
  return 0;
end;


assert(g(2,1) == 1 and h(2,1) == 1)
assert(g(1,2,'a') == 1 and h(1,2,'a') == 1)
assert(g(1,2,nil,1,'x') == 1 and h(1,2,nil,1,'x') == 1)
assert(g(1,2,nil,nil,'x') == 0 and h(1,2,nil,nil,'x') == 0)
assert(g(1,2,nil,1,nil) == 0 and h(1,2,nil,1,nil) == 0)

assert(1 and 2<3 == true and 2<3 and 'a'<'b' == true)
x = 2<3 and not 3; assert(x==false)
x = 2<1 or (2>1 and 'a'); assert(x=='a')


do
  local a; if nil then a=1; else a=2; end;    -- this nil comes as PUSHNIL 2
  assert(a==2)
end

local function F (a)
  return a,2,3
end

a,b = F(1)~=nil; assert(a == true and b == nil);
a,b = F(nil)==nil; assert(a == true and b == nil)

----------------------------------------------------------------
------------------------------------------------------------------

-- sometimes will be 0, sometimes will not...


------------------------------------------------------------------

-- testing some syntax errors (chosen through 'gcov')

