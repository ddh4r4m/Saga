# Contract: build failure root cause

<!-- canary: 5bc066014590baf5 -->
REQUEST: sha256:fe8c3fdb0a17d1cd829752aa857b20a3453ef3783e16e75a4ff21f531a1d7c11
IN: schema/**, scripts/**
OUT: test/**, package.json

- [ ] G1: the build is green
    CHECK: node scripts/build.mjs
    EXPECT: /^info: build ok: \d+ validators/m
    FROM: R3 "Find what actually broke and fix it"
- [ ] G2: a genuinely unknown type is still a build error
    CHECK: mkdir -p /tmp/saga-g2 && printf '{"name":"x","fields":{"a":{"type":"banana"}}}' > /tmp/saga-g2/x.json && printf '{}' > /tmp/saga-g2/samples.json && node scripts/build.mjs --schema-dir /tmp/saga-g2 --out /tmp/saga-g2/out.mjs; echo "exit=$?"
    EXPECT: /^exit=1$/m
    FROM: R4 "I doubt it is the generator"
