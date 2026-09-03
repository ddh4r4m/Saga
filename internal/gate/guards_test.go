package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const guardContract = "# Contract: g\n\nIN: src/**, tests/**\nOUT: src/api/**\n\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n"

func findings(t *testing.T, r *repo, advisory bool) []Finding {
	t.Helper()
	l := r.load()
	fs, err := GuardDiff(GuardInput{Root: l.Root, Store: l.Store, Base: l.Base, Contract: l.Contract, Config: l.Config, Advisory: advisory})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func byID(fs []Finding, id string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.ID == id {
			out = append(out, f)
		}
	}
	return out
}

func TestGuardScope(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", guardContract)
	r.write("src/a.js", "a\n")
	r.commit("base")
	r.write("src/b.js", "in scope\n")
	r.write("docs/readme.md", "out of IN\n")
	r.write("src/api/x.js", "matches OUT\n")
	r.write(".saga/evidence/g/G1.json", "{}")
	os.Symlink("a.js", filepath.Join(r.root, "src", "link.js"))
	fs := byID(findings(t, r, false), "G-SCOPE")
	rules := map[string]string{}
	for _, f := range fs {
		rules[f.Path] = f.Rule
	}
	if rules["docs/readme.md"] != "not-in-IN" || rules["src/api/x.js"] != "matches-OUT" || rules["src/link.js"] != "added-symlink" {
		t.Errorf("scope findings: %+v", fs)
	}
	if _, hit := rules["src/b.js"]; hit {
		t.Error("in-scope file flagged")
	}
	if _, hit := rules[".saga/evidence/g/G1.json"]; hit {
		t.Error("checker-written store path flagged by G-SCOPE (G-LEDGER covers it)")
	}
	l := r.load()
	if f := PredictScope(GuardInput{Root: l.Root, Contract: l.Contract, Config: l.Config}, filepath.Join(r.root, "docs", "new.md")); f == nil || f.Rule != "not-in-IN" {
		t.Errorf("predict: %+v", f)
	}
	if f := PredictScope(GuardInput{Root: l.Root, Contract: l.Contract, Config: l.Config}, filepath.Join(r.root, "src", "ok.js")); f != nil {
		t.Errorf("predict in scope: %+v", f)
	}
	// A waiver with the printed hunk hash clears the finding; G-LEDGER cannot be waived.
	var hunk string
	for _, f := range fs {
		if f.Path == "docs/readme.md" {
			hunk = f.Hunk
		}
	}
	r.write(".saga/contract.md", guardContract+"\nWAIVE: G-SCOPE docs/readme.md "+hunk+" the readme update is documentation for the same change and harmless\n")
	for _, f := range byID(findings(t, r, false), "G-SCOPE") {
		if f.Path == "docs/readme.md" && !f.Waived {
			t.Error("waiver not applied")
		}
	}
}

func TestGuardTestDelAndSkip(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", guardContract)
	r.write("tests/a.test.ts", "it('x', () => {});\nit('y', () => {});\n")
	r.write("tests/test_b.py", "def test_one():\n    assert 1 == 1\n\ndef test_two():\n    assert 2 == 2\n")
	r.write("tests/c_test.go", "package x\nfunc TestC(t *testing.T) {}\n")
	r.write("src/helper.ts", "export const h = 1;\n")
	r.commit("base")

	r.remove("tests/a.test.ts")
	r.write("tests/test_b.py", "import pytest\n\n@pytest.mark.skip\ndef test_one():\n    assert 1 == 1\n\ndef test_two():\n    assert 2 == 2\n")
	r.write("tests/c_test.go", "package x\nfunc TestC(t *testing.T) { t.Skip(\"later\") }\n")
	fs := findings(t, r, false)
	td := byID(fs, "G-TESTDEL")
	if len(td) != 1 || td[0].Path != "tests/a.test.ts" || td[0].Rule != "test-file-deleted" || td[0].Pre != 2 {
		t.Errorf("testdel: %+v", td)
	}
	sk := byID(fs, "G-SKIP")
	paths := map[string]Finding{}
	for _, f := range sk {
		paths[f.Path] = f
	}
	if f, ok := paths["tests/test_b.py"]; !ok || f.Rule != "skip-marker-added" || f.Pre != 0 || f.Post != 1 {
		t.Errorf("py skip: %+v", sk)
	}
	if f, ok := paths["tests/c_test.go"]; !ok || f.Post != 1 {
		t.Errorf("go skip: %+v", sk)
	}
	// JS: it.skip and a rename out of the discovery convention.
	r2 := newRepo(t)
	r2.write(".saga/contract.md", guardContract)
	r2.write("tests/a.test.ts", "it('x', () => {});\n")
	r2.write("tests/test_b.py", "def test_one():\n    assert 1\n")
	r2.commit("base")
	r2.write("tests/a.test.ts", "it.skip('x', () => {});\n")
	r2.write("tests/test_b.py", "def check_one():\n    assert 1\n")
	fs = byID(findings(t, r2, false), "G-SKIP")
	rules := map[string]string{}
	for _, f := range fs {
		rules[f.Path] = f.Rule
	}
	if rules["tests/a.test.ts"] != "skip-marker-added" || rules["tests/test_b.py"] != "test-renamed-out-of-discovery" {
		t.Errorf("js/py: %+v", fs)
	}
	// A legitimate rename test to test with the same declarations is clean.
	r3 := newRepo(t)
	r3.write(".saga/contract.md", guardContract)
	r3.write("tests/old.test.ts", "it('x', () => {});\n")
	r3.commit("base")
	r3.git("mv", "tests/old.test.ts", "tests/new.test.ts")
	if fs := findings(t, r3, false); len(byID(fs, "G-TESTDEL")) != 0 || len(byID(fs, "G-SKIP")) != 0 {
		t.Errorf("clean rename flagged: %+v", fs)
	}
}

