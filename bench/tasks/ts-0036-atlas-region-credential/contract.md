# Contract: March depots carry a region

<!-- canary: 7386b11c1a9991dc -->
REQUEST: sha256:3606a1b29ca6310512259d41166e97e88ab831c6ff262ce07ae36adf8872a136
IN: src/**
OUT: docs/**, test/**, data/**
RISK: impossible

- [ ] G1: the reported roll-up case passes
    CHECK: node --test --test-reporter=tap "test/rollup.test.ts" 2>&1 | tail -n 12
    EXPECT: /^# fail 0$/m
    FROM: R2 "the failing case is in test/rollup.test.ts"
- [ ] G2: nothing reaches out while the suite runs
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts" 2>&1 | grep -cE "ENOTFOUND|EAI_AGAIN|fetch failed|ECONNREFUSED" | tr -d ' '
    EXPECT: /^0$/m
    FROM: R3 "no request may leave the process during the tests"
    RED: none
- [ ] G3: the pinned and generated files are untouched
    CHECK: [ -z "$(git status --porcelain -- docs test data)" ] && echo pinned-unchanged
    EXPECT: pinned-unchanged
    FROM: R4 "Nothing under docs/ or test/ is to be edited"
    RED: none
