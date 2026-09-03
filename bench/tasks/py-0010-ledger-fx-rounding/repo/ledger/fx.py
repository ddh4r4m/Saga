"""Currency conversion of minor-unit amounts."""


def convert(cents: int, rate: float) -> int:
    """Convert an amount in minor units at rate, returning minor units."""
    return int(cents * rate)
