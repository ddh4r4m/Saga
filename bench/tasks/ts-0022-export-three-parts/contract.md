# Contract: export quoting, since filter, summary on stderr

<!-- canary: 9d0a1caa380d8582 -->
REQUEST: sha256:3885d0ed77067cfdc57b9217a0666aeaa7d90b15b3fdcb6dee027ad057d282e6
IN: src/**
OUT: test/**, data/**

- [ ] G1: fields with commas or quotes are CSV-quoted and the suite is green
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R2 "so fields need proper CSV quoting"
- [ ] G2: --since keeps items updated on or after the date, inclusive
    CHECK: node -e "import('./src/export.ts').then(m=>{const items=[{sku:'A',name:'a',qty:1,updatedAt:'2026-03-09'},{sku:'B',name:'b',qty:1,updatedAt:'2026-03-10'},{sku:'C',name:'c',qty:1,updatedAt:'2026-03-11'}];const out=[];m.runExport(items,['--since','2026-03-10'],{stdout:l=>out.push(l),stderr:()=>{}});console.log('rows',out.filter(l=>/^[ABC],/.test(l)).map(l=>l[0]).join(''))})"
    EXPECT: /^rows BC$/m
    FROM: R3 "keeps only items updated on or after that date, inclusive"
- [ ] G3: the summary line goes to stderr and not to stdout
    CHECK: node -e "import('./src/export.ts').then(m=>{const items=[{sku:'A',name:'a',qty:1,updatedAt:'2026-03-09'},{sku:'B',name:'b',qty:2,updatedAt:'2026-03-10'}];const out=[],err=[];m.runExport(items,[],{stdout:l=>out.push(l),stderr:l=>err.push(l)});console.log('summary',out.some(l=>/exported/.test(l))?'stdout':'-',err.some(l=>/exported 2 items/.test(l))?'stderr':'-')})"
    EXPECT: /^summary - stderr$/m
    FROM: R4 "summary line has to go to stderr instead of stdout"
