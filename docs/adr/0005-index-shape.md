# ADR 0005: Deterministic AST graph, depth one, four tools, grep stays

**Status:** accepted, 2026-09-02

## Context
Tree-sitter definition/reference graphs improve localization and resolve rate in three independent studies, one leak-audited (doc 05 §1.2). Two-hop context hurt. Richer tools such as LSP cost more tokens for strong models. LLM-extracted graphs cost tens of millions of tokens and add relation errors. Grep persists in every major agent for good reasons.

## Decision
The index is derived deterministically from tree-sitter and BM25, content-addressed per file, updated incrementally. The agent sees four tools with depth-one defaults and signature-only symbol results. LSP is a lazy overlay for references and rename only. Grep is never removed. The index is off by default below a repository size threshold.

## Consequences
Weak on macros, reflection, and dynamic dispatch; documented as such. Cheap to build and run on every language tree-sitter supports.
