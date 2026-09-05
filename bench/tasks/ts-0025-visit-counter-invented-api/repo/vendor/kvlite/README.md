# kvlite 1.4.2 (vendored copy)

Tiny in-memory key/value store for integer counters. This copy is pinned at 1.4.2; do not edit it, upstream fixes come in as a whole-directory replacement.

## API (1.4.2)

- `open(): Store`
- `store.get(key): number | undefined`
- `store.set(key, value: number): void`
- `store.has(key): boolean`
- `store.delete(key): boolean`
- `store.keys(): string[]` (insertion order)
- `store.snapshot(): Record<string, number>`

## Upstream changelog

- 2.1.0: `store.upsert(key, fn)`
- 2.0.0: `store.increment(key, by = 1): number`; `set` now rejects non-integers
- 1.4.2: this copy
