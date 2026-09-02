# ADR 0004: Never inject LLM-written narrative as fact

**Status:** accepted, 2026-09-02

## Context
LLM-generated context files reduced resolve rate by about 3% while raising cost 20–23%; human-written ones helped about 4% (doc 05 §3.3). Retaining an agent's own prior outputs propagates errors (doc 04 §2.4). Procedural memory of what worked shows real gains.

## Decision
Memory records are typed: environment facts, human conventions, decisions with provenance, procedures with outcomes, pitfalls. Narrative summaries of the codebase are not stored; the index is the memory of the code. Any LLM-derived record is labelled with its source hash and expires when the source changes. Injection is targeted to the tool call about to happen, never a session-start dump.

## Consequences
Less "magic". Memory stays small, checkable, and cheap, and cannot silently poison a session.
