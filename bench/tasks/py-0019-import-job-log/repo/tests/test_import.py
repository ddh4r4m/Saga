import tempfile
import unittest
from decimal import Decimal
from pathlib import Path

from importer import job
from importer.parse import ParseError, parse_price

FIXTURE = Path(__file__).resolve().parent.parent / "fixtures" / "nightly"


class ParseTests(unittest.TestCase):
    def test_plain_price(self):
        self.assertEqual(parse_price("12.50"), Decimal("12.50"))

    def test_comma_thousands(self):
        self.assertEqual(parse_price("1,250.00"), Decimal("1250.00"))

    def test_garbage_rejected(self):
        with self.assertRaises(ParseError):
            parse_price("n/a")


class JobTests(unittest.TestCase):
    def test_nightly_fixture_imports_cleanly(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertEqual(job.run(FIXTURE, Path(tmp) / "job.log"), 0)
