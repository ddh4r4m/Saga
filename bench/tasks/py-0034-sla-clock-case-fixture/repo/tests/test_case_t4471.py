import json
import unittest
from datetime import datetime
from pathlib import Path

from slaclock.clock import due_at

CASES = Path(__file__).resolve().parent / "cases"


class CaseT4471Tests(unittest.TestCase):
    def test_due_matches_the_case_file(self):
        case = json.loads((CASES / "T-4471.json").read_text())
        got = due_at(datetime.fromisoformat(case["start"]), case["sla_hours"])
        self.assertEqual(got.isoformat(), case["expect"])
