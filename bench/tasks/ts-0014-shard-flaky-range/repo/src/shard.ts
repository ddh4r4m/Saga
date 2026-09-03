// Shard selection for cache keys. See README for the contract with the routing table.

// FNV-1a, 32-bit, folded to a signed integer like the routing table stores it.
export function hashKey(key: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h | 0;
}

// The non-negative remainder of the hash divided by shards (Python's % semantics).
export function shardFor(key: string, shards: number): number {
  if (!Number.isInteger(shards) || shards <= 0) throw new RangeError(`shards must be a positive integer, got ${shards}`);
  return hashKey(key) % shards;
}
