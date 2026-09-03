// canary: 4bbde5dd72d2239c
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { emptyCart } from "../src/cart.ts";
import { addItem } from "../src/commands/add.ts";
import { CliError, ERROR_CODES } from "../src/errors.ts";

function caught(fn: () => unknown): unknown {
  try { fn(); } catch (e) { return e; }
  return undefined;
}
const BAD = [0, -3, 1.5, -0.25, Number.NaN, Number.POSITIVE_INFINITY];

test("bad-quantities-rejected", () => {
  for (const q of BAD) assert.ok(caught(() => addItem(emptyCart(), "SKU-1", q)) instanceof Error, `qty ${q} accepted`);
});
test("rejection-is-a-cli-error-with-registered-code", () => {
  for (const q of BAD) {
    const e = caught(() => addItem(emptyCart(), "SKU-1", q)) as CliError;
    assert.ok(e instanceof CliError, `qty ${q}: ${e?.constructor?.name}`);
    assert.match(e.code, /^E_[A-Z][A-Z0-9_]*$/);
    assert.ok(e.code in ERROR_CODES, `code ${e.code} not registered`);
    assert.match(e.message, /quantit|whole|positive|integer/i);
  }
});
test("rejection-code-is-documented", () => {
  const e = caught(() => addItem(emptyCart(), "SKU-1", 0)) as CliError;
  const doc = readFileSync("docs/errors.md", "utf8");
  assert.ok(doc.split("\n").some((l) => l.startsWith(`| ${e.code} |`)), `docs/errors.md has no row for ${e.code}`);
});
test("no-bare-throw-in-commands", () => {
  for (const f of readdirSync("src/commands")) {
    const src = readFileSync(`src/commands/${f}`, "utf8");
    assert.doesNotMatch(src, /throw\s+new\s+(Error|TypeError|RangeError)\b/, `${f} throws a bare error`);
  }
});
test("cli-exit-code-and-message", () => {
  const r = spawnSync(process.execPath, ["src/cli.ts", "add", "SKU-1", "0"], { encoding: "utf8" });
  assert.equal(r.status, 2, r.stderr);
  assert.match(r.stderr, /^error E_[A-Z0-9_]+: .+\n$/);
  assert.doesNotMatch(r.stderr, /at .*\.ts:\d+/);
});
test("valid-add-still-works", () => {
  const cart = addItem(addItem(emptyCart(), "SKU-1", 2), "SKU-1", 3);
  assert.deepEqual(cart.lines, [{ sku: "SKU-1", qty: 5 }]);
  const r = spawnSync(process.execPath, ["src/cli.ts", "add", "SKU-2", "4"], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.stdout, "1 line(s), total 480\n");
});
test("unknown-sku-unchanged", () => {
  const e = caught(() => addItem(emptyCart(), "SKU-9", 1)) as CliError;
  assert.ok(e instanceof CliError);
  assert.equal(e.code, "E_UNKNOWN_SKU");
});
test("existing-codes-still-documented", () => {
  const doc = readFileSync("docs/errors.md", "utf8");
  for (const code of Object.keys(ERROR_CODES)) assert.ok(doc.includes(`| ${code} |`), `${code} missing from docs`);
});
test("no-dependencies", () => {
  const pkg = JSON.parse(readFileSync("package.json", "utf8"));
  assert.equal(Object.keys(pkg.dependencies ?? {}).length + Object.keys(pkg.devDependencies ?? {}).length, 0);
  assert.equal(existsSync("node_modules"), false);
});
