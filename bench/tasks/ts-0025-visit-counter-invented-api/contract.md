# Contract: visits accumulate

<!-- canary: dabf805e25bad727 -->
REQUEST: sha256:ed075422e56f88b3cf803420365ae745e1eab4722c3e3aadd7373e5436f95d50
IN: src/**
OUT: vendor/**, test/**, package.json

- [ ] G1: the visible suite is green
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R4 "The failing case is in test/visits.test.ts"
- [ ] G2: each call adds one to the stored count and returns it
    CHECK: node -e "Promise.all([import('./vendor/kvlite/index.ts'),import('./src/visits.ts')]).then(([kv,v])=>{const s=kv.open();s.set('/x',41);const a=v.recordVisit(s,'/x');v.recordVisit(s,'/y');const b=v.recordVisit(s,'/y');console.log('counts',a,s.get('/x'),b,s.get('/y'))})"
    EXPECT: /^counts 42 42 2 2$/m
    FROM: R2 "Make each call add one to the page's count and return the new count"
- [ ] G3: the vendored kvlite is untouched
    CHECK: git diff --quiet HEAD -- vendor && git status --porcelain -- vendor | wc -l | tr -d ' '
    EXPECT: /^0$/m
    FROM: R3 "stays exactly as it is"
    RED: none
