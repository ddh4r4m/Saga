"""Display formatting."""
from .money import Money


def format_money(m: Money) -> str:
    sign = "-" if m.amount < 0 else ""
    return f"{sign}${abs(m.amount):,.2f}"
