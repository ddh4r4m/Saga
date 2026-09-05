# stockctl

Small command line tool over the stock catalogue in `data/items.json`.

## Commands

- `stockctl export [--format csv]` prints the catalogue as CSV, one row per item in SKU order, header first. Ends with a summary line `exported N items`.

Run with `node src/cli.ts <command> [options]` or `npm run export -- [options]`.

## Development

`npm test` runs the suite under `test/`. Source is under `src/`; nothing needs building.
