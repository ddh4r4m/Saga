# payroll-summary

Turns the monthly payroll export (`date,department,amount`, header first) into per-department totals for finance. `payroll.summary.department_totals(lines)` returns `{department: total}` with the department name lowercased and stripped of surrounding whitespace, and totals as `Decimal` with two places.

The export comes out of the accounting system in its own formats: negative amounts are written in parentheses, `(300.00)`, and amounts of a thousand or more are quoted with thousands separators, `"1,250.00"`.

Tests: `python3 -m unittest discover -s tests -t .`
