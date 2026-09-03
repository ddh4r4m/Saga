"""CSV export consumed by the finance team."""
from .cart import Line, line_total


def to_csv(lines: list[Line]) -> str:
    rows = ["name,qty,total"]
    for name, price, qty in lines:
        rows.append(f"{name},{qty},{line_total(price, qty).amount}")
    return "\n".join(rows) + "\n"
