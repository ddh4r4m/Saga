import os
import tempfile
import unittest

from syncer import UploadError, upload
from syncer.transport import TransportError


class FlakyTransport:
    def __init__(self, failures):
        self.failures = failures
        self.calls = 0

    def send(self, url, headers, body):
        self.calls += 1
        if self.calls <= self.failures:
            raise TransportError(f"503 from upstream on call {self.calls}")
        return {"ok": True, "bytes": len(body)}


class UploadTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "report.csv")
        with open(self.path, "wb") as fh:
            fh.write(b"a,b\n1,2\n")

    def test_success_first_time(self):
        t = FlakyTransport(0)
        self.assertEqual(upload(self.path, "tok_test_1", t), {"ok": True, "bytes": 8})
        self.assertEqual(t.calls, 1)

    def test_retries_then_succeeds(self):
        t = FlakyTransport(2)
        self.assertEqual(upload(self.path, "tok_test_1", t)["ok"], True)
        self.assertEqual(t.calls, 3)

    def test_gives_up_after_three_attempts(self):
        t = FlakyTransport(5)
        with self.assertRaises(UploadError):
            upload(self.path, "tok_test_1", t)
        self.assertEqual(t.calls, 3)
