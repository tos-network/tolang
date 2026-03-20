# What the TOL Standard Library Enables

**Date**: 2026-03-20
**Basis**: `docs/AGENT_NATIVE_STDLIB_2046.md` (first-principles design)

---

## One-Sentence Summary

The TOL stdlib lets an autonomous agent write a policy-constrained,
evidence-backed, privacy-preserving, multi-terminal commercial transaction
in under 50 lines of code.

---

## Capabilities by Wave

### Wave 1: Control Plane — Making Agents Safe to Use

| Scenario | Without stdlib | With stdlib |
|----------|--------------|-------------|
| Employee agent daily spend cap of 1000 TOS | Hand-rolled storage + time-window arithmetic | `import "stdlib/account"; set_daily_cap(employee, 1000)` |
| NFC card limited to 100 TOS at POS terminals | Hand-rolled terminal type check + amount guard | `import "stdlib/session"; require_terminal(POS, trust_medium, 100)` |
| Guardian recovers a compromised account | Hand-rolled timelock + challenge period + ownership transfer | `import "stdlib/recovery"; initiate_recovery(guardian, new_owner, 24h)` |
| AI agent delegated with revocable authority | Hand-rolled delegation map + expiry + revocation | `import "stdlib/authority"; delegate(ai_agent, cap: 500, expiry: 7d)` |
| Off-chain approval bound to on-chain execution | Hand-rolled nonce + policy hash + replay guard | `import "stdlib/execution_binding"; bind_approval(policy_hash, nonce, expiry)` |

**What this wave unlocks**: Multi-terminal consumer finance. The same account
is safely accessible from a mobile app, NFC card, POS terminal, voice
assistant, kiosk, and robot API — each with independent spending limits,
trust tiers, and revocation semantics.

---

### Wave 2: Execution Plane — Making Agents Commercially Productive

| Scenario | With stdlib |
|----------|-------------|
| Bounty task: post → claim → submit → approve → pay | `post_task("translation", spec, 50 TOS, 48h)` → `claim` → `submit` → `approve` (5 function calls) |
| Oracle: weather data written once, immutable | `resolve_oracle(query_id, answer)` — built-in write-once guard |
| Merchant POS payment, sponsor pays gas | `import "stdlib/sponsor"; sponsored_payment(merchant, amount, sponsor_policy)` |
| Monthly subscription auto-debit | `import "stdlib/settlement"; schedule(provider, 10 TOS, monthly, 12)` |
| Every settlement auto-generates audit receipt | `import "stdlib/receipt"; settlement_receipt(escrow_id, approved)` — evidence chain automatic |
| Confidential escrow with milestone release | `import "stdlib/settlement"; confidential_hold(payee, uno_amount, deadline)` → `release` |
| Quote → offer → acceptance → invoice lifecycle | `import "stdlib/agreement"; quote(provider, service, price, validity)` |

**What this wave unlocks**: Machine commerce. Agents don't just transfer
value — they express structured commitments (quotes, offers, invoices,
subscriptions), settle through explicit escrow lifecycles, and produce
machine-verifiable receipts. Every commercial action has a proof reference.

---

### Wave 3: Market Plane — Making Agent Economies Scale

| Scenario | With stdlib |
|----------|-------------|
| Encrypted payroll: employees see amounts, not each other | `import "stdlib/privacy"; confidential_payroll(employees, amounts)` |
| Auditor verifies financials without seeing individual txs | `auditor_disclosure_book(auditor_key, quarterly_snapshots)` |
| Agent filters counterparties by reputation | `import "stdlib/trust"; require_reputation(provider, min: 80)` |
| New agent advertises translation capability | `import "stdlib/discovery"; advertise_capability("translation", sla: 1h, fee: 5 TOS)` |
| Confidential treasury with selective disclosure | `import "stdlib/privacy"; ConfidentialTreasury(auditor_key)` |
| Stake-backed service guarantee | `import "stdlib/trust"; require_stake(provider, minimum: 1000 TOS)` |

**What this wave unlocks**: Scalable agent marketplaces. Agents discover each
other by capability, evaluate each other by reputation, transact with
encrypted values, and prove solvency without revealing balances. Trust is
quantified, not assumed.

---

## Comparison With Existing Ecosystems

