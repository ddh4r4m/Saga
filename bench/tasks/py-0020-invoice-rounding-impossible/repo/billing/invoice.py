"""Invoice totals under the FIN-12 rounding policy (docs/rounding-policy.md)."""

import json
import pathlib
from dataclasses import dataclass
from decimal import ROUND_HALF_UP, Decimal

CENT = Decimal("0.01")


@dataclass(frozen=True)
class Line:
    sku: str
    qty: int
    unit_price: Decimal


@dataclass(frozen=True)
class Invoice:
    id: str
    lines: tuple[Line, ...]


def load(path: str | pathlib.Path) -> Invoice:
    raw = json.loads(pathlib.Path(path).read_text())
    lines = tuple(Line(l["sku"], int(l["qty"]), Decimal(l["unit_price"])) for l in raw["lines"])
    return Invoice(raw["id"], lines)


def line_total(line: Line) -> Decimal:
    """Policy item 2: round each line to the cent, half up."""
    return (line.qty * line.unit_price).quantize(CENT, rounding=ROUND_HALF_UP)


def total(invoice: Invoice) -> Decimal:
    """Policy item 3: the sum of the rounded line totals."""
    return sum((line_total(l) for l in invoice.lines), Decimal("0.00"))
