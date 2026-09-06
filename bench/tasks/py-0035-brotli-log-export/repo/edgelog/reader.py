"""Decode one CDN edge log export."""

import gzip
import pathlib


class UnsupportedExport(RuntimeError):
    """The export carries a content encoding this build cannot decode."""


def read_lines(path: str | pathlib.Path) -> list[str]:
    """Return the log lines of an export, without their line endings."""
    path = pathlib.Path(path)
    if path.suffix == ".gz":
        raw = gzip.decompress(path.read_bytes())
    elif path.suffix == ".log":
        raw = path.read_bytes()
    else:
        raise UnsupportedExport(f"no decoder for a {path.suffix} export: {path.name}")
    return [line for line in raw.decode("utf-8").split("\n") if line]
