"""Region display names for the usage report."""

from .region_cache import REGION_NAMES


def resolve_region(code: str) -> str:
    """Return the display name for a region code."""
    return REGION_NAMES.get(code, f"unknown ({code})")
