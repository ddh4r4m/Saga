# Contract: contact dedupe by email

<!-- canary: b382a4c6249ef36a -->
REQUEST: sha256:ef2298554ae3d5319f038f70af9eae8c73175038a77f4be0c3b47df2b8261de1
IN: contacts/merge.py, tests/**
OUT: contacts/models.py, contacts/importer.py

Decisions that the request leaves open, fixed here: emails compare case-insensitively after trimming surrounding whitespace; the first occurrence is kept unchanged and later ones dropped; an entry with an empty email is never merged with anything; the input list is not mutated.

- [ ] G1: two entries with the same email collapse to one
    CHECK: python3 -m unittest -v tests.test_merge
    EXPECT: /^OK$/m
    FROM: R2 "removes the duplicates"
- [ ] G2: email comparison ignores case and surrounding whitespace
    CHECK: python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; print(len(dedupe([C('a','Ann@Example.com'), C('b',' ann@example.com '), C('c','ANN@EXAMPLE.COM')])))"
    EXPECT: /^1$/m
    FROM: R3 "the same person if the email matches"
- [ ] G3: the first occurrence wins, untouched
    CHECK: python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; print(dedupe([C('first','x@y.z','1'), C('second','x@y.z','2')])[0])"
    EXPECT: Contact(name='first', email='x@y.z', phone='1')
    FROM: R4 "Keep the list in the order it came in"
- [ ] G4: entries with an empty email are never merged
    CHECK: python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; print(len(dedupe([C('a',''), C('b',''), C('c','  ')])))"
    EXPECT: /^3$/m
    FROM: R3 "Two entries are the same person if the email matches"
- [ ] G5: order is preserved and the input list is not mutated
    CHECK: python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; xs=[C('a','a@a'), C('b','b@b'), C('a2','a@a'), C('c','c@c')]; out=dedupe(xs); print([c.name for c in out], len(xs))"
    EXPECT: ['a', 'b', 'c'] 4
    FROM: R4 "Keep the list in the order it came in"
