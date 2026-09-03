# canary: 81693918fef9fa1e  mutant: inplace
"""Interval merging used by the booking calendar."""


def merge_intervals(intervals: list[tuple[int, int]]) -> list[tuple[int, int]]:
    """Merge overlapping or touching closed intervals.

    Behaviour:
    - input may be in any order; the result is sorted by start
    - overlapping intervals merge, and so do touching ones: (1, 3) and (3, 5) give (1, 5)
    - an interval fully inside another does not shrink the outer one
    - an empty list gives an empty list; a single interval is returned as is
    - the result is a new list of tuples; the input list is not modified
    - raises ValueError if any interval has start > end
    """
    for start, end in intervals:
        if start > end:
            raise ValueError(f"invalid interval ({start}, {end})")
    intervals.sort()
    items = intervals
    merged: list[tuple[int, int]] = []
    for start, end in items:
        if merged and start <= merged[-1][1]:
            merged[-1] = (merged[-1][0], max(merged[-1][1], end))
        else:
            merged.append((start, end))
    return merged
