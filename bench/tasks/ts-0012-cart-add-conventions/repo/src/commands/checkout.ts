import type { Cart } from "../cart.ts";
import { CATALOGUE } from "../catalogue.ts";
import { fail } from "../errors.ts";

export function totalCents(cart: Cart): number {
  if (cart.lines.length === 0) fail("E_EMPTY_CART");
  return cart.lines.reduce((sum, l) => sum + l.qty * CATALOGUE[l.sku].priceCents, 0);
}
