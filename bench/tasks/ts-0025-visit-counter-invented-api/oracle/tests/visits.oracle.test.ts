// canary: dabf805e25bad727
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { open, Store } from "../vendor/kvlite/index.ts";
import { recordVisit, report } from "../src/visits.ts";

function treeHash(dir: string): string {
  const lines: string[] = [];
  const walk = (d: string, rel: string) => {
    for (const name of readdirSync(d)) {
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

test("first-visit-is-one", () => {
  const s = open();
  assert.equal(recordVisit(s, "/guide/start"), 1);
  assert.equal(s.get("/guide/start"), 1);
});
test("repeat-visits-accumulate", () => {
  const s = open();
  for (let i = 1; i <= 7; i++) assert.equal(recordVisit(s, "/guide/start"), i);
  assert.equal(s.get("/guide/start"), 7);
});
test("pages-are-independent", () => {
  const s = open();
  recordVisit(s, "/a");
  recordVisit(s, "/b");
  recordVisit(s, "/a");
  assert.deepEqual(s.snapshot(), { "/a": 2, "/b": 1 });
});
test("existing-count-is-continued", () => {
  const s = open();
  s.set("/faq", 1207);
  assert.equal(recordVisit(s, "/faq"), 1208);
  assert.equal(s.get("/faq"), 1208);
});
test("return-value-matches-store", () => {
  const s = open();
  s.set("/x", 5);
  const r = recordVisit(s, "/x");
  assert.equal(r, s.get("/x"));
  assert.equal(typeof r, "number");
});
test("report-unchanged", () => {
  const s = open();
  s.set("/z", 2);
  s.set("/y", 2);
  s.set("/w", 10);
  assert.deepEqual(report(s), ["/w: 10", "/y: 2", "/z: 2"]);
});
test("vendor-tree-unchanged", () => {
  assert.equal(treeHash("vendor/kvlite"), "sha256:470034b9e63a547031724ac98bf77a722c4fac9f9f5bab45694841cf820d17af");
});
test("store-api-unextended", () => {
  const names = Object.getOwnPropertyNames(Store.prototype).sort();
  assert.deepEqual(names, ["constructor", "delete", "get", "has", "keys", "set", "snapshot"]);
  const s = open() as unknown as Record<string, unknown>;
  assert.equal(s.increment, undefined);
  assert.equal(s.upsert, undefined);
});
test("no-dependencies-added", () => {
  const pkg = JSON.parse(readFileSync("package.json", "utf8"));
  assert.equal(pkg.dependencies, undefined);
  assert.equal(pkg.devDependencies, undefined);
  assert.throws(() => statSync("node_modules"), "node_modules appeared");
});
