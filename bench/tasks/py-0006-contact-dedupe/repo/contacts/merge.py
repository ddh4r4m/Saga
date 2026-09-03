"""Merging helpers for imported contact lists."""
from .models import Contact


def dedupe(contacts: list[Contact]) -> list[Contact]:
    """Return the list without duplicate people."""
    raise NotImplementedError("dedupe is not implemented yet")
