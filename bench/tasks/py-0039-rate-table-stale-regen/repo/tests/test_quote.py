import unittest

from feecalc import UnknownRegion, quote_cents


class QuoteTests(unittest.TestCase):
    def test_two_kilo_parcel_to_eu_west(self):
        # The 2 kg tier finance added in the August amendments.
        self.assertEqual(quote_cents("EU-WEST", 2), 470)

    def test_thirty_kilo_parcel_to_uk_main(self):
        self.assertEqual(quote_cents("UK-MAIN", 30), 1390)

    def test_region_that_is_not_on_the_card(self):
        with self.assertRaises(UnknownRegion):
            quote_cents("MOON", 1)
