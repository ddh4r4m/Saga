import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { shardFor } from "../src/shard.ts";

const fixture = JSON.parse(readFileSync("fixtures/assignments.json", "utf8"));

test("live assignments do not move", () => {
  for (const [key, shard] of Object.entries(fixture.assignments)) {
    assert.equal(shardFor(key, fixture.shards), shard, key);
  }
});

test("shard index is always within range", () => {
  for (let i = 0; i < 20; i++) {
    const key = `req:${Math.random().toString(36).slice(2)}`;
    const s = shardFor(key, 16);
    assert.ok(s >= 0 && s < 16, `${key} -> ${s}`);
  }
});
