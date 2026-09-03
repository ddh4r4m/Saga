import { priceCents, discountCents } from "./core.js";
import { formatMoney } from "./format.js";

export function render(base, qty, percent) {
  const gross = priceCents(base, qty);
  const net = gross - discountCents(gross, percent);
  return `<span class="pw">${formatMoney(net)}</span>`;
}
