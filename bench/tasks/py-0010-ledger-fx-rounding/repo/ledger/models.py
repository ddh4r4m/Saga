"""Core records."""
from dataclasses import dataclass


@dataclass(frozen=True)
class Account:
    code: str
    name: str
    currency: str


@dataclass(frozen=True)
class Entry:
    id: str
    date: str
    debit: str
    credit: str
    amount_cents: int
    currency: str
