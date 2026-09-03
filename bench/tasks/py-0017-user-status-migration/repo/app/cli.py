"""Admin CLI: list | show <id>."""

import sys

from . import db, seed, users


def format_user(u: dict) -> str:
    return f"{u['id']:>4}  {u['name']:<20} {u['email']}"


def main(argv: list[str], conn=None) -> int:
    if conn is None:
        conn = db.connect()
        db.migrate(conn)
        seed.load_seed(conn)
    if not argv:
        print("usage: accounts list | show <id>")
        return 2
    cmd, args = argv[0], argv[1:]
    if cmd == "list":
        for u in users.list_users(conn):
            print(format_user(u))
        return 0
    if cmd == "show" and len(args) == 1:
        u = users.get_user(conn, int(args[0]))
        if u is None:
            print(f"no user {args[0]}")
            return 1
        print(format_user(u))
        return 0
    print("usage: accounts list | show <id>")
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
