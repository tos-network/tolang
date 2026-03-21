# Caller Capability Syntax

## Purpose

This document is the design home for compiler-enforced caller capability
syntax, centered on:

`@requires(caller: Cap)`

The goal is to move common access-control patterns out of hand-rolled contract
logic and into the language, sema, lowering, artifact, and metadata layers.

## Why this matters

Today, many stdlib seeds still use manually written caller checks such as:

- `require(msg.sender == owner)`
- `require(msg.sender == sponsor)`
- `require(msg.sender == relayer)`

That is workable, but it does not match the agent-native ambition of Tolang.

If caller capability is a first-class language concept, agents and tools can
inspect, route, and reason about authority requirements before execution.

## Scope

This document should define:

- source syntax
- semantic meaning
- interaction with existing `capability` declarations
- interaction with `@delegated`, `@verifiable`, and `@pay`
- lowering strategy
- ABI / metadata emission
- failure behavior and diagnostics

## Non-goals

This document is not the place to fully design:

- off-chain registry policy
- proof systems for capabilities
- package publishing identity

Those belong in broader protocol documents.

## Core questions

1. What exact syntax should be supported in v1?
2. Does the capability apply to caller identity, caller role, or both?
3. How does it compose with delegation and account abstraction?
4. What bytecode/runtime checks does lowering emit?
5. How should this appear in ABI, discovery, and agent-package metadata?
6. What is the migration path for existing hand-rolled checks?

## Proposed sections

1. Problem statement
2. Syntax candidates
3. Sema rules
4. Lowering model
5. Runtime assumptions
6. ABI / metadata impact
7. Diagnostics and error model
8. Compatibility and migration
9. Test plan

## Initial work packages

- define the minimum viable syntax
- define sema validation rules
- define metadata representation
- define interaction with delegation and policy-wallet patterns
- define regression coverage across parser, sema, lowering, and runtime

## Acceptance for this design doc

This document is ready for implementation when it makes clear:

- exactly what the syntax means
- how the compiler enforces it
- what metadata is emitted
- how existing stdlib contracts should migrate

## Related documents

- `docs/AGENT_NATIVE_STDLIB_2046.md`
- `docs/TOLANG_SHORTCOMINGS.md`
- `docs/FEATURE_MATURITY_MATRIX.md`
- `docs/TOLANG_LANGUAGE_DESIGN_PRINCIPLES.md`

