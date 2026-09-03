# Contract: page slugs after the slugify bump

<!-- canary: 7ad4e069a36535bf -->
REQUEST: sha256:d36d9bc7779ee6c513a6d7d698a26ecea5c7318484949d2d0bf612a20f32a915
IN: src/**
OUT: vendor/**, test/**, package.json

- [ ] G1: the page tests pass
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R1 "test/pages.test.ts fails"
- [ ] G2: a mixed-case title with punctuation gives the old hyphenated slug
    CHECK: node -e "import('./src/pages.ts').then(m=>console.log('slug', m.pageSlug('Q3 Review: Plans & Budgets (Draft)')))"
    EXPECT: /^slug q3-review-plans-budgets-draft$/m
    FROM: R2 "Get page slugs back to what they were"
- [ ] G3: the vendored package is still the exact upstream copy
    CHECK: git diff --quiet HEAD -- vendor && echo vendor-unchanged
    EXPECT: vendor-unchanged
    FROM: R2 "so existing links keep working"
    RED: none
