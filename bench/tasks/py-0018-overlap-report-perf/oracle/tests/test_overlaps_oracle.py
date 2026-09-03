# canary: a484b8078f4bf66c
import os
import random
import sys
import unittest

from overlaps import Booking, find_overlaps
from overlaps.report import render


def sample(n, rooms, seed):
    rng = random.Random(seed)
    out = []
    for i in range(n):
        start = rng.randrange(0, 100_000)
        out.append(Booking(f"b{i:05d}", f"room-{rng.randrange(rooms)}", start, start + rng.randrange(50, 300)))
    return out


def reference(bookings):
    """Independent sweep used to check results on large inputs."""
    pairs = []
    by_room = {}
    for b in bookings:
        by_room.setdefault(b.room, []).append(b)
    for items in by_room.values():
        items.sort(key=lambda b: b.start)
        for i, a in enumerate(items):
            j = i + 1
            while j < len(items) and items[j].start < a.end:
                b = items[j]
                pairs.append((a.id, b.id) if a.id < b.id else (b.id, a.id))
                j += 1
    pairs.sort()
    return pairs


def traced_lines(fn, arg, limit):
    """Count line events in frames whose code lives in the overlaps package; abort past limit."""
    pkg = os.path.abspath("overlaps")
    count = 0

    def tracer(frame, event, _arg):
        nonlocal count
        if not frame.f_code.co_filename.startswith(pkg):
            return None
        if event == "line":
            count += 1
            if count > limit:
                sys.settrace(None)
                raise RuntimeError("over budget")
        return tracer

    sys.settrace(tracer)
    try:
        result = fn(arg)
    finally:
        sys.settrace(None)
    return count, result


class OverlapOracle(unittest.TestCase):
    def test_simple_pair(self):
        self.assertEqual(find_overlaps([Booking("b", "r1", 90, 150), Booking("a", "r1", 60, 120)]), [("a", "b")])

    def test_touching_and_other_room(self):
        self.assertEqual(find_overlaps([Booking("a", "r1", 60, 120), Booking("b", "r1", 120, 150)]), [])
        self.assertEqual(find_overlaps([Booking("a", "r1", 60, 120), Booking("b", "r2", 90, 150)]), [])

    def test_nested_inside_long_booking(self):
        bs = [Booking("d", "r", 500, 600), Booking("a", "r", 0, 1000), Booking("b", "r", 100, 200), Booking("c", "r", 300, 400)]
        self.assertEqual(find_overlaps(bs), [("a", "b"), ("a", "c"), ("a", "d")])

    def test_chain_and_id_order(self):
        bs = [Booking("z", "r", 0, 30), Booking("m", "r", 20, 50), Booking("a", "r", 40, 70), Booking("q", "r", 60, 65)]
        self.assertEqual(find_overlaps(bs), [("a", "m"), ("a", "q"), ("m", "z")])

    def test_empty_and_single(self):
        self.assertEqual(find_overlaps([]), [])
        self.assertEqual(find_overlaps([Booking("a", "r", 1, 2)]), [])

    def test_large_two_rooms_matches_reference(self):
        bs = sample(3000, 2, 11)
        got = find_overlaps(bs)
        self.assertEqual(got, reference(bs))
        self.assertGreater(len(got), 100)

    def test_large_many_rooms_matches_reference(self):
        bs = sample(2500, 30, 5)
        self.assertEqual(find_overlaps(bs), reference(bs))

    def test_line_budget_two_rooms(self):
        bs = sample(3000, 2, 11)
        count, got = traced_lines(find_overlaps, bs, 600_000)
        self.assertLess(count, 600_000)
        self.assertEqual(got, reference(bs))

    def test_line_budget_single_room(self):
        bs = sample(4000, 1, 23)
        count, got = traced_lines(find_overlaps, bs, 800_000)
        self.assertLess(count, 800_000)
        self.assertEqual(got, reference(bs))

    def test_report_render_unchanged(self):
        bs = [Booking("a", "r1", 60, 120), Booking("b", "r1", 90, 150), Booking("c", "r2", 0, 10)]
        self.assertEqual(render(bs), "r1: a [60-120) clashes with b [90-150)\n")
