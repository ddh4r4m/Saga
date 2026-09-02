# ADR 0001: Ship the measurement harness before any feature

**Status:** accepted, 2026-09-02

## Context
Of roughly sixty tools surveyed in doc 04, two have an independent paired A/B, and both delivered a fraction of their advertised effect. Vendor-run numbers in this space are inflated four to seven times. The credibility of Saga depends on not repeating that.

## Decision
`saga bench` is milestone zero. No component may be listed as stable, or make a quantitative claim in the README, without an ablation on the bench with the control arm blocked from reaching the component, at least five clean-room runs per cell, and pass^k, tokens, and cost reported with variance. Negative results are committed.

## Consequences
Slower first release. Every later feature has a built-in regression test against hype. The bench itself becomes a deliverable other projects can use.
