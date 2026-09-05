import { HEADER, toCsvRow } from "./csv.ts";
import { sortBySku, type Item } from "./items.ts";

export class UsageError extends Error {}

export interface ExportOptions {
  format: "csv";
}

export interface Io {
  stdout(line: string): void;
  stderr(line: string): void;
}

export function parseExportArgs(argv: string[]): ExportOptions {
  const opts: ExportOptions = { format: "csv" };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--format") {
      const v = argv[++i];
      if (v !== "csv") throw new UsageError(`unsupported format: ${v}`);
      opts.format = "csv";
    } else {
      throw new UsageError(`unknown option: ${arg}`);
    }
  }
  return opts;
}

export function runExport(items: Item[], argv: string[], io: Io): void {
  const opts = parseExportArgs(argv);
  const rows = sortBySku(items);
  io.stdout(toCsvRow(HEADER));
  for (const it of rows) io.stdout(toCsvRow([it.sku, it.name, String(it.qty), it.updatedAt]));
  io.stdout(`exported ${rows.length} items`);
  void opts;
}
