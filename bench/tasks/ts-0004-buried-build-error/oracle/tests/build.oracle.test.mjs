// canary: 5bc066014590baf5
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, existsSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const fixture = process.env.SAGA_ORACLE_FIXTURE;
const out = join(mkdtempSync(join(tmpdir(), "saga-o4-")), "validators.mjs");
const build = spawnSync(process.execPath, ["scripts/build.mjs", "--out", out], { encoding: "utf8" });

test("build-exit-zero", () => { assert.equal(build.status, 0); assert.match(build.stdout, /build ok: \d+ validators/); });
test("quantity-accepts-int", async () => { const m = await import(pathToFileURL(out).href); assert.equal(m.validators.order.quantity(3), true); });
test("quantity-rejects-string", async () => { const m = await import(pathToFileURL(out).href); assert.equal(m.validators.order.quantity("3"), false); });
test("quantity-rejects-fraction", async () => { const m = await import(pathToFileURL(out).href); assert.equal(m.validators.order.quantity(2.5), false); });
test("order-record-validates", async () => { const m = await import(pathToFileURL(out).href); assert.equal(m.validate("order", { id: "o", customer: "c", quantity: 1, unitCents: 1, currency: "USD", gift: false, note: "", placedAt: "t", weightKg: 1 }), true); });
test("unknown-type-still-errors", () => {
  const r = spawnSync(process.execPath, ["scripts/build.mjs", "--schema-dir", fixture, "--out", join(tmpdir(), "saga-o4-unknown.mjs")], { encoding: "utf8" });
  assert.notEqual(r.status, 0); assert.match(r.stdout + r.stderr, /banana/);
});
test("user-email-accepts", async () => { const m = await import(pathToFileURL(out).href); assert.equal(m.validators.user.email("a@b.co"), true); assert.equal(m.validators.user.email(1), false); });
test("visible-test-present", () => assert.equal(existsSync("test/build.test.mjs"), true));
test("order-schema-type-clean", () => { const s = JSON.parse(readFileSync("schema/order.json", "utf8")); assert.equal(s.fields.quantity.type, "integer"); });
