import { test } from "node:test";
import assert from "node:assert/strict";
import { cookieDomain } from "../src/cookies/scope.ts";
import { setCookieHeader } from "../src/cookies/session.ts";

test("single-label-suffix-customers-are-unchanged", () => {
  assert.equal(cookieDomain("shop.example.com"), "example.com");
  assert.equal(cookieDomain("example.com"), "example.com");
  assert.equal(cookieDomain("localhost"), "localhost");
});

test("set-cookie-header-carries-the-scope", () => {
  const header = setCookieHeader({ sid: "s-1", host: "shop.example.com" });
  assert.equal(header, "sid=s-1; Domain=example.com; Path=/; HttpOnly; SameSite=Lax");
});
