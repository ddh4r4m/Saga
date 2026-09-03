export function formatMoney(cents, currency = "$") {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  return `${sign}${currency}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
}
