export function priceCents(base, qty) {
  return base * qty;
}

export function discountCents(cents, percent) {
  return Math.round((cents * percent) / 100);
}
