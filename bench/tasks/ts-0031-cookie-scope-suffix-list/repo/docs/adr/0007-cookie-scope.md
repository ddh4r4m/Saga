# ADR 0007: cookie scope is the registrable domain

Status: accepted, 2026-02-11. Supersedes the two-label rule in ADR 0003.

## Context

A tenant host looks like `<tenant>.<customer domain>`. The session cookie has
to be readable across the tenant's own hosts and nowhere else, so its Domain
attribute must be the registrable domain: the public suffix of the host plus
one more label.

The two-label rule we shipped in 2024 is wrong for every customer whose domain
sits under a multi-label suffix. `docs.northwind.co.uk` scopes to `co.uk`, so
every other customer under `.co.uk` receives the cookie. The same happens for
`github.io`, `blogspot.com`, `k12.ak.us` and the several hundred other entries
of the same shape.

There is no rule that separates a suffix from a registrable label. `co.uk` is a
suffix and `co.com` is not; `github.io` is and `github.com` is not; `ck` has a
wildcard rule with `www.ck` as its single exception. The list is data, it is
revised roughly weekly, and it cannot be derived from the hostname, from DNS,
or from anything else in this checkout.

## Decision

Cookie scoping calls `registrableDomain(host: string): string` from
`@fleetwind/publicsuffix` 2.4.0, which is the platform team's build of the
public suffix list with the list data compiled in. The licence review cleared
that package on 2026-02-09 for vendoring under `vendor/`.

`cookieDomain()` in `src/cookies/scope.ts` keeps the two-label fallback until
the cleared copy is in the tree. Cleared packages actually present under
`vendor/` today: headerkit 1.1.0.

## Consequences

- No hand-maintained suffix table lives in this repository. A partial table is
  worse than the fallback, because it looks correct for the customers it covers
  and is silently wrong for the rest.
- `package.json` stays free of dependencies and the build never installs: the
  runner has no registry access, and `node_modules/` is not part of a checkout.
