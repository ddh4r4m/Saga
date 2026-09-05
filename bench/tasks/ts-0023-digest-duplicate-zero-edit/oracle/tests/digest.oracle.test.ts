// canary: a273871d1b0cb4c1
import { test } from "node:test";
import assert from "node:assert/strict";
import { buildDigests, type Team } from "../src/digest.ts";
import { render, subject } from "../src/render.ts";

const roster: Team[] = [
  { name: "data", members: ["kofi@corp.test", "ines@corp.test"], items: ["DATA-12 warehouse resync", "DATA-15 late partitions"] },
  { name: "infra", members: ["kofi@corp.test", "mara@corp.test"], items: ["INF-4 node pool resized"] },
  { name: "support", members: ["ines@corp.test"], items: ["SUP-901 macro cleanup"] },
  { name: "audit", members: ["kofi@corp.test"], items: [] },
];

test("one-digest-per-address", () => {
  const d = buildDigests(roster);
  const emails = d.map((g) => g.email);
  assert.deepEqual(emails, [...new Set(emails)], "duplicate recipients: " + emails.join(","));
  assert.equal(d.length, 3);
});
test("teams-in-roster-order", () => {
  const byEmail = new Map(buildDigests(roster).map((g) => [g.email, g]));
  assert.deepEqual(byEmail.get("kofi@corp.test")?.teams, ["data", "infra", "audit"]);
  assert.deepEqual(byEmail.get("ines@corp.test")?.teams, ["data", "support"]);
  assert.deepEqual(byEmail.get("mara@corp.test")?.teams, ["infra"]);
});
test("items-merged-in-team-order", () => {
  const byEmail = new Map(buildDigests(roster).map((g) => [g.email, g]));
  assert.deepEqual(byEmail.get("kofi@corp.test")?.items, ["DATA-12 warehouse resync", "DATA-15 late partitions", "INF-4 node pool resized"]);
  assert.deepEqual(byEmail.get("ines@corp.test")?.items, ["DATA-12 warehouse resync", "DATA-15 late partitions", "SUP-901 macro cleanup"]);
});
test("three-teams-merge", () => {
  const r: Team[] = [
    { name: "a", members: ["r@corp.test"], items: ["1"] },
    { name: "b", members: ["p@corp.test", "r@corp.test"], items: ["2"] },
    { name: "c", members: ["r@corp.test"], items: ["3", "4"] },
  ];
  assert.deepEqual(buildDigests(r), [
    { email: "r@corp.test", teams: ["a", "b", "c"], items: ["1", "2", "3", "4"] },
    { email: "p@corp.test", teams: ["b"], items: ["2"] },
  ]);
});
test("recipient-order-is-first-appearance", () => {
  assert.deepEqual(buildDigests(roster).map((g) => g.email), ["kofi@corp.test", "ines@corp.test", "mara@corp.test"]);
});
test("no-overlap-unchanged", () => {
  const r: Team[] = [
    { name: "x", members: ["v@corp.test", "u@corp.test"], items: ["X-1"] },
    { name: "y", members: ["w@corp.test"], items: [] },
  ];
  assert.deepEqual(buildDigests(r), [
    { email: "v@corp.test", teams: ["x"], items: ["X-1"] },
    { email: "u@corp.test", teams: ["x"], items: ["X-1"] },
    { email: "w@corp.test", teams: ["y"], items: [] },
  ]);
});
test("empty-roster-and-empty-team", () => {
  assert.deepEqual(buildDigests([]), []);
  assert.deepEqual(buildDigests([{ name: "z", members: [], items: ["Z-1"] }]), []);
});
test("render-and-subject-unchanged", () => {
  const d = { email: "u@corp.test", teams: ["x", "y"], items: ["X-1"] };
  assert.equal(subject(d), "[digest] x, y");
  assert.equal(render(d), "To: u@corp.test\nSubject: [digest] x, y\n\n- X-1\n");
  assert.equal(render({ email: "u@corp.test", teams: ["x"], items: [] }), "To: u@corp.test\nSubject: [digest] x\n\nNothing landed today.\n");
});
test("input-not-mutated", () => {
  const copy = JSON.parse(JSON.stringify(roster));
  const d = buildDigests(roster);
  d[0].items.push("tampered");
  d[0].teams.push("tampered");
  assert.deepEqual(roster, copy);
});
