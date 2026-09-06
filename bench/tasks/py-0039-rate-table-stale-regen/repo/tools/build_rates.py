#!/usr/bin/env python3
"""Build the rate table module from the rate card.

    python3 tools/build_rates.py [card.csv] > feecalc/rates_table.py

Reads the card (default data/rate_card.csv) and prints the module on
stdout. The checked-in table must be exactly what this prints.
"""

import csv
import sys

DEFAULT_CARD = "data/rate_card.csv"

PREAMBLE = (
    '"""Generated from data/rate_card.csv by tools/build_rates.py. Do not edit by hand."""\n'
    "\n"
    "# (limit_kg, fee_cents) per region, tiers ascending by limit_kg.\n"
    "RATES = {\n"
)


def read_card(path):
    with open(path, newline="", encoding="utf-8") as fh:
        return list(csv.DictReader(fh))


def build(rows):
    """Group the card rows into a list of (limit_kg, fee_cents) per region."""
    table = {}
    for row in rows:
        region = row["region"].strip()
        table.setdefault(region, []).append((int(row["tier_kg"]), int(row["fee_cents"])))
    return table


def render(table):
    out = [PREAMBLE]
    for region in sorted(table):
        tiers = ", ".join(f"({limit}, {fee})" for limit, fee in table[region])
        out.append(f'    "{region}": [{tiers}],\n')
    out.append("}\n")
    return "".join(out)


def main(argv):
    card = argv[1] if len(argv) > 1 else DEFAULT_CARD
    sys.stdout.write(render(build(read_card(card))))


if __name__ == "__main__":
    main(sys.argv)
