// Invoice rendering for the customer portal.

export interface Line {
  description: string;
  cents: number;
}

function formatMoney(cents: number): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  const whole = Math.floor(abs / 100).toLocaleString("en-US");
  const frac = String(abs % 100).padStart(2, "0");
  return `${sign}$${whole}.${frac}`;
}

export function renderInvoice(lines: Line[]): string {
  const rows = lines.map((l) => `${l.description.padEnd(20)} ${formatMoney(l.cents).padStart(12)}`);
  const total = lines.reduce((sum, l) => sum + l.cents, 0);
  rows.push(`${"TOTAL".padEnd(20)} ${formatMoney(total).padStart(12)}`);
  return rows.join("\n");
}
