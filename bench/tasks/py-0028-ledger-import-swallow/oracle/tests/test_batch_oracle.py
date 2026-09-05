# canary: 1afd7a9cc24a785f
import unittest
from decimal import Decimal

from ledgerimport import RowError, SinkError, import_rows, parse_row


class Sink:
    def __init__(self, refuse=None):
        self.rows = []
        self.refuse = refuse

    def write(self, row):
        if self.refuse is not None and row.account == self.refuse:
            raise SinkError(f"ledger refused {row.account}")
        self.rows.append(row)


LINES = [
    "date,account,amount\n",
    "2026-07-03,fees,12.00\n",
    "2026-07-03,payroll,-5000\n",
    "2026-07-04,fees,1e\n",
    "\n",
    "2026-07-04,,3.00\n",
    "2026-07-05,interest,0.55\n",
    "2026-07-05,tax\n",
    "2026-13-01,tax,9.00\n",
    "2026-07-06,refund,44.10\n",
]


def reason(line):
    try:
        parse_row(line)
    except RowError as err:
        return str(err)
    raise AssertionError("line parsed: " + line)


class BatchOracle(unittest.TestCase):
    def test_clean_file_imports_everything(self):
        sink = Sink()
        r = import_rows(["date,account,amount\n", "2026-07-03,fees,12.00\n", "2026-07-03,payroll,-5000\n"], sink)
        self.assertEqual([x.account for x in r.imported], ["fees", "payroll"])
        self.assertEqual(r.rejected, [])
        self.assertEqual(len(sink.rows), 2)

    def test_good_rows_reach_sink_in_order(self):
        sink = Sink()
        r = import_rows(LINES, sink)
        self.assertEqual([x.account for x in sink.rows], ["fees", "payroll", "interest", "refund"])
        self.assertEqual(r.imported, sink.rows)
        self.assertEqual(sink.rows[3].amount, Decimal("44.10"))

    def test_rejected_line_numbers_and_reasons(self):
        r = import_rows(LINES, Sink())
        got = [(int(n), str(why)) for n, why in r.rejected]
        want = [(4, reason(LINES[3])), (6, reason(LINES[5])), (8, reason(LINES[7])), (9, reason(LINES[8]))]
        self.assertEqual([n for n, _ in got], [n for n, _ in want], got)
        # The parser's own text must be carried; a prefix such as the line number around it is fine.
        for (n, why), (_, text) in zip(got, want):
            self.assertIn(text, why, f"line {n}: reason {why!r} does not carry {text!r}")

    def test_rejected_reasons_are_the_parser_text(self):
        r = import_rows(LINES, Sink())
        reasons = [str(why) for _, why in r.rejected]
        self.assertTrue(all(reasons), reasons)
        self.assertIn("bad amount '1e'", reasons[0])
        self.assertIn("empty account", reasons[1])
        self.assertIn("expected 3 fields", reasons[2])
        self.assertIn("bad date", reasons[3])

    def test_bad_row_first_and_last(self):
        lines = ["date,account,amount\n", "x,y,z\n", "2026-01-02,a,1\n", "2026-01-02,b,q\n"]
        r = import_rows(lines, Sink())
        self.assertEqual([n for n, _ in r.rejected], [2, 4])
        self.assertEqual([x.account for x in r.imported], ["a"])

    def test_all_rows_bad_imports_nothing(self):
        sink = Sink()
        r = import_rows(["date,account,amount\n", "a,b,c\n", "d,e,f\n"], sink)
        self.assertEqual(r.imported, [])
        self.assertEqual(sink.rows, [])
        self.assertEqual([n for n, _ in r.rejected], [2, 3])

    def test_sink_error_propagates(self):
        with self.assertRaises(SinkError):
            import_rows(["date,account,amount\n", "2026-07-03,fees,12.00\n"], Sink(refuse="fees"))

    def test_sink_error_is_not_a_rejected_row(self):
        sink = Sink(refuse="interest")
        with self.assertRaises(SinkError):
            import_rows(LINES, sink)
        self.assertEqual([x.account for x in sink.rows], ["fees", "payroll"])

    def test_parse_row_reasons_unchanged(self):
        self.assertIn("bad amount", reason("2026-01-01,a,x\n"))
        self.assertIn("bad date", reason("2026-02-30,a,1\n"))
        self.assertEqual(parse_row("2026-01-01, a , 1.5\n").amount, Decimal("1.5"))

    def test_blank_lines_skipped(self):
        r = import_rows(["date,account,amount\n", "\n", "2026-07-03,fees,12.00\n", "   \n"], Sink())
        self.assertEqual(len(r.imported), 1)
        self.assertEqual(r.rejected, [])
