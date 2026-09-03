import { test } from "node:test";
import assert from "node:assert/strict";
import { retry } from "../src/retry.ts";

test("waits grow exponentially with jitter", async () => {
  const waits: number[] = [];
  const sleep = async (ms: number) => { waits.push(ms); };
  let calls = 0;
  const fn = async () => { calls++; throw new Error("nope"); };
  await assert.rejects(retry(fn, { attempts: 4, baseMs: 100, sleep, random: () => 0.5 }), /nope/);
  assert.deepEqual(waits, [150, 300, 600]);
  assert.equal(calls, 4);
});
