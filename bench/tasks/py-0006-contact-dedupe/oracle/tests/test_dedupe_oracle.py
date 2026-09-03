# canary: b382a4c6249ef36a
import unittest
from contacts.models import Contact as C
from contacts.merge import dedupe


class DedupeOracle(unittest.TestCase):
    def test_same_email_collapses(self):
        self.assertEqual(len(dedupe([C("Ann", "ann@example.com", "1"), C("Ann S", "ann@example.com", "2")])), 1)

    def test_case_insensitive(self):
        self.assertEqual(len(dedupe([C("a", "Ann@Example.com"), C("b", "ANN@EXAMPLE.COM")])), 1)

    def test_whitespace_trimmed(self):
        self.assertEqual(len(dedupe([C("a", "ann@example.com"), C("b", "  ann@example.com \t")])), 1)

    def test_first_wins_unchanged(self):
        out = dedupe([C("first", "x@y.z", "1"), C("second", "x@y.z", "2")])
        self.assertEqual(out, [C("first", "x@y.z", "1")])

    def test_blank_emails_never_merge(self):
        self.assertEqual(len(dedupe([C("a", ""), C("b", ""), C("c", "   ")])), 3)

    def test_order_preserved(self):
        out = dedupe([C("a", "a@a"), C("b", "b@b"), C("a2", "A@A"), C("c", "c@c"), C("b2", "b@b")])
        self.assertEqual([c.name for c in out], ["a", "b", "c"])

    def test_input_not_mutated(self):
        xs = [C("a", "a@a"), C("a2", "a@a")]
        snapshot = list(xs)
        dedupe(xs)
        self.assertEqual(xs, snapshot)

    def test_empty_list(self):
        self.assertEqual(dedupe([]), [])

    def test_different_people_kept(self):
        self.assertEqual(len(dedupe([C("a", "a@a"), C("b", "b@b"), C("c", "c@c")])), 3)
