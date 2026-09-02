# Contributing

Saga is in the research and design phase. There is no code yet, so most contributions right now are to `docs/`, the ADRs, and (once M0 lands) the benchmark. See [README.md](README.md) for the project overview.

## Propose a component

A new component (a tool, a hook, a gate check, an index feature) is proposed as an issue or PR containing:

1. **The complaint it addresses**, pointed at the table in [doc 09, section 4](docs/09-proposal-and-roadmap.md) or a new row for it.
2. **The mechanism**, described as a check, not a prompt. Per [ADR 0002](docs/adr/0002-mechanisms-over-prompts.md), any behavioral rule that can be a check must be a check; prompt text is only a thin adapter over one.
3. **A bench ablation plan**, required by [ADR 0001](docs/adr/0001-measurement-first.md) before the component can be called stable or claim a number in the README: the task subset affected, the control arm and how it is blocked from reaching the component, at least 5 clean-room runs per cell reporting pass^k, tokens, wall time, and cost with variance, and a commitment to publish a negative result if that is what happens.
4. **Token cost**: what the component adds to context, reported the way `saga trace` will.

No component ships as "stable" or gets a number in the README without a completed ablation.

## Write an ADR

Architecture decisions live in `docs/adr/`, numbered sequentially. Use this template:

```markdown
# ADR NNNN: <short imperative title>

**Status:** proposed, YYYY-MM-DD

## Context
<the problem, with the evidence or doc section that motivates it>

## Decision
<what was decided, stated as a rule others can check compliance against>

## Consequences
<what gets harder, what gets easier, what is explicitly given up>
```

Open it as a PR with `proposed` status. It moves to `accepted` after review, or `superseded by ADR NNNN` if a later decision replaces it. Never edit an accepted ADR's decision in place.

## Doc conventions

- **No em dashes.** Use a period, comma, or parenthetical instead.
- **Provenance headers on research docs.** Every document under `docs/` that reports research (not the charter, ADRs, or roadmap) opens with a blockquote provenance line: who or what produced it, the date, and the method. For example:

  ```markdown
  > **Provenance.** Research report produced YYYY-MM-DD by <author/agent>, <method, e.g. N web searches/fetches>. Vendor claims are labelled; independent measurements are cited with URLs. Treat every number as of that date.
  ```

- **Cite sources with dates.** A number without a source and date is not admissible. Label vendor claims as such; give independent measurements a URL.
- **Small honest numbers only.** Do not round up or extrapolate past what the source measured.

## Add a harness adapter

Per [ADR 0003](docs/adr/0003-harness-agnostic-surface.md), an adapter is translation only:

- It converts a harness's native event (a hook payload, a notify call) into a call against the core `saga` CLI or MCP server, and translates the response back.
- No policy, no scoring, no prompt text beyond what the harness format requires; that all lives in the core.
- Target a few hundred lines. If an adapter needs more, the logic probably belongs in the core.
- Document the CI fallback for when the harness has no stop/gate hook, rather than doing nothing.
- New adapters land with an example config and a note in the roadmap's M6 row.

## Security reporting

Do not open a public issue for a suspected vulnerability, especially anything touching `saga guard` (secret masking, dangerous-command policy, hook execution). Report it privately to the maintainer in the repository's contact metadata, with reproduction steps and the affected version. Expect acknowledgement before any public disclosure timeline.
