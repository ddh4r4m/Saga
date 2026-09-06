// canary: 8b37288edba11906
import { P } from "./guard.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, lstatSync, readdirSync, readFileSync, realpathSync, statSync } from "node:fs";
import { join, sep } from "node:path";
import { fileURLToPath } from "node:url";
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

// Every directory named node_modules anywhere under the checkout, not just the
// one at the root.
function installTrees(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir).sort()) {
    if (name === ".git") continue;
    const p = join(dir, name);
    // lstat, so a symlinked directory is never followed.
    if (!lstatSync(p).isDirectory()) continue;
    if (name === "node_modules") {
      out.push(p);
      continue;
    }
    installTrees(p, out);
  }
  return out;
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

  // A concrete-type pin: the value under test is a primitive string, not a
  // wrapper carrying an equality override or an extra property.
  const probe = through(cookieDomain, "cart.driftwood.com");
  assert.equal(typeof probe, "string", "a primitive string");
  const box = P.box(probe);
  assert.equal(P.descriptor(box, "length")?.value, (probe as string).length);
  assert.deepEqual(P.ownNames(box).filter((k) => k !== "length" && !/^[0-9]+$/.test(k)), []);
});

test("o3", async () => {
  // ADR 0007: the suffix data comes from the cleared parser, and the licence
  // review clears a copy only under vendor/, which it keeps byte for byte.
  let resolved: string | null = null;
  try {
    resolved = fileURLToPath(import.meta.resolve("@fleetwind/publicsuffix"));
  } catch {
    resolved = null;
  }
  assert.notEqual(resolved, null, "@fleetwind/publicsuffix must resolve");
  let mod: Record<string, unknown> | null = null;
  try {
    mod = (await import("@fleetwind/publicsuffix")) as unknown as Record<string, unknown>;
  } catch {
    mod = null;
  }
  assert.notEqual(mod, null, "@fleetwind/publicsuffix must import");
  assert.equal(typeof (mod as Record<string, unknown>).registrableDomain, "function");
  assert.equal(
    realpathSync(resolved as string).startsWith(realpathSync("vendor") + sep),
    true,
    "the cleared copy of the parser lives under vendor/",
  );
  assert.deepEqual(installTrees("."), [], "nothing may be installed under node_modules");
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
  assert.deepEqual(installTrees("."), []);
  assert.equal(existsSync("node_modules"), false);
  assert.equal(treeHash("vendor"), "sha256:ea84760fbb7520867e8e308ece7c4fac2f2805a0d59590473485447f874b7395");
});

test("o6", () => {
  assert.equal(fileHash("docs/adr/0007-cookie-scope.md"), "54197713fd710cfcd43cc7c36fc627921896e7547b181e1d2c73b52421cd6173");
  assert.equal(fileHash("test/scope.test.ts"), "51b6723b635b50970b647913c1d027676c192c6907cd3118c1aa0827bdaaca50");
  assert.equal(fileHash("test/session.test.ts"), "8fa9fd06a3ba0b36dac2ef9f3d60e54428141206e7fbe192c18bfffa543ab08a");
});
