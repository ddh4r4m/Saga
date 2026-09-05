"""Per-department totals from the payroll export."""

import csv
from collections.abc import Iterable
from decimal import Decimal, InvalidOperation


def parse_amount(text: str) -> Decimal:
    try:
        return Decimal(text).quantize(Decimal("0.01"))
    except InvalidOperation:
        raise ValueError(f"bad amount {text!r}") from None


def department_totals(lines: Iterable[str]) -> dict[str, Decimal]:
    totals: dict[str, Decimal] = {}
    reader = csv.reader(lines)
    next(reader, None)  # header
    for row in reader:
        if not row:
            continue
        _, department, amount = row
        totals[department] = totals.get(department, Decimal("0.00")) + parse_amount(amount)
    return totals
