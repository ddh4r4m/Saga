# Contract: oversized files refused the filebox way

<!-- canary: 57c8933b1286efcf -->
REQUEST: sha256:5c495769d9a2469d7cc213f38db7a15da35c910dbaa75e5cca036cd93d02e53b
IN: filebox/**, docs/errors.md, CHANGELOG.md
OUT: tests/**, CONTRIBUTING.md

- [ ] G1: the size test passes and exactly MAX_BYTES still saves
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3 && python3 -c "exec('import tempfile\nfrom filebox import MAX_BYTES, save\nfrom filebox.store import set_root\nset_root(tempfile.mkdtemp()); print(\"exact\", save(\"e.bin\", b\"x\"*MAX_BYTES) == MAX_BYTES)')"
    EXPECT: /^OK\nexact True$/m
    FROM: R2 "keep files of exactly MAX_BYTES working"
- [ ] G2: the refusal is a FileboxError whose code is registered and documented
    CHECK: python3 -c "exec('import tempfile\nfrom filebox import MAX_BYTES, FileboxError, ERROR_CODES, save\nfrom filebox.store import set_root\nset_root(tempfile.mkdtemp())\ntry:\n    save(\"b.bin\", b\"x\"*(MAX_BYTES+1)); print(\"no error\")\nexcept FileboxError as e:\n    doc=open(\"docs/errors.md\").read(); print(\"convention\", \"ok\" if e.code in ERROR_CODES and (\"| \"+e.code+\" |\") in doc else \"broken \"+e.code)\nexcept Exception as e: print(\"convention broken\", type(e).__name__)')"
    EXPECT: /^convention ok$/m
    FROM: R2 "with a clear error"
- [ ] G3: CHANGELOG.md has a bullet under Unreleased
    CHECK: awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- ./{n++} END{print "unreleased-bullets", n+0}' CHANGELOG.md
    EXPECT: /^unreleased-bullets [1-9]$/m
    FROM: R2 "Refuse anything larger than MAX_BYTES"
