import unittest
from decimal import Decimal

from payroll import department_totals


class SummaryTests(unittest.TestCase):
    def test_march_totals_match_finance(self):
        with open("fixtures/march.csv", encoding="utf-8") as fh:
            totals = department_totals(fh)
        self.assertEqual(
            totals,
            {
                "ops": Decimal("3940.50"),
                "sales": Decimal("3870.25"),
                "support": Decimal("1154.50"),
            },
        )
