"""Schema and migrations. Append to MIGRATIONS; never edit a shipped entry."""

import sqlite3

MIGRATIONS: list[tuple[int, str]] = [
    (
        1,
        """
        CREATE TABLE users (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT NOT NULL,
            created_at TEXT NOT NULL
        )
        """,
    ),
    (2, "CREATE UNIQUE INDEX users_email ON users (email)"),
]


def connect(path: str = ":memory:") -> sqlite3.Connection:
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    return conn


def schema_version(conn: sqlite3.Connection) -> int:
    conn.execute("CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)")
    row = conn.execute("SELECT MAX(version) AS v FROM schema_version").fetchone()
    return row["v"] or 0


def migrate(conn: sqlite3.Connection, target: int | None = None) -> int:
    """Apply every migration above the current version, up to target. Returns the new version."""
    current = schema_version(conn)
    for version, sql in MIGRATIONS:
        if version <= current:
            continue
        if target is not None and version > target:
            break
        conn.execute(sql)
        conn.execute("INSERT INTO schema_version (version) VALUES (?)", (version,))
        current = version
    conn.commit()
    return current
