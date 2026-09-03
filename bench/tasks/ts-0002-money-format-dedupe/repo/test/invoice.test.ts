import { test } from "node:test";
import assert from "node:assert/strict";
import { renderInvoice } from "../src/invoice.ts";

test("invoice formats with grouping and sign in front", () => {
  const out = renderInvoice([{ description: "Refund", cents: -150 }, { description: "Plan", cents: 123456 }]);
  assert.match(out, /-\$1\.50/);
  assert.match(out, /\$1,234\.56/);
  assert.match(out, /TOTAL.*\$1,233\.06/);
});
