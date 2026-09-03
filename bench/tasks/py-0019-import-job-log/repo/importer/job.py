"""The nightly import job."""

import csv
import json
import pathlib
import sys
from datetime import datetime
from decimal import Decimal

from .parse import ParseError, parse_row


class Log:
    def __init__(self, path: pathlib.Path | None) -> None:
        self._fh = open(path, "w", encoding="utf-8") if path else None

    def _emit(self, level: str, msg: str) -> None:
        line = f"{datetime.now():%Y-%m-%dT%H:%M:%S} {level} {msg}"
        if self._fh:
            self._fh.write(line + "\n")
        else:
            print(line, file=sys.stderr)

    def info(self, msg: str) -> None:
        self._emit("INFO", msg)

    def warning(self, msg: str) -> None:
        self._emit("WARN", msg)

    def error(self, msg: str) -> None:
        self._emit("ERROR", msg)

    def close(self) -> None:
        if self._fh:
            self._fh.close()


def import_file(path: pathlib.Path, log: Log) -> tuple[int, Decimal]:
    imported, total = 0, Decimal("0")
    with open(path, newline="", encoding="utf-8") as fh:
        for n, row in enumerate(csv.DictReader(fh), start=1):
            try:
                item = parse_row(row)
            except ParseError as e:
                log.warning(f"parse: {path.name} row {n}: {e}; row skipped")
                continue
            imported += 1
            total += item.qty * item.unit_price
            log.info(f"parse: {path.name} row {n}: {item.sku} x{item.qty} @ {item.unit_price}")
    return imported, total


def run(nightly_dir: str | pathlib.Path, log_path: str | pathlib.Path | None = None) -> int:
    """Import every vendor file in the manifest. Returns 0 on success, 1 on any reconcile failure."""
    base = pathlib.Path(nightly_dir)
    manifest = json.loads((base / "manifest.json").read_text())
    log = Log(pathlib.Path(log_path) if log_path else None)
    log.info(f"job start: {len(manifest)} vendors from {base.name}")
    ok = True
    for entry in manifest:
        vendor, path = entry["vendor"], base / entry["file"]
        log.info(f"vendor {vendor}: reading {path.name}")
        imported, total = import_file(path, log)
        if imported != entry["rows"] or total != Decimal(entry["total"]):
            log.error(
                f"reconcile: vendor {vendor}: manifest says {entry['rows']} rows / {entry['total']}, "
                f"imported {imported} rows / {total}; aborting"
            )
            ok = False
        else:
            log.info(f"reconcile: vendor {vendor}: {imported} rows / {total} ok")
    log.info("job end: " + ("ok" if ok else "FAILED, nothing written"))
    log.close()
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(run(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else None))
