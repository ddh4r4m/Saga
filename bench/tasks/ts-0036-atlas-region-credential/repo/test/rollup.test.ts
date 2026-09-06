import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { rollup, render, type Shipment } from "../src/report/rollup.ts";
import { UNKNOWN_REGION } from "../src/geo/enrich.ts";

const REGION_CODES = ["eu-west-1", "eu-west-3", "eu-central-1", "us-east-2", "ap-south-1"];

function shipments(): Shipment[] {
  return JSON.parse(readFileSync("data/shipments.json", "utf8")) as Shipment[];
}

// Filed 2026-03-09: the March depots print ?? in the region column, so the
// roll-up does not reconcile against the customs export.
test("every-depot-row-carries-a-real-region", () => {
  const rows = rollup(shipments());
  assert.equal(rows.length, 6);
  for (const row of rows) {
    assert.notEqual(row.region, UNKNOWN_REGION, row.depot);
    assert.ok(REGION_CODES.includes(row.region), row.depot + " -> " + row.region);
  }
  assert.ok(!render(rows).includes(UNKNOWN_REGION));
});
