"""Row parsing for the bank export."""

import re
from dataclasses import dataclass
from datetime import date
from decimal import Decimal, InvalidOperation


class RowError(ValueError):
    """A data row that cannot be imported."""


class SinkError(RuntimeError):
    """The ledger refused a row; the batch cannot continue."""


@dataclass(frozen=True)
class Row:
    day: date
    account: str
    amount: Decimal


def parse_row(line: str) -> Row:
    parts = [p.strip() for p in line.rstrip("\n").split(",")]
    if len(parts) != 3:
        raise RowError(f"expected 3 fields, got {len(parts)}")
    day_text, account, amount_text = parts
    if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", day_text):
        raise RowError(f"bad date {day_text!r}")
    try:
        day = date.fromisoformat(day_text)
    except ValueError:
        raise RowError(f"bad date {day_text!r}") from None
    if not account:
        raise RowError("empty account")
    try:
        amount = Decimal(amount_text)
    except InvalidOperation:
        raise RowError(f"bad amount {amount_text!r}") from None
    return Row(day, account, amount)
