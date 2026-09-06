# metering

Monthly usage reporting for the customer portal. `metering.report.build_report` turns usage rows into report rows; the region display name on each row comes from the Grid9 region registry (see `docs/region-registry.md`). The fixtures under `fixtures/usage/` are real exports with the account names replaced.

Tests: `python3 -m unittest discover -v -s tests -t .`
