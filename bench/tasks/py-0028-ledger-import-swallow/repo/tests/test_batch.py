import unittest
from decimal import Decimal

from ledgerimport import import_rows


class ListSink:
    def __init__(self):
        self.rows = []

    def write(self, row):
        self.rows.append(row)


class BatchTests(unittest.TestCase):
    def test_good_rows_are_imported(self):
        sink = ListSink()
        result = import_rows(["date,account,amount\n", "2026-09-01,ops,120.50\n", "2026-09-01,rent,-900.00\n"], sink)
        self.assertEqual(len(result.imported), 2)
        self.assertEqual(sink.rows[1].amount, Decimal("-900.00"))

    def test_bad_row_does_not_stop_the_batch(self):
        sink = ListSink()
        with open("fixtures/day.csv", encoding="utf-8") as fh:
            result = import_rows(fh, sink)
        self.assertEqual([r.account for r in result.imported], ["ops", "rent", "sales"])
        self.assertEqual(len(sink.rows), 3)
