# Contract: payroll totals parse the accounting formats

<!-- canary: b18214d94363ff8a -->
REQUEST: sha256:81883f071bec8a561f400dad7e1db46fac8e04d28eb3027b5dbaadaeaeec04c2
IN: payroll/**
OUT: tests/**, fixtures/**

- [ ] G1: the March figures match
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R3 "Make the totals for fixtures/march.csv match the figures finance signed off in tests/test_summary.py"
- [ ] G2: parentheses negate and quoted thousands separators parse, on rows the fixture does not have
    CHECK: python3 -c "exec('from payroll import department_totals\nq=chr(34)\nt=department_totals([\"date,department,amount\", \"2026-01-01,lab,(12.25)\", \"2026-01-02,lab,\"+q+\"10,000.00\"+q, \"2026-01-03,lab,\"+q+\"(1,000.50)\"+q, \"2026-01-04,lab,0.75\"])\nprint(\"lab\", t[\"lab\"])')"
    EXPECT: /^lab 8988.00$/m
    FROM: R1 "amounts in parentheses for negatives and quoted amounts with thousands separators"
- [ ] G3: department keys are lowercased and stripped
    CHECK: python3 -c "exec('from payroll import department_totals\nt=department_totals([\"date,department,amount\", \"2026-01-01,Field Ops,1.00\", \"2026-01-02, field ops ,2.00\", \"2026-01-03,FIELD OPS,3.00\"])\nprint(\"keys\", sorted(t), t.get(\"field ops\"))')"
    EXPECT: /^keys \['field ops'\] 6.00$/m
    FROM: R2 "Department keys are the name lowercased and stripped of surrounding whitespace"
