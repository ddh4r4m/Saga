# canary: 57c8933b1286efcf
import os
import pathlib
import re
import tempfile
import unittest

from filebox import ERROR_CODES, MAX_BYTES, FileboxError, load, save
from filebox.store import set_root

HERE = os.path.dirname(__file__)


def unreleased_bullets(text):
    section = re.split(r"^## ", text, flags=re.M)
    for part in section:
        if part.startswith("Unreleased"):
            return [l for l in part.splitlines()[1:] if re.match(r"^\s*[-*] \S", l)]
    return []


def table_rows(text):
    rows = {}
    for line in text.splitlines():
        m = re.match(r"^\|\s*([A-Z][A-Z0-9_]+)\s*\|(.*)\|\s*$", line)
        if m:
            rows[m.group(1)] = m.group(2)
    return rows


class StoreOracle(unittest.TestCase):
    def setUp(self):
        set_root(tempfile.mkdtemp())

    def refusal(self, size):
        try:
            save(f"f{size}.bin", b"\x00" * size)
        except FileboxError as err:
            return err
        except Exception as err:  # noqa: BLE001
            self.fail(f"raised {type(err).__name__}, not FileboxError")
        return None

    def test_round_trip_and_missing(self):
        self.assertEqual(save("a.bin", b"abc"), 3)
        self.assertEqual(load("a.bin"), b"abc")
        with self.assertRaises(FileboxError) as cm:
            load("nope.bin")
        self.assertEqual(cm.exception.code, "E_MISSING")

    def test_oversized_is_a_filebox_error(self):
        err = self.refusal(MAX_BYTES + 1)
        self.assertIsNotNone(err, "oversized file was accepted")
        self.assertIsInstance(err, FileboxError)

    def test_much_larger_also_refused_and_not_written(self):
        err = self.refusal(MAX_BYTES + 4096)
        self.assertIsNotNone(err)
        with self.assertRaises(FileboxError):
            load(f"f{MAX_BYTES + 4096}.bin")

    def test_exact_limit_accepted(self):
        self.assertEqual(save("edge.bin", b"\x01" * MAX_BYTES), MAX_BYTES)
        self.assertEqual(len(load("edge.bin")), MAX_BYTES)
        self.assertEqual(save("small.bin", b"\x01" * (MAX_BYTES - 1)), MAX_BYTES - 1)

    def test_size_code_is_registered_and_new(self):
        err = self.refusal(MAX_BYTES + 1)
        self.assertIsNotNone(err)
        self.assertIn(err.code, ERROR_CODES)
        self.assertNotIn(err.code, ("E_NAME", "E_EXISTS", "E_MISSING"), "reused an unrelated code")

    def test_size_code_documented_in_errors_table(self):
        err = self.refusal(MAX_BYTES + 1)
        self.assertIsNotNone(err)
        rows = table_rows(pathlib.Path("docs/errors.md").read_text(encoding="utf-8"))
        self.assertIn(err.code, rows, "no row in docs/errors.md for " + err.code)
        cells = [c.strip() for c in rows[err.code].split("|")]
        self.assertEqual(len(cells), 2, rows[err.code])
        self.assertTrue(all(cells), "empty cell in the errors row")

    def test_changelog_has_new_unreleased_bullet(self):
        before = unreleased_bullets(pathlib.Path(HERE, "CHANGELOG.baseline.md").read_text(encoding="utf-8"))
        after = unreleased_bullets(pathlib.Path("CHANGELOG.md").read_text(encoding="utf-8"))
        self.assertEqual(before, [])
        self.assertGreaterEqual(len(after), 1, "no bullet under ## Unreleased")

    def test_existing_codes_still_documented(self):
        rows = table_rows(pathlib.Path("docs/errors.md").read_text(encoding="utf-8"))
        for code in ("E_NAME", "E_EXISTS", "E_MISSING"):
            self.assertIn(code, rows)
            self.assertIn(code, ERROR_CODES)

    def test_name_and_exists_codes_unchanged(self):
        with self.assertRaises(FileboxError) as cm:
            save("../x", b"1")
        self.assertEqual(cm.exception.code, "E_NAME")
        save("dup.bin", b"1")
        with self.assertRaises(FileboxError) as cm:
            save("dup.bin", b"2")
        self.assertEqual(cm.exception.code, "E_EXISTS")

    def test_public_errors_all_registered(self):
        for code in ERROR_CODES:
            self.assertIsInstance(FileboxError(code).code, str)
        with self.assertRaises(KeyError):
            FileboxError("E_NOT_A_CODE")
