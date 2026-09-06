import unittest
from pathlib import Path

from metering.report import build_report, load_usage

FIXTURES = Path(__file__).resolve().parent.parent / "fixtures" / "usage"


class JanuaryReportTests(unittest.TestCase):
    def test_every_region_on_the_report_has_a_name(self):
        # Three regions opened in January; the portal must not print a placeholder.
        rows = build_report(load_usage(FIXTURES / "2026-01.json"))
        unresolved = sorted({r["region"] for r in rows if r["region"].startswith("unknown (")})
        self.assertEqual(unresolved, [])
