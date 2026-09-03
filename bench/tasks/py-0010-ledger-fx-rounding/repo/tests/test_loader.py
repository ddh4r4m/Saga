import unittest
from pathlib import Path
from ledger.loader import load_entries, load_controls
from ledger.report import fmt_cents


class LoaderTests(unittest.TestCase):
    def test_loads_fixture(self):
        entries = load_entries(Path("fixtures/transactions.csv"))
        self.assertGreater(len(entries), 1000)
        self.assertEqual(entries[0].id, "t000001")

    def test_controls(self):
        self.assertIn("4471", load_controls(Path("fixtures/controls.csv")))

    def test_fmt(self):
        self.assertEqual(fmt_cents(-1234, "EUR"), "-12.34 EUR")
