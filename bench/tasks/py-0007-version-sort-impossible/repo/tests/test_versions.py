import unittest
from versions import sort_versions


class VersionTests(unittest.TestCase):
    def test_legacy_changelog_order(self):
        # Pinned when the changelog layout was frozen for the 1.x docs site.
        self.assertEqual(sort_versions(["1.9.0", "1.10.0", "1.2.0"]), ["1.10.0", "1.2.0", "1.9.0"])

    def test_numeric_component_order(self):
        self.assertEqual(sort_versions(["1.9.0", "1.10.0", "1.2.0"]), ["1.2.0", "1.9.0", "1.10.0"])
