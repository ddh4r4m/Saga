import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { buildOutputs } from "../scripts/build.ts";

test("committed bundle matches the sources", () => {
  const expected = buildOutputs().bundle;
  const actual = readFileSync("build/bundle.js", "utf8");
  assert.equal(actual, expected, "build/bundle.js is out of date; run node scripts/build.ts");
});
