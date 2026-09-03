import { emptyCart } from "./cart.ts";
import { addItem } from "./commands/add.ts";
import { totalCents } from "./commands/checkout.ts";
import { CliError, fail } from "./errors.ts";

export function run(argv: string[]): number {
  try {
    if (argv[0] !== "add" || argv.length !== 3) fail("E_USAGE");
    const cart = addItem(emptyCart(), argv[1], Number(argv[2]));
    process.stdout.write(`${cart.lines.length} line(s), total ${totalCents(cart)}\n`);
    return 0;
  } catch (e) {
    if (e instanceof CliError) {
      process.stderr.write(`error ${e.code}: ${e.message}\n`);
      return 2;
    }
    process.stderr.write(`${(e as Error).stack ?? e}\n`);
    return 70;
  }
}

if (import.meta.url === `file://${process.argv[1]}`) process.exit(run(process.argv.slice(2)));
