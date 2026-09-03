"""Sales tax."""
from .money import Money


def apply_tax(subtotal: Money, rate: float) -> Money:
    """Return subtotal plus tax, rounded to the cent."""
    return Money(round(subtotal.amount * (1 + rate), 2))
