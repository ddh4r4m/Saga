# Contract: slugify separator collapse

<!-- canary: 13c4fae5008836ad -->
REQUEST: sha256:8311be14f714cf72e3c29c5e89b84fd1e7d72df0824f37f220c64f28c9e8d2b4
IN: src/slug.ts, test/**
OUT: package.json

- [x] G1: runs of separators collapse and edges are trimmed
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts" && node -e "import('./src/slug.ts').then(m => console.log(m.slugify('--Hello  World!--')))"
    EXPECT: /^hello-world$/m
    FROM: R2 "runs of separators collapse to a single hyphen"
    FROM: R2 "hyphens never sit at either edge"
    EVIDENCE: sha256:eae2f2124d66f36645263ab98f8c28147152db70878a62fe833329ee2f1551fe
- [x] G2: accented letters fold to ASCII
    CHECK: node -e "import('./src/slug.ts').then(m => console.log(m.slugify('Café au lait')))"
    EXPECT: /^cafe-au-lait$/m
    FROM: R3 "Accented letters should fold to their plain ASCII letter"
    EVIDENCE: sha256:741e5c332d6d6c7d9762fa002ff3fd8dfca29083aeb943e8f5c7550a0bc5c29d
- [x] G3: the signature is unchanged
    CHECK: grep -c "export function slugify(input: string): string" src/slug.ts
    EXPECT: /^1$/m
    FROM: R4 "Do not change the public signature"
    RED: none
    EVIDENCE: sha256:3476d5f76a2584e14ca0d76f0d610be0f1597cc1dc93ff83ae0f59522f4d87c4
