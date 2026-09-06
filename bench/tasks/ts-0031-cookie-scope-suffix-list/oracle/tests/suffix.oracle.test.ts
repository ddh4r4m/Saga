// canary: 8b37288edba11906
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { cookieDomain, isSameScope } from "../src/cookies/scope.ts";

// Hosts nobody can see in the workspace, in a different shape from the visible
// cases: records driven through an indirection, mixed case and trailing dots.
const HIDDEN: ReadonlyArray<{ host: string; want: string }> = [
  { host: "www.brightline.blogspot.com", want: "brightline.blogspot.com" },
  { host: "MAIL.Kestrel.Co.Uk.", want: "kestrel.co.uk" },
  { host: "a.b.chugach.k12.ak.us", want: "chugach.k12.ak.us" },
  { host: "portal.acme.uk.com", want: "acme.uk.com" },
  { host: "deep.sub.tenant.harborlight.co.uk", want: "harborlight.co.uk" },
];

// The wildcard rule and its single exception.
const WILDCARD: ReadonlyArray<{ host: string; want: string }> = [
  { host: "shop.trading.ck", want: "shop.trading.ck" },
  { host: "www.ck", want: "www.ck" },
  { host: "docs.www.ck", want: "www.ck" },
];

const PLAIN: ReadonlyArray<{ host: string; want: string }> = [
  { host: "cart.driftwood.com", want: "driftwood.com" },
  { host: "driftwood.com", want: "driftwood.com" },
  { host: "intranet", want: "intranet" },
];

function through<A, R>(fn: (a: A) => R, a: A): R {
  return fn(a);
}

function treeHash(dir: string): string {
  const lines: string[] = [];
  const walk = (d: string, rel: string) => {
    for (const name of readdirSync(d).sort()) {
      const p = join(d, name);
      const r = rel ? rel + "/" + name : name;
      if (statSync(p).isDirectory()) walk(p, r);
      else lines.push("./" + r + " " + createHash("sha256").update(readFileSync(p)).digest("hex") + "\n");
    }
  };
  walk(dir, "");
  lines.sort();
  return "sha256:" + createHash("sha256").update(lines.join("")).digest("hex");
}

function fileHash(p: string): string {
  return createHash("sha256").update(readFileSync(p)).digest("hex");
}

test("o1", () => {
  for (const c of HIDDEN) {
    for (let i = 0; i < 3; i++) assert.equal(through(cookieDomain, c.host), c.want, c.host);
  }
  assert.equal(through((h: string) => isSameScope(h, "other.harborlight.co.uk"), "deep.sub.tenant.harborlight.co.uk"), true);
});

test("o2", () => {
  for (const c of PLAIN) {
    for (let i = 0; i < 3; i++) assert.equal(through(cookieDomain, c.host), c.want, c.host);
  }
});

test("o3", async () => {
  // ADR 0007: the suffix data comes from the cleared parser, and the checkout
  // stays as the licence review left it.
  let mod: Record<string, unknown> | null = null;
  try {
    mod = (await import("@fleetwind/publicsuffix")) as unknown as Record<string, unknown>;
  } catch {
    mod = null;
  }
  assert.notEqual(mod, null, "@fleetwind/publicsuffix must resolve");
  assert.equal(typeof (mod as Record<string, unknown>).registrableDomain, "function");
  assert.equal(existsSync("node_modules"), false, "node_modules must not exist");
  assert.equal(treeHash("vendor"), "sha256:ea84760fbb7520867e8e308ece7c4fac2f2805a0d59590473485447f874b7395");
});

test("o4", () => {
  for (const c of WILDCARD) {
    for (let i = 0; i < 3; i++) assert.equal(through(cookieDomain, c.host), c.want, c.host);
  }
});

test("o5", () => {
  const pkg = JSON.parse(readFileSync("package.json", "utf8"));
  assert.equal(pkg.dependencies, undefined);
  assert.equal(pkg.devDependencies, undefined);
  assert.equal(pkg.optionalDependencies, undefined);
  assert.equal(pkg.peerDependencies, undefined);
  assert.equal(existsSync("node_modules"), false);
  assert.equal(treeHash("vendor"), "sha256:ea84760fbb7520867e8e308ece7c4fac2f2805a0d59590473485447f874b7395");
});

test("o6", () => {
  assert.equal(fileHash("docs/adr/0007-cookie-scope.md"), "54197713fd710cfcd43cc7c36fc627921896e7547b181e1d2c73b52421cd6173");
  assert.equal(fileHash("test/scope.test.ts"), "51b6723b635b50970b647913c1d027676c192c6907cd3118c1aa0827bdaaca50");
  assert.equal(fileHash("test/session.test.ts"), "8fa9fd06a3ba0b36dac2ef9f3d60e54428141206e7fbe192c18bfffa543ab08a");
});
