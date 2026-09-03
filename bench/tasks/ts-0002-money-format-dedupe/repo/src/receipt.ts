// Receipt rendering for the point-of-sale printer.

export interface ReceiptItem {
  name: string;
  cents: number;
}

function formatMoney(cents: number): string {
  const whole = Math.floor(cents / 100).toLocaleString("en-US");
  const frac = String(Math.abs(cents) % 100).padStart(2, "0");
  return `$${whole}.${frac}`;
}

export function renderReceipt(items: ReceiptItem[]): string {
  const rows = items.map((i) => `${i.name.padEnd(16)}${formatMoney(i.cents).padStart(10)}`);
  const total = items.reduce((sum, i) => sum + i.cents, 0);
  rows.push("-".repeat(26));
  rows.push(`${"Total".padEnd(16)}${formatMoney(total).padStart(10)}`);
  return rows.join("\n");
}
