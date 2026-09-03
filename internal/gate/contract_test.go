package gate

import (
	"strings"
	"testing"
)

const fullContract = `# Contract: vendor feed importer

CONTRACT: vendor-import
IN: src/import/**, tests/import/**
OUT: src/api/**, **/*.lock
BASE: HEAD
RISK: impossible

Some prose that mentions CHECK: inside a sentence is fine.

` + "```" + `
- [ ] NOTAGATE: inside a fence
    CHECK: ignored
` + "```" + `

- [ ] G1: a valid fixture imports every record
    CHECK: node scripts/check-import.mjs fixtures/valid.json
    EXPECT: import verification passed
    RED: mutation
    WITNESS: scripts/check-import.mjs, fixtures/valid.json

- [ ] G2: malformed records are rejected with a line number
    CHECK: node scripts/check-reject.mjs
    EXPECT: /rejected 3 records at lines 4, 9, 17/
    RED: control
    RED-CHECK: node scripts/check-reject.mjs --against fixtures/all-valid.json
    RED-EXPECT: rejected 0 records

- [ ] G3: the importer's public API is unchanged
  CHECK: npx api-extractor run --local && git diff --exit-code etc/importer.api.md
  EXPECT: api report is up to date

- [x] G4: the migration wording matches the product decision
    EVIDENCE: sha256:0000000000000000000000000000000000000000000000000000000000000000

ABANDON: G4 decision owner unavailable; handoff recorded in issue 123
WAIVE: G-ASSERT tests/import/parse.test.ts 3f9a12cd7b04 consolidated four equality assertions into one deep-equal; count drop is mechanical
`

func TestParseValidCorpus(t *testing.T) {
	cases := map[string]string{
		"minimal":                            minimalContract,
		"full":                               fullContract,
		"bench-style comment before headers": "# Contract: x\n\n<!-- canary: abc -->\nREQUEST: sha256:" + strings.Repeat("a", 64) + "\nIN: src/**\n\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    FROM: R1 \"quote\"\n",
		"no trailing newline":                "# T\nIN: a\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x",
		"two-space indent":                   "# T\nIN: a\n- [ ] G1: o\n  CHECK: true\n  EXPECT: x\n",
		"prose after a gate":                 "# T\nIN: a\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x\nA prose line.\n",
		"manual gate":                        "# T\nIN: a\n- [ ] G1: reviewed by a human\n",
	}
	for name, src := range cases {
		c, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(c.Gates) == 0 {
			t.Errorf("%s: no gates", name)
		}
	}
	c, err := Parse([]byte(fullContract))
	if err != nil {
		t.Fatal(err)
	}
	if c.Slug != "vendor-import" || c.Risk != "impossible" || len(c.In) != 2 || len(c.Out) != 2 || c.Base != "HEAD" {
		t.Errorf("headers: %+v", c)
	}
	if len(c.Gates) != 4 {
		t.Fatalf("gates: %d (fenced gate must be skipped)", len(c.Gates))
	}
	if g := c.Gate("G1"); g == nil || len(g.Witness) != 2 || g.RedMode() != RedMutation {
		t.Errorf("G1: %+v", g)
	}
	if g := c.Gate("G3"); g == nil || !g.Runnable() || g.Indent != "  " {
		t.Errorf("G3: %+v", g)
	}
	if g := c.Gate("G4"); g == nil || g.Runnable() || !g.Checked || g.Evidence == "" {
		t.Errorf("G4: %+v", g)
	}
	if c.Abandoned("G4") == nil || len(c.Waive) != 1 || c.Waive[0].Hunk != "3f9a12cd7b04" {
		t.Errorf("statements: %+v %+v", c.Abandon, c.Waive)
	}
	if got := Slug("Contract: Rate Limiter!"); got != "rate-limiter" {
		t.Errorf("slug %q", got)
	}
}

