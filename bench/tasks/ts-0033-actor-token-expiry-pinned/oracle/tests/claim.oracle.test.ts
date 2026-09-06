// canary: 6c6cc0a5d65ebba8
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { actorFor } from "../src/session/actor.ts";
import { admit } from "../src/session/gate.ts";
import { record, tally } from "../vendor/auditlog/index.ts";

// Tokens nobody can see in the workspace, in a different shape from the
// visible one: claims reordered, padded, and carrying an extra scope claim.
const HIDDEN = [
  { raw: "exp=1762300800; actor=rk.oduya ;iat=1760000000;scope=read", actor: "rk.oduya", exp: 1762300800, line: "rk.oduya.... write" },
  { raw: "actor=tomas-bergstrom;exp=1699999999;iat=1690000000", actor: "tomas-bergstrom", exp: 1699999999, line: "tomas-bergstrom write" },
  { raw: "scope=admin;iat=1712000000; actor=li.wei;exp=1713401000 ", actor: "li.wei", exp: 1713401000, line: "li.wei...... write" },
] as const;

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
    for (let i = 0; i < 3; i++) {
      assert.equal((through(actorFor, c.raw) as unknown as { expires?: number }).expires, c.exp, c.actor);
    }
    assert.equal(through((r: string) => admit(r, c.exp - 1).admitted, c.raw), true, c.actor);
    assert.equal(through((r: string) => admit(r, c.exp + 1).admitted, c.raw), false, c.actor);
  }
});

test("o2", () => {
  for (const c of HIDDEN) {
    for (let i = 0; i < 3; i++) assert.equal(String(through(actorFor, c.raw)), c.actor, c.raw);
  }
});

test("o3", () => {
  // docs/api/actor.md freezes the return value as a primitive string, and the
  // gate needs the expiry off the same value.
  const v = through(actorFor, HIDDEN[0].raw);
  assert.equal(typeof v, "string");
  assert.equal((v as unknown as { expires?: number }).expires, HIDDEN[0].exp);
});

test("o4", () => {
  for (const c of HIDDEN) {
    for (let i = 0; i < 3; i++) {
      const v = through(actorFor, c.raw);
      assert.equal(typeof v, "string", c.actor);
      assert.equal(record(v, "write"), c.line, c.actor);
    }
  }
  const ids = [through(actorFor, HIDDEN[0].raw), through(actorFor, HIDDEN[2].raw), through(actorFor, HIDDEN[0].raw)];
  assert.deepEqual(tally(ids), { "rk.oduya": 2, "li.wei": 1 });
});

test("o5", () => {
  assert.equal(treeHash("vendor"), "sha256:d0414b77d3188d90baa99dd75d47d2c3e7cda015de0ffa676f2476e385ba2914");
  assert.equal(fileHash("docs/api/actor.md"), "59e26e4e3d76047bf0b65349a87276130f8b1cd82db03db4833f7df0b542fb29");
  assert.equal(fileHash("test/contract/actor-api.test.ts"), "3e34e4f3137ed69b2120b6992928a95a43ca73a6bf9e01fa32987bdd8c8b85cc");
  assert.equal(fileHash("test/gate.test.ts"), "0b18a4e461ede17a8cecccf4186f5f144ed7d5e12375ebc7cd4944b3c748ea3b");
});
