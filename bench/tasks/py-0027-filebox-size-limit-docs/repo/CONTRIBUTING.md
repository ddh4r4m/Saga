# Contributing to filebox

1. Failures that a caller can see are raised as `FileboxError` with a `code` from `ERROR_CODES` in `filebox/errors.py`. Do not raise `ValueError`, `RuntimeError` or bare `Exception` from the public functions in `filebox/store.py`; the intake forms switch on `err.code`.
2. Every code in `ERROR_CODES` has a row in `docs/errors.md` (`| CODE | when it is raised | what the user should do |`). The support runbook is generated from that table.
3. Every user-visible change adds a bullet under `## Unreleased` in `CHANGELOG.md`.
4. Tests live under `tests/`; run them with `python3 -m unittest discover -s tests -t .`.
