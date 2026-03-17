# Policy Wallet Templates — GTOS 2046 Architecture

Canonical TOL contract templates implementing the 2046 architecture's policy wallet primitives. These contracts provide programmable, policy-bound account abstraction for the GTOS LVM.

## Contracts

### PolicyWallet.tol
The main account contract (AA-compatible). Combines all policy primitives into a single programmable account: spend caps, allowlists, terminal-scoped authority, guardian recovery with timelock, delegated agent authority, and suspension.

### SpendGuard.tol
Focused spend-cap and allowlist policy module. Enforces daily aggregate limits, per-transaction limits, and an optional recipient allowlist. Designed for standalone use or composition with other policy contracts.

### GuardianRecovery.tol
Threshold-based guardian recovery. Multiple guardians vote to recover a wallet's ownership. Requires a configurable number of approvals (threshold) and enforces a timelock before execution. Uses a nonce-based approval invalidation scheme for safe cancellation.

### TerminalAuthority.tol
Per-terminal-class spending limits and trust enforcement. Configures value limits and trust tier requirements for different terminal types (app, card, POS, voice, kiosk, robot, API). Supports individual terminal device revocation.

### DelegatedAgent.tol
Agent delegation with bounded authority. Grants time-bounded, value-capped authority to delegated agents. Tracks cumulative allowance consumption. Includes the `DelegateManager` capability declaration.

## Compilation

```
tol compile examples/policy_wallet/PolicyWallet.tol
tol compile examples/policy_wallet/SpendGuard.tol
tol compile examples/policy_wallet/GuardianRecovery.tol
tol compile examples/policy_wallet/TerminalAuthority.tol
tol compile examples/policy_wallet/DelegatedAgent.tol
```

## Architecture

These templates follow the 2046 architecture's separation of concerns:

- **PolicyWallet** is the unified AA account that end users deploy.
- **SpendGuard**, **GuardianRecovery**, **TerminalAuthority**, and **DelegatedAgent** are modular policy contracts that can be deployed independently or used as reference implementations for the corresponding subsystems within PolicyWallet.
