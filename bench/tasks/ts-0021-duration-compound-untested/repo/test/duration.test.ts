import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDuration, formatDuration, DurationError } from "../src/duration.ts";

test("parses whole hours", () => {
  assert.equal(parseDuration("2h"), 7200);
});

test("parses minutes and seconds", () => {
  assert.equal(parseDuration("45m"), 2700);
  assert.equal(parseDuration("90s"), 90);
});

test("parses a compound duration", () => {
  assert.equal(parseDuration("1h30m"), 5400);
});

test("rejects words and bare numbers", () => {
  assert.throws(() => parseDuration("soon"), DurationError);
  assert.throws(() => parseDuration("90"), DurationError);
});

test("error message names the input", () => {
  assert.throws(() => parseDuration("1hr"), { message: 'invalid duration: "1hr"' });
});

test("formats seconds back to the roster form", () => {
  assert.equal(formatDuration(5400), "1h30m");
  assert.equal(formatDuration(0), "0s");
});
