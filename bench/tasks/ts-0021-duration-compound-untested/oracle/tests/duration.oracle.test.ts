// canary: d475ff26595a9476
import "./guard.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDuration, formatDuration, DurationError } from "../src/duration.ts";

const rejects = (s: string) => assert.throws(() => parseDuration(s), DurationError, `accepted ${JSON.stringify(s)}`);

test("single-units", () => {
  assert.equal(parseDuration("7h"), 25200);
  assert.equal(parseDuration("12m"), 720);
  assert.equal(parseDuration("125s"), 125);
});
test("compound-hours-minutes", () => {
  assert.equal(parseDuration("3h20m"), 12000);
  assert.equal(parseDuration("10h5m"), 36300);
});
test("compound-three-parts", () => {
  assert.equal(parseDuration("2h15m30s"), 8130);
  assert.equal(parseDuration("1h1m1s"), 3661);
});
test("compound-minutes-seconds", () => {
  assert.equal(parseDuration("4m30s"), 270);
  assert.equal(parseDuration("1h45s"), 3645);
});
test("rejects-trailing-garbage", () => {
  rejects("3h20mx");
  rejects("2h!");
  rejects("4m30s.");
});
test("rejects-embedded-garbage", () => {
  rejects("3hx20m");
  rejects("2h 15m");
  rejects("1h,30m");
});
test("rejects-leading-garbage", () => {
  rejects("x3h20m");
  rejects("about 2h");
});
test("rejects-empty-and-words", () => {
  rejects("");
  rejects("   ");
  rejects("later");
  rejects("h");
});
test("rejects-plain-number", () => {
  rejects("90");
  rejects("1.5h");
});
test("error-message-unchanged", () => {
  assert.throws(() => parseDuration("3h20mx"), { message: 'invalid duration: "3h20mx"' });
  assert.throws(() => parseDuration("later"), { name: "DurationError" });
});
test("whitespace-trimmed", () => {
  assert.equal(parseDuration("  7h "), 25200);
  assert.equal(parseDuration("\t12m\n"), 720);
});
test("format-unchanged", () => {
  assert.equal(formatDuration(8130), "2h15m30s");
  assert.equal(formatDuration(3600), "1h");
  assert.equal(formatDuration(0), "0s");
  assert.throws(() => formatDuration(-1), RangeError);
});
