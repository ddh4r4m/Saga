import { test } from "node:test";
import assert from "node:assert/strict";
import { loadConfig } from "../src/config.ts";

test("loads a basic env file", () => {
  const cfg = loadConfig("PORT=9090\nHOST=db.internal\nDEBUG=true\n");
  assert.equal(cfg.port, 9090);
  assert.equal(cfg.host, "db.internal");
  assert.equal(cfg.debug, true);
});
