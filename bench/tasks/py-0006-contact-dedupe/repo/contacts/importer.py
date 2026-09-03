"""CSV import (unchanged by this task)."""
import csv
from io import StringIO
from .models import Contact


def read_contacts(text: str) -> list[Contact]:
    rows = csv.DictReader(StringIO(text))
    return [Contact(r.get("name", ""), r.get("email", ""), r.get("phone", "")) for r in rows]
