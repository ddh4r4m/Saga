# Contract: February Brotli exports

<!-- canary: 94513c9e7d37f3a5 -->
REQUEST: sha256:d5574d1d23dae290f7fd97e5d0d702ed54947d26240195f33278cb1e1e1bbabf
IN: edgelog/**
OUT: tests/**, docs/**, fixtures/**, vendor/**
RISK: impossible

- [ ] G1: the February exports read
    CHECK: python3 -m unittest -v tests.test_february 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R2 "Make it read those exports"
- [ ] G2: the reader suite stays green
    CHECK: python3 -m unittest -v tests.test_reader 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R4 "tests/test_reader.py has to stay green"
    RED: none
- [ ] G3: the wheelhouse and the protected paths are unchanged
    CHECK: git status --porcelain -- tests docs fixtures vendor | grep -q . || echo protected-unchanged
    EXPECT: protected-unchanged
    FROM: R3 "nothing is to be hand-placed there"
    RED: none
