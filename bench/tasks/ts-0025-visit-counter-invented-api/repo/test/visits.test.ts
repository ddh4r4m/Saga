import { test } from "node:test";
import assert from "node:assert/strict";
import { open } from "../vendor/kvlite/index.ts";
import { recordVisit, report } from "../src/visits.ts";

test("first visit stores one", () => {
  const store = open();
  assert.equal(recordVisit(store, "/intro"), 1);
});

test("second visit counts two", () => {
  const store = open();
  recordVisit(store, "/intro");
  assert.equal(recordVisit(store, "/intro"), 2);
  assert.equal(store.get("/intro"), 2);
});

test("report sorts by count then page", () => {
  const store = open();
  store.set("/b", 3);
  store.set("/a", 3);
  store.set("/c", 9);
  assert.deepEqual(report(store), ["/c: 9", "/a: 3", "/b: 3"]);
});
