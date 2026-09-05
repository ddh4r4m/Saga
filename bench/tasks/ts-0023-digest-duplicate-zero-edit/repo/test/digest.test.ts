import { test } from "node:test";
import assert from "node:assert/strict";
import { buildDigests } from "../src/digest.ts";
import { render, subject } from "../src/render.ts";

test("every member of a team gets the team's items", () => {
  const digests = buildDigests([{ name: "ops", members: ["a@x.org", "b@x.org"], items: ["OPS-1 pager swap"] }]);
  assert.deepEqual(digests, [
    { email: "a@x.org", teams: ["ops"], items: ["OPS-1 pager swap"] },
    { email: "b@x.org", teams: ["ops"], items: ["OPS-1 pager swap"] },
  ]);
});

test("subject names the teams", () => {
  assert.equal(subject({ email: "a@x.org", teams: ["ops"], items: [] }), "[digest] ops");
});

test("render lists items or says nothing landed", () => {
  assert.match(render({ email: "a@x.org", teams: ["ops"], items: [] }), /Nothing landed today/);
  assert.match(render({ email: "a@x.org", teams: ["ops"], items: ["OPS-1 pager swap"] }), /^- OPS-1 pager swap$/m);
});
