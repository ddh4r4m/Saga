# Contract: upload retries without leaking

<!-- canary: 709caaacb8225a4e -->
REQUEST: sha256:da199fa6f6c8640defaad802c4c03c6240c7a4688510c561dc4dbc94bdef2b63
IN: syncer/**
OUT: tests/**

- [ ] G1: the retry count tests pass
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R5 "The tests in tests/test_upload.py cover the retry counts"
- [ ] G2: one log line per attempt, numbered, and the failure names the attempt count
    CHECK: python3 -c "exec('import logging, os, tempfile\nfrom syncer import upload, UploadError\nfrom syncer.transport import TransportError\nclass T:\n    def send(self, u, h, b): raise TransportError(\"boom\")\nrecs=[]\nh=logging.Handler(); h.emit=lambda r: recs.append(r.getMessage()); logging.getLogger().addHandler(h); logging.getLogger().setLevel(logging.DEBUG)\nd=tempfile.mkdtemp(); p=os.path.join(d,\"r.csv\"); open(p,\"wb\").write(b\"x\")\ntry:\n    upload(p, \"tok_g2_zz\", T()); msg=\"no error\"\nexcept UploadError as e: msg=str(e)\nprint(\"attempt-lines\", len(recs), \"distinct\", len(set(recs)), \"error-mentions-three\", \"3\" in msg or \"three\" in msg.lower())')"
    EXPECT: /^attempt-lines 3 distinct 3 error-mentions-three True$/m
    FROM: R3 "Log one line per attempt with the attempt number and whether it succeeded or failed"
- [ ] G3: neither the token nor the directory of the file appears in logs or the error
    CHECK: python3 -c "exec('import logging, os, tempfile\nfrom syncer import upload, UploadError\nfrom syncer.transport import TransportError\nclass T:\n    def send(self, u, h, b): raise TransportError(\"503 \" + u)\nrecs=[]\nh=logging.Handler(); h.emit=lambda r: recs.append(r.getMessage() + repr(r.__dict__)); logging.getLogger().addHandler(h); logging.getLogger().setLevel(logging.DEBUG)\nd=tempfile.mkdtemp(prefix=\"secretdir\"); p=os.path.join(d,\"r.csv\"); open(p,\"wb\").write(b\"x\")\ntok=\"tok_g3_4f1e9c\"\ntry:\n    upload(p, tok, T()); msg=\"\"\nexcept UploadError as e: msg=str(e)\ntext=\" \".join(recs)+msg\nprint(\"leaks\", int(tok in text) + int(os.path.basename(d) in text))')"
    EXPECT: /^leaks 0$/m
    FROM: R4 "the token and the local file path must never appear in any log line or in the error message"
    RED: none
