import { test } from "node:test";
import assert from "node:assert/strict";
import { makePage, pageSlug } from "../src/pages.ts";

test("release notes page slug", () => {
  assert.equal(pageSlug("release notes 2026"), "release-notes-2026");
});

test("makePage carries the slug", () => {
  assert.equal(makePage("getting started", "...").slug, "getting-started");
});
