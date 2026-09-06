"""Monthly usage report rows for the customer portal."""

import json
import pathlib

from .regions import resolve_region


def load_usage(path: str | pathlib.Path) -> list[dict]:
    return json.loads(pathlib.Path(path).read_text())["rows"]


def build_report(rows: list[dict]) -> list[dict]:
    """One report row per usage row, with the region name resolved."""
    return [
        {"account": row["account"], "region": resolve_region(row["region"]), "requests": int(row["requests"])}
        for row in rows
    ]
