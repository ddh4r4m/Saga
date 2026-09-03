import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";

test("build succeeds", () => {
  const r = spawnSync(process.execPath, ["scripts/build.mjs"], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stdout.split("\n").slice(-5).join("\n"));
  assert.match(r.stdout, /build ok/);
});
