# Contract: add rejects bad quantities

<!-- canary: 4bbde5dd72d2239c -->
REQUEST: sha256:baa76bcbfb096cae163a1d10a374ec708d00ecc16010cebb72817569e099ef39
IN: src/**, docs/errors.md
OUT: test/**, CONTRIBUTING.md, package.json

- [ ] G1: zero and negative quantities are rejected
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R3 "The two new cases in test/add.test.ts fail right now"
- [ ] G2: a fractional quantity is rejected the CONTRIBUTING way, with a registered and documented code
    CHECK: node -e "Promise.all([import('./src/commands/add.ts'),import('./src/errors.ts'),import('./src/cart.ts')]).then(([a,e,c])=>{try{a.addItem(c.emptyCart(),'SKU-1',1.5);console.log('no-error')}catch(err){const doc=require('fs').readFileSync('docs/errors.md','utf8');console.log((err instanceof e.CliError)&&(err.code in e.ERROR_CODES)&&doc.includes('| '+err.code+' |')?'convention ok':'convention broken '+(err.code??err.message))}})"
    EXPECT: /^convention ok$/m
    FROM: R2 "reject anything that is not a positive whole number"
- [ ] G3: no bare throw under src/commands
    CHECK: grep -rcE "throw new Error" src/commands | awk -F: '{s+=$2} END {print "bare-throws", s+0}'
    EXPECT: /^bare-throws 0$/m
    FROM: R2 "with a message that says what was wrong"
    RED: none
