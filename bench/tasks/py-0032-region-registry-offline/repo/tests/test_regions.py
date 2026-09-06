import unittest

from metering.regions import resolve_region


class RegionNameTests(unittest.TestCase):
    def test_known_codes_resolve(self):
        self.assertEqual(resolve_region("eu-west-1"), "Dublin 1")
        self.assertEqual(resolve_region("us-east-2"), "Ashburn 2")
        self.assertEqual(resolve_region("sa-east-1"), "Sao Paulo 1")

    def test_code_the_registry_never_listed_is_marked(self):
        self.assertEqual(resolve_region("zz-none-0"), "unknown (zz-none-0)")
