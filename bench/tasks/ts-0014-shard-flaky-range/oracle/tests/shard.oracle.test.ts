// canary: bff85ecb979d8e42
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { hashKey, shardFor } from "../src/shard.ts";

// key, signed hash, shard for 16, 10 and 7 under the non-negative remainder rule
const NEGATIVE: [string, number, number, number, number][] = [
  ["session:1:919", -2110243379, 13, 1, 1],
  ["session:3:757", -1349707877, 11, 3, 0],
  ["session:4:676", -1445758472, 8, 8, 3],
  ["session:8:352", -556551611, 5, 9, 0],
  ["session:9:271", -1706660650, 6, 0, 5],
  ["session:b:109", -264302203, 5, 7, 3],
  ["session:d:947", -1620661791, 1, 9, 0],
  ["session:e:866", -565598032, 0, 8, 1],
  ["session:f:785", -1379898317, 3, 3, 0],
  ["session:k:380", -1270199861, 11, 9, 0],
  ["session:m:218", -1136521125, 11, 5, 2],
  ["session:p:975", -2091486740, 12, 0, 0],
];
const fixture = JSON.parse(readFileSync("fixtures/assignments.json", "utf8"));

test("negative-hash-keys-in-range", () => {
  for (const [key] of NEGATIVE) for (const n of [16, 10, 7, 3]) {
    const s = shardFor(key, n);
    assert.ok(Number.isInteger(s) && s >= 0 && s < n, `${key} % ${n} -> ${s}`);
  }
});
test("negative-hash-keys-use-non-negative-remainder", () => {
  // + 0 folds a -0 result into 0; both are the same shard index.
  for (const [key, , s16, s10, s7] of NEGATIVE) {
    assert.equal(shardFor(key, 16) + 0, s16, `${key} % 16`);
    assert.equal(shardFor(key, 10) + 0, s10, `${key} % 10`);
    assert.equal(shardFor(key, 7) + 0, s7, `${key} % 7`);
  }
});
test("hash-function-unchanged", () => {
  for (const [key, h] of NEGATIVE) assert.equal(hashKey(key), h, key);
  assert.equal(hashKey(""), 0x811c9dc5 | 0);
});
test("live-assignments-unchanged", () => {
  for (const [key, shard] of Object.entries(fixture.assignments)) assert.equal(shardFor(key, fixture.shards), shard, key);
  assert.equal(Object.keys(fixture.assignments).length, 20);
});
test("many-keys-in-range", () => {
  for (let i = 0; i < 5000; i++) {
    const key = `req:${i.toString(36)}:${(i * 2654435761) % 4093}`;
    const s = shardFor(key, 12);
    assert.ok(s >= 0 && s < 12, `${key} -> ${s}`);
    assert.equal(s + 0, ((hashKey(key) % 12) + 12) % 12);
  }
});
test("invalid-shard-count-rejected", () => {
  assert.throws(() => shardFor("x", 0), RangeError);
  assert.throws(() => shardFor("x", 2.5), RangeError);
});