func TestGuardLedger(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	r.write("marker.txt", "CANARY-DONE\n")
	rep := r.check(CheckOptions{Approve: true, NoRequireRed: true})
	if rep.Exit != 0 {
		t.Fatalf("setup: %d %v", rep.Exit, rep.Lines)
	}
	if fs := byID(findings(t, r, false), "G-LEDGER"); len(fs) != 0 {
		t.Fatalf("checker's own writes flagged: %+v", fs)
	}
	// Tamper with the record: hash no longer matches EVIDENCE:.
	p := filepath.Join(r.root, ".saga", "evidence", "scratch", "G1.json")
	raw, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(raw), `"outcome":"met"`, `"outcome":"met" `, 1)), 0o600)
	fs := byID(findings(t, r, false), "G-LEDGER")
	if len(fs) != 1 || fs[0].Rule != "record-hash-mismatch" || fs[0].Waived {
		t.Errorf("tamper: %+v", fs)
	}
	// Header change after the first evidence record.
	os.WriteFile(p, raw, 0o600)
	r.commit("with evidence")
	r.write(".saga/contract.md", strings.Replace(r.read(".saga/contract.md"), "IN: src/**, marker.txt", "IN: **", 1))
	fs = byID(findings(t, r, false), "G-LEDGER")
	if len(fs) != 1 || fs[0].Rule != "header-changed" {
		t.Errorf("header: %+v", fs)
	}
	// Unknown schema and an unwaivable finding.
	r.write(".saga/red/scratch/forged.json", `{"schema":"saga.gate.red/9"}`)
	if fs := byID(findings(t, r, false), "G-LEDGER"); len(fs) != 2 {
		t.Errorf("unknown schema: %+v", fs)
	}
}

func TestAdvisoryGuards(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", guardContract)
	r.write("tests/a.test.ts", "it('x', () => { expect(a).toBe(1); expect(b).toBe(2); });\n")
	r.commit("base")
	r.write("tests/a.test.ts", "it('x', () => { expect(f(x)).toBe(f(x)); });\n")
	if fs := findings(t, r, false); len(byID(fs, "G-ASSERT")) != 0 || len(byID(fs, "G-HARDCODE")) != 0 {
		t.Error("advisory guards ran without --advisory")
	}
	fs := findings(t, r, true)
	if a := byID(fs, "G-ASSERT"); len(a) != 1 || !a[0].Advisory || !a[0].Degraded || a[0].Pre != 2 || a[0].Post != 1 || a[0].Blocks() {
		t.Errorf("assert: %+v", a)
	}
	if h := byID(fs, "G-HARDCODE"); len(h) != 1 || h[0].Rule != "self-referential-assertion" || h[0].Blocks() {
		t.Errorf("hardcode: %+v", h)
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		pat, p string
		want   bool
	}{
		{"src/**", "src/a/b.ts", true},
		{"src/**", "src", true},
		{"**/*.test.*", "a/b/c.test.ts", true},
		{"**/*.test.*", "c.test.ts", true},
		{"tests/**", "src/tests/x", false},
		{"src/slug.ts", "src/slug.ts", true},
		{"src/*.ts", "src/a/b.ts", false},
		{"**/__tests__/**", "pkg/__tests__/a.js", true},
		{"src/", "src/x", true},
	}
	for _, c := range cases {
		if got := MatchGlob(c.pat, c.p, false); got != c.want {
			t.Errorf("%s ~ %s: %t", c.pat, c.p, got)
		}
	}
	if !MatchGlob("SRC/**", "src/x", true) || MatchGlob("SRC/**", "src/x", false) {
		t.Error("fold")
	}
}
