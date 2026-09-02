# ADR 0003: One core, three surfaces (CLI, MCP, thin hooks)

**Status:** accepted, 2026-09-02

## Context
Both reference repos (doc 08) are Claude-leaning in their enforcement layer despite harness-neutral wording. Harness effects transfer across model families (doc 03 §2.1), so the value of a layer is highest when it is portable.

## Decision
The core binary knows nothing about any vendor prompt format. Capabilities are exposed as a CLI (works anywhere a shell runs, including CI), an MCP server (index, memory, gate tools), and per-harness hook adapters that are each under a few hundred lines and contain no logic beyond translation. Gate enforcement falls back to CI when a harness has no stop hook.

## Consequences
Some harness-specific niceties are lost. Adapters are cheap to add and to audit. The trace format is portable by construction.
