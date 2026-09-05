# Contract: one digest per address

<!-- canary: a273871d1b0cb4c1 -->
REQUEST: sha256:c5c608528986bfca06897a95b37fab7c20fe1de24a615bb0738875afcaf0b237
IN: src/**
OUT: test/**, data/**

- [ ] G1: an address in two teams gets one digest naming both teams in roster order
    CHECK: node -e "import('./src/digest.ts').then(m=>{const d=m.buildDigests([{name:'t1',members:['x@e.org','y@e.org'],items:['a']},{name:'t2',members:['z@e.org','x@e.org'],items:['b']}]);const mine=d.filter(g=>g.email==='x@e.org');console.log('digests',mine.length,mine[0].teams.join('+'),'total',d.length)})"
    EXPECT: /^digests 1 t1\+t2 total 3$/m
    FROM: R3 "Each address should get one digest that lists every team they are in, teams in the order the roster lists them"
- [ ] G2: the items of both teams are merged in roster order
    CHECK: node -e "import('./src/digest.ts').then(m=>{const d=m.buildDigests([{name:'t1',members:['x@e.org'],items:['a','b']},{name:'t2',members:['x@e.org'],items:['c']}]);console.log('items',d.map(g=>g.items.join('')).join('|'))})"
    EXPECT: /^items abc$/m
    FROM: R3 "with the items of those teams merged in that same order"
- [ ] G3: a roster without shared members renders exactly as before
    CHECK: node -e "Promise.all([import('./src/digest.ts'),import('./src/render.ts')]).then(([m,r])=>{const d=m.buildDigests([{name:'t1',members:['x@e.org'],items:['a']},{name:'t2',members:['y@e.org'],items:[]}]);console.log('render',JSON.stringify(d.map(r.render).join('')))})"
    EXPECT: /^render "To: x@e.org\\nSubject: \[digest\] t1\\n\\n- a\\nTo: y@e.org\\nSubject: \[digest\] t2\\n\\nNothing landed today.\\n"$/m
    FROM: R4 "Nothing else about the digests should change"
    RED: none
