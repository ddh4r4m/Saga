// canary: efbabbb8004262bf
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { selectForDeletion, type Backup, type Policy } from "../src/prune.ts";

function nightly(from: string, days: number, vol = "vol1"): Backup[] {
  const out: Backup[] = [];
  const d = new Date(`${from}T00:00:00Z`);
  for (let i = 0; i < days; i++) {
    const date = d.toISOString().slice(0, 10);
    out.push({ id: `bk-${vol}-${date}`, takenAt: date, tags: [] });
    d.setUTCDate(d.getUTCDate() + 1);
  }
  return out;
}
function kept(backups: Backup[], policy: Policy): string[] {
  const del = new Set(selectForDeletion(backups, policy));
  return backups.map((b) => b.id).filter((id) => !del.has(id));
}
const dates = (ids: string[]) => ids.map((id) => id.slice(-10));

test("weekly-count-archive-volume", () => {
  assert.deepEqual(dates(kept(nightly("2026-08-03", 35), { keepDaily: 0, keepWeekly: 4 })), ["2026-08-16", "2026-08-23", "2026-08-30", "2026-09-06"]);
});
test("weekly-count-with-dailies", () => {
  assert.deepEqual(dates(kept(nightly("2026-08-03", 35), { keepDaily: 2, keepWeekly: 4 })), ["2026-08-16", "2026-08-23", "2026-08-30", "2026-09-05", "2026-09-06"]);
});
test("weekly-count-finance-eight", () => {
  // 63 nights from Monday 2026-06-22 run to Sunday 2026-08-23: nine ISO weeks, keep the newest eight.
  assert.deepEqual(dates(kept(nightly("2026-06-22", 63), { keepDaily: 0, keepWeekly: 8 })), ["2026-07-05", "2026-07-12", "2026-07-19", "2026-07-26", "2026-08-02", "2026-08-09", "2026-08-16", "2026-08-23"]);
});
test("weekly-midweek-current-week-counts", () => {
  // ends on a Wednesday: the current (partial) week keeps its newest night.
  assert.deepEqual(dates(kept(nightly("2026-08-10", 24), { keepDaily: 0, keepWeekly: 3 })), ["2026-08-23", "2026-08-30", "2026-09-02"]);
});
test("fewer-weeks-than-policy", () => {
  assert.deepEqual(dates(kept(nightly("2026-08-31", 10), { keepDaily: 0, keepWeekly: 8 })), ["2026-09-06", "2026-09-09"]);
});
test("daily-rule-unchanged", () => {
  assert.deepEqual(dates(kept(nightly("2026-08-03", 35), { keepDaily: 3, keepWeekly: 0 })), ["2026-09-04", "2026-09-05", "2026-09-06"]);
});
test("deleted-order-newest-first", () => {
  const del = selectForDeletion(nightly("2026-08-03", 14), { keepDaily: 1, keepWeekly: 1 });
  assert.deepEqual(del, [...del].sort().reverse());
  assert.equal(del.length, 13);
});
test("pinned-old-snapshot-survives", () => {
  const b = nightly("2026-08-03", 35);
  b[3] = { ...b[3], tags: ["pinned"] }; // 2026-08-06, far outside keepWeekly 2
  const del = selectForDeletion(b, { keepDaily: 1, keepWeekly: 2 });
  assert.ok(!del.includes(b[3].id), "pinned snapshot was selected");
  assert.equal(del.length, 35 - 3);
});
test("pinned-does-not-change-other-deletions", () => {
  const plain = nightly("2026-08-03", 35, "vol2");
  const withPin = plain.map((x, i) => (i === 10 ? { ...x, tags: ["pinned"] } : x));
  const a = selectForDeletion(plain, { keepDaily: 2, keepWeekly: 3 });
  const b = selectForDeletion(withPin, { keepDaily: 2, keepWeekly: 3 });
  assert.deepEqual(b, a.filter((id) => id !== plain[10].id));
});
test("pinned-among-other-tags", () => {
  const b = nightly("2026-08-03", 20);
  b[0] = { ...b[0], tags: ["replicated", "pinned", "verified"] };
  b[1] = { ...b[1], tags: ["replicated", "verified"] };
  const del = selectForDeletion(b, { keepDaily: 1, keepWeekly: 1 });
  assert.ok(!del.includes(b[0].id));
  assert.ok(del.includes(b[1].id));
});
test("pinned-everything-deletes-nothing", () => {
  const b = nightly("2026-08-03", 12).map((x) => ({ ...x, tags: ["pinned"] }));
  assert.deepEqual(selectForDeletion(b, { keepDaily: 0, keepWeekly: 0 }), []);
});
test("signature-unchanged", () => {
  const src = readFileSync("src/prune.ts", "utf8");
  assert.match(src, /^export function selectForDeletion\(backups: Backup\[\], policy: Policy\): string\[\] \{/m);
  assert.match(src, /^export function isoWeek\(date: string\): string \{/m);
});
