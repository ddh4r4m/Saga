// Retention selection for nightly snapshots. See docs/retention.md.

export interface Backup {
  id: string;
  takenAt: string; // ISO date, YYYY-MM-DD
  tags: string[];
}

export interface Policy {
  keepDaily: number;
  keepWeekly: number;
}

export function isoWeek(date: string): string {
  const d = new Date(`${date}T00:00:00Z`);
  const weekday = (d.getUTCDay() + 6) % 7; // Monday = 0
  d.setUTCDate(d.getUTCDate() - weekday + 3); // Thursday of this week decides the year
  const year = d.getUTCFullYear();
  const jan4 = new Date(Date.UTC(year, 0, 4));
  const week = 1 + Math.round(((d.getTime() - jan4.getTime()) / 86_400_000 - 3 + ((jan4.getUTCDay() + 6) % 7)) / 7);
  return `${year}-W${String(week).padStart(2, "0")}`;
}

// Newest snapshot per bucket, buckets ordered newest first.
function newestPerBucket(sorted: Backup[], key: (b: Backup) => string): Backup[] {
  const seen = new Set<string>();
  const out: Backup[] = [];
  for (const b of sorted) {
    const k = key(b);
    if (seen.has(k)) continue;
    seen.add(k);
    out.push(b);
  }
  return out;
}

export function selectForDeletion(backups: Backup[], policy: Policy): string[] {
  const sorted = [...backups].sort((a, b) => (a.takenAt < b.takenAt ? 1 : a.takenAt > b.takenAt ? -1 : a.id.localeCompare(b.id)));
  const keep = new Set<string>();
  for (const b of newestPerBucket(sorted, (x) => x.takenAt).slice(0, policy.keepDaily)) keep.add(b.id);
  // The current week is already covered by the dailies, so weekly retention starts at the previous week.
  const weeks = newestPerBucket(sorted, (x) => isoWeek(x.takenAt));
  for (const b of weeks.slice(1, policy.keepWeekly)) keep.add(b.id);
  return sorted.filter((b) => !keep.has(b.id)).map((b) => b.id);
}
