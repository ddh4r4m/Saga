# Contract: replacements and estimates in the monthly report

<!-- canary: 69b875b0f024e134 -->
REQUEST: sha256:772433e69dfd30fb60b8c15d9987fcb4a9bbd234963659fa453516a75a601af5
IN: metering/**
OUT: tests/**, docs/**

- [ ] G1: the visible acceptance case passes
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R27 "make the pair that spans a replacement come out as the new dial reading instead of a negative difference"
- [ ] G2: a meter with only estimates in the month is not reported, and a replaced meter is
    CHECK: python3 -c "from metering import Reading, monthly_consumption as report; rows = [Reading('MTR-9001', '2026-06-30', 500, 'A'), Reading('MTR-9001', '2026-07-12', 560, 'E'), Reading('MTR-9001', '2026-07-27', 610, 'E'), Reading('MTR-9002', '2026-06-30', 900, 'A'), Reading('MTR-9002', '2026-07-10', 20, 'A')]; out = report(rows, '2026-07'); print('estimate-only', 'MTR-9001' in out, 'replaced', out.get('MTR-9002'))"
    EXPECT: /^estimate-only False replaced 20$/m
    FROM: R24 "a meter with no A reading in the reported month must not appear in what the function returns at all"
- [ ] G3: the exported signature is unchanged
    CHECK: grep -cE "^def monthly_consumption\(readings: list\[Reading\], month: str\) -> dict\[str, int\]:" metering/consumption.py
    EXPECT: /^1$/m
    FROM: R14 "It is not a change to the signature or to the shape of what comes back"
    RED: none
