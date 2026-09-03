import random
import time
import unittest

from overlaps import Booking, find_overlaps


def sample(n, rooms, seed=7):
    rng = random.Random(seed)
    out = []
    for i in range(n):
        start = rng.randrange(0, 100_000)
        out.append(Booking(f"b{i:05d}", f"room-{rng.randrange(rooms)}", start, start + rng.randrange(50, 300)))
    return out


class OverlapTests(unittest.TestCase):
    def test_simple_clash(self):
        a = Booking("a", "r1", 60, 120)
        b = Booking("b", "r1", 90, 150)
        self.assertEqual(find_overlaps([b, a]), [("a", "b")])

    def test_touching_is_not_a_clash(self):
        a = Booking("a", "r1", 60, 120)
        b = Booking("b", "r1", 120, 150)
        self.assertEqual(find_overlaps([a, b]), [])

    def test_other_room_is_not_a_clash(self):
        a = Booking("a", "r1", 60, 120)
        b = Booking("b", "r2", 90, 150)
        self.assertEqual(find_overlaps([a, b]), [])

    def test_nightly_sized_input_under_budget(self):
        bookings = sample(20000, 40)
        t0 = time.perf_counter()
        pairs = find_overlaps(bookings)
        elapsed = time.perf_counter() - t0
        self.assertGreater(len(pairs), 0)
        self.assertLess(elapsed, 2.0, f"find_overlaps took {elapsed:.1f}s on 20000 bookings")
