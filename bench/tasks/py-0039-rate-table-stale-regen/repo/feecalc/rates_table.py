"""Generated from data/rate_card.csv by tools/build_rates.py. Do not edit by hand."""

# (limit_kg, fee_cents) per region, tiers ascending by limit_kg.
RATES = {
    "APAC": [(1, 875), (5, 1385), (10, 1975), (30, 3580)],
    "EU-EAST": [(1, 405), (5, 650), (10, 905), (30, 1715)],
    "EU-WEST": [(1, 380), (5, 590), (10, 850), (30, 1590)],
    "LATAM": [(1, 815), (5, 1260), (10, 1820), (30, 3320)],
    "UK-MAIN": [(1, 299), (5, 462), (10, 695), (30, 1340)],
    "US-EAST": [(1, 730), (5, 1135), (10, 1655), (30, 2985)],
}
