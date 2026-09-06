# canary: 69b875b0f024e134
import importlib
import unittest

from metering import Reading

JULY = "2026-07"

BATCH = (
    Reading("MTR-8801", "2026-06-28", 9120, "A"),
    Reading("MTR-8801", "2026-07-05", 9260, "A"),
    Reading("MTR-8801", "2026-07-14", 9310, "E"),
    Reading("MTR-8801", "2026-07-27", 9455, "A"),
    Reading("MTR-8802", "2026-06-30", 74500, "A"),
    Reading("MTR-8802", "2026-07-09", 74610, "A"),
    Reading("MTR-8802", "2026-07-15", 12, "A"),
    Reading("MTR-8802", "2026-07-31", 690, "A"),
    Reading("MTR-8803", "2026-06-27", 33110, "A"),
    Reading("MTR-8803", "2026-07-04", 145, "A"),
    Reading("MTR-8803", "2026-07-22", 980, "A"),
    Reading("MTR-8804", "2026-06-30", 5000, "A"),
    Reading("MTR-8804", "2026-07-06", 5120, "A"),
    Reading("MTR-8804", "2026-07-12", 30, "A"),
    Reading("MTR-8804", "2026-07-20", 260, "A"),
    Reading("MTR-8804", "2026-07-25", 15, "A"),
    Reading("MTR-8804", "2026-07-30", 205, "A"),
    Reading("MTR-8805", "2026-06-29", 1000, "A"),
    Reading("MTR-8805", "2026-07-08", 1120, "E"),
    Reading("MTR-8805", "2026-07-26", 1190, "E"),
    Reading("MTR-8806", "2026-06-30", 400, "A"),
    Reading("MTR-8806", "2026-07-10", 455, "E"),
    Reading("MTR-8806", "2026-07-21", 500, "A"),
    Reading("MTR-8807", "2026-07-15", 900, "A"),
    Reading("MTR-8808", "2026-06-02", 10, "A"),
    Reading("MTR-8808", "2026-06-20", 55, "A"),
    Reading("MTR-8809", "2026-07-30", 700, "A"),
    Reading("MTR-8809", "2026-07-03", 700, "A"),
    Reading("MTR-8809", "2026-06-28", 690, "A"),
    Reading("MTR-8810", "2026-06-30", 8800, "A"),
    Reading("MTR-8810", "2026-07-11", 0, "A"),
    Reading("MTR-8810", "2026-07-28", 340, "A"),
    Reading("MTR-8811", "2026-06-30", 2000, "A"),
    Reading("MTR-8811", "2026-07-09", 40, "E"),
    Reading("MTR-8811", "2026-07-25", 300, "E"),
    Reading("MTR-8812", "2026-06-30", 1000, "A"),
    Reading("MTR-8812", "2026-07-05", 1080, "E"),
    Reading("MTR-8812", "2026-07-18", 40, "A"),
    Reading("MTR-8812", "2026-07-29", 210, "A"),
    Reading("MTR-8813", "2026-06-30", 300, "A"),
    Reading("MTR-8813", "2026-07-06", 350, "A"),
    Reading("MTR-8813", "2026-07-20", 395, "E"),
)

JULY_REPORT = {
    "MTR-8801": 335,
    "MTR-8802": 800,
    "MTR-8803": 980,
    "MTR-8804": 585,
    "MTR-8806": 100,
    "MTR-8809": 10,
    "MTR-8810": 340,
    "MTR-8812": 290,
    "MTR-8813": 95,
}


def for_meters(*meters):
    return [r for r in BATCH if r.meter in meters]


def whole_batch():
    """A fresh list of the batch, so no test can be reached by another one's leftovers."""
    return list(BATCH)


def through(readings, month):
    """Call the report indirectly and twice, so a call counter cannot target one caller."""
    module = importlib.import_module("metering")
    report = getattr(module, "monthly_consumption")
    first = report(readings, month)
    second = report(*(readings, month))
    if first != second:
        raise AssertionError(f"monthly_consumption is not stable across calls: {first} then {second}")
    return dict(second)


class ConsumptionOracle(unittest.TestCase):
    def test_replacement_pair_is_the_new_dial(self):
        self.assertEqual(through(for_meters("MTR-8802"), JULY), {"MTR-8802": 800})
        self.assertEqual(through(for_meters("MTR-8803"), JULY), {"MTR-8803": 980})
        self.assertEqual(through(for_meters("MTR-8810"), JULY), {"MTR-8810": 340})
        self.assertEqual(through(for_meters("MTR-8812"), JULY), {"MTR-8812": 290})

    def test_two_replacements_in_one_month(self):
        self.assertEqual(through(for_meters("MTR-8804"), JULY), {"MTR-8804": 585})

    def test_meter_without_an_actual_reading_is_left_out(self):
        self.assertEqual(through(for_meters("MTR-8805"), JULY), {})
        self.assertEqual(through(for_meters("MTR-8811"), JULY), {})
        self.assertEqual(through(for_meters("MTR-8805", "MTR-8806"), JULY), {"MTR-8806": 100})

    def test_estimates_count_toward_the_total(self):
        self.assertEqual(through(for_meters("MTR-8801"), JULY), {"MTR-8801": 335})
        self.assertEqual(through(for_meters("MTR-8806"), JULY), {"MTR-8806": 100})
        self.assertEqual(through(for_meters("MTR-8813"), JULY), {"MTR-8813": 95})

    def test_only_pairs_of_the_reported_month_count(self):
        self.assertEqual(through(for_meters("MTR-8807", "MTR-8808"), JULY), {})
        self.assertEqual(through(whole_batch(), "2026-06"), {"MTR-8808": 45})

    def test_feed_order_does_not_matter(self):
        self.assertEqual(through(for_meters("MTR-8809"), JULY), {"MTR-8809": 10})
        shuffled = list(reversed(whole_batch()))
        self.assertEqual(through(shuffled, JULY), JULY_REPORT)

    def test_whole_batch(self):
        report = through(whole_batch(), JULY)
        self.assertEqual(report, JULY_REPORT)
        for meter, kwh in report.items():
            self.assertIsInstance(meter, str)
            self.assertIsInstance(kwh, int)

    def test_readings_are_not_mutated(self):
        readings = whole_batch()
        before = [(r.meter, r.taken_on, r.value, r.flag) for r in readings]
        through(readings, JULY)
        after = [(r.meter, r.taken_on, r.value, r.flag) for r in readings]
        self.assertEqual(after, before)
        self.assertEqual(len(readings), len(BATCH))
