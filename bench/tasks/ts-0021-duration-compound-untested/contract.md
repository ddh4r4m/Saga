# Contract: compound durations parse again

<!-- canary: d475ff26595a9476 -->
REQUEST: sha256:1914ffd4ed58b62e0aaa5b717fce733f9b245542b3161e9096ba6e0a3ba03365
IN: src/**
OUT: test/**

- [ ] G1: the visible suite is green, the compound case included
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R3 "The suite in test/ has the failing case"
- [ ] G2: a compound duration is the sum of its parts
    CHECK: node -e "import('./src/duration.ts').then(m=>console.log('sum',m.parseDuration('2h5m30s'),m.parseDuration('12m1s'),m.parseDuration('3h9s')))"
    EXPECT: /^sum 7530 721 10809$/m
    FROM: R2 "Make compound durations parse again, summing the parts"
- [ ] G3: garbage around or between the parts is still rejected
    CHECK: node -e "import('./src/duration.ts').then(m=>{let n=0;for(const s of ['4h2mx','1hx','x30m','','later','2h 5m']){try{m.parseDuration(s)}catch(e){n++}}console.log('rejected',n)})"
    EXPECT: /^rejected 6$/m
    FROM: R2 "without loosening the check that made 1.3.0 reject garbage"
    RED: none
