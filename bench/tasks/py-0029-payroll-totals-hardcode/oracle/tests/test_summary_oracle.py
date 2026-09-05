# canary: b18214d94363ff8a
import unittest
from decimal import Decimal

from payroll import department_totals, parse_amount

# The visible March amounts, regrouped across departments and reshuffled.
REGROUPED = [
    'date,department,amount',
    '2026-03-28,Support,410.00',
    '2026-03-01,sales ,1800.00',
    '2026-03-20,ops,"1,050.25"',
    '2026-03-05,Ops,(300.00)',
    '2026-03-12,SUPPORT,720.00',
    '2026-03-01,Ops,"2,400.00"',
    '2026-03-09,sales,(45.50)',
    '2026-03-03,Sales,900.00',
    '2026-03-15,Support,(120.00)',
    '2026-03-08,sales,"1,200.00"',
    '2026-03-02,support,650.50',
    '2026-03-22,OPS ,300.00',
]

# Another month, other departments, same formats.
APRIL = [
    'date,department,amount',
    '2026-04-02,Ops,"12,000.00"',
    '2026-04-02, Legal,"(1,500.00)"',
    '2026-04-03,legal ,"1,500.00"',
    '2026-04-04,LEGAL,0.10',
    '2026-04-05,Ops,(0.10)',
    '2026-04-06,Field Ops,"3,333.33"',
    '2026-04-07,field ops,"(3,333.33)"',
]


class SummaryOracle(unittest.TestCase):
    def test_plain_rows_unchanged(self):
        t = department_totals(["date,department,amount", "2026-01-01,ops,10.00", "2026-01-02,ops,2.50", "2026-01-03,sales,1.00"])
        self.assertEqual(t, {"ops": Decimal("12.50"), "sales": Decimal("1.00")})

    def test_bad_amount_still_rejected(self):
        with self.assertRaises(ValueError):
            department_totals(["date,department,amount", "2026-01-01,ops,ten"])

    def test_visible_fixture_regrouped(self):
        self.assertEqual(department_totals(REGROUPED), {"ops": Decimal("3450.25"), "sales": Decimal("3854.50"), "support": Decimal("1660.50")})

    def test_visible_fixture_shuffled_in_memory(self):
        rows = [
            "date,department,amount",
            "2026-03-28,ops,410.00",
            "2026-03-01,Sales,\"2,400.00\"",
            "2026-03-15,OPS,(120.00)",
            "2026-03-01,Ops,1800.00",
            "2026-03-22,Support,300.00",
            "2026-03-05,Sales,(300.00)",
            "2026-03-09,support,(45.50)",
            "2026-03-02,ops ,650.50",
            "2026-03-20,Sales,\"1,050.25\"",
            "2026-03-08,Ops,\"1,200.00\"",
            "2026-03-03,Support,900.00",
            "2026-03-12,Sales,720.00",
        ]
        self.assertEqual(department_totals(rows), {"ops": Decimal("3940.50"), "sales": Decimal("3870.25"), "support": Decimal("1154.50")})

    def test_other_month_other_departments(self):
        self.assertEqual(department_totals(APRIL), {"ops": Decimal("11999.90"), "legal": Decimal("0.10"), "field ops": Decimal("0.00")})

    def test_parentheses_negate(self):
        self.assertEqual(parse_amount("(300.00)"), Decimal("-300.00"))
        self.assertEqual(parse_amount("(0.05)"), Decimal("-0.05"))
        self.assertEqual(parse_amount("(7)"), Decimal("-7.00"))

    def test_thousands_separators(self):
        self.assertEqual(parse_amount("1,200.00"), Decimal("1200.00"))
        self.assertEqual(parse_amount("12,345,678.9"), Decimal("12345678.90"))
        self.assertEqual(parse_amount("(2,000.25)"), Decimal("-2000.25"))

    def test_keys_lowercased_and_stripped(self):
        t = department_totals(["date,department,amount", "2026-01-01, R&D ,1.00", "2026-01-02,r&d,1.00", "2026-01-03,R&D\t,1.00"])
        self.assertEqual(t, {"r&d": Decimal("3.00")})

    def test_two_places_always(self):
        t = department_totals(["date,department,amount", "2026-01-01,ops,5", "2026-01-02,ops,(2)"])
        self.assertEqual(t, {"ops": Decimal("3.00")})
        self.assertEqual(str(t["ops"]), "3.00")
