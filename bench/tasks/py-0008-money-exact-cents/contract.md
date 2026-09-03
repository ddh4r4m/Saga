# Contract: exact cents in Money

<!-- canary: 938d4961ccd5198a -->
REQUEST: sha256:b65a2a3de8474eba090007fd1bf665a1a42eadbc98641f8bd81cf2749d496f6a
IN: pricing/**, tests/**
OUT: pyproject.toml

- [ ] G1: totals are exact and the CSV shows two decimals
    CHECK: python3 -m unittest discover -v -s tests -t .
    EXPECT: /^OK$/m
    FROM: R1 "totals drift"
    FROM: R2 "Store cents as an integer instead of a float amount"
- [ ] G2: cents are held as an integer
    CHECK: python3 -c "from pricing.money import Money; m=Money(0.10)*3; print(type(m.cents).__name__, m.cents)"
    EXPECT: /^int 30$/m
    FROM: R2 "Store cents as an integer"
- [ ] G3: display, tax and CSV layout are unchanged
    CHECK: python3 -c "from pricing.money import Money; from pricing.format import format_money; from pricing.tax import apply_tax; from pricing.report import to_csv; print(format_money(Money(1234.5)), format_money(Money(-0.05)), apply_tax(Money(20.00), 0.0725) == Money(21.45), to_csv([('a', Money(2), 2)]).splitlines()[0])"
    EXPECT: $1,234.50 -$0.05 True name,qty,total
    FROM: R3 "the display format, the tax rounding and the CSV layout stay as they are"
