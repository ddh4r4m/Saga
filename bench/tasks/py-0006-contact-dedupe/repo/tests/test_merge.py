import unittest
from contacts.models import Contact
from contacts.merge import dedupe


class DedupeTests(unittest.TestCase):
    def test_same_email_collapses(self):
        a = Contact("Ann", "ann@example.com", "1")
        b = Contact("Ann Smith", "ann@example.com", "2")
        self.assertEqual(len(dedupe([a, b])), 1)
