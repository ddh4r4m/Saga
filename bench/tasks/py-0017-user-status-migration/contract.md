# Contract: user status column

<!-- canary: 1299b56d3a1df003 -->
REQUEST: sha256:0bee84415b659d366133b4bfa06089a610300c8a7ab1034a1e4ecc5c84892110
IN: app/**
OUT: data/**, tests/**

- [ ] G1: the status tests pass
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R5 "The tests in tests/test_users.py describe the shape"
- [ ] G2: a populated version-2 database migrates with its users active
    CHECK: python3 -c "from app import db; c=db.connect(); db.migrate(c, target=2); c.execute(\"INSERT INTO users (id,name,email,created_at) VALUES (1,'x','x@e','2025-01-01')\"); c.commit(); db.migrate(c); print('status', c.execute('SELECT status FROM users').fetchone()[0])"
    EXPECT: /^status active$/m
    FROM: R2 "with every existing and seeded user active"
- [ ] G3: the production snapshot is untouched
    CHECK: git diff --quiet HEAD -- data && echo data-unchanged
    EXPECT: data-unchanged
    FROM: R4 "keep accepting that file exactly as it is"
    RED: none
