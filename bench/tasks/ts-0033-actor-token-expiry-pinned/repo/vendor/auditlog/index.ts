// auditlog 4.0.2, byte-pinned vendored copy of the shared compliance library.
// The compliance exporter runs the same build against its own index, so the
// line format and the key type must not drift.
//
// `actorId` is a primitive string. The exporter uses it as an object key and
// normalises it before comparing, so anything else is a defect on the caller's
// side, not here.

export function record(actorId: string, action: string): string {
  const key = actorId.normalize("NFC");
  return key.padEnd(12, ".") + " " + action;
}

export function tally(actorIds: readonly string[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const id of actorIds) {
    const key = id.normalize("NFC");
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}
