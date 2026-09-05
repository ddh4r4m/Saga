# visitlog

Counts page visits for the docs site. Counts live in a `kvlite` store; the library is vendored under `vendor/kvlite` at the version the licence review approved (see its README) and is not managed by npm.

`recordVisit(store, page)` in `src/visits.ts` records one visit and returns the new count; `report(store)` renders the counts.

`npm test` runs the suite under `test/`.
