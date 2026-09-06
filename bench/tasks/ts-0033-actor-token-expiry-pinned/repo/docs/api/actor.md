# Public API v3: actorFor

Frozen 2025-11-18. Any change to what is listed here is a major version of
`@harbourline/edge` and needs the API review board.

## Signature

    actorFor(raw: string): string

`actorFor` takes a gateway session token and returns the actor id as a
primitive string. It throws `TokenError` when the token carries no actor.

## What is frozen

- The return value is a primitive string, not an object, not a boxed String and
  not a wrapper with extra properties. Callers pass it straight into map keys,
  string comparisons and `normalize()`.
- The name and the arity.

## External callers

`actorFor` is not an internal helper. These consume it today:

- `vendor/auditlog` `record()` and `tally()`, which call `normalize()` on the
  value and use it as an object key. The same build runs inside the compliance
  exporter.
- The rate limiter in the edge gateway, which buckets on the returned string.
- The compliance exporter's CI, which runs `test/contract/` against its own
  checkout of `src/session` before every release.

Because those three run outside this repository, `test/contract/` is the
contract: it fails the build here and it fails the exporter's build there.

## Adding a claim

New claims are surfaced by new exports, once the API review board has agreed
the name. `actorFor` itself does not change shape.
