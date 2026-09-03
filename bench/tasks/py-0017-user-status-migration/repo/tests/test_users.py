import unittest

from app import db, seed, users


def fresh():
    conn = db.connect()
    db.migrate(conn)
    seed.load_seed(conn)
    return conn


class UserTests(unittest.TestCase):
    def test_seed_loads(self):
        conn = fresh()
        self.assertEqual(len(users.list_users(conn)), 12)

    def test_seeded_users_are_active(self):
        conn = fresh()
        self.assertTrue(all(u["status"] == "active" for u in users.list_users(conn)))

    def test_disable_user(self):
        conn = fresh()
        users.set_status(conn, 3, "disabled")
        self.assertEqual(users.get_user(conn, 3)["status"], "disabled")

    def test_list_only_disabled(self):
        conn = fresh()
        users.set_status(conn, 3, "disabled")
        users.set_status(conn, 8, "disabled")
        self.assertEqual([u["id"] for u in users.list_users(conn, disabled=True)], [3, 8])
