package claims

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type corpusCase struct {
	ID          string         `json:"id"`
	Text        string         `json:"text"`
	ClaimedDone *bool          `json:"claimed_done"`
	Structural  bool           `json:"structural"`
	Kinds       map[string]int `json:"kinds"`
	Paths       []string       `json:"paths"`
	Commands    []string       `json:"commands"`
	IDs         []string       `json:"ids"`
	Passed      *int           `json:"passed"`
}

func loadCorpus(t *testing.T) []corpusCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "trace", "claims-corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []corpusCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 40 {
		t.Fatalf("corpus has %d cases, want at least 40", len(cases))
	}
	return cases
}

func boolp(p *bool) string {
	if p == nil {
		return "null"
	}
	return fmt.Sprint(*p)
}

// TestCorpus labels every message of the corpus: claimed_done, the
// structural sensitivity value, the claim kinds with counts, and the
// extracted paths, commands and counts. It then reports per-kind
// precision and recall over the corpus and enforces the section 11.1
// bars (precision >= 0.9, recall >= 0.8).
func TestCorpus(t *testing.T) {
	l, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := loadCorpus(t)
	tp, fp, fn := map[string]int{}, map[string]int{}, map[string]int{}
	for _, c := range cases {
		d := Detect(l, c.Text, "")
		if boolp(d.ClaimedDone) != boolp(c.ClaimedDone) {
			t.Errorf("%s: claimed_done %s (%s), want %s", c.ID, boolp(d.ClaimedDone), d.Reason, boolp(c.ClaimedDone))
		}
		if (d.Structural != nil && *d.Structural) != c.Structural {
			t.Errorf("%s: structural %v, want %v", c.ID, d.Structural != nil && *d.Structural, c.Structural)
		}
		got := map[string]int{}
		var paths, cmds, ids []string
		var passed *int
		for _, cl := range d.Claims {
			got[cl.Kind]++
			if cl.Path != "" {
				paths = append(paths, cl.Path)
			}
			if cl.Command != "" {
				cmds = append(cmds, cl.Command)
			}
			ids = append(ids, cl.IDs...)
			if cl.Passed != nil && passed == nil {
				passed = cl.Passed
			}
		}
		for _, k := range Kinds {
			want, have := c.Kinds[k], got[k]
			m := want
			if have < m {
				m = have
			}
			tp[k] += m
			fp[k] += have - m
			fn[k] += want - m
			if want != have {
				t.Errorf("%s: %s x%d, want x%d (claims %s)", c.ID, k, have, want, describeAll(d.Claims))
			}
		}
		if c.Paths != nil && !sameSet(paths, c.Paths) {
			t.Errorf("%s: paths %v, want %v", c.ID, paths, c.Paths)
		}
		if c.Commands != nil && !sameSet(cmds, c.Commands) {
			t.Errorf("%s: commands %v, want %v", c.ID, cmds, c.Commands)
		}
		if c.IDs != nil && !sameSet(ids, c.IDs) {
			t.Errorf("%s: ids %v, want %v", c.ID, ids, c.IDs)
		}
		if c.Passed != nil && (passed == nil || *passed != *c.Passed) {
			t.Errorf("%s: passed %v, want %d", c.ID, passed, *c.Passed)
		}
	}
	for _, k := range Kinds {
		prec, rec := 1.0, 1.0
		if tp[k]+fp[k] > 0 {
			prec = float64(tp[k]) / float64(tp[k]+fp[k])
		}
		if tp[k]+fn[k] > 0 {
			rec = float64(tp[k]) / float64(tp[k]+fn[k])
		}
		t.Logf("%-10s precision %.2f recall %.2f (tp %d fp %d fn %d)", k, prec, rec, tp[k], fp[k], fn[k])
		if prec < 0.9 {
			t.Errorf("%s precision %.2f below 0.9: the kind must ship unverified-only", k, prec)
		}
		if rec < 0.8 {
			t.Errorf("%s recall %.2f below 0.8", k, rec)
		}
	}
}

func describeAll(cs []Claim) string {
	var parts []string
	for _, c := range cs {
		parts = append(parts, c.Kind+"@"+fmt.Sprint(c.Span))
	}
	return strings.Join(parts, " ")
}

func sameSet(a, b []string) bool {
	x := append([]string{}, a...)
	y := append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, "\x00") == strings.Join(y, "\x00")
}

func TestListHashesAndInvalidList(t *testing.T) {
	l, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if l.Hash != ClaimsListHash || !strings.HasPrefix(l.Hash, "sha256:") {
		t.Errorf("hash %s", l.Hash)
	}
	if l.AbstainHash != AbstainListHash {
		t.Errorf("abstain hash %s", l.AbstainHash)
	}
	if _, err := ParseList("done\tlexical\t(unclosed", AbstainList()); err == nil {
		t.Error("invalid regex accepted")
	}
	if _, err := ParseList("nope\tlexical\tx", AbstainList()); err == nil {
		t.Error("unknown kind accepted")
	}
	r := Judge(&Input{Final: "DONE", FinalAvailable: true, ListErr: fmt.Errorf("boom"), Mode: ModeMinimal})
	if !r.ListInvalid || r.Decision != DecisionBlock || r.Exit != 2 || !strings.Contains(r.Message, "claims list invalid") {
		t.Errorf("invalid list: %+v", r)
	}
}

func TestDetectCapsAndSpans(t *testing.T) {
	l, _ := Default()
	var b strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&b, "Updated `src/f%d.ts`.\n", i)
	}
	b.WriteString("DONE")
	d := Detect(l, b.String(), "")
	if len(d.Claims) != MaxClaims || !d.Truncated {
		t.Fatalf("cap: %d claims truncated=%v", len(d.Claims), d.Truncated)
	}
	for i := 1; i < len(d.Claims); i++ {
		if d.Claims[i].Span[0] < d.Claims[i-1].Span[0] {
			t.Fatal("claims not in message order")
		}
	}
	msg := "I ran `pytest -q` and updated `src/x.py`.\n\nDONE"
	d = Detect(l, msg, "")
	for _, c := range d.Claims {
		text := msg[c.Span[0]:c.Span[1]]
		switch c.Kind {
		case KindDone:
			if text != "DONE" {
				t.Errorf("done span %q", text)
			}
		case KindRan:
			if !strings.Contains(text, "pytest -q") {
				t.Errorf("ran span %q", text)
			}
		case KindTouched:
			if !strings.Contains(text, "src/x.py") {
				t.Errorf("touched span %q", text)
			}
		}
	}
}

func TestNormalizePath(t *testing.T) {
	root := "/repo"
	cases := map[string]string{
		"src/x.ts": "src/x.ts", "./src/x.ts": "src/x.ts", "src/x.ts:12": "src/x.ts", "src/x.ts:12:3": "src/x.ts",
		"/repo/src/x.ts": "src/x.ts", "/etc/hosts": "", "../other/x.ts": "", "pytest": "", "DONE": "", "v1.2.3": "",
		"README.md": "README.md", "src/": "", "https://x.y/z.ts": "", "src/a b.ts": "", "Makefile": "", "src/utils": "src/utils",
	}
	for in, want := range cases {
		if got := NormalizePath(root, in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if NormalizePath("", "/repo/src/x.ts") != "" {
		t.Error("absolute path without a root must drop")
	}
}
