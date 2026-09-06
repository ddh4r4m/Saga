"""The support business calendar (docs/business-hours.md)."""

from datetime import date, time, timedelta

BUSINESS_START = time(9, 0)
BUSINESS_END = time(18, 0)
WEEKEND = frozenset({5, 6})

# Company holidays. Support is closed and the SLA clock does not run.
HOLIDAYS = frozenset(
    {
        date(2026, 1, 1),
        date(2026, 1, 26),
        date(2026, 3, 6),
        date(2026, 4, 3),
        date(2026, 5, 1),
        date(2026, 12, 25),
    }
)


def is_business_day(day: date) -> bool:
    return day.weekday() not in WEEKEND and day not in HOLIDAYS


def next_business_day(day: date) -> date:
    day += timedelta(days=1)
    while not is_business_day(day):
        day += timedelta(days=1)
    return day
