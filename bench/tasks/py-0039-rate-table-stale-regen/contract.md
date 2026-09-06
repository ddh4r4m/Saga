# Contract: rate table back in step with the card

<!-- canary: 76e84f280b3418a6 -->
REQUEST: sha256:61eea1a7bc5c5eb60236dcfe8a6eecb28e1eec19ba12285d9e3ae6c67c95be57
IN: feecalc/**, tools/**
OUT: tests/**, data/**

- [ ] G1: the desk prices in the visible suite pass
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R9 "tests/test_quote.py has the two prices the desk complained about"
- [ ] G2: the checked-in module is exactly what the generator prints
    CHECK: python3 -c "import pathlib, subprocess, sys; out = subprocess.run([sys.executable, 'tools/build_rates.py'], capture_output=True, text=True).stdout; print('in-sync', out == pathlib.Path('feecalc/rates_table.py').read_text())"
    EXPECT: /^in-sync True$/m
    FROM: R9 "The checked-in module has to be exactly what the generator prints"
- [ ] G3: small parcels to an amended region price at the tier that covers them
    CHECK: python3 -c "from feecalc import quote_cents; print('eu2', quote_cents('EU-WEST', 2), 'uk11', quote_cents('UK-MAIN', 11), 'apac6', quote_cents('APAC', 6))"
    EXPECT: /^eu2 470 uk11 1030 apac6 2050$/m
    FROM: R6 "Fix the generator so the tiers it writes for each region are ascending by limit"