// TestParseFailClosed is the section 2.6 test matrix: every row is a
// ParseError with the row number.
func TestParseFailClosed(t *testing.T) {
	h := "# T\nIN: src/**\n"
	g := "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n"
	sha := "sha256:" + strings.Repeat("b", 64)
	cases := []struct {
		rule int
		src  string
	}{
		{1, h + "just prose\n"},
		{2, h + g + "- [ ] G1: again\n    CHECK: true\n    EXPECT: y\n"},
		{3, h + "- [ ] : no id\n    CHECK: true\n    EXPECT: x\n"},
		{3, h + "- [ ] G1 no colon\n"},
		{4, h + "- [ ] G1: o\n    CHECK: true\n"},
		{4, h + "- [ ] G1: o\n    EXPECT: x\n"},
		{5, h + "- [ ] G1: o\nCHECK: true\n    EXPECT: x\n"},
		{5, h + "- [ ] G1: o\n\n    CHECK: true\n    EXPECT: x\n"},
		{5, h + "- [ ] G1: o\n      CHECK: true\n      EXPECT: x\n"},
		{6, h + "- [ ] G1: o\n\tCHECK: true\n\tEXPECT: x\n"},
		{7, h + "- [ ] G1: o\n    CHECK: true\n    CHECK: false\n    EXPECT: x\n"},
		{8, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: /(unclosed/\n"},
		{8, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: /a/q\n"},
		{9, "# T\nIN: /abs/**\n" + g},
		{9, "# T\nIN: src/**\nOUT: ../up/**\n" + g},
		{9, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    CWD: /tmp\n"},
		{9, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    WITNESS: a/../../b\n"},
		{10, "# T\nREQUEST: " + sha + "\nIN: src/**\n" + g},
		{10, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    FROM: R1 \"q\"\n"},
		{13, h + g + "ABANDON: G9 gone\n"},
		{13, h + g + "ABANDON: G1\n"},
		{14, h + g + "    ABANDON: G1 reason\n"},
		{14, h + g + "IN: late/**\n"},
		{15, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    RED: control\n"},
		{15, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    RED-CHECK: false\n    RED-EXPECT: y\n"},
		{17, "# T\nIN: a\n" + strings.Repeat("x", MaxContractBytes)},
		{18, h + g + "WAIVE: G-NOPE a/b 3f9a12cd7b04 one two three four five six seven eight\n"},
		{18, h + g + "WAIVE: G-SKIP a/b zz9a12cd7b04 one two three four five six seven eight\n"},
		{18, h + g + "WAIVE: G-SKIP a/b 3f9a12cd7b04 too short\n"},
		{18, h + g + "WAIVE: G-LEDGER a/b 3f9a12cd7b04 one two three four five six seven eight\n"},
		{19, "# T\nIN: a\nRISK: hard\n" + g},
		{19, "# T\nIN: a\nRISK: impossible\nRISK: impossible\n" + g},
		{0, "no title\n" + g},
		{0, h + "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    RED: sometimes\n"},
		{0, "# T\n" + g},
	}
	for i, tc := range cases {
		_, err := Parse([]byte(tc.src))
		if err == nil {
			t.Errorf("case %d (rule %d): parsed", i, tc.rule)
			continue
		}
		pe, ok := err.(*ParseError)
		if !ok {
			t.Errorf("case %d: not a ParseError: %v", i, err)
			continue
		}
		if pe.Rule != tc.rule {
			t.Errorf("case %d: rule %d, want %d (%v)", i, pe.Rule, tc.rule, err)
		}
		if tc.rule > 0 && pe.ID() != "2.6-"+itoa(tc.rule) {
			t.Errorf("case %d: id %s", i, pe.ID())
		}
	}
}

func TestCRLFPreservedAndRewrite(t *testing.T) {
	src := strings.ReplaceAll(minimalContract, "\n", "\r\n")
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if c.EOL != "\r\n" {
		t.Fatalf("eol %q", c.EOL)
	}
	sha := "sha256:" + strings.Repeat("c", 64)
	out := string(c.Rewrite(map[string]EvidenceUpdate{"G1": {Checked: true, Evidence: sha}}))
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Errorf("LF leaked into a CRLF file: %q", out)
	}
	if !strings.Contains(out, "- [x] G1:") || !strings.Contains(out, "\r\n    EVIDENCE: "+sha+"\r\n") {
		t.Errorf("rewrite: %q", out)
	}
	// The checker's own writes leave contract_hash unchanged.
	c2, err := Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if c.Hash() != c2.Hash() {
		t.Error("contract_hash moved under the checker's own write")
	}
	// Unmet removes the line and unchecks.
	out2 := string(c2.Rewrite(map[string]EvidenceUpdate{"G1": {Checked: false}}))
	if strings.Contains(out2, "EVIDENCE:") || !strings.Contains(out2, "- [ ] G1:") {
		t.Errorf("unmet rewrite: %q", out2)
	}
	if out2 != src {
		t.Errorf("round trip differs:\n%q\n%q", out2, src)
	}
}

// Deleting any single byte from a valid contract yields a parse error or
// a contract that is still not ALL MET; never a false green.
func TestSingleDeletionNeverFalseGreen(t *testing.T) {
	src := []byte(minimalContract)
	for i := range src {
		mut := append(append([]byte{}, src[:i]...), src[i+1:]...)
		c, err := Parse(mut)
		if err != nil {
			continue
		}
		for _, g := range c.Gates {
			if g.Evidence != "" {
				t.Errorf("deletion at %d produced an EVIDENCE: line", i)
			}
		}
	}
}
