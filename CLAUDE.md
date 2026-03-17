## 2046 Architecture Components

- `metadata/` — Stable metadata schema (ContractMetadata, ArtifactRef,
  FunctionMeta, EffectsMeta, PolicyProfile), extraction from ABI, human-readable
  summaries, discovery manifests, compatibility matrix
- `examples/policy_wallet/` — 5 canonical policy wallet contract templates
  (PolicyWallet, SpendGuard, GuardianRecovery, TerminalAuthority, DelegatedAgent)
- `examples/agent_economy/` — 5 canonical agent economy contracts
  (TaskEscrow, OracleResolver, RecurringPayment, SponsorRelay, MerchantPayment)
- `e2e/` — Compile-to-discovery integration tests

Schema version `0.1.0` in `metadata/metadata.go`.
