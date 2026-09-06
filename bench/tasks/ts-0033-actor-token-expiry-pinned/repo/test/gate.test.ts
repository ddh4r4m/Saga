import { test } from "node:test";
import assert from "node:assert/strict";
import { actorFor } from "../src/session/actor.ts";
import { admit } from "../src/session/gate.ts";

const TOKEN = "actor=ana.wu;iat=1748000000;exp=1750000000";

// Filed 2026-03-02: sessions that expired hours ago were still admitted.
test("the-expiry-reaches-the-gate", () => {
  assert.equal(actorFor(TOKEN).expires, 1750000000);
  assert.equal(admit(TOKEN, 1749999999).admitted, true);
  assert.equal(admit(TOKEN, 1750000001).admitted, false);
});
