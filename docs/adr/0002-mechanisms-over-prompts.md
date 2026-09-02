# ADR 0002: Prefer machine-checked mechanisms over prompt text

**Status:** accepted, 2026-09-02

## Context
Instruction compliance decays 5.6% per function written and drops to 20-60% by turn 6-10 (doc 04 §2.7). Prompt-only tools show about 10% cost effect and no detectable quality effect. Hooks, sandboxes, structural indexes, and verification gates have evidence that does not depend on the model reading anything.

## Decision
Any behavioural rule that can be expressed as a check is implemented as a check (hook, gate, diff guard, lint). Prompt text is allowed only as a thin adapter over a checked mechanism, must be under a declared token budget, and must report its cost in `saga trace`.

## Consequences
Saga will look less impressive than a 300-skill repo and will be harder to build. It will also keep working when the model stops listening, and across model upgrades.
