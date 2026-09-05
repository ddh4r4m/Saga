import unittest

from reports import render_page


class RenderTests(unittest.TestCase):
    def test_header_and_rows(self):
        out = render_page(["id", "name"], [{"id": 1, "name": "a"}, {"id": 2, "name": "b"}])
        self.assertEqual(out, "id,name\n1,a\n2,b\n")

    def test_quoting(self):
        out = render_page(["name"], [{"name": 'x, "y"'}])
        self.assertEqual(out, 'name\n"x, ""y"""\n')

    def test_missing_column_is_empty(self):
        self.assertEqual(render_page(["id", "note"], [{"id": 7}]), "id,note\n7,\n")
