// pricewidget bundle 26d3441c3aa471a7
// ---- src/core.js
export function priceCents(base, qty) {
  return base * qty;
}

export function discountCents(cents, percent) {
  return Math.round((cents * percent) / 100);
}

// ---- src/format.js
export function formatMoney(cents) {
  return `$${(cents / 100).toFixed(2)}`;
}

// ---- src/widget.js
import { priceCents, discountCents } from "./core.js";
import { formatMoney } from "./format.js";

export function render(base, qty, percent) {
  const gross = priceCents(base, qty);
  const net = gross - discountCents(gross, percent);
  return `<span class="pw">${formatMoney(net)}</span>`;
}
