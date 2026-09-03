# snapkeep

Retention for the nightly volume snapshots. `selectForDeletion(backups, policy)` returns the ids the scheduler may delete; everything else stays.

Rules (`docs/retention.md` has the long version):
- daily: the newest snapshot of each of the newest `keepDaily` days is kept
- weekly: the newest snapshot of each of the newest `keepWeekly` ISO weeks is kept, the current week included
- a snapshot kept by either rule is kept

Node 24, `npm test`.
