# Contributing to cartctl

The order desk drives cartctl from scripts that parse its output, so the conventions below are not style, they are interface.

## Errors

1. Every user-facing failure goes through `fail(code, message)` from `src/errors.ts`. Never `throw new Error(...)` from anything under `src/commands/`; a bare Error reaches the user as a stack trace and the desk scripts cannot match it.
2. Codes are `E_` followed by upper snake case, registered in `ERROR_CODES` in `src/errors.ts`, and documented with one table row in `docs/errors.md` (code, message, when it happens). `fail` refuses a code that is not registered, and review refuses a code that is not documented.
3. The CLI prints `error <code>: <message>` on stderr and exits 2 for a `CliError`; anything else is a bug and exits 70.

## Commands

4. A command is a pure function over the cart in `src/commands/`; it returns the new cart and does not print.
5. No new dependencies; the desk machines install nothing.

## Tests

6. Tests live in `test/` and run with `node --test`.
