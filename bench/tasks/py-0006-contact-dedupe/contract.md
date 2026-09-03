# Contract: contact dedupe by email
<!-- canary: b382a4c6249ef36a -->
REQUEST: sha256:c7d192978279e9abb5878359b30b35be15c13115e4a37d7ea869b093adfe81fc
IN: contacts/merge.py, tests/**
OUT: contacts/models.py, contacts/importer.py
Decisions the request leaves open, fixed here: the first occurrence is kept unchanged and later ones dropped; an entry with an empty or blank email is never merged with anything; the input list is not mutated.

- [ ] G1: entries with the same email collapse to one, ignoring case and surrounding whitespace
    CHECK: python3 -m unittest tests.test_merge 2>&1 | tail -n 1 && python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; print(len(dedupe([C('a','Ann@Example.com'), C('b',' ann@example.com '), C('c','ANN@EXAMPLE.COM')])))"
    EXPECT: /^OK\n1$/m
    FROM: R3 "those differences must not stop a match"
    FROM: R4 "the same person if the email matches"
- [ ] G2: entries with an empty email are never merged
    CHECK: python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; print(len(dedupe([C('a',''), C('b',''), C('c','  ')])))"
    EXPECT: /^3$/m
    FROM: R4 "Two entries are the same person if the email matches"
- [ ] G3: the first occurrence wins untouched, order is kept and the input is not mutated
    CHECK: python3 -c "from contacts.models import Contact as C; from contacts.merge import dedupe; xs=[C('a','a@a','1'), C('b','b@b'), C('a2','a@a','2'), C('c','c@c')]; out=dedupe(xs); print(out[0], [c.name for c in out], len(xs))"
    EXPECT: Contact(name='a', email='a@a', phone='1') ['a', 'b', 'c'] 4
    FROM: R5 "Keep the list in the order it came in"
