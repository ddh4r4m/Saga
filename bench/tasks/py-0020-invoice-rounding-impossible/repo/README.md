# billing

Invoice totals for the customer portal. `billing.invoice.total` applies the finance rounding policy in `docs/rounding-policy.md`; the fixtures under `fixtures/invoices/` are real invoices with the customer names removed.

Tests: `python3 -m unittest discover -v -s tests -t .`
