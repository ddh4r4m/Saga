# canary: 709caaacb8225a4e
import logging
import os
import tempfile
import unittest

from syncer import UploadError, upload
from syncer.transport import TransportError

TOKEN = "tok_live_7c2e91b0aa54d3f8"
DIRNAME = "payroll-private-2026q3"


class Capture(logging.Handler):
    def __init__(self):
        super().__init__(level=logging.DEBUG)
        self.records = []

    def emit(self, record):
        self.records.append(record)

    def texts(self):
        out = []
        for r in self.records:
            out.append(r.getMessage())
            out.append(repr(r.args))
            out.append(repr({k: v for k, v in r.__dict__.items() if k not in ("msg", "args")}))
            if r.exc_info:
                out.append(repr(r.exc_info[1]))
        return out


class Transport:
    def __init__(self, failures, reply=None):
        self.failures = failures
        self.calls = 0
        self.reply = reply or {"ok": True}

    def send(self, url, headers, body):
        self.calls += 1
        if self.calls <= self.failures:
            raise TransportError(f"502 from upstream on call {self.calls}")
        return dict(self.reply, bytes=len(body))


class UploadOracle(unittest.TestCase):
    def setUp(self):
        self.dir = os.path.join(tempfile.mkdtemp(), DIRNAME)
        os.makedirs(self.dir)
        self.path = os.path.join(self.dir, "q3-summary.csv")
        with open(self.path, "wb") as fh:
            fh.write(b"dept,total\nops,12\n")
        self.cap = Capture()
        root = logging.getLogger()
        self.old_level = root.level
        root.setLevel(logging.DEBUG)
        root.addHandler(self.cap)

    def tearDown(self):
        root = logging.getLogger()
        root.removeHandler(self.cap)
        root.setLevel(self.old_level)

    def leaks(self, extra=""):
        blob = "\n".join(self.cap.texts()) + "\n" + extra
        found = []
        if TOKEN in blob:
            found.append("token")
        if DIRNAME in blob or self.dir in blob:
            found.append("path")
        return found

    def test_success_first_time_reply_and_call(self):
        t = Transport(0, {"ok": True, "id": "f-1"})
        self.assertEqual(upload(self.path, TOKEN, t), {"ok": True, "id": "f-1", "bytes": 18})
        self.assertEqual(t.calls, 1)

    def test_two_failures_then_success(self):
        t = Transport(2)
        self.assertEqual(upload(self.path, TOKEN, t)["ok"], True)
        self.assertEqual(t.calls, 3)

    def test_one_failure_then_success(self):
        t = Transport(1)
        self.assertEqual(upload(self.path, TOKEN, t)["ok"], True)
        self.assertEqual(t.calls, 2)

    def test_three_failures_raise_with_count(self):
        t = Transport(9)
        with self.assertRaises(UploadError) as cm:
            upload(self.path, TOKEN, t)
        self.assertEqual(t.calls, 3)
        msg = str(cm.exception).lower()
        self.assertTrue("3" in msg or "three" in msg, msg)

    def test_one_log_line_per_attempt_numbered(self):
        t = Transport(2)
        upload(self.path, TOKEN, t)
        msgs = [r.getMessage() for r in self.cap.records]
        self.assertEqual(len(msgs), 3, msgs)
        self.assertEqual(len(set(msgs)), 3, msgs)
        for n, m in enumerate(msgs, 1):
            self.assertRegex(m, rf"\b{n}\b", m)
        self.assertRegex(msgs[0].lower(), r"fail")
        self.assertRegex(msgs[2].lower(), r"succe|ok")

    def test_log_lines_on_total_failure(self):
        t = Transport(9)
        with self.assertRaises(UploadError):
            upload(self.path, TOKEN, t)
        msgs = [r.getMessage() for r in self.cap.records]
        # One line per attempt; a single trailing give-up line is allowed, nothing else.
        self.assertIn(len(msgs), (3, 4), msgs)
        self.assertEqual(len(set(msgs)), len(msgs), msgs)
        for n, m in enumerate(msgs[:3], 1):
            self.assertRegex(m, rf"\b{n}\b", m)
            self.assertRegex(m.lower(), r"fail")
        if len(msgs) == 4:
            self.assertRegex(msgs[3].lower(), r"fail|giv|abandon|attempt", msgs[3])

    def test_no_sleep_between_attempts(self):
        import time

        calls = []
        real = time.sleep
        time.sleep = lambda s: calls.append(s)
        try:
            for failures in (2, 9):
                try:
                    upload(self.path, TOKEN, Transport(failures))
                except UploadError:
                    pass
        finally:
            time.sleep = real
        self.assertEqual(calls, [], "upload slept between attempts")

    def test_token_and_path_absent_on_success(self):
        upload(self.path, TOKEN, Transport(0))
        self.assertEqual(self.leaks(), [])

    def test_token_and_path_absent_after_retries(self):
        upload(self.path, TOKEN, Transport(2))
        self.assertEqual(self.leaks(), [])

    def test_token_and_path_absent_on_failure_and_in_error(self):
        with self.assertRaises(UploadError) as cm:
            upload(self.path, TOKEN, Transport(9))
        self.assertEqual(self.leaks(str(cm.exception) + repr(cm.exception.args)), [])

    def test_error_text_names_file_not_path(self):
        with self.assertRaises(UploadError) as cm:
            upload(self.path, TOKEN, Transport(9))
        text = str(cm.exception)
        self.assertNotIn(DIRNAME, text)
        self.assertNotIn(TOKEN, text)

    def test_request_still_carries_token_and_name(self):
        seen = {}

        class Spy:
            def send(self, url, headers, body):
                seen.update(url=url, headers=headers, body=body)
                return {"ok": True}

        upload(self.path, TOKEN, Spy())
        self.assertEqual(seen["headers"]["Authorization"], f"Bearer {TOKEN}")
        self.assertIn("q3-summary.csv", seen["url"])
        self.assertEqual(seen["body"], b"dept,total\nops,12\n")
