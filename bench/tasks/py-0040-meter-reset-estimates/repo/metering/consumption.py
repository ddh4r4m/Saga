"""Monthly consumption per meter, from the field readings. See docs/readings.md."""

from dataclasses import dataclass


@dataclass(frozen=True)
class Reading:
    meter: str
    taken_on: str  # ISO date, YYYY-MM-DD
    value: int  # dial value in kWh
    flag: str  # "A" actual, "E" estimated


def in_month(taken_on: str, month: str) -> bool:
    """True when the reading date falls in month, given as YYYY-MM."""
    return taken_on.startswith(f"{month}-")


def monthly_consumption(readings: list[Reading], month: str) -> dict[str, int]:
    """kWh consumed per meter during month, keyed by meter id."""
    by_meter: dict[str, list[Reading]] = {}
    for reading in readings:
        by_meter.setdefault(reading.meter, []).append(reading)
    report: dict[str, int] = {}
    for meter, meter_readings in by_meter.items():
        ordered = sorted(meter_readings, key=lambda r: (r.taken_on, r.value))
        total = 0
        counted = False
        for earlier, later in zip(ordered, ordered[1:]):
            if not in_month(later.taken_on, month):
                continue
            total += later.value - earlier.value
            counted = True
        if counted:
            report[meter] = total
    return report
