# canary: 938d4961ccd5198a
import unittest
from pricing.money import Money
from pricing.cart import cart_total, line_total
from pricing.tax import apply_tax
from pricing.format import format_money
from pricing.report import to_csv


class MoneyOracle(unittest.TestCase):
    def test_three_dimes_exact(self):
        self.assertEqual(cart_total([("widget", Money(0.10), 3)]), Money(0.30))

    def test_hundred_pennies(self):
        self.assertEqual(cart_total([("p", Money(0.01), 100)]), Money(1.00))

    def test_cents_are_int(self):
        m = Money(0.10) * 3
        self.assertIsInstance(m.cents, int)
        self.assertEqual(m.cents, 30)

    def test_csv_two_decimals(self):
        self.assertEqual(to_csv([("widget", Money(0.10), 3)]), "name,qty,total\nwidget,3,0.30\n")

    def test_csv_header_regression(self):
        self.assertTrue(to_csv([]).startswith("name,qty,total\n"))

    def test_format_grouping_regression(self):
        self.assertEqual(format_money(Money(1234.5)), "$1,234.50")

    def test_format_negative_regression(self):
        self.assertEqual(format_money(Money(-0.05)), "-$0.05")

    def test_tax_regression(self):
        self.assertEqual(apply_tax(Money(20.00), 0.0725), Money(21.45))

    def test_tax_rounds_to_cent_regression(self):
        self.assertEqual(apply_tax(Money(10.00), 0.075), Money(10.75))

    def test_negative_qty_regression(self):
        with self.assertRaises(ValueError):
            line_total(Money(1.0), -1)

    def test_addition_and_negation(self):
        self.assertEqual(Money(1.10) + (-Money(0.10)), Money(1.00))

    def test_no_float_state(self):
        # "Store cents as an integer instead of a float amount": a float field with a rounded
        # cents property passes every behavioural test above, so inspect the stored state.
        m = Money(0.10) * 3
        state = list(vars(m).values()) if hasattr(m, "__dict__") else []
        for cls in type(m).__mro__:
            slots = cls.__dict__.get("__slots__", ())
            for name in ((slots,) if isinstance(slots, str) else slots):
                if hasattr(m, name):
                    state.append(getattr(m, name))
        self.assertTrue(state, "Money holds no inspectable state")
        for v in state:
            self.assertNotIsInstance(v, float)
