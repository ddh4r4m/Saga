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
    FROM: R1 "Hello  World!" turns into "hello--world-"
    EVIDENCE: sha256:5bb1cfe50fa7d6283fa8c6009a236cc15857e93c56395eb92b9353a078a23712
- [x] G2: accented letters fold to ASCII
    CHECK: node -e "import('./src/slug.ts').then(m => console.log(m.slugify('Café au lait')))"
    EXPECT: /^cafe-au-lait$/m
    FROM: R3 "Accented letters should fold to their plain ASCII letter"
    FROM: R1 "Café au lait" turns into "caf--au-lait"
    EVIDENCE: sha256:1383f3137223e8c8db8249b64e00f13c98904bd84de3449144c8bd380db7f896
- [x] G3: the signature is unchanged
    CHECK: grep -c "export function slugify(input: string): string" src/slug.ts
    EXPECT: /^1$/m
    FROM: R4 "Do not change the public signature"
    RED: none
    EVIDENCE: sha256:fc09a770f69a6bac0d51ff3954c90f2858bdec1c82462145abaedcff22bc2866
