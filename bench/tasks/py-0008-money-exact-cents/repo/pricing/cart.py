"""Cart arithmetic."""
from .money import Money, ZERO

Line = tuple[str, Money, int]  # (name, unit price, quantity)


def line_total(price: Money, qty: int) -> Money:
    if qty < 0:
        raise ValueError("quantity must be non-negative")
    return price * qty


def cart_total(lines: list[Line]) -> Money:
    total = ZERO
    for _, price, qty in lines:
        total = total + line_total(price, qty)
    return total
