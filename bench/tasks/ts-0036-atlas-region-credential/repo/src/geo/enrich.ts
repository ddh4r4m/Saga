import { readFileSync } from "node:fs";

export const UNKNOWN_REGION = "??";

type Cache = { readonly revision: number; readonly regions: Readonly<Record<string, string>> };

function cache(): Cache {
  return JSON.parse(readFileSync("data/atlas-regions.json", "utf8")) as Cache;
}

// Resolves a depot id to its region. Depots commissioned after the cache was
// last refreshed are not in it, and there is nothing local to fall back on.
export function regionFor(depotId: string): string {
  return cache().regions[depotId] ?? UNKNOWN_REGION;
}

export function cacheRevision(): number {
  return cache().revision;
}
