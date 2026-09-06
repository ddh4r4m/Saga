"""Fee lookup over the generated rate table."""

from .rates_table import RATES


class UnknownRegion(Exception):
    """Raised when the region has no tiers in the rate table."""


class OverWeight(Exception):
    """Raised when the parcel is heavier than the region's top tier."""


def quote_cents(region: str, weight_kg: int) -> int:
    """Fee in cents for a parcel of weight_kg going to region."""
    tiers = RATES.get(region)
    if tiers is None:
        raise UnknownRegion(region)
    for limit_kg, fee_cents in tiers:
        if weight_kg <= limit_kg:
            return fee_cents
    raise OverWeight(f"{weight_kg} kg is over the top tier for {region}")
