package gate

import (
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/canon"
)

func TestSegmenter(t *testing.T) {
	text := `Article slugs are coming out mangled. "Hello  World!" turns into "hello--world-". Fix slugify so runs collapse, e.g. double hyphens. Do not change the public signature.

# Heading here
- item one
- item two?

` + "```\ncode. Block.\n```\n"
	sents := Segment(text)
	want := []string{
		`Article slugs are coming out mangled. "Hello  World!" turns into "hello--world-".`,
		"Fix slugify so runs collapse, e.g. double hyphens.",
		"Do not change the public signature.",
		"# Heading here",
		"- item one",
		"- item two?",
		"```\ncode. Block.\n```",
	}
	if len(sents) != len(want) {
		t.Fatalf("got %d sentences: %+v", len(sents), sents)
	}
	for i, w := range want {
		if sents[i].Text != w || sents[i].ID != "R"+itoa(i+1) {
			t.Errorf("R%d: %q, want %q", i+1, sents[i].Text, w)
		}
	}
	if !strings.HasPrefix(Numbered("One. Two."), "SEGMENTER: v1\nR1  One.\nR2  Two.\n") {
		t.Errorf("numbered: %q", Numbered("One. Two."))
	}
}

func TestVerifyRequest(t *testing.T) {
	req := []byte("Import valid records from the vendor feed. Reject malformed records and report the line number.")
	hash := canon.SHA256(req)
	mk := func(from string) *Contract {
		c, err := Parse([]byte("# T\nREQUEST: " + hash + "\nIN: a\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n    FROM: " + from + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if s, err := VerifyRequest(mk(`R2 "Reject   malformed records"`), req, hash); err != nil || len(s) != 2 {
		t.Errorf("whitespace-normalised quote should verify: %v", err)
	}
	if _, err := VerifyRequest(mk(`R1 "Reject malformed records"`), req, hash); err == nil || err.(*ParseError).Rule != 11 {
		t.Errorf("quote in the wrong sentence: %v", err)
	}
	if _, err := VerifyRequest(mk(`R3 "anything"`), req, hash); err == nil || err.(*ParseError).Rule != 11 {
		t.Errorf("missing sentence: %v", err)
	}
	if _, err := VerifyRequest(mk(`R1 "Import valid"`), append(req, '!'), canon.SHA256(append(req, '!'))); err == nil || err.(*ParseError).Rule != 12 {
		t.Errorf("hash mismatch: %v", err)
	}
	if _, err := VerifyRequest(mk(`R1 "Import valid"`), nil, ""); err == nil || err.(*ParseError).Rule != 12 {
		t.Errorf("missing request: %v", err)
	}
	// No REQUEST: means no verification and no coverage.
	c, _ := Parse([]byte(minimalContract))
	if s, err := VerifyRequest(c, nil, ""); err != nil || s != nil {
		t.Errorf("no request: %v %v", s, err)
	}
}

func TestClassify(t *testing.T) {
	if Classify(Sentence{Text: "Can you also check the logs?"}, false) != Ignored {
		t.Error("interrogative should be ignored")
	}
	if Classify(Sentence{Text: "Thanks!"}, false) != Ignored {
		t.Error("greeting should be ignored")
	}
	if Classify(Sentence{Text: "Reject malformed records and report the line number."}, false) != Uncovered {
		t.Error("requirement should be uncovered")
	}
	if Classify(Sentence{Text: "x"}, true) != Covered {
		t.Error("covered")
	}
}
