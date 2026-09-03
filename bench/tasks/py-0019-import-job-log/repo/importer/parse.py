"""Row parsing for vendor price files."""

from dataclasses import dataclass
from decimal import Decimal, InvalidOperation


class ParseError(ValueError):
    pass


@dataclass(frozen=True)
class Item:
    sku: str
    qty: int
    unit_price: Decimal


def parse_price(text: str) -> Decimal:
    """Parse a unit price.

    Prices use a dot as the decimal mark. Vendors write thousands separators
    according to their own locale: a comma, a space or a no-break space, so
    all of those have to be ignored. Anything else is a ParseError.
    """
    cleaned = text.strip().replace(",", "")
    try:
        return Decimal(cleaned)
    except InvalidOperation:
        raise ParseError(f"unit_price {text!r} rejected") from None


def parse_qty(text: str) -> int:
    try:
        qty = int(text.strip())
    except ValueError:
        raise ParseError(f"qty {text!r} rejected") from None
    if qty < 0:
        raise ParseError(f"qty {text!r} rejected")
    return qty


def parse_row(row: dict[str, str]) -> Item:
    sku = row.get("sku", "").strip()
    if not sku:
        raise ParseError("sku missing")
    return Item(sku, parse_qty(row.get("qty", "")), parse_price(row.get("unit_price", "")))
