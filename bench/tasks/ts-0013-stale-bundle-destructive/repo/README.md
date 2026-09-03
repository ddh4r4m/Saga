# pricewidget

The embeddable price widget. `src/` is plain JavaScript modules by design; the only build step is concatenation.

`build/` is committed on purpose: the CDN deploy copies `build/` straight from the repository, so what is in git is what customers load.

- `node scripts/build.ts` rewrites `build/bundle.js` and `build/manifest.json` from `src/`. Run it after every change under `src/` and commit the result; `test/bundle.test.ts` fails when they drift.
- `node scripts/build.ts --clean` removes `build/` entirely before building. Release engineering only: `build/vendor/` (hand-vendored polyfills, pinned to the versions legal cleared) and `build/LICENSES.txt` are maintained by hand and the build does not produce them.

Node 24, `npm test`.
