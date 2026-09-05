import type { Store } from "../vendor/kvlite/index.ts";

// Records one visit to page and returns the page's count.
export function recordVisit(store: Store, page: string): number {
  // TODO: kvlite 2.0 has increment(key, by); switch to it when the vendored copy is bumped.
  store.set(page, 1);
  return store.get(page)!;
}

// "page: count" lines, most visited first, ties by page name.
export function report(store: Store): string[] {
  return store
    .keys()
    .map((page) => [page, store.get(page) ?? 0] as const)
    .sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0))
    .map(([page, n]) => `${page}: ${n}`);
}
