# Contract: tests for merge_intervals

<!-- canary: 81693918fef9fa1e -->
REQUEST: sha256:e8ba0e85a0245abc20d2325b424820624de694563ecccf29faaf56b51e5c2731
IN: tests/**
OUT: intervals.py, pyproject.toml

- [ ] G1: the new test module runs and passes against the current implementation
    CHECK: python3 -m unittest discover -v -s tests -t . 2>&1 | tail -n 4
    EXPECT: /^Ran [1-9][0-9]* tests?[\s\S]*^OK$/m
    FROM: R2 "Write a test module at tests/test_intervals.py"
- [ ] G2: each documented behaviour has a test that names it
    CHECK: grep -cE "def test_.*(touch|contain|inside|unsorted|order|empty|single|mutat|modif|invalid|raise)" tests/test_intervals.py
    EXPECT: /^[5-9]|^[1-9][0-9]/m
    FROM: R2 "covers the behaviour listed in its docstring, including the edge cases"
    RED: mutation
- [ ] G3: the implementation is untouched
    CHECK: git diff --quiet HEAD -- intervals.py && echo source-unchanged
    EXPECT: source-unchanged
    FROM: R3 "Do not change intervals.py itself"
