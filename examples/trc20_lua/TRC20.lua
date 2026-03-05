-- TRC20 token contract written in raw Lua for the gtos VM.
--
-- Storage layout:
--   tos.get/set("total_supply")             → uint256 total supply
--   tos.mapGet("balances", addr)            → uint256 balance
--   tos.mapGet2("allowances", owner, spender) → uint256 allowance
--
-- Dispatch: tos.dispatch routes calldata selector to handler functions.
-- Constructor: tos.oncreate runs once at deploy time.

local ZERO = tos.ZERO_ADDRESS

-- ── Helpers ──────────────────────────────────────────────────────────────────

local function mapGet2(name, k1, k2)
    -- Two-level mapping: allowances[owner][spender]
    -- We encode the key as keccak256(k1 .. "::" .. name .. "::" .. k2)
    local inner = k1 .. "::" .. name
    return tos.mapGet(inner, k2)
end

local function mapSet2(name, k1, k2, val)
    local inner = k1 .. "::" .. name
    tos.mapSet(inner, k2, val)
end

-- ── Constructor ───────────────────────────────────────────────────────────────

tos.oncreate(function()
    local data = tos.msg.data          -- ABI-encoded constructor args
    local initialSupply = tos.abi.decode(data, "uint256")
    local owner = tos.msg.sender

    tos.set("total_supply", initialSupply)
    tos.mapSet("balances", owner, initialSupply)
    tos.emit("Transfer", "address", ZERO, "address", owner, "uint256", initialSupply)
end)

-- ── View functions ────────────────────────────────────────────────────────────

local function totalSupply()
    local s = tos.get("total_supply")
    tos.result(tos.abi.encode("uint256", s))
end

local function balanceOf(account)
    local bal = tos.mapGet("balances", account) or "0"
    tos.result(tos.abi.encode("uint256", bal))
end

local function allowance(owner, spender)
    local a = mapGet2("allowances", owner, spender) or "0"
    tos.result(tos.abi.encode("uint256", a))
end

-- ── State-changing functions ──────────────────────────────────────────────────

local function transfer(to, value)
    tos.require(to ~= ZERO, "ZERO_ADDRESS")

    local from = tos.msg.sender
    local fromBal = tos.mapGet("balances", from) or "0"
    tos.require(fromBal >= value, "INSUFFICIENT_BALANCE")

    tos.mapSet("balances", from, fromBal - value)
    local toBal = tos.mapGet("balances", to) or "0"
    tos.mapSet("balances", to, toBal + value)
    tos.emit("Transfer", "address", from, "address", to, "uint256", value)
    tos.result(tos.abi.encode("bool", true))
end

local function approve(spender, value)
    tos.require(spender ~= ZERO, "ZERO_ADDRESS")

    local owner = tos.msg.sender
    mapSet2("allowances", owner, spender, value)
    tos.emit("Approval", "address", owner, "address", spender, "uint256", value)
    tos.result(tos.abi.encode("bool", true))
end

local function transferFrom(from, to, value)
    tos.require(from ~= ZERO, "ZERO_ADDRESS")
    tos.require(to ~= ZERO, "ZERO_ADDRESS")

    local spender = tos.msg.sender
    local allowed = mapGet2("allowances", from, spender) or "0"
    tos.require(allowed >= value, "INSUFFICIENT_ALLOWANCE")

    local fromBal = tos.mapGet("balances", from) or "0"
    tos.require(fromBal >= value, "INSUFFICIENT_BALANCE")

    mapSet2("allowances", from, spender, allowed - value)
    tos.mapSet("balances", from, fromBal - value)
    local toBal = tos.mapGet("balances", to) or "0"
    tos.mapSet("balances", to, toBal + value)
    tos.emit("Transfer", "address", from, "address", to, "uint256", value)
    tos.result(tos.abi.encode("bool", true))
end

-- ── Dispatch ──────────────────────────────────────────────────────────────────

tos.dispatch({
    ["totalSupply()"] = function(cd)
        totalSupply()
    end,
    ["balanceOf(address)"] = function(cd)
        local account = tos.abi.decode(cd, "address")
        balanceOf(account)
    end,
    ["allowance(address,address)"] = function(cd)
        local owner, spender = tos.abi.decode(cd, "address", "address")
        allowance(owner, spender)
    end,
    ["transfer(address,uint256)"] = function(cd)
        local to, value = tos.abi.decode(cd, "address", "uint256")
        transfer(to, value)
    end,
    ["approve(address,uint256)"] = function(cd)
        local spender, value = tos.abi.decode(cd, "address", "uint256")
        approve(spender, value)
    end,
    ["transferFrom(address,address,uint256)"] = function(cd)
        local from, to, value = tos.abi.decode(cd, "address", "address", "uint256")
        transferFrom(from, to, value)
    end,
    ["fallback()"] = function()
        tos.revert("UNKNOWN_SELECTOR")
    end,
})
