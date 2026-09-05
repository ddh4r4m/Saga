import { test } from "node:test";
import assert from "node:assert/strict";
import { lintIni } from "../../src/lint.ts";

test("lint reports duplicate keys with both lines", () => {
  assert.deepEqual(lintIni("a=1\nb=2\na=3\n"), ['W001 line 3: duplicate key "a" (first at line 1)']);
});

test("lint reports whitespace around keys", () => {
  assert.deepEqual(lintIni("name=x\n PORT = 1\n"), ['W002 line 2: whitespace around key " PORT "']);
});

test("lint reports empty values", () => {
  assert.deepEqual(lintIni("name=\n"), ['W003 line 1: empty value for "name"']);
});
