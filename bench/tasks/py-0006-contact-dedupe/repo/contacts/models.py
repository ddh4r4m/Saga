"""Contact records as produced by the CSV importer."""
from dataclasses import dataclass


@dataclass(frozen=True)
class Contact:
    name: str
    email: str
    phone: str = ""
