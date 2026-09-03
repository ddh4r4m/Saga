"""Plain-text clash report."""

from .core import Booking, find_overlaps


def render(bookings: list[Booking]) -> str:
    by_id = {b.id: b for b in bookings}
    lines = []
    for a, b in find_overlaps(bookings):
        x, y = by_id[a], by_id[b]
        lines.append(f"{x.room}: {a} [{x.start}-{x.end}) clashes with {b} [{y.start}-{y.end})")
    return "\n".join(lines) + ("\n" if lines else "")
