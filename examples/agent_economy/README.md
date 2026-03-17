# Agent Economy Examples

Canonical agent-economy patterns for the TOL smart contract language.

## Contracts

- **TaskEscrow.tol** -- Agent-to-agent task marketplace with escrow deposit, worker submission, poster approval/rejection, and oracle-backed dispute resolution.

- **OracleResolver.tol** -- Time-bounded oracle data feed. Authorized feeders fulfill write-once queries before expiry; consumers read resolved values with proof-friendly `@verifiable` views.

- **RecurringPayment.tol** -- Configurable recurring payment / subscription. Owner authorizes periodic transfers; any agent (keeper/cron) can trigger execution after interval elapses.

- **SponsorRelay.tol** -- Sponsor-aware relay for delegated execution. Sponsors deposit funds, relayers execute calls on behalf of users with per-relayer gas budgets. Uses `@delegated` for signature verification.

- **MerchantPayment.tol** -- Merchant payment flow for card-present, POS, and voice-triggered scenarios. Terminal-class restrictions, settlement with refund windows, and `@verifiable` receipt metadata.

## Patterns Demonstrated

| Pattern | Contracts |
|---|---|
| Escrow / release / slash | TaskEscrow |
| Oracle data feeds | OracleResolver |
| Capability-gated access | TaskEscrow, OracleResolver |
| Sponsor-delegated execution | SponsorRelay |
| Terminal-scoped authority | MerchantPayment |
| Proof-friendly metadata (`@verifiable`) | OracleResolver, MerchantPayment |
| Recurring / cron triggers | RecurringPayment |

## Compilation

```bash
tol compile examples/agent_economy/TaskEscrow.tol
tol compile examples/agent_economy/OracleResolver.tol
tol compile examples/agent_economy/RecurringPayment.tol
tol compile examples/agent_economy/SponsorRelay.tol
tol compile examples/agent_economy/MerchantPayment.tol
```
