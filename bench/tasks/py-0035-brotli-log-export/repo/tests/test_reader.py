import unittest
from pathlib import Path

from edgelog.reader import read_lines
from edgelog.summary import summarise

EXPORTS = Path(__file__).resolve().parent.parent / "fixtures" / "exports"


class ReaderTests(unittest.TestCase):
    def test_plain_export(self):
        lines = read_lines(EXPORTS / "2026-01-30.log")
        self.assertEqual(len(lines), 5)
        self.assertEqual(lines[0], "2026-01-30T00:00:11Z GET /v1/assets/1180.css 200 4412")

    def test_gzip_export(self):
        lines = read_lines(EXPORTS / "2026-01-31.log.gz")
        self.assertEqual(len(lines), 6)
        self.assertEqual(lines[-1], "2026-01-31T14:02:12Z GET /v1/assets/1181.js 200 90118")

    def test_january_summary(self):
        got = summarise([EXPORTS / "2026-01-30.log", EXPORTS / "2026-01-31.log.gz"])
        self.assertEqual(got, {"2xx": 7, "3xx": 2, "4xx": 2})
