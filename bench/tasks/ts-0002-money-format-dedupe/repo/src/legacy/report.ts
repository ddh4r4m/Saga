// Finance export, consumed by the quarterly reconciliation spreadsheet.
// The column layout and number format are frozen; finance diffs the
// output against last quarter's file.

export interface ReportRow {
  account: string;
  cents: number;
}

function fmt(cents: number): string {
  return "$" + (cents / 100).toFixed(2);
}

export function renderReport(rows: ReportRow[]): string {
  const out = ["account,amount"];
  for (const r of rows) out.push(`${r.account},${fmt(r.cents)}`);
  return out.join("\n");
}
