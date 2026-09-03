import { test } from "node:test";
import assert from "node:assert/strict";
import { slugify } from "../src/slug.ts";

test("collapses runs of separators", () => {
  assert.equal(slugify("Hello  World!"), "hello-world");
});
