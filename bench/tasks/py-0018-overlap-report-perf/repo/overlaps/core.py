"""Clash detection between bookings."""

from dataclasses import dataclass


@dataclass(frozen=True)
class Booking:
    id: str
    room: str
    start: int  # minutes since midnight
    end: int  # exclusive


def overlaps(a: Booking, b: Booking) -> bool:
    return a.room == b.room and a.start < b.end and b.start < a.end


def find_overlaps(bookings: list[Booking]) -> list[tuple[str, str]]:
    """Return every clashing pair as (id_a, id_b) with id_a < id_b, sorted.

    Two bookings clash when they are in the same room and their half-open
    intervals intersect. A booking fully inside a longer one clashes with it;
    a booking that ends exactly when another starts does not.
    """
    pairs: list[tuple[str, str]] = []
    for i in range(len(bookings)):
        a = bookings[i]
        for j in range(i + 1, len(bookings)):
            b = bookings[j]
            if overlaps(a, b):
                pairs.append((a.id, b.id) if a.id < b.id else (b.id, a.id))
    pairs.sort()
    return pairs
