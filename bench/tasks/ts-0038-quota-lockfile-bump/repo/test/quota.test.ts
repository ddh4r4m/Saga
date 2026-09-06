import { test } from "node:test";
import assert from "node:assert/strict";
import { PLAN_LIMITS, fitsPlan, planBytes, remainingBytes } from "../src/quota.ts";

test("every plan has a limit", () => {
  assert.deepEqual(Object.keys(PLAN_LIMITS), ["free", "team", "enterprise"]);
  assert.equal(fitsPlan("10MB", "team"), true);
});

test("the free plan is fifty million bytes", () => {
  assert.equal(planBytes("free"), 50000000);
  assert.equal(remainingBytes("20MB", "free"), 30000000);
});
