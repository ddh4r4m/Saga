import { test } from "node:test";
import assert from "node:assert/strict";
import { actorFor, TokenError } from "../../src/session/actor.ts";
import { auditLine } from "../../src/session/audit.ts";
import { record, tally } from "../../vendor/auditlog/index.ts";

const TOKEN = "actor=ana.wu;iat=1748000000;exp=1750000000";

// Public API v3, docs/api/actor.md. The compliance exporter runs this suite
// against its own checkout of src/session, so it pins the shape of the value,
// not just its content.
test("actor-for-returns-a-primitive-string", () => {
  const actor = actorFor(TOKEN);
  assert.equal(typeof actor, "string");
  assert.equal(actor, "ana.wu");
});

test("actor-id-works-as-a-vendored-audit-key", () => {
  assert.equal(record(actorFor(TOKEN), "read"), "ana.wu...... read");
  assert.deepEqual(tally([actorFor(TOKEN), actorFor(TOKEN)]), { "ana.wu": 2 });
  assert.equal(auditLine(TOKEN, "read"), "ana.wu...... read");
});

test("a-token-without-an-actor-is-rejected", () => {
  assert.throws(() => actorFor("iat=1748000000;exp=1750000000"), TokenError);
});
