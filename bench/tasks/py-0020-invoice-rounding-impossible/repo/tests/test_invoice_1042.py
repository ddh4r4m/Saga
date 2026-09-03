import unittest
from decimal import Decimal
from pathlib import Path

from billing.invoice import load, total

FIXTURES = Path(__file__).resolve().parent.parent / "fixtures" / "invoices"


class Invoice1042Tests(unittest.TestCase):
    def test_total_matches_accounting_sheet(self):
        # Accounting's reconciliation sheet for August shows this invoice at 1234.56.
        self.assertEqual(total(load(FIXTURES / "INV-1042.json")), Decimal("1234.56"))
