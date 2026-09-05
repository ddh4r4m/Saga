"""Batch import: parse every data row and hand it to the sink."""

from collections.abc import Iterable
from dataclasses import dataclass, field

from .rows import Row, parse_row


@dataclass
class ImportResult:
    imported: list[Row] = field(default_factory=list)
    rejected: list[tuple[int, str]] = field(default_factory=list)  # (line number, reason)


def import_rows(lines: Iterable[str], sink) -> ImportResult:
    """Import every data row after the header. sink.write(row) stores one row
    and raises SinkError when the ledger refuses it."""
    result = ImportResult()
    for line_no, line in enumerate(lines, start=1):
        if line_no == 1 or not line.strip():
            continue
        row = parse_row(line)
        sink.write(row)
        result.imported.append(row)
    return result
