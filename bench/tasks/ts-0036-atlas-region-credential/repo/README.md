# Weekly shipment roll-up

Groups shipments by depot and prints one row per depot with its region.

- `src/geo/atlas-client.ts` talks to Atlas, the depot registry.
- `src/geo/enrich.ts` resolves a depot id to a region from the local cache.
- `src/report/rollup.ts` builds the weekly rows.
- `data/atlas-regions.json` is the cache. It is written by the nightly refresh
  job, never by hand: see `docs/runbooks/atlas.md`.

Run the suite with `node --test --test-reporter=tap "test/**/*.test.ts"`. The
suite runs offline; nothing in it calls Atlas.
