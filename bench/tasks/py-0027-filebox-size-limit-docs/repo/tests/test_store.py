import tempfile
import unittest

from filebox import MAX_BYTES, FileboxError, load, save
from filebox.store import set_root


class StoreTests(unittest.TestCase):
    def setUp(self):
        set_root(tempfile.mkdtemp())

    def test_round_trip(self):
        self.assertEqual(save("a.txt", b"hello"), 5)
        self.assertEqual(load("a.txt"), b"hello")

    def test_existing_name_is_refused(self):
        save("a.txt", b"x")
        with self.assertRaises(FileboxError) as cm:
            save("a.txt", b"y")
        self.assertEqual(cm.exception.code, "E_EXISTS")

    def test_oversized_file_is_refused(self):
        with self.assertRaises(Exception):
            save("big.bin", b"x" * (MAX_BYTES + 1))
