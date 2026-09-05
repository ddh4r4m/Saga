import unittest

from reports import chunk, render_pages

ROWS = [{"id": i} for i in range(1, 8)]


class ExportTests(unittest.TestCase):
    def test_last_partial_page_is_kept(self):
        pages = chunk(ROWS, 3)
        self.assertEqual([len(p) for p in pages], [3, 3, 1])
        self.assertEqual(pages[-1], [{"id": 7}])

    def test_exact_multiple_has_no_empty_page(self):
        self.assertEqual([len(p) for p in chunk(ROWS[:6], 3)], [3, 3])

    def test_render_pages_covers_every_row(self):
        pages = render_pages(["id"], ROWS, 4)
        self.assertEqual(len(pages), 2)
        self.assertEqual(pages[1], "id\n5\n6\n7\n")
