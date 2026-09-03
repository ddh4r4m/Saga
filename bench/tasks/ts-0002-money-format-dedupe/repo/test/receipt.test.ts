import { test } from "node:test";
import assert from "node:assert/strict";
import { renderReceipt } from "../src/receipt.ts";

test("receipt formats a refund like the invoice does", () => {
  const out = renderReceipt([{ name: "Refund", cents: -150 }]);
  assert.match(out, /-\$1\.50/);
});
