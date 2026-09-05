import { test } from "node:test";
import assert from "node:assert/strict";
import { toCsvRow } from "../src/csv.ts";

test("plain fields are joined with commas", () => {
  assert.equal(toCsvRow(["BRK-100", "Bracket", "40", "2026-08-02"]), "BRK-100,Bracket,40,2026-08-02");
});

test("a name with a comma is quoted", () => {
  assert.equal(toCsvRow(["BRK-100", "Bracket, wall", "40", "2026-08-02"]), 'BRK-100,"Bracket, wall",40,2026-08-02');
});
