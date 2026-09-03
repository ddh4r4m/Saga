package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// corpus is the committed bench/tasks set: tasks still being authored
// in the working tree (untracked) are not part of the red proof yet.
func corpus(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "bench", "tasks")
	dirs, err := Find(root)
	if err != nil || len(dirs) == 0 {
		t.Skip("bench/tasks not found")
	}
	if out, err := exec.Command("git", "-C", root, "ls-tree", "--name-only", "HEAD", ".").Output(); err == nil {
		tracked := map[string]bool{}
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			tracked[filepath.Base(l)] = true
		}
		kept := dirs[:0]
		for _, d := range dirs {
			if tracked[filepath.Base(d)] {
				kept = append(kept, d)
			}
		}
		dirs = kept
	}
	sort.Strings(dirs)
	return dirs
}

func TestLoadCorpus(t *testing.T) {
	for _, dir := range corpus(t) {
		tk, err := Load(dir)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(dir), err)
			continue
		}
		h, err := tk.SnapshotHash()
		if err != nil || h != tk.Repo.Snapshot {
			t.Errorf("%s: snapshot %s, computed %s (%v)", tk.ID, tk.Repo.Snapshot, h, err)
		}
		if tk.WallLimit() <= 0 || tk.CostCap() <= 0 || tk.MaxTurns() != DefaultMaxTurns {
			t.Errorf("%s: limits %v %v %d", tk.ID, tk.WallLimit(), tk.CostCap(), tk.MaxTurns())
		}
		_, broken, _ := tk.Controls()
		if len(broken) == 0 {
			t.Errorf("%s: no broken patch", tk.ID)
		}
	}
}

func writeTask(t *testing.T, edits map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ts-9999-fixture")
	os.MkdirAll(filepath.Join(dir, "repo", "src"), 0o755)
	os.MkdirAll(filepath.Join(dir, "oracle"), 0o755)
	os.MkdirAll(filepath.Join(dir, "controls"), 0o755)
	files := map[string]string{
		"task.toml": `# canary: deadbeefcafef00d
schema = "saga.bench.task/1"
id = "ts-9999-fixture"
language = "typescript"
size = "S"
expected_minutes = 1
cost_hint_usd = 0.01
created = 2026-09-03
source = { kind = "synthetic", ref = "x", note = "y" }
tags = ["plain"]
[repo]
kind = "snapshot"
path = "repo"
snapshot = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
[env]
image = "unpinned:local"
setup = "setup.sh"
network = "offline"
timeout_multiplier = 3
[oracle]
run = "oracle/run.sh"
baseline_must_fail = true
`,
		"prompt.md":               "fix it\n",
		"contract.md":             "<!-- canary: deadbeefcafef00d -->\nIN: src/**\nOUT: docs/**\n",
		"CANARY":                  "deadbeefcafef00d\n",
		"setup.sh":                "# canary: deadbeefcafef00d\nexit 0\n",
		"oracle/run.sh":           "# canary: deadbeefcafef00d\ngrep -q fixed src/a.txt && { echo t1 PASS; exit 0; }; echo t1 FAIL; exit 1\n",
		"controls/gold.patch":     "# canary: deadbeefcafef00d\n",
		"controls/broken-1.patch": "# canary: deadbeefcafef00d\n",
		"repo/src/a.txt":          "broken\n",
	}
	for k, v := range edits {
		files[k] = v
	}
	for name, body := range files {
		if body == "<delete>" {
			os.Remove(filepath.Join(dir, name))
			continue
		}
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]map[string]string{
		"missing gold":   {"controls/gold.patch": "<delete>"},
		"missing broken": {"controls/broken-1.patch": "<delete>"},
		"missing canary": {"CANARY": "<delete>"},
		"bad size":       {"task.toml": "schema = \"saga.bench.task/1\"\nid = \"ts-9999-fixture\"\nlanguage = \"typescript\"\nsize = \"XXL\"\nexpected_minutes = 1\ncost_hint_usd = 0.01\ncreated = 2026-09-03\nsource = { kind = \"synthetic\" }\ntags = [\"plain\"]\n[repo]\nkind = \"snapshot\"\nsnapshot = \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\n[env]\nimage = \"x\"\nnetwork = \"offline\"\ntimeout_multiplier = 3\n"},
		"unknown field":  {"task.toml": "schema = \"saga.bench.task/1\"\nid = \"ts-9999-fixture\"\nlanguage = \"typescript\"\nsize = \"S\"\nexpected_minutes = 1\ncost_hint_usd = 0.01\ncreated = 2026-09-03\nsource = { kind = \"synthetic\" }\ntags = [\"plain\"]\nbogus = 1\n[repo]\nkind = \"snapshot\"\nsnapshot = \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\n[env]\nimage = \"x\"\nnetwork = \"offline\"\ntimeout_multiplier = 3\n"},
		"no primary tag": {"task.toml": "schema = \"saga.bench.task/1\"\nid = \"ts-9999-fixture\"\nlanguage = \"typescript\"\nsize = \"S\"\nexpected_minutes = 1\ncost_hint_usd = 0.01\ncreated = 2026-09-03\nsource = { kind = \"synthetic\" }\ntags = [\"mutation\"]\n[repo]\nkind = \"snapshot\"\nsnapshot = \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\n[env]\nimage = \"x\"\nnetwork = \"offline\"\ntimeout_multiplier = 3\n"},
	}
	for name, edits := range cases {
		dir := writeTask(t, edits)
		if _, err := Load(dir); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	if _, err := Load(writeTask(t, nil)); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"src/**", "src/a/b.ts", true},
		{"src/**", "src/a.ts", true},
		{"src/**", "test/a.ts", false},
		{"src/legacy/**", "src/legacy/report.ts", true},
		{"src/money.ts", "src/money.ts", true},
		{"src/money.ts", "src/money.tsx", false},
		{"**/*.test.ts", "test/a.test.ts", true},
		{"**/*.test.ts", "a.test.ts", true},
		{"test/*.ts", "test/sub/a.ts", false},
		{"package.json", "package.json", true},
		{"src/legacy", "src/legacy/x.ts", true},
	}
	for _, c := range cases {
		if got := GlobMatch(c.pattern, c.path); got != c.want {
			t.Errorf("GlobMatch(%q, %q) = %v", c.pattern, c.path, got)
		}
	}
	if InScope([]string{"src/**"}, []string{"src/legacy/**"}, "src/legacy/a.ts") || !InScope([]string{"src/**"}, nil, "src/a.ts") || InScope([]string{"src/**"}, nil, "package.json") {
		t.Error("InScope")
	}
}

func TestIsTestPath(t *testing.T) {
	for _, p := range []string{"test/slug.test.ts", "tests/test_merge.py", "pkg/a_test.go", "src/__tests__/x.js", "tests/__init__.py"} {
		if !IsTestPath(p) {
			t.Errorf("%s not a test path", p)
		}
	}
	for _, p := range []string{"src/slug.ts", "contacts/merge.py", "README.md", "scripts/build.mjs"} {
		if IsTestPath(p) {
			t.Errorf("%s is a test path", p)
		}
	}
}
