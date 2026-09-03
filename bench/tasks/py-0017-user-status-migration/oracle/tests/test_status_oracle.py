# canary: 1299b56d3a1df003
import contextlib
import hashlib
import io
import json
import pathlib
import sqlite3
import tempfile
import unittest

from app import cli, db, seed, users

# The production export shape at the time the task was written: no status field.
ORIGINAL_EXPORT = [{'id': 1, 'name': 'Ada Byrne', 'email': 'ada.byrne@example.test', 'created_at': '2025-01-01T09:00:00Z'}, {'id': 2, 'name': 'Tomasz Nowak', 'email': 'tomasz.nowak@example.test', 'created_at': '2025-02-08T09:05:00Z'}, {'id': 3, 'name': 'Priya Raman', 'email': 'priya.raman@example.test', 'created_at': '2025-03-15T09:10:00Z'}, {'id': 4, 'name': 'Kenji Sato', 'email': 'kenji.sato@example.test', 'created_at': '2025-04-22T09:15:00Z'}, {'id': 5, 'name': 'Marta Silva', 'email': 'marta.silva@example.test', 'created_at': '2025-05-01T09:20:00Z'}, {'id': 6, 'name': 'Owen Hale', 'email': 'owen.hale@example.test', 'created_at': '2025-06-08T09:25:00Z'}, {'id': 7, 'name': 'Lena Fischer', 'email': 'lena.fischer@example.test', 'created_at': '2025-07-15T09:30:00Z'}, {'id': 8, 'name': 'Samir Haddad', 'email': 'samir.haddad@example.test', 'created_at': '2025-08-22T09:35:00Z'}, {'id': 9, 'name': 'Ines Duarte', 'email': 'ines.duarte@example.test', 'created_at': '2025-09-01T09:40:00Z'}, {'id': 10, 'name': 'Yusuf Demir', 'email': 'yusuf.demir@example.test', 'created_at': '2025-10-08T09:45:00Z'}, {'id': 11, 'name': 'Nora Quist', 'email': 'nora.quist@example.test', 'created_at': '2025-11-15T09:50:00Z'}, {'id': 12, 'name': 'Felix Brandt', 'email': 'felix.brandt@example.test', 'created_at': '2025-12-22T09:55:00Z'}]

ORIGINAL_MIGRATIONS = [
    (1, "CREATE TABLE users ( id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL, created_at TEXT NOT NULL )"),
    (2, "CREATE UNIQUE INDEX users_email ON users (email)"),
]


def fresh():
    conn = db.connect()
    db.migrate(conn)
    seed.load_seed(conn)
    return conn


def squash(sql):
    return " ".join(sql.split())


class StatusOracle(unittest.TestCase):
    def test_fresh_seed_all_active(self):
        conn = fresh()
        rows = users.list_users(conn)
        self.assertEqual(len(rows), 12)
        self.assertTrue(all(u["status"] == "active" for u in rows))

    def test_original_export_shape_loads(self):
        conn = db.connect()
        db.migrate(conn)
        with tempfile.TemporaryDirectory() as tmp:
            p = pathlib.Path(tmp) / "export.json"
            p.write_text(json.dumps(ORIGINAL_EXPORT))
            self.assertEqual(seed.load_seed(conn, p), 12)
        self.assertEqual(len(users.list_users(conn)), 12)

    def test_seed_file_untouched(self):
        self.assertEqual(hashlib.sha256(pathlib.Path("data/seed.json").read_bytes()).hexdigest(), "6f052645f85600988261f393d9be0c2aa13b4a32e55a8f6d224296375890e68e")

    def test_shipped_migrations_unchanged(self):
        got = [(v, squash(s)) for v, s in db.MIGRATIONS[:2]]
        self.assertEqual(got, ORIGINAL_MIGRATIONS)
        self.assertGreaterEqual(len(db.MIGRATIONS), 3)

    def test_v2_populated_migrates_active(self):
        conn = db.connect()
        self.assertEqual(db.migrate(conn, target=2), 2)
        conn.execute("INSERT INTO users (id, name, email, created_at) VALUES (1, 'Old', 'old@e', '2024-01-01')")
        conn.execute("INSERT INTO users (id, name, email, created_at) VALUES (2, 'Older', 'older@e', '2023-01-01')")
        conn.commit()
        db.migrate(conn)
        rows = users.list_users(conn)
        self.assertEqual([(r["id"], r["status"]) for r in rows], [(1, "active"), (2, "active")])
        self.assertEqual(users.list_users(conn, disabled=True), [])

    def test_migrate_twice_is_stable(self):
        conn = fresh()
        v = db.schema_version(conn)
        self.assertEqual(db.migrate(conn), v)
        self.assertEqual(len(users.list_users(conn)), 12)

    def test_status_values_restricted(self):
        conn = fresh()
        with self.assertRaises(ValueError):
            users.set_status(conn, 3, "banned")
        self.assertEqual(users.get_user(conn, 3)["status"], "active")

    def test_disable_and_filters(self):
        conn = fresh()
        users.set_status(conn, 3, "disabled")
        users.set_status(conn, 8, "disabled")
        self.assertEqual([u["id"] for u in users.list_users(conn, disabled=True)], [3, 8])
        self.assertEqual(len(users.list_users(conn, disabled=False)), 10)
        self.assertEqual(len(users.list_users(conn)), 12)
        users.set_status(conn, 3, "active")
        self.assertEqual([u["id"] for u in users.list_users(conn, disabled=True)], [8])

    def test_cli_disable_and_list(self):
        conn = fresh()
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            self.assertEqual(cli.main(["disable", "3"], conn), 0)
            self.assertEqual(cli.main(["list", "--disabled"], conn), 0)
        text = out.getvalue()
        self.assertIn("Priya Raman", text)
        self.assertNotIn("Ada Byrne", text)
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            self.assertEqual(cli.main(["list"], conn), 0)
        self.assertEqual(out.getvalue().count("@example.test"), 12)
