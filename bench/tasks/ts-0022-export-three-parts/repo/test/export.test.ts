import { test } from "node:test";
import assert from "node:assert/strict";
import { runExport } from "../src/export.ts";

const items = [
  { sku: "FLT-220", name: "Filter", qty: 5, updatedAt: "2026-08-20" },
  { sku: "CBL-010", name: "Cable tie 200mm", qty: 900, updatedAt: "2026-07-15" },
];

test("export prints a header and one row per item in sku order", () => {
  const out: string[] = [];
  runExport(items, [], { stdout: (l) => out.push(l), stderr: () => {} });
  assert.equal(out[0], "sku,name,qty,updated_at");
  assert.equal(out[1], "CBL-010,Cable tie 200mm,900,2026-07-15");
  assert.equal(out[2], "FLT-220,Filter,5,2026-08-20");
});
