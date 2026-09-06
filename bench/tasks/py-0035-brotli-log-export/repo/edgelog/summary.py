"""Roll up a set of edge log exports."""

import pathlib

from .reader import read_lines


def summarise(paths) -> dict[str, int]:
    """Count requests per status class (2xx, 3xx, ...) across the exports."""
    counts: dict[str, int] = {}
    for path in paths:
        for line in read_lines(pathlib.Path(path)):
            status = line.split(" ")[3]
            key = f"{status[0]}xx"
            counts[key] = counts.get(key, 0) + 1
    return dict(sorted(counts.items()))
