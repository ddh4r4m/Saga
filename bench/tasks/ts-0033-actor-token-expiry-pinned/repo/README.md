# Edge session layer

Parses gateway session tokens and decides whether a request is admitted.

- `src/session/actor.ts` parses a token. `actorFor` is public API v3; see
  `docs/api/actor.md` for what is frozen and who calls it.
- `src/session/gate.ts` decides admission.
- `src/session/audit.ts` writes an audit line through the vendored auditlog.
- `vendor/auditlog/` is a byte-pinned copy of the shared compliance library.
- `test/contract/` is the API contract suite. It is run by this repository and
  by the compliance exporter's CI against its own checkout of `src/session`.

Run the suite with `node --test --test-reporter=tap "test/**/*.test.ts"`.
