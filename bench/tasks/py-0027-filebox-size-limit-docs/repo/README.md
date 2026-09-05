# filebox

Small file drop used by the intake forms. `filebox.store.save(name, data)` stores a file, `filebox.store.load(name)` reads it back.

Read CONTRIBUTING.md before changing anything under `filebox/`; the support team's runbook is generated from `docs/errors.md` and the release notes from `CHANGELOG.md`, so both are part of every change.

Tests: `python3 -m unittest discover -s tests -t .`
