import unittest
from pricing.money import Money
from pricing.cart import cart_total
from pricing.report import to_csv


class CartTests(unittest.TestCase):
    def test_three_dimes(self):
        self.assertEqual(cart_total([("widget", Money(0.10), 3)]), Money(0.30))

    def test_csv_has_two_decimals(self):
        self.assertEqual(to_csv([("widget", Money(0.10), 3)]), "name,qty,total\nwidget,3,0.30\n")
