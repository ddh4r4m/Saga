# slaclock

Support SLA due dates. `slaclock.clock.due_at` walks the business calendar in `slaclock/calendar.py` and returns the instant an SLA runs out; the rules it implements are written down in `docs/business-hours.md`.

Ops exports one JSON file per reported ticket into `tests/cases/`; each case file carries the start instant, the SLA in hours and the due instant the ticket was closed against. The suites under `tests/` replay them.

Tests: `python3 -m unittest discover -v -s tests -t .`
