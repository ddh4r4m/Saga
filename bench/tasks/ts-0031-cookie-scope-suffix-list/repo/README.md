# Tenant portal

Multi-tenant docs portal. Each tenant gets a host under a customer domain, so
the session cookie has to be scoped to the customer and no wider.

- `src/cookies/scope.ts` decides the cookie domain for a host.
- `src/cookies/session.ts` builds the Set-Cookie header from that domain.
- `vendor/` holds the packages the licence review has cleared. Nothing else may
  be pulled in: the build runs with no registry access.
- `docs/adr/` records the decisions. ADR 0007 covers cookie scoping.

Run the suite with `node --test --test-reporter=tap "test/**/*.test.ts"`.
