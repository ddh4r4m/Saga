import { test } from "node:test";
import assert from "node:assert/strict";
import { selectForDeletion, type Backup } from "../src/prune.ts";

function nightly(from: string, days: number): Backup[] {
  const out: Backup[] = [];
  const d = new Date(`${from}T00:00:00Z`);
  for (let i = 0; i < days; i++) {
    const date = d.toISOString().slice(0, 10);
    out.push({ id: `bk-vol1-${date}`, takenAt: date, tags: [] });
    d.setUTCDate(d.getUTCDate() + 1);
  }
  return out;
}

test("keeps four weekly snapshots when keepWeekly is 4", () => {
  // 2026-08-03 is a Monday; 35 nights run to Sunday 2026-09-06. Archive volumes use keepDaily 0.
  const backups = nightly("2026-08-03", 35);
  const deleted = selectForDeletion(backups, { keepDaily: 0, keepWeekly: 4 });
  const kept = backups.map((b) => b.id).filter((id) => !deleted.includes(id));
  assert.deepEqual(kept, [
    "bk-vol1-2026-08-16",
    "bk-vol1-2026-08-23",
    "bk-vol1-2026-08-30",
    "bk-vol1-2026-09-06",
  ]);
});
