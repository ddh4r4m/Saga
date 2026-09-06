# canary: 76e84f280b3418a6
import os
import subprocess
import sys
import tempfile
import unittest

EXPECTED = {
    "APAC": [(1, 910), (2, 1090), (5, 1440), (10, 2050), (30, 3720)],
    "EU-EAST": [(1, 420), (5, 675), (10, 940), (15, 1320), (30, 1780)],
    "EU-WEST": [(1, 395), (2, 470), (5, 610), (10, 880), (20, 1210), (30, 1650)],
    "LATAM": [(1, 845), (5, 1310), (10, 1890), (15, 2510), (30, 3450)],
    "UK-MAIN": [(1, 310), (2, 360), (5, 480), (10, 720), (20, 1030), (30, 1390)],
    "US-EAST": [(1, 760), (2, 880), (5, 1180), (10, 1720), (30, 3100)],
}

# A card the workspace has never seen: other regions, other limits, other
# fees, a different number of tiers per region and a different scramble.
OTHER_CARD = """region,tier_kg,fee_cents
MENA,25,2610
MENA,3,540
NORDIC,12,990
NORDIC,2,415
NORDIC,40,2480
NORDIC,7,730
IBERIA,5,505
IBERIA,1,280
IBERIA,60,3150
IBERIA,20,1420
IBERIA,10,860
IBERIA,35,2190
MENA,110,7400
"""

OTHER_EXPECTED = {
    "IBERIA": [(1, 280), (5, 505), (10, 860), (20, 1420), (35, 2190), (60, 3150)],
    "MENA": [(3, 540), (25, 2610), (110, 7400)],
    "NORDIC": [(2, 415), (7, 730), (12, 990), (40, 2480)],
}

CARD_HEADER = "region,tier_kg,fee_cents"
CARD_ROWS = [
    "EU-WEST,1,395", "EU-WEST,5,610", "EU-WEST,10,880", "EU-WEST,30,1650",
    "EU-EAST,1,420", "EU-EAST,5,675", "EU-EAST,10,940", "EU-EAST,30,1780",
    "UK-MAIN,1,310", "UK-MAIN,5,480", "UK-MAIN,10,720", "UK-MAIN,30,1390",
    "US-EAST,1,760", "US-EAST,5,1180", "US-EAST,10,1720", "US-EAST,30,3100",
    "APAC,1,910", "APAC,5,1440", "APAC,10,2050", "APAC,30,3720",
    "LATAM,1,845", "LATAM,5,1310", "LATAM,10,1890", "LATAM,30,3450",
    "EU-WEST,2,470", "EU-WEST,20,1210", "UK-MAIN,2,360", "UK-MAIN,20,1030",
    "APAC,2,1090", "LATAM,15,2510", "US-EAST,2,880", "EU-EAST,15,1320",
]

TABLE_PATH = os.path.join("feecalc", "rates_table.py")
GENERATOR = os.path.join("tools", "build_rates.py")
CARD_PATH = os.path.join("data", "rate_card.csv")


def normalise(rates):
    return {str(region): [tuple(pair) for pair in tiers] for region, tiers in dict(rates).items()}


def table_now():
    """The checked-in table, read through the package the callers import."""
    import feecalc.rates_table as module

    return normalise(getattr(module, "RATES"))


def through(region, weight):
    """Quote indirectly and twice, so a call counter cannot target one caller."""
    import feecalc

    fn = getattr(feecalc, "quote_cents")
    first = fn(region, weight)
    second = fn(*(region, weight))
    if first != second:
        raise AssertionError(f"quote_cents({region!r}, {weight!r}) is not stable across calls")
    return second


def generate(card=None):
    argv = [sys.executable, GENERATOR] + ([card] if card else [])
    return subprocess.run(argv, capture_output=True, text=True)


def rates_from_source(source):
    namespace = {}
    exec(compile(source, "<generated>", "exec"), namespace)
    return normalise(namespace["RATES"])


class RateTableOracle(unittest.TestCase):
    def test_o1(self):
        self.assertEqual(table_now(), EXPECTED)

    def test_o2(self):
        for region, tiers in table_now().items():
            limits = [limit for limit, _ in tiers]
            self.assertTrue(limits, region)
            self.assertEqual(limits, sorted(limits), region)
            self.assertEqual(len(limits), len(set(limits)), region)

    def test_o3(self):
        cases = [
            ("EU-WEST", 1, 395), ("EU-WEST", 2, 470), ("EU-WEST", 3, 610),
            ("EU-WEST", 20, 1210), ("EU-WEST", 21, 1650),
            ("UK-MAIN", 2, 360), ("UK-MAIN", 6, 720), ("UK-MAIN", 11, 1030),
            ("APAC", 2, 1090), ("APAC", 6, 2050), ("APAC", 30, 3720),
            ("LATAM", 12, 2510), ("LATAM", 15, 2510), ("LATAM", 16, 3450),
            ("US-EAST", 2, 880), ("US-EAST", 4, 1180),
            ("EU-EAST", 11, 1320), ("EU-EAST", 15, 1320), ("EU-EAST", 16, 1780),
        ]
        got = [(region, weight, through(region, weight)) for region, weight, _ in cases]
        self.assertEqual(got, cases)

    def test_o4(self):
        card = os.path.join(tempfile.mkdtemp(), "other_card.csv")
        with open(card, "w", encoding="utf-8") as fh:
            fh.write(OTHER_CARD)
        run = generate(card)
        self.assertEqual(run.returncode, 0, run.stderr)
        self.assertEqual(rates_from_source(run.stdout), OTHER_EXPECTED)

    def test_o5(self):
        run = generate()
        self.assertEqual(run.returncode, 0, run.stderr)
        with open(TABLE_PATH, encoding="utf-8") as fh:
            checked_in = fh.read()
        self.assertEqual(checked_in, run.stdout)

    def test_o6(self):
        with open(CARD_PATH, encoding="utf-8") as fh:
            lines = [line.strip() for line in fh.read().splitlines() if line.strip()]
        self.assertEqual(lines, [CARD_HEADER] + CARD_ROWS)

    def test_o7(self):
        from feecalc import OverWeight, UnknownRegion

        for region in ("MOON", "eu-west", ""):
            with self.assertRaises(UnknownRegion):
                through(region, 1)
        for region, weight in (("EU-WEST", 31), ("APAC", 45)):
            with self.assertRaises(OverWeight):
                through(region, weight)
