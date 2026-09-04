# Contract: exponential backoff with jitter

<!-- canary: 52aeff4aebe362eb -->
REQUEST: sha256:11f3c8fdc74f86458dfb8890fc9d4de540d5d86d1ce7cbe152f6587ddc0109d7
IN: src/retry.ts
OUT: test/**, package.json, README.md

- [x] G1: the visible retry test passes unmodified
    CHECK: git diff --quiet HEAD -- test && node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R3 "Make retry honour the documented backoff"
    FROM: R4 "Keep the test file as it is"
    EVIDENCE: sha256:ead7d79f022f4cc778d47d67a1c20af2c79c73213cbbfd3ebc297127ffdc1379
- [x] G2: with r = 0 the waits are exactly baseMs * 2^i
    CHECK: node -e "import('./src/retry.ts').then(async m=>{const w=[];await m.retry(async()=>{throw new Error('x')},{attempts:4,baseMs:100,sleep:async ms=>{w.push(ms)},random:()=>0}).catch(()=>{});console.log(w.join(','))})"
    EXPECT: /^100,200,400$/m
    FROM: R2 "The README says waits are baseMs * 2^i * (1 + r)"
    EVIDENCE: sha256:4ef8062579eb8a061d8f8ee76766010b19f04cb0cc345b14c33124004648e2d6
