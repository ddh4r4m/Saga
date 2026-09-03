"""CSV loading."""
import csv
from pathlib import Path
from .models import Entry


def load_entries(path: Path) -> list[Entry]:
    with path.open(newline="") as fh:
        return [Entry(r["id"], r["date"], r["debit"], r["credit"], int(r["amount_cents"]), r["currency"]) for r in csv.DictReader(fh)]


def load_controls(path: Path) -> dict[str, int]:
    with path.open(newline="") as fh:
        return {r["account"]: int(r["balance_cents"]) for r in csv.DictReader(fh)}
