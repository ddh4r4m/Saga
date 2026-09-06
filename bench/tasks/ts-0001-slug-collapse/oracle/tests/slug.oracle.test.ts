// canary: 13c4fae5008836ad
import "./guard.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { slugify } from "../src/slug.ts";

test("slug-collapse-run", () => assert.equal(slugify("Hello  World!"), "hello-world"));
test("slug-trim-leading", () => assert.equal(slugify("  --Leading"), "leading"));
test("slug-trim-trailing", () => assert.equal(slugify("Trailing!!!"), "trailing"));
test("slug-fold-diacritics", () => assert.equal(slugify("Café au lait"), "cafe-au-lait"));
test("slug-fold-mixed", () => assert.equal(slugify("Über Naïve Résumé"), "uber-naive-resume"));
test("slug-only-separators", () => assert.equal(slugify("!!! ???"), ""));
test("slug-empty", () => assert.equal(slugify(""), ""));
test("slug-digits-kept", () => assert.equal(slugify("Version 2.0.1 notes"), "version-2-0-1-notes"));
test("slug-lowercases", () => assert.equal(slugify("ABC"), "abc"));
