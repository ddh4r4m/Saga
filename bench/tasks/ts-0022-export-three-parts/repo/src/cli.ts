import { readFileSync } from "node:fs";
import { runExport, UsageError } from "./export.ts";
import type { Item } from "./items.ts";

const [command, ...rest] = process.argv.slice(2);
const io = {
  stdout: (line: string) => process.stdout.write(line + "\n"),
  stderr: (line: string) => process.stderr.write(line + "\n"),
};

try {
  if (command === "export") {
    const items = JSON.parse(readFileSync("data/items.json", "utf8")) as Item[];
    runExport(items, rest, io);
  } else {
    throw new UsageError(`unknown command: ${command ?? "(none)"}`);
  }
} catch (err) {
  if (err instanceof UsageError) {
    process.stderr.write(`stockctl: ${err.message}\n`);
    process.exit(2);
  }
  throw err;
}
