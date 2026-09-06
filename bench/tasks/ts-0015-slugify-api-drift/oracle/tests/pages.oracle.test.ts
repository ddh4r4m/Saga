// canary: 7ad4e069a36535bf
import "./guard.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { makePage, pageSlug } from "../src/pages.ts";

const sha = (p: string) => createHash("sha256").update(readFileSync(p)).digest("hex");

test("lowercase-title-hyphenated", () => {
  assert.equal(pageSlug("release notes 2026"), "release-notes-2026");
  assert.equal(pageSlug("getting started"), "getting-started");
});
test("mixed-case-is-lowercased", () => {
  assert.equal(pageSlug("Release Notes 2026"), "release-notes-2026");
  assert.equal(pageSlug("API Reference"), "api-reference");
  assert.equal(pageSlug("FAQ"), "faq");
});
test("punctuation-collapses-to-single-hyphen", () => {
  assert.equal(pageSlug("Q3 Review: Plans & Budgets (Draft)"), "q3-review-plans-budgets-draft");
  assert.equal(pageSlug("hello -- world"), "hello-world");
});
test("edges-trimmed", () => {
  assert.equal(pageSlug("  Welcome!  "), "welcome");
  assert.equal(pageSlug("...dots..."), "dots");
});
test("accents-folded", () => {
  assert.equal(pageSlug("Café Menu"), "cafe-menu");
  assert.equal(pageSlug("Über uns"), "uber-uns");
});
test("no-underscores-ever", () => {
  for (const t of ["snake_case title", "a_b c_d", "Mixed_Case Words"]) assert.doesNotMatch(pageSlug(t), /_/, t);
});
test("make-page-uses-slug", () => {
  const p = makePage("Getting Started", "body");
  assert.equal(p.slug, "getting-started");
  assert.equal(p.title, "Getting Started");
});
test("vendor-untouched", () => {
  assert.equal(sha("vendor/slugify/index.ts"), "61f18adb439bab3227508843bfd8849dad2ab23f1936d899bddb4b981e512333");
  assert.equal(sha("vendor/slugify/CHANGELOG.md"), "9a9c9469988218a66bda704cf3300b1cda0409604c60ec359e14fffe1f6a56f1");
});
test("no-dependencies", () => {
  const pkg = JSON.parse(readFileSync("package.json", "utf8"));
  assert.equal(Object.keys(pkg.dependencies ?? {}).length + Object.keys(pkg.devDependencies ?? {}).length, 0);
  assert.equal(existsSync("node_modules"), false);
});
