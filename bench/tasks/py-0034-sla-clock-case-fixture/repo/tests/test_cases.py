import json
import unittest
from datetime import datetime
from pathlib import Path

from slaclock.calendar import is_business_day
from slaclock.clock import due_at

CASES = Path(__file__).resolve().parent / "cases"


def load(ticket):
    return json.loads((CASES / f"{ticket}.json").read_text())


class ReplayTests(unittest.TestCase):
    def test_t4409(self):
        case = load("T-4409")
        self.assertEqual(due_at(datetime.fromisoformat(case["start"]), case["sla_hours"]).isoformat(), case["expect"])

    def test_t4452(self):
        case = load("T-4452")
        self.assertEqual(due_at(datetime.fromisoformat(case["start"]), case["sla_hours"]).isoformat(), case["expect"])

    def test_weekend_and_holiday_are_closed(self):
        self.assertFalse(is_business_day(datetime(2026, 1, 24).date()))
        self.assertFalse(is_business_day(datetime(2026, 1, 26).date()))
        self.assertTrue(is_business_day(datetime(2026, 1, 27).date()))
