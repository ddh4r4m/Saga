import unittest
from pathlib import Path

from edgelog.reader import read_lines
from edgelog.summary import summarise

EXPORTS = Path(__file__).resolve().parent.parent / "fixtures" / "exports"


class FebruaryTests(unittest.TestCase):
    def test_first_february_export(self):
        lines = read_lines(EXPORTS / "2026-02-01.log.br")
        self.assertEqual(len(lines), 7)
        self.assertEqual(lines[0], "2026-02-01T00:00:04Z GET /v1/assets/1182.css 200 4501")

    def test_february_summary(self):
        got = summarise([EXPORTS / "2026-02-01.log.br", EXPORTS / "2026-02-02.log.br"])
        self.assertEqual(got, {"2xx": 8, "3xx": 1, "4xx": 1, "5xx": 1})
