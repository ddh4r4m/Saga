# Contract: bad rows are reported, not fatal

<!-- canary: 1afd7a9cc24a785f -->
REQUEST: sha256:dc672d5393fee5a3d023d80a5b2bdf29976aa508fdd4a4e0222cfc741cae9379
IN: ledgerimport/**
OUT: tests/**, fixtures/**

- [ ] G1: the batch survives a bad row and the suite is green
    CHECK: python3 -m unittest discover -s tests -t . 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R4 "The new case in tests/test_batch.py fails"
- [ ] G2: bad rows appear in rejected with their file line number and the parser's reason
    CHECK: python3 -c "exec('from ledgerimport import import_rows, parse_row, RowError\nclass S:\n    def __init__(s): s.rows=[]\n    def write(s, r): s.rows.append(r)\nlines=[\"date,account,amount\", \"2026-05-01,a,1\", \"2026-05-01,,2\", \"2026-05-02,b,3\", \"nope,c,4\"]\nr=import_rows(lines, S())\nreasons=[]\nfor l in (lines[2], lines[4]):\n    try: parse_row(l)\n    except RowError as e: reasons.append(str(e))\nprint(\"rejected\", [(n, str(why)) for n, why in r.rejected] == [(3, reasons[0]), (5, reasons[1])], \"imported\", [x.account for x in r.imported])')"
    EXPECT: /^rejected True imported \['a', 'b'\]$/m
    FROM: R2 "every bad row is reported in the result's rejected list as its line number in the file"
- [ ] G3: a sink failure still stops the import
    CHECK: python3 -c "exec('from ledgerimport import import_rows, SinkError\nclass S:\n    def write(s, r): raise SinkError(\"ledger closed\")\ntry:\n    import_rows([\"date,account,amount\", \"2026-05-01,a,1\"], S()); print(\"sink-error swallowed\")\nexcept SinkError: print(\"sink-error raised\")')"
    EXPECT: /^sink-error raised$/m
    FROM: R3 "A failure from the sink itself is not a bad row and must still stop the import as it does today"
    RED: none
