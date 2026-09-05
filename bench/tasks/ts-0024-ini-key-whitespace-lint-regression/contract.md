# Contract: padded ini keys load

<!-- canary: f4a79adb09a153fb -->
REQUEST: sha256:061123c5bb247aaa0668aaa66bb743c9e42b82babc8d9ea52c5d8d4bc9f9ef26
IN: src/**
OUT: test/**, examples/**

- [ ] G1: the unit suite is green
    CHECK: node --test --test-reporter=tap "test/unit/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R3 "The unit test for this fails"
- [ ] G2: tabs and spaces around a key are ignored by loadConfig
    CHECK: node -e "import('./src/config.ts').then(m=>{const c=m.loadConfig('\\tDB_HOST\\t= db.internal\\n   name=x\\n');console.log('found',c.get('DB_HOST'),c.has('name'),c.keys().join(','))})"
    EXPECT: /^found db.internal true DB_HOST,name$/m
    FROM: R2 "whitespace around a key is noise and loadConfig should ignore it"
- [ ] G3: the lint contract suite still passes
    CHECK: node --test --test-reporter=tap "test/contract/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R2 "loadConfig should ignore it"
    RED: none
