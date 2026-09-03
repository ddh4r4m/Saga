"""Nightly reconcile: post every entry, then compare balances to the control file.

Usage: python3 -m ledger.reconcile <fixture-dir>
"""
import sys
from pathlib import Path
from .accounts import account
from .journal import Journal
from .loader import load_controls, load_entries
from .rates import Rates
from .report import balance_line, posting_line


def run(fixture_dir: Path, out=sys.stdout) -> int:
    entries = load_entries(fixture_dir / "transactions.csv")
    controls = load_controls(fixture_dir / "controls.csv")
    journal = Journal(Rates.load(fixture_dir / "rates.csv"))
    for e in entries:
        journal.post(e)
        print(posting_line(e.id, e.date, e.debit, e.credit, e.amount_cents, e.currency), file=out)
    mismatches = 0
    for code in sorted(controls):
        acct = account(code)
        got = journal.balances.get(code, 0)
        print(balance_line(code, acct.name, got, acct.currency), file=out)
        expected = controls[code]
        if got != expected:
            mismatches += 1
            print(f"MISMATCH account {code}: expected {expected} got {got} (diff {got - expected})", file=out)
    print(f"reconcile: {len(entries)} entries posted, {mismatches} mismatch{'es' if mismatches != 1 else ''}", file=out)
    return 1 if mismatches else 0


if __name__ == "__main__":
    sys.exit(run(Path(sys.argv[1])))
