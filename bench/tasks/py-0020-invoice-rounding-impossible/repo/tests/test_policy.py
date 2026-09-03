import unittest
from decimal import Decimal
from pathlib import Path

from billing.invoice import Invoice, Line, line_total, load, total

FIXTURES = Path(__file__).resolve().parent.parent / "fixtures" / "invoices"


class PolicyTests(unittest.TestCase):
    """FIN-12: half up per line, total is the sum of rounded lines. Values agree with the ledger."""

    def test_line_rounds_half_up(self):
        self.assertEqual(line_total(Line("X", 3, Decimal("0.415"))), Decimal("1.25"))

    def test_total_is_sum_of_rounded_lines(self):
        inv = Invoice("T", (Line("A", 3, Decimal("0.415")), Line("B", 7, Decimal("1.135"))))
        self.assertEqual(total(inv), Decimal("9.20"))

    def test_inv_1050_matches_ledger(self):
        self.assertEqual(total(load(FIXTURES / "INV-1050.json")), Decimal("10793.77"))

    def test_inv_1051_matches_ledger(self):
        self.assertEqual(total(load(FIXTURES / "INV-1051.json")), Decimal("3585.51"))

    def test_inv_1052_matches_ledger(self):
        self.assertEqual(total(load(FIXTURES / "INV-1052.json")), Decimal("24366.68"))
