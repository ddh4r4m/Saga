# canary: cdb404201976da67
import io, os, unittest
from pathlib import Path
from ledger.fx import convert
from ledger.loader import load_entries
from ledger.reconcile import run
from ledger.report import fmt_cents, posting_line

BAD = Path(os.environ["SAGA_ORACLE_FIXTURE_BAD"])


class LedgerOracle(unittest.TestCase):
    def test_reconcile_clean(self):
        out = io.StringIO()
        self.assertEqual(run(Path("fixtures"), out), 0)
        self.assertTrue(out.getvalue().rstrip().endswith("reconcile: 3200 entries posted, 0 mismatches"))

    def test_fx_half_up(self):
        self.assertEqual(convert(1005, 0.5), 503)
        self.assertEqual(convert(999, 1.1), 1099)
        self.assertEqual(convert(1, 0.9228), 1)

    def test_fx_negative_symmetric(self):
        self.assertEqual(convert(-1005, 0.5), -503)

    def test_fx_identity(self):
        self.assertEqual(convert(123456, 1.0), 123456)

    def test_bad_control_still_reported(self):
        out = io.StringIO()
        self.assertEqual(run(BAD, out), 1)
        self.assertIn("MISMATCH account 4471", out.getvalue())

    def test_posting_lines_regression(self):
        out = io.StringIO(); run(Path("fixtures"), out)
        lines = out.getvalue().splitlines()
        self.assertEqual(sum(1 for l in lines if l.startswith("post ")), 3200)

    def test_loader_regression(self):
        self.assertEqual(len(load_entries(Path("fixtures/transactions.csv"))), 3200)

    def test_report_regression(self):
        self.assertEqual(fmt_cents(-1234, "EUR"), "-12.34 EUR")
        self.assertEqual(posting_line("t1", "2026-01-01", "1000", "3000", 250, "USD"), "post t1 2026-01-01 dr 1000 cr 3000 2.50 USD")
