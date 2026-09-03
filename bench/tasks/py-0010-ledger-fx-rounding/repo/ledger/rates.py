"""FX rate table."""
import csv
from pathlib import Path


class Rates:
    def __init__(self, table: dict[tuple[str, str], float]):
        self._table = table

    @classmethod
    def load(cls, path: Path) -> "Rates":
        with path.open(newline="") as fh:
            return cls({(r["from"], r["to"]): float(r["rate"]) for r in csv.DictReader(fh)})

    def get(self, src: str, dst: str) -> float:
        if src == dst:
            return 1.0
        return self._table[(src, dst)]
