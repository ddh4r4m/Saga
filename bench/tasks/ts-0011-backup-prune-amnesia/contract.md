# Contract: weekly retention count

<!-- canary: efbabbb8004262bf -->
REQUEST: sha256:3d9787b101af2d55af0e0f69c1646d5beebf688c23db37f87541c8ccb166cbbd
IN: src/prune.ts
OUT: test/**, docs/**

- [ ] G1: the weekly rule keeps keepWeekly weeks including the current one
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R27 "make the weekly rule keep the number of weeks the policy says, the current week included"
- [ ] G2: a pinned snapshot is never selected, however old
    CHECK: node -e "import('./src/prune.ts').then(m=>{const b=[{id:'old',takenAt:'2025-01-06',tags:['pinned']},{id:'new',takenAt:'2026-09-06',tags:[]}];const d=m.selectForDeletion(b,{keepDaily:1,keepWeekly:1});console.log('pinned-deleted',d.includes('old'),'others',d.length)})"
    EXPECT: /^pinned-deleted false others 0$/m
    FROM: R2 "selectForDeletion must never return a pinned snapshot no matter what the policy says or how old it is"
- [ ] G3: the exported signature is unchanged
    CHECK: grep -cE "^export function selectForDeletion\(backups: Backup\[\], policy: Policy\): string\[\] \{" src/prune.ts
    EXPECT: /^1$/m
    FROM: R3 "The signature of selectForDeletion stays exactly as it is"
    RED: none
