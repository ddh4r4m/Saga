package task

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/cli"
)

func needRunners(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"git", "bash", "node", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

// TestVerifyCorpus is the integration test: every task in bench/tasks
// passes its own red proof through the Go verifier.
func TestVerifyCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	for _, dir := range corpus(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			res := Verify(context.Background(), dir, VerifyOptions{})
			if !res.OK() {
				t.Errorf("verify failed:\n%s", res.Text())
			}
			if res.Hash == "" || !strings.HasPrefix(res.Hash, "sha256:") {
				t.Errorf("no content hash")
			}
		})
	}
}

func TestVerifyStaticCorpus(t *testing.T) {
	for _, dir := range corpus(t) {
		res := Verify(context.Background(), dir, VerifyOptions{Static: true})
		if !res.OK() {
			t.Errorf("%s static checks failed:\n%s", filepath.Base(dir), res.Text())
		}
	}
}

// TestVerifyDefective covers the section 10.2 defective-task corpus in
// miniature: each defect is rejected with the section 2.4 exit code.
func TestVerifyDefective(t *testing.T) {
	needRunners(t)
	goldPatch := "# canary: deadbeefcafef00d\ndiff --git a/src/a.txt b/src/a.txt\n--- a/src/a.txt\n+++ b/src/a.txt\n@@ -1 +1 @@\n-broken\n+fixed\n"
	brokenPatch := "# canary: deadbeefcafef00d\ndiff --git a/src/a.txt b/src/a.txt\n--- a/src/a.txt\n+++ b/src/a.txt\n@@ -1 +1 @@\n-broken\n+still broken\n"
	valid := map[string]string{"controls/gold.patch": goldPatch, "controls/broken-1.patch": brokenPatch}
	stamp := func(dir string) {
		tk, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		h, _ := tk.SnapshotHash()
		raw, _ := os.ReadFile(filepath.Join(dir, "task.toml"))
		out := strings.Replace(string(raw), "sha256:0000000000000000000000000000000000000000000000000000000000000000", h, 1)
		os.WriteFile(filepath.Join(dir, "task.toml"), []byte(out), 0o644)
	}
	run := func(edits map[string]string) *VerifyResult {
		all := map[string]string{}
		for k, v := range valid {
			all[k] = v
		}
		for k, v := range edits {
			all[k] = v
		}
		dir := writeTask(t, all)
		stamp(dir)
		return Verify(context.Background(), dir, VerifyOptions{})
	}
	if res := run(nil); !res.OK() {
		t.Fatalf("valid twin rejected:\n%s", res.Text())
	}
	cases := []struct {
		name  string
		edits map[string]string
		want  cli.Code
		check string
	}{
		{"oracle passes on baseline", map[string]string{"oracle/run.sh": "# canary: deadbeefcafef00d\necho t1 PASS; exit 0\n"}, cli.ExitFinding, "baseline-red"},
		{"gold fails", map[string]string{"controls/gold.patch": brokenPatch}, cli.ExitFinding, "gold-green"},
		{"broken passes", map[string]string{"controls/broken-1.patch": goldPatch}, cli.ExitFinding, "broken-1-red"},
		{"gold line in prompt", map[string]string{"prompt.md": "make src/a.txt read: fixed // please change the file so that it reads exactly this\n", "controls/gold.patch": "# canary: deadbeefcafef00d\ndiff --git a/src/a.txt b/src/a.txt\n--- a/src/a.txt\n+++ b/src/a.txt\n@@ -1 +1 @@\n-broken\n+fixed // please change the file so that it reads exactly this\n"}, cli.ExitContamination, "leak"},
		{"canary in prompt", map[string]string{"prompt.md": "fix it deadbeefcafef00d\n"}, cli.ExitContamination, "canary"},
		{"canary missing from setup", map[string]string{"setup.sh": "exit 0\n"}, cli.ExitContamination, "canary"},
		{"nondeterministic oracle", map[string]string{"oracle/run.sh": "# canary: deadbeefcafef00d\necho \"t$RANDOM PASS\"; exit 0\n", "task.toml": strings.Replace(mustRead(t, writeTask(t, nil), "task.toml"), "baseline_must_fail = true", "baseline_must_fail = false", 1)}, cli.ExitFinding, "determinism"},
		{"snapshot hash wrong", map[string]string{"task.toml": strings.Replace(mustRead(t, writeTask(t, nil), "task.toml"), "sha256:0000000000000000000000000000000000000000000000000000000000000000", "sha256:1111111111111111111111111111111111111111111111111111111111111111", 1)}, cli.ExitIntegrity, "snapshot"},
	}
	for _, c := range cases {
		res := run(c.edits)
		if res.Code != c.want {
			t.Errorf("%s: code %v, want %v\n%s", c.name, res.Code, c.want, res.Text())
			continue
		}
		found := false
		for _, ch := range res.Checks {
			if ch.Name == c.check && !ch.OK {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: check %s did not fail\n%s", c.name, c.check, res.Text())
		}
	}
}

func mustRead(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
