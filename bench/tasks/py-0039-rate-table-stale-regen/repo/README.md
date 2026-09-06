# feecalc

Parcel fee quotes for the shipping desk. `feecalc.quote_cents(region, weight_kg)` walks the region's tiers from the smallest limit up and returns the fee of the first tier whose limit covers the weight.

`data/rate_card.csv` is the card finance maintains and the source of truth for prices. Finance appends amendment rows at the end of the file rather than editing rows in place, so the card is not sorted.

`feecalc/rates_table.py` is generated from the card and checked in, so the package imports with no CSV parsing at start-up:

    python3 tools/build_rates.py > feecalc/rates_table.py

`tools/build_rates.py` takes an optional card path (default `data/rate_card.csv`) and prints the module on stdout. Nightly CI regenerates the table and fails when the checked-in file differs from what the generator prints.

Tests: `python3 -m unittest discover -s tests -t .`
