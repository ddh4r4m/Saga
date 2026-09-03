package gate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/store"
)

// Finding is one diff-guard result (gate-spec section 5).
type Finding struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Hunk         string `json:"hunk"`
	Rule         string `json:"rule"`
	Pre          int    `json:"pre"`
	Post         int    `json:"post"`
	Degraded     bool   `json:"degraded"`
	Advisory     bool   `json:"advisory"`
	Waived       bool   `json:"waived"`
	WaiverReason string `json:"waiver_reason,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

// Blocks reports whether the finding blocks: not waived and not advisory.
func (f Finding) Blocks() bool { return !f.Waived && !f.Advisory }

// GuardInput is what one guard run needs.
type GuardInput struct {
	Root     string
	Store    *store.Store
	Base     string
	Contract *Contract
	Config   Config
	// Only restricts to the listed guard ids when non-empty.
	Only []string
	// Advisory enables the experimental guards (G-ASSERT, G-HARDCODE).
	Advisory bool
	// Paths restricts the diff to these repo-relative paths (incremental
	// and predict modes); empty means the full diff.
	Paths []string
}

func (in GuardInput) wants(id string) bool {
	if len(in.Only) == 0 {
		return true
	}
	for _, o := range in.Only {
		if strings.EqualFold(o, id) {
			return true
		}
	}
	return false
}

func hunkHash(parts ...string) string { return short12([]byte(strings.Join(parts, "\x00"))) }

// GuardDiff runs the guards over `git diff <base>` plus untracked files
// and returns every finding, waivers applied, in a stable order.
func GuardDiff(in GuardInput) ([]Finding, error) {
	entries, err := DiffPaths(in.Root, in.Base, in.Paths...)
	if err != nil {
		return nil, err
	}
	var out []Finding
	if in.wants("G-SCOPE") {
		out = append(out, guardScope(in, entries)...)
	}
	if in.wants("G-TESTDEL") {
		out = append(out, guardTestDel(in, entries)...)
	}
	if in.wants("G-SKIP") {
		out = append(out, guardSkip(in, entries)...)
	}
	if in.wants("G-LEDGER") && in.Store != nil {
		out = append(out, guardLedger(in)...)
	}
	if in.Advisory {
		if in.wants("G-ASSERT") {
			out = append(out, guardAssert(in, entries)...)
		}
		if in.wants("G-HARDCODE") {
			out = append(out, guardHardcode(in, entries)...)
		}
	}
	for i := range out {
		for _, w := range in.Contract.Waive {
			if w.Guard == out[i].ID && w.Path == out[i].Path && w.Hunk == out[i].Hunk && out[i].ID != "G-LEDGER" {
				out[i].Waived, out[i].WaiverReason = true, w.Reason
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// PredictScope is the PreToolUse form: does an edit to path violate
// IN:/OUT:? Returns the finding, or nil when in scope.
func PredictScope(in GuardInput, path string) *Finding {
	r, ok := rel(in.Root, path)
	if !ok {
		return &Finding{ID: "G-SCOPE", Path: canon.RelPath(in.Root, path), Hunk: hunkHash("G-SCOPE", "predict", path), Rule: "outside-repo"}
	}
	return scopeOf(in, "M", r)
}

func scopeOf(in GuardInput, status, p string) *Finding {
	fold := FoldCase()
	if strings.HasPrefix(p, ".saga/") || MatchAny(in.Config.ScopeExempt, p, fold) {
		return nil
	}
	for _, g := range in.Contract.Out {
		if MatchGlob(g, p, fold) {
			return &Finding{ID: "G-SCOPE", Path: p, Hunk: hunkHash("G-SCOPE", status, p), Rule: "matches-OUT", Detail: g}
		}
	}
	if !MatchAny(in.Contract.In, p, fold) {
		return &Finding{ID: "G-SCOPE", Path: p, Hunk: hunkHash("G-SCOPE", status, p), Rule: "not-in-IN", Detail: strings.Join(in.Contract.In, ", ")}
	}
	return nil
}

func guardScope(in GuardInput, entries []DiffEntry) []Finding {
	var out []Finding
	for _, e := range entries {
		for _, p := range []string{e.OldPath, e.Path} {
			if p == "" {
				continue
			}
			if f := scopeOf(in, e.Status, p); f != nil {
				out = append(out, *f)
			}
		}
		if e.Status != "D" && IsSymlink(in.Root, e.Path) {
			out = append(out, Finding{ID: "G-SCOPE", Path: e.Path, Hunk: hunkHash("G-SCOPE", "symlink", e.Path), Rule: "added-symlink"})
		}
	}
	return out
}

// Language heuristics of section 5.2 for the regex guards.
type lang struct {
	name    string
	testDef *regexp.Regexp // discovery-convention declarations
	skip    *regexp.Regexp // skip / only / disable markers
	assert  *regexp.Regexp // assertion forms (advisory G-ASSERT)
	funcDef *regexp.Regexp // any function declaration
}

var langs = map[string]*lang{
	"js": {
		name:    "js",
		testDef: regexp.MustCompile(`(?m)^\s*(?:it|test)\s*\(`),
		skip:    regexp.MustCompile(`(?m)\b(?:it|test|describe|context)\.(?:skip|only|todo)\b|\b(?:xit|xtest|xdescribe|fdescribe|fit)\s*\(|\btest\.concurrent\.skip\b`),
		assert:  regexp.MustCompile(`\bexpect\s*\(|\bassert\.\w+\s*\(|\bassert\s*\(`),
		funcDef: regexp.MustCompile(`(?m)^\s*(?:function\s+\w+|const\s+\w+\s*=\s*(?:async\s*)?\(|(?:it|test)\s*\()`),
	},
	"py": {
		name:    "py",
		testDef: regexp.MustCompile(`(?m)^\s*(?:def\s+test_\w*|class\s+Test\w*)`),
		skip:    regexp.MustCompile(`(?m)@pytest\.mark\.(?:skip|skipif|xfail)\b|@unittest\.skip\w*|\bpytest\.skip\s*\(|\bself\.skipTest\s*\(|\bunittest\.SkipTest\b|\bpytest\.xfail\s*\(`),
		assert:  regexp.MustCompile(`(?m)^\s*assert\b|\bself\.assert\w*\s*\(|\bpytest\.raises\s*\(`),
		funcDef: regexp.MustCompile(`(?m)^\s*def\s+\w+`),
	},
	"go": {
		name:    "go",
		testDef: regexp.MustCompile(`(?m)^func\s+Test\w*\s*\(`),
		skip:    regexp.MustCompile(`\bt\.Skip(?:Now|f)?\s*\(|\btesting\.Short\s*\(\)|//go:build ignore`),
		assert:  regexp.MustCompile(`\bt\.(?:Fatal|Fatalf|Error|Errorf)\s*\(|\b(?:require|assert)\.\w+\s*\(`),
		funcDef: regexp.MustCompile(`(?m)^func\s+\w+`),
	},
}

func langOf(p string) *lang {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".mts", ".cts":
		return langs["js"]
	case ".py":
		return langs["py"]
	case ".go":
		return langs["go"]
	}
	return nil
}

func countAll(re *regexp.Regexp, b []byte) int {
	if re == nil {
		return 0
	}
	return len(re.FindAllIndex(b, -1))
}

// isTestFile applies test_globs, then the declaration heuristic.
func isTestFile(in GuardInput, p string, content []byte) bool {
	if MatchAny(in.Config.TestGlobs, p, FoldCase()) {
		return true
	}
	if l := langOf(p); l != nil && content != nil {
		return countAll(l.testDef, content) > 0
	}
	return false
}

func workingFile(root, p string) ([]byte, bool) {
	raw, err := os.ReadFile(join(root, p))
	if err != nil {
		return nil, false
	}
	return raw, true
}

func guardTestDel(in GuardInput, entries []DiffEntry) []Finding {
	var out []Finding
	for _, e := range entries {
		switch e.Status {
		case "D":
			pre, _ := ShowAt(in.Root, in.Base, e.Path)
			if isTestFile(in, e.Path, pre) {
				l := langOf(e.Path)
				n := 0
				if l != nil {
					n = countAll(l.testDef, pre)
				}
				out = append(out, Finding{ID: "G-TESTDEL", Path: e.Path, Hunk: hunkHash("G-TESTDEL", "D", e.Path), Rule: "test-file-deleted", Pre: n})
			}
		case "R":
			pre, _ := ShowAt(in.Root, in.Base, e.OldPath)
			if !isTestFile(in, e.OldPath, pre) {
				continue
			}
			post, _ := workingFile(in.Root, e.Path)
			l := langOf(e.OldPath)
			preN, postN := 0, 0
			if l != nil {
				preN, postN = countAll(l.testDef, pre), countAll(l.testDef, post)
			}
			if !isTestFile(in, e.Path, post) {
				out = append(out, Finding{ID: "G-TESTDEL", Path: e.OldPath, Hunk: hunkHash("G-TESTDEL", "R", e.OldPath, e.Path), Rule: "test-renamed-out", Pre: preN, Post: postN, Detail: e.Path})
			} else if postN < preN {
				out = append(out, Finding{ID: "G-TESTDEL", Path: e.Path, Hunk: hunkHash("G-TESTDEL", "R", e.OldPath, e.Path), Rule: "test-decl-drop-on-rename", Pre: preN, Post: postN})
			}
		}
	}
	return out
}

func guardSkip(in GuardInput, entries []DiffEntry) []Finding {
	var out []Finding
	for _, e := range entries {
		if e.Status == "D" {
			continue
		}
		l := langOf(e.Path)
		if l == nil {
			continue
		}
		post, ok := workingFile(in.Root, e.Path)
		if !ok {
			continue
		}
		prePath := e.Path
		if e.OldPath != "" {
			prePath = e.OldPath
		}
		pre, _ := ShowAt(in.Root, in.Base, prePath)
		if !isTestFile(in, e.Path, post) && !isTestFile(in, prePath, pre) {
			continue
		}
		preS, postS := countAll(l.skip, pre), countAll(l.skip, post)
		if postS > preS {
			var lines []string
			for _, ln := range strings.Split(string(post), "\n") {
				if l.skip.MatchString(ln) {
					lines = append(lines, strings.TrimSpace(ln))
				}
			}
			out = append(out, Finding{ID: "G-SKIP", Path: e.Path, Hunk: hunkHash("G-SKIP", e.Path, strings.Join(lines, "\n")), Rule: "skip-marker-added", Pre: preS, Post: postS})
		}
		// A test function renamed out of the discovery convention: the
		// declaration count dropped while the function count did not.
		if pre != nil {
			preT, postT := countAll(l.testDef, pre), countAll(l.testDef, post)
			preF, postF := countAll(l.funcDef, pre), countAll(l.funcDef, post)
			if postT < preT && preF-postF < preT-postT {
				out = append(out, Finding{ID: "G-SKIP", Path: e.Path, Hunk: hunkHash("G-SKIP", "rename", e.Path, itoa(preT), itoa(postT)), Rule: "test-renamed-out-of-discovery", Pre: preT, Post: postT})
			}
		}
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

func guardLedger(in GuardInput) []Finding {
	var out []Finding
	add := func(rule, path string, detail string) {
		out = append(out, Finding{ID: "G-LEDGER", Path: path, Hunk: hunkHash("G-LEDGER", rule, path), Rule: rule, Detail: detail})
	}
	s := in.Store
	// Unknown schemas anywhere in the store.
	for _, kind := range []string{"evidence", "red"} {
		root := s.Path(kind)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") {
				return nil
			}
			relp, _ := rel(in.Root, p)
			raw, rerr := os.ReadFile(p)
			if rerr != nil {
				add("record-unreadable", relp, "")
				return nil
			}
			var head struct {
				Schema string `json:"schema"`
			}
			want := EvidenceSchema
			if kind == "red" {
				want = RedSchema
			}
			if json.Unmarshal(raw, &head) != nil || head.Schema != want {
				add("record-unknown-schema", relp, head.Schema)
			}
			return nil
		})
	}
	c := in.Contract
	hasEvidence := false
	for _, g := range c.Gates {
		if _, err := os.Stat(evidencePath(s, c.Slug, g.ID)); err == nil {
			hasEvidence = true
		}
		if g.Evidence == "" {
			continue
		}
		relp, _ := rel(in.Root, evidencePath(s, c.Slug, g.ID))
		rec, hash, err := LoadEvidence(s, c.Slug, g.ID)
		if rec == nil || err != nil {
			add("evidence-dangling", relp, g.ID)
			continue
		}
		if hash != g.Evidence {
			add("record-hash-mismatch", relp, g.ID)
		}
		if rec.RedProof != nil {
			rrel, _ := rel(in.Root, redPath(s, c.Slug, g.ID))
			_, rhash, rerr := LoadRed(s, c.Slug, g.ID)
			if rerr != nil || rhash != *rec.RedProof {
				add("red-hash-mismatch", rrel, g.ID)
			}
		}
	}
	// Header changes after the first evidence record, and a deleted contract.
	if raw, ok := ShowAt(in.Root, in.Base, ContractPath); ok {
		if _, err := os.Lstat(join(in.Root, ContractPath)); errors.Is(err, fs.ErrNotExist) {
			add("contract-deleted", ContractPath, "")
		} else if hasEvidence {
			if bc, err := Parse(raw); err == nil {
				if strings.Join(bc.In, ",") != strings.Join(c.In, ",") || strings.Join(bc.Out, ",") != strings.Join(c.Out, ",") || bc.Base != c.Base || bc.Request != c.Request {
					add("header-changed", ContractPath, "IN/OUT/BASE/REQUEST differ from BASE")
				}
			}
		}
	}
	return out
}

var (
	reSelfRefJS = regexp.MustCompile(`expect\(([^()]+(?:\([^()]*\))?)\)\.(?:toBe|toEqual|toStrictEqual)\(\s*([^()]+(?:\([^()]*\))?)\s*\)`)
	reSelfRefPy = regexp.MustCompile(`(?:assertEqual|assertIs)\(\s*([^,]+?)\s*,\s*([^,)]+?)\s*\)|^\s*assert\s+(\S.*?)\s*==\s*(\S.*?)\s*$`)
)

// guardAssert is the advisory, regex-only (degraded) form of G-ASSERT: the
// assertion-form count decreased in a test file present in both images.
func guardAssert(in GuardInput, entries []DiffEntry) []Finding {
	var out []Finding
	for _, e := range entries {
		if e.Status != "M" && e.Status != "R" {
			continue
		}
		l := langOf(e.Path)
		if l == nil {
			continue
		}
		prePath := e.Path
		if e.OldPath != "" {
			prePath = e.OldPath
		}
		pre, ok := ShowAt(in.Root, in.Base, prePath)
		if !ok {
			continue
		}
		post, _ := workingFile(in.Root, e.Path)
		if !isTestFile(in, e.Path, post) {
			continue
		}
		preA, postA := countAll(l.assert, pre), countAll(l.assert, post)
		if postA < preA {
			out = append(out, Finding{ID: "G-ASSERT", Path: e.Path, Hunk: hunkHash("G-ASSERT", e.Path, itoa(preA), itoa(postA)), Rule: "assertion-count-drop", Pre: preA, Post: postA, Degraded: true, Advisory: true})
		}
	}
	return out
}

// guardHardcode is the advisory self-referential-assertion form of
// G-HARDCODE (experimental, section 11).
func guardHardcode(in GuardInput, entries []DiffEntry) []Finding {
	var out []Finding
	for _, e := range entries {
		if e.Status == "D" {
			continue
		}
		post, ok := workingFile(in.Root, e.Path)
		if !ok || !isTestFile(in, e.Path, post) {
			continue
		}
		for _, ln := range strings.Split(string(post), "\n") {
			var self bool
			if m := reSelfRefJS.FindStringSubmatch(ln); m != nil && strings.TrimSpace(m[1]) == strings.TrimSpace(m[2]) {
				self = true
			}
			if m := reSelfRefPy.FindStringSubmatch(ln); m != nil {
				a, b := m[1], m[2]
				if a == "" {
					a, b = m[3], m[4]
				}
				if strings.TrimSpace(a) == strings.TrimSpace(b) && a != "" {
					self = true
				}
			}
			if self {
				out = append(out, Finding{ID: "G-HARDCODE", Path: e.Path, Hunk: hunkHash("G-HARDCODE", e.Path, strings.TrimSpace(ln)), Rule: "self-referential-assertion", Degraded: true, Advisory: true, Detail: canon.CleanText(strings.TrimSpace(ln))})
			}
		}
	}
	return out
}
