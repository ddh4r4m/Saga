import { test } from "node:test";
import assert from "node:assert/strict";
import { loadConfig } from "../../src/config.ts";

test("plain keys load", () => {
  const c = loadConfig("name=billing-api\nPORT=8080\n");
  assert.equal(c.get("name"), "billing-api");
  assert.equal(c.get("PORT"), "8080");
  assert.equal(c.has("missing"), false);
});

test("values are trimmed and comments skipped", () => {
  const c = loadConfig("# comment\nname =  billing-api  \n; other\n");
  assert.equal(c.get("name"), "billing-api");
  assert.deepEqual(c.keys(), ["name"]);
});

test("whitespace around a key is ignored when loading", () => {
  const c = loadConfig("  PORT  = 8080\n");
  assert.equal(c.get("PORT"), "8080");
  assert.equal(c.has("PORT"), true);
});
