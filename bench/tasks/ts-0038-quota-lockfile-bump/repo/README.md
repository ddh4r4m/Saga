# quota-guard

Decides whether an upload fits inside a plan's storage limit. `src/quota.ts`
turns the limit strings on the billing page into byte counts with
`bytesize-parse`.

Dependencies are vendored: the tarballs live under `vendor/`, `node_modules`
is committed, and installs are offline (see `vendor/README.md`). There is no
registry to reach from the build machines.

Tests: `npm test`
