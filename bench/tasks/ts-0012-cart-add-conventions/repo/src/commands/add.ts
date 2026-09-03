import type { Cart } from "../cart.ts";
import { CATALOGUE } from "../catalogue.ts";
import { fail } from "../errors.ts";

// Add qty of sku to the cart, merging with an existing line for the same sku.
export function addItem(cart: Cart, sku: string, qty: number): Cart {
  if (!(sku in CATALOGUE)) fail("E_UNKNOWN_SKU", `unknown sku ${sku}`);
  const lines = cart.lines.map((l) => ({ ...l }));
  const existing = lines.find((l) => l.sku === sku);
  if (existing) existing.qty += qty;
  else lines.push({ sku, qty });
  return { lines };
}
