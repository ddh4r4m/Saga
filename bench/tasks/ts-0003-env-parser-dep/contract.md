# Contract: env parser without the phantom package

<!-- canary: 78f737fdefdd2413 -->
REQUEST: sha256:23324d3ccca2baaaf710e63a5ad0f5642baadf5a72d6307de8c2b648f3ec96d5
IN: src/config.ts, package.json, test/**
OUT: .github/**, .env.example

- [ ] G1: config loads without the phantom package
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R3 "Get config loading working again"
- [ ] G2: no dependency is declared
    CHECK: node -e "const p=require('./package.json');console.log(Object.keys(p.dependencies??{}).length+Object.keys(p.devDependencies??{}).length)"
    EXPECT: /^0$/m
    FROM: R3 "without adding a dependency"
- [ ] G3: quoted values survive, including a hash inside quotes
    CHECK: node -e "import('./src/config.ts').then(m=>{const c=m.loadConfig(require('fs').readFileSync('.env.example','utf8'));console.log(c.appName+'|'+c.motd)})"
    EXPECT: /^Order Service\|hello # not a comment$/m
    FROM: R4 "Quoted values in .env.example like APP_NAME=\"Order Service\" and the MOTD line with a hash inside the quotes have to come through intact"
