import { test } from "node:test";
import assert from "node:assert/strict";
import { cacheRevision, regionFor } from "../src/geo/enrich.ts";

test("cached-depots-resolve", () => {
  assert.equal(regionFor("DEP-1180"), "eu-west-1");
  assert.equal(regionFor("DEP-1301"), "us-east-2");
  assert.equal(cacheRevision(), 812);
});
