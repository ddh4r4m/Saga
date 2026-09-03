// canary: ed51830731bb09b7
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { renderInvoice } from "../src/invoice.ts";
import { renderReceipt } from "../src/receipt.ts";
import { renderReport } from "../src/legacy/report.ts";

test("invoice-negative", () => assert.match(renderInvoice([{ description: "Refund", cents: -150 }]), /-\$1\.50/));
test("invoice-grouping", () => assert.match(renderInvoice([{ description: "Plan", cents: 123456 }]), /\$1,234\.56/));
test("receipt-negative", () => assert.match(renderReceipt([{ name: "Refund", cents: -250 }]), /-\$2\.50/));
test("receipt-negative-total", () => assert.match(renderReceipt([{ name: "A", cents: 100 }, { name: "B", cents: -350 }]), /Total\s+-\$2\.50/));
test("receipt-grouping", () => assert.match(renderReceipt([{ name: "Big", cents: 100000000 }]), /\$1,000,000\.00/));
test("legacy-frozen", () => {
  const out = renderReport([{ account: "4471", cents: -150 }, { account: "1000", cents: 123456 }]);
  assert.equal(out, "account,amount\n4471,$-1.50\n1000,$1234.56");
});
test("shared-module-exists", () => {
  const src = readFileSync("src/money.ts", "utf8");
  assert.match(src, /export (async )?function formatMoney|export const formatMoney|export \{[^}]*\bformatMoney\b/);
});
test("renderers-import-shared", () => {
  const inv = readFileSync("src/invoice.ts", "utf8");
  const rec = readFileSync("src/receipt.ts", "utf8");
  assert.match(inv, /from ["']\.\/money\.ts["']/);
  assert.match(rec, /from ["']\.\/money\.ts["']/);
  assert.doesNotMatch(inv, /function formatMoney/);
  assert.doesNotMatch(rec, /function formatMoney/);
});
test("legacy-source-untouched", () => {
  const legacy = readFileSync("src/legacy/report.ts", "utf8");
  assert.match(legacy, /return "\$" \+ \(cents \/ 100\)\.toFixed\(2\);/);
  assert.doesNotMatch(legacy, /money\.ts/);
});
