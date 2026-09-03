"""Load the production snapshot in data/seed.json."""

import json
import pathlib
import sqlite3

DEFAULT_SEED = pathlib.Path(__file__).resolve().parent.parent / "data" / "seed.json"


def load_seed(conn: sqlite3.Connection, path: pathlib.Path | str = DEFAULT_SEED) -> int:
    records = json.loads(pathlib.Path(path).read_text())
    for r in records:
        conn.execute(
            "INSERT INTO users (id, name, email, created_at) VALUES (?, ?, ?, ?)",
            (r["id"], r["name"], r["email"], r["created_at"]),
        )
    conn.commit()
    return len(records)
