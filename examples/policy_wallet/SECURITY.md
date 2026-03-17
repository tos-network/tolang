# Policy Wallet Security Model

## Overview

The policy wallet contract suite implements a programmable account abstraction
(AA) wallet with layered security controls: spend caps, allowlists, terminal
authority, guardian recovery, delegation, and suspension.

## Security Assumptions

1. **Single owner model.** Each wallet has exactly one owner agent at any time.
   The owner is the ultimate authority for all non-recovery operations.

2. **Guardian is trusted but limited.** The guardian can initiate recovery and
   suspend the wallet, but cannot spend funds, modify policies, or lift
   suspension. The guardian and owner must be distinct agents.

3. **Timelock as a safety net.** Recovery operations require a mandatory
   timelock period during which the current owner can cancel. The timelock
   must be non-zero to prevent instant hostile takeover.

4. **Delegates are bounded.** Delegated agents operate within explicit value
   caps and time windows. Expiry is checked before allowance consumption to
   prevent expired delegates from executing transactions.

5. **Terminal authority is owner-gated.** Only the wallet owner (or the
   composing contract) can call `checkTerminalAuth`, preventing external
   actors from manipulating daily spend counters.

6. **u256 arithmetic.** All value arithmetic uses u256. Overflow in
   `daily_spent + value` is checked via `require(daily_spent + value <= limit)`
   which will revert if the addition overflows. In practice, u256 overflow
   from real value transfers is computationally infeasible.

## Audit Findings and Fixes Applied

### Critical

| # | Contract | Issue | Fix |
|---|----------|-------|-----|
| 1 | DelegatedAgent | `use()` had no authorization -- any external caller could drain a delegate's allowance | Added `require(msg.sender == owner \|\| msg.sender == delegate)` |
| 2 | TerminalAuthority | `checkTerminalAuth()` was callable by anyone, allowing DoS by exhausting daily spend limits | Added `require(msg.sender == owner)` |
| 3 | PolicyWallet | Delegate expiry checked AFTER allowance in `execute()` -- expired delegate could pass allowance check | Reordered: expiry check now happens before allowance check |

### High

| # | Contract | Issue | Fix |
|---|----------|-------|-----|
| 4 | GuardianRecovery | `removeGuardian()` did not decrement `guardian_count`, causing count to drift upward permanently | Added `guardian_count = guardian_count - 1` and threshold feasibility check |
| 5 | TaskEscrow | `reclaimExpired()` callable by anyone -- third parties could force-cancel tasks the poster intended to keep | Added `require(msg.sender == poster)` |
| 6 | GuardianRecovery | Constructor allowed `threshold = 0`, enabling instant recovery without any guardian approval | Added `require(_threshold > 0)` |

### Medium

| # | Contract | Issue | Fix |
|---|----------|-------|-----|
| 7 | PolicyWallet | Zero-address checks missing in constructor for owner/guardian | Added `require != agent(0)` checks |
| 8 | PolicyWallet | Owner and guardian could be the same agent, undermining separation of concerns | Added `require(_owner != _guardian)` |
| 9 | PolicyWallet | `completeRecovery()` left stale `recovery_new_owner` and `recovery_initiated_at` in storage | Clear both fields after recovery completes |
| 10 | PolicyWallet | `initiateRecovery()` accepted zero-address or current owner as new owner | Added validation checks |
| 11 | PolicyWallet | `authorizeDelegate()` accepted zero-address or owner as delegate | Added validation checks |
| 12 | PolicyWallet | `execute()` allowed zero-value transfers and zero-address recipients | Added `require(value > 0)` and `require(to != agent(0))` |
| 13 | GuardianRecovery | `startRecovery()` accepted zero-address or current owner as proposed owner | Added validation checks |
| 14 | GuardianRecovery | `executeRecovery()` left stale `proposed_owner`, `initiated_at`, `approval_count` | Clear all fields after execution |
| 15 | GuardianRecovery | `removeGuardian()` could reduce guardians below threshold | Added `require(guardian_count >= threshold)` post-removal |
| 16 | DelegatedAgent | `grant()` accepted zero-address delegate and zero allowance | Added validation checks |
| 17 | TaskEscrow | `acceptTask()` allowed poster to accept their own task | Added `require(msg.sender != task_poster)` |

### Low

| # | Contract | Issue | Fix |
|---|----------|-------|-----|
| 18 | All contracts | Missing zero-address checks in constructors | Added `require(_owner != agent(0))` to all constructors |
| 19 | SpendGuard | `checkAndSpend()` accepted zero-value spends and zero-address recipients | Added validation checks |
| 20 | SponsorRelay | `authorizeRelayer()` accepted zero-address relayer | Added `require(relayer != agent(0))` |
| 21 | SponsorRelay | `relay()` accepted zero-address target and zero cost | Added validation checks |
| 22 | MerchantPayment | `pay()` allowed self-payment and zero-address merchant | Added validation checks |
| 23 | MerchantPayment | Constructor allowed zero refund window | Added `require > 0` |

## Known Limitations

1. **No reentrancy guard in `execute()`.** The PolicyWallet `execute()` emits
   an event but does not perform an external call (the actual transfer is
   handled by the AA protocol layer after validation). If the protocol changes
   to allow inline calls, a reentrancy guard should be added.

2. **Guardian removal does not compact the index array.** The
   `GuardianRecovery` contract marks guardians inactive via `is_guardian` but
   does not remove them from the `guardians` index mapping. This is a known
   gas optimization tradeoff -- the index may contain stale entries.

3. **Dispute bond handling in TaskEscrow.** The worker's dispute bond is
   escrowed but the current `resolveDispute()` only releases the original task
   reward. A production deployment should also release or slash the dispute
   bond based on the resolution outcome.

4. **No multi-sig or time-delay on policy changes.** Spend cap updates,
   allowlist changes, and delegation grants take effect immediately when called
   by the owner. A production wallet may want a confirmation delay for
   critical policy changes.

5. **Front-running on `approveTask()`.** A poster could observe a worker's
   `submitTask()` transaction in the mempool and front-run with a rejection.
   This is mitigated by the dispute mechanism but not fully prevented.

6. **Daily spend counter uses wall-clock days.** The daily reset is based on
   `block.timestamp_ms / 86400000`, which means a spend near midnight can
   effectively double the daily limit across the day boundary.

7. **Single oracle resolver in TaskEscrow.** The dispute resolution depends on
   a single oracle agent. If the oracle is compromised or unavailable, disputed
   tasks may be locked indefinitely.

## Contract Dependency Graph

```
PolicyWallet (account contract)
  |-- SpendGuard       (composable spend-cap module)
  |-- TerminalAuthority (per-terminal-class limits)
  |-- DelegatedAgent    (bounded agent delegation)
  |-- GuardianRecovery  (threshold-based recovery)
```

## Recommendations for Production

- Set `recovery_timelock` to at least 48 hours (172800000 ms).
- Use a guardian threshold of at least 2-of-3.
- Enable the allowlist for high-value wallets.
- Configure terminal class policies before enabling terminal-based spending.
- Monitor `RecoveryInitiated` events and set up alerts for the owner.
- Implement dispute bond distribution in TaskEscrow before deployment.
