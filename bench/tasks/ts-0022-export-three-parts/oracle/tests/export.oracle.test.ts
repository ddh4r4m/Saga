// canary: 9d0a1caa380d8582
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { toCsvRow } from "../src/csv.ts";
import { runExport, UsageError } from "../src/export.ts";
import type { Item } from "../src/items.ts";

const catalogue: Item[] = [
  { sku: "VLV-300", name: "Valve, 3/4\", brass", qty: 7, updatedAt: "2026-06-30" },
  { sku: "HSE-020", name: "Hose 2m", qty: 30, updatedAt: "2026-07-01" },
  { sku: "SEL-001", name: "Seal kit \"deluxe\"", qty: 2, updatedAt: "2026-07-01" },
  { sku: "CLP-050", name: "Clip, small", qty: 500, updatedAt: "2026-07-02" },
  { sku: "ADP-009", name: "Adapter", qty: 11, updatedAt: "2026-05-14" },
];

function run(items: Item[], argv: string[]) {
  const out: string[] = [];
  const err: string[] = [];
  runExport(items, argv, { stdout: (l) => out.push(l), stderr: (l) => err.push(l) });
  return { out, err };
}
const dataRows = (lines: string[]) => lines.slice(1).filter((l) => !/^exported /.test(l));

test("csv-plain-unchanged", () => {
  assert.equal(toCsvRow(["HSE-020", "Hose 2m", "30", "2026-07-01"]), "HSE-020,Hose 2m,30,2026-07-01");
});
test("csv-comma-field-quoted", () => {
  assert.equal(toCsvRow(["CLP-050", "Clip, small", "500", "2026-07-02"]), 'CLP-050,"Clip, small",500,2026-07-02');
});
test("csv-quote-field-doubled", () => {
  assert.equal(toCsvRow(["SEL-001", 'Seal kit "deluxe"', "2", "2026-07-01"]), 'SEL-001,"Seal kit ""deluxe""",2,2026-07-01');
});
test("csv-comma-and-quote-together", () => {
  assert.equal(toCsvRow(["VLV-300", 'Valve, 3/4" brass', "7", "2026-06-30"]), 'VLV-300,"Valve, 3/4"" brass",7,2026-06-30');
});
test("header-and-sku-order", () => {
  const { out } = run(catalogue, []);
  assert.equal(out[0], "sku,name,qty,updated_at");
  assert.deepEqual(dataRows(out).map((l) => l.slice(0, 7)), ["ADP-009", "CLP-050", "HSE-020", "SEL-001", "VLV-300"]);
});
test("since-absent-keeps-all", () => {
  assert.equal(dataRows(run(catalogue, []).out).length, 5);
});
test("since-inclusive-on-the-date", () => {
  const rows = dataRows(run(catalogue, ["--since", "2026-07-01"]).out).map((l) => l.slice(0, 7));
  assert.deepEqual(rows, ["CLP-050", "HSE-020", "SEL-001"]);
});
test("since-drops-day-before", () => {
  const rows = dataRows(run(catalogue, ["--since", "2026-07-02"]).out).map((l) => l.slice(0, 7));
  assert.deepEqual(rows, ["CLP-050"]);
  assert.deepEqual(dataRows(run(catalogue, ["--since", "2026-07-03"]).out), []);
});
test("since-with-format-in-either-order", () => {
  const a = dataRows(run(catalogue, ["--format", "csv", "--since", "2026-06-30"]).out).length;
  const b = dataRows(run(catalogue, ["--since", "2026-06-30", "--format", "csv"]).out).length;
  assert.equal(a, 4);
  assert.equal(b, 4);
});
test("summary-on-stderr-not-stdout", () => {
  const { out, err } = run(catalogue, []);
  assert.ok(!out.some((l) => /exported/.test(l)), "summary still on stdout");
  assert.ok(err.some((l) => /^exported 5 items$/.test(l)), "no summary on stderr: " + JSON.stringify(err));
});
test("summary-counts-filtered-items", () => {
  const { out, err } = run(catalogue, ["--since", "2026-07-01"]);
  assert.ok(!out.some((l) => /exported/.test(l)));
  assert.ok(err.some((l) => /^exported 3 items$/.test(l)), JSON.stringify(err));
});
test("cli-stdout-is-clean-csv", () => {
  const r = spawnSync(process.execPath, ["src/cli.ts", "export", "--since", "2026-08-20"], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr);
  const lines = r.stdout.trim().split("\n");
  assert.equal(lines[0], "sku,name,qty,updated_at");
  assert.equal(lines.length, 4, r.stdout);
  assert.ok(lines.every((l) => !/exported/.test(l)), r.stdout);
  assert.match(r.stderr, /exported 3 items/);
});
test("cli-unknown-option-exits-2", () => {
  const r = spawnSync(process.execPath, ["src/cli.ts", "export", "--sinse", "2026-08-20"], { encoding: "utf8" });
  assert.equal(r.status, 2);
  assert.throws(() => run(catalogue, ["--bogus"]), UsageError);
});
