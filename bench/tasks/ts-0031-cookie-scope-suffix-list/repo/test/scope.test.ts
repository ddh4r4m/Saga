import { test } from "node:test";
import assert from "node:assert/strict";
import { cookieDomain, isSameScope } from "../src/cookies/scope.ts";

// Reported by the platform team on 2026-03-04: two customers on .co.uk saw
// each other's session cookies.
test("customer-cookies-do-not-span-a-multi-label-suffix", () => {
  assert.equal(cookieDomain("docs.northwind.co.uk"), "northwind.co.uk");
  assert.equal(cookieDomain("pages.northwind.github.io"), "northwind.github.io");
  assert.equal(isSameScope("docs.northwind.co.uk", "docs.seabird.co.uk"), false);
});
