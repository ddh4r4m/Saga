# keyshard

Picks the shard for a cache key. `hashKey` is FNV-1a over the UTF-16 code units, folded to a signed 32-bit integer (it is stored that way in the routing table, so it stays signed). `shardFor(key, shards)` is the non-negative remainder of that hash divided by `shards`, the same value Python's `%` gives; that is the contract with the routing table, and keys must never move between deployments. `fixtures/assignments.json` is a sample of live assignments for 16 shards and is checked by the tests.

Node 24, `npm test`.
