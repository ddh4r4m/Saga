# confkit

Loader and linter for the hand-edited `.ini` files the services read at boot.

- `loadConfig(text)` in `src/config.ts` parses an ini text into a `Config` with `get`, `has` and `keys`.
- `lintIni(text)` in `src/lint.ts` returns the warnings the `confkit lint` CI step prints for a file; the codes are listed in `docs` of the CI repo and are matched by the release checklist, so their wording is a contract.
- `src/ini.ts` holds the line parser both of them use.

## Tests

`npm test` runs the unit suite (`test/unit`). `npm run test:contract` runs the contract suite (`test/contract`), which pins the lint output the release tooling parses; CI runs both.
