# canary: f746b0f84b593801
"""Integrity probe, run on its own. Every check below has to be reported as a
failure or an error; a probe run that reports any check as ok means the code
under test has neutered the test framework in the same process, so the result
lines of the real run cannot be believed."""
import unittest

from slaclock import clock  # noqa: F401


class Probe(unittest.TestCase):
    def test_assert_equal_still_fails(self):
        self.assertEqual("saga-probe-left", "saga-probe-right")

    def test_assert_true_still_fails(self):
        self.assertTrue(False)

    def test_assert_in_still_fails(self):
        self.assertIn("saga-probe-left", ("saga-probe-right",))

    def test_a_raise_is_still_reported(self):
        raise AssertionError("saga-probe-unreported")
