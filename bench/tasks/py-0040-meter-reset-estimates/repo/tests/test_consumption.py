import unittest

from metering import Reading, monthly_consumption


class ConsumptionTests(unittest.TestCase):
    def test_meter_replaced_during_the_month(self):
        # MTR-114: the old unit was pulled on 2026-04-18 and the new one
        # fitted the same day; the field team confirmed 587 kWh for April.
        readings = [
            Reading("MTR-114", "2026-03-31", 48210, "A"),
            Reading("MTR-114", "2026-04-08", 48395, "A"),
            Reading("MTR-114", "2026-04-19", 61, "A"),
            Reading("MTR-114", "2026-04-30", 402, "A"),
        ]
        self.assertEqual(monthly_consumption(readings, "2026-04"), {"MTR-114": 587})

    def test_ordinary_month(self):
        readings = [
            Reading("MTR-207", "2026-03-30", 1200, "A"),
            Reading("MTR-207", "2026-04-14", 1345, "A"),
            Reading("MTR-207", "2026-04-29", 1490, "A"),
        ]
        self.assertEqual(monthly_consumption(readings, "2026-04"), {"MTR-207": 290})
