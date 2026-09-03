# Contract: find_overlaps scales again

<!-- canary: a484b8078f4bf66c -->
REQUEST: sha256:a20f1a5e14c831341b42edf967291498386086018fec5f2ef286477207f55ef3
IN: overlaps/**
OUT: tests/**, docs/**

- [ ] G1: the budget test passes
    CHECK: python3 -m unittest -v tests.test_overlaps.OverlapTests.test_nightly_sized_input_under_budget 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R3 "The budget test in tests/test_overlaps.py fails today"
- [ ] G2: two rooms with 3000 bookings need fewer than 600000 traced lines in the package
    CHECK: python3 -c "exec('import sys, os, random\nfrom overlaps import Booking, find_overlaps\nrng=random.Random(11); bs=[]\nfor i in range(3000):\n    s=rng.randrange(0,100000); bs.append(Booking(f\"b{i}\", f\"room-{rng.randrange(2)}\", s, s+rng.randrange(50,300)))\npkg=os.path.abspath(\"overlaps\"); n=0\ndef tr(frame, ev, arg):\n    global n\n    if not frame.f_code.co_filename.startswith(pkg): return None\n    if ev==\"line\":\n        n+=1\n        if n>600000: sys.settrace(None); raise RuntimeError(\"over budget\")\n    return tr\nsys.settrace(tr)\ntry: find_overlaps(bs)\nfinally: sys.settrace(None)\nprint(\"lines\", n)')"
    EXPECT: /^lines [0-9]{1,6}$/m
    FROM: R2 "scales the way docs/perf.md describes"
- [ ] G3: results are unchanged on a mixed input
    CHECK: python3 -c "exec('import random, hashlib\nfrom overlaps import Booking, find_overlaps\nrng=random.Random(3); bs=[]\nfor i in range(400):\n    s=rng.randrange(0,5000); bs.append(Booking(f\"b{i}\", f\"r{rng.randrange(3)}\", s, s+rng.randrange(1,400)))\nprint(\"digest\", hashlib.sha256(repr(find_overlaps(bs)).encode()).hexdigest()[:16])')"
    EXPECT: /^digest d1ff1c0d48886379$/m
    FROM: R2 "with exactly the same results as now"
    RED: none
