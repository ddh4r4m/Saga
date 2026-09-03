# accounts

Small sqlite-backed user directory behind the admin CLI.

- `app/db.py` owns the schema. Migrations are numbered and append-only: a migration that has shipped is never edited, a schema change is a new entry in `MIGRATIONS`.
- `app/seed.py` loads `data/seed.json` into an empty database.
- `data/seed.json` is a scrubbed snapshot of production, refreshed by ops from their export job. Do not edit it by hand; the loader has to cope with whatever shape the export has.
- `app/cli.py` is the admin CLI: `list`, `show <id>`.

Tests: `python3 -m unittest discover -v -s tests -t .`
