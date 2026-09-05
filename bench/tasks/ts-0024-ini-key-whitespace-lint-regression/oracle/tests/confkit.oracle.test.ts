// canary: f4a79adb09a153fb
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { loadConfig } from "../src/config.ts";
import { lintIni } from "../src/lint.ts";

test("plain-keys-and-values", () => {
  const c = loadConfig("region=eu-west\nretries=4\n");
  assert.equal(c.get("region"), "eu-west");
  assert.equal(c.get("retries"), "4");
  assert.deepEqual(c.keys(), ["region", "retries"]);
});
test("comments-and-blank-lines", () => {
  const c = loadConfig("\n# a\n; b\nregion=eu\n\n");
  assert.deepEqual(c.keys(), ["region"]);
});
test("padded-key-spaces", () => {
  const c = loadConfig("  region = eu-west\nretries  =4\n");
  assert.equal(c.get("region"), "eu-west");
  assert.equal(c.has("retries"), true);
  assert.equal(c.get("retries"), "4");
});
test("padded-key-tabs", () => {
  const c = loadConfig("\tregion\t= eu-west\n \t retries=4\n");
  assert.equal(c.get("region"), "eu-west");
  assert.equal(c.get("retries"), "4");
  assert.deepEqual(c.keys(), ["region", "retries"]);
});
test("padded-duplicate-later-wins", () => {
  const c = loadConfig("region=eu\n  region = us\n");
  assert.equal(c.get("region"), "us");
  assert.deepEqual(c.keys(), ["region"]);
});
test("inner-whitespace-kept", () => {
  const c = loadConfig(" display name = Billing\n");
  assert.equal(c.get("display name"), "Billing");
});
test("lint-warns-whitespace-around-key", () => {
  assert.deepEqual(lintIni("region=eu\n  retries =4\n\tzone=a\n"), [
    'W002 line 2: whitespace around key "  retries "',
    'W002 line 3: whitespace around key "\tzone"',
  ]);
});
test("lint-duplicates-and-empty", () => {
  assert.deepEqual(lintIni("a=1\n a =2\nb=\n"), [
    'W002 line 2: whitespace around key " a "',
    'W001 line 2: duplicate key "a" (first at line 1)',
    'W003 line 3: empty value for "b"',
  ]);
});
test("lint-example-file-unchanged", () => {
  const text = readFileSync("examples/service.ini", "utf8");
  assert.deepEqual(lintIni(text), ['W002 line 4: whitespace around key "  DB_HOST "']);
  const c = loadConfig(text);
  assert.equal(c.get("db-pool"), "12");
});
test("example-file-padded-key-loads", () => {
  const c = loadConfig(readFileSync("examples/service.ini", "utf8"));
  assert.equal(c.get("DB_HOST"), "db.internal");
  assert.deepEqual(c.keys(), ["name", "PORT", "DB_HOST", "db-pool"]);
});
