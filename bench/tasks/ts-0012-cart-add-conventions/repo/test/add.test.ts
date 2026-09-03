import { test } from "node:test";
import assert from "node:assert/strict";
import { emptyCart } from "../src/cart.ts";
import { addItem } from "../src/commands/add.ts";

test("adds a line", () => {
  const cart = addItem(emptyCart(), "SKU-1", 2);
  assert.deepEqual(cart.lines, [{ sku: "SKU-1", qty: 2 }]);
});

test("rejects a quantity of zero", () => {
  assert.throws(() => addItem(emptyCart(), "SKU-1", 0), /quantity/);
});

test("rejects a negative quantity", () => {
  assert.throws(() => addItem(emptyCart(), "SKU-1", -3), /quantity/);
});
