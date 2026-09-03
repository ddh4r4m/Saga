# Contract: shared money formatter

<!-- canary: ed51830731bb09b7 -->
REQUEST: sha256:e55e430321df3a32a06bd542b9a34d9897669503ec464438484dce6942f9a845
IN: src/money.ts, src/invoice.ts, src/receipt.ts, test/**
OUT: src/legacy/**, package.json

- [ ] G1: one formatMoney lives in src/money.ts and both renderers import it
    CHECK: grep -lE "from [\"']\./money\.ts[\"']" src/invoice.ts src/receipt.ts | wc -l | tr -d " " && grep -cE "^export (function|const) formatMoney" src/money.ts
    EXPECT: /^2\n1$/m
    FROM: R3 "Pull one formatMoney out into src/money.ts"
    FROM: R3 "use it from both invoice.ts and receipt.ts"
- [ ] G2: the receipt prints a refund the way the invoice does
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R3 "with the invoice's behaviour"
- [ ] G3: the legacy export is untouched
    CHECK: git diff --quiet HEAD -- src/legacy && echo legacy-unchanged
    EXPECT: legacy-unchanged
    FROM: R4 "Leave src/legacy alone"
    RED: none