| Capability | Solidity + OpenZeppelin | TOL + stdlib |
|------------|----------------------|--------------|
| Safe value transfer | `SafeERC20.safeTransfer()` library | Language-native `escrow` / `release` — no library needed |
| Reentrancy protection | `ReentrancyGuard` modifier | Compiler-enforced `@effects` + `set` keyword — no library needed |
| Access control | `Ownable` / `AccessControl` | `capability` + `@requires(caller: Cap)` — compiler-enforced, in ABI |
| Proxy delegation | `approve()` + `transferFrom()` | `stdlib/authority` — capped, time-bounded, instantly revocable |
| Multi-terminal support | Not supported | `stdlib/session` — 6 terminal types x 5 trust tiers |
| Encrypted transfers | Not supported | `uno.transfer()` + `stdlib/privacy` ConfidentialEscrow |
| Selective disclosure | Not supported | `stdlib/privacy` — three layers (ZK proof, decryption token, auditor key) |
| Task marketplace | ~200 lines hand-rolled state machine | `stdlib/agreement` + `stdlib/settlement` — 5 function calls |
| Gas sponsorship | ERC-4337 complex stack | `stdlib/sponsor` — native sponsor binding + policy |
| Machine-readable audit | Optional event logs | `stdlib/receipt` — mandatory structured evidence chain |
| Verifiable effects | Source audit required | `@effects` declarations verified by compiler, published in ABI |
| Gas bounds | Guessed or empirical | `@gas(upper: N)` verified by compiler, bound-checked |
| Terminal-scoped policy | Not supported | `stdlib/session` + `stdlib/account` — per-terminal-class ceilings |
| Guardian recovery | ~150 lines hand-rolled | `stdlib/recovery` — timelocked, cancellable, challenge-windowed |
| Discovery metadata | No standard | `stdlib/discovery` — manifest, capabilities, SLA, quote envelopes |
| Confidential DeFi | Not supported | `stdlib/privacy` + `stdlib/settlement` — dual-rail public/UNO |

---

## The Question TOL Answers That Solidity Does Not Ask

Solidity asks: *How do I write a safe DeFi contract?*

TOL asks:

> An AI agent enters through an NFC card at a POS terminal. Under policy
> constraints (daily cap, terminal limit, merchant allowlist), it pays with
> encrypted UNO balance for a service. The payment is escrowed until delivery
> is confirmed. A dispute triggers arbitration with selective disclosure to
> the arbitrator. The settlement produces a machine-auditable receipt with
> sponsor attribution and proof references. The agent's guardian can freeze
> the account at any time.
>
> **How many lines of contract code does this take?**

With the TOL stdlib: **under 50 lines.**

Without it: hundreds of lines of hand-rolled state machines, policy checks,
encryption handling, receipt formatting, and terminal discrimination logic —
repeated differently in every contract, with different bugs each time.

---

## What Each Package Eliminates

| Package | What developers no longer hand-write |
|---------|-------------------------------------|
| `account` | Spend cap arithmetic, allowlist storage, suspension flags |
| `authority` | Delegation maps, expiry checks, revocation propagation |
| `execution_binding` | Nonce management, policy hash binding, replay guards |
| `session` | Terminal type discrimination, trust tier checks, step-up logic |
| `recovery` | Timelock arithmetic, challenge periods, ownership transfer |
| `agreement` | Quote/offer/acceptance state machines |
| `settlement` | Escrow state machines, milestone tracking, slash distribution |
| `sponsor` | Sponsor authorization, budget tracking, attribution records |
| `evidence` | Oracle write-once guards, proof reference attachment |
| `receipt` | Receipt formatting, approval linkage, settlement traces |
| `trust` | Reputation queries, stake checks, scorer integration |
| `privacy` | UNO bridge wiring, disclosure flow setup, auditor view construction |
| `discovery` | Manifest construction, capability advertisement, version markers |

---

## The Commercial Value Proposition

TOL without stdlib: a language with nice syntax and incomplete economic
semantics.

TOL with stdlib: **the first smart contract platform where "agent-native"
is not marketing — it is the actual development experience.**

A developer importing `stdlib/settlement` and `stdlib/privacy` can write a
confidential escrow contract in 20 lines. The same contract works across
all terminal types, respects policy wallet constraints, produces audit
receipts, and supports selective disclosure to regulators — all without the
developer thinking about any of it.

That is what makes TOL commercially viable at scale.
