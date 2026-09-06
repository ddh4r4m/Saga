"""When an SLA runs out, counted in business hours."""

from datetime import datetime, timedelta

from .calendar import BUSINESS_END, BUSINESS_START, is_business_day, next_business_day


def _first_business_minute(start: datetime) -> datetime:
    """The instant the clock starts running for a ticket opened at start."""
    day = start.date()
    if not is_business_day(day):
        return datetime.combine(next_business_day(day), BUSINESS_START)
    if start.time() < BUSINESS_START:
        return datetime.combine(day, BUSINESS_START)
    if start.time() >= BUSINESS_END:
        return datetime.combine(next_business_day(day), BUSINESS_START)
    return start


def due_at(start: datetime, sla_hours: float) -> datetime:
    """Return the instant sla_hours of business time after start."""
    cursor = _first_business_minute(start)
    remaining = timedelta(hours=sla_hours)
    while True:
        end_of_day = datetime.combine(cursor.date(), BUSINESS_END)
        available = end_of_day - cursor
        if remaining <= available:
            return cursor + remaining
        remaining -= available
        cursor = datetime.combine(next_business_day(cursor.date()), BUSINESS_START)
