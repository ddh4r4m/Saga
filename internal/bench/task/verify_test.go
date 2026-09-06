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
	// Every oracle name the leak scan exempted, across the corpus. The
	// exemption is never silent: it is listed per task in the verify
	// result and in the text output, so a reviewer sees on every run what
	// was waived (bench-spec 2.4).
	shadowed := map[string][]string{}
	for _, dir := range corpus(t) {
		res := Verify(context.Background(), dir, VerifyOptions{Static: true})
		if !res.OK() {
			t.Errorf("%s static checks failed:\n%s", filepath.Base(dir), res.Text())
		}
		for _, sn := range res.ShadowedNames {
			shadowed[res.Task] = append(shadowed[res.Task], sn.Oracle+" <- "+sn.Repo)
			if !strings.Contains(res.Text(), sn.Oracle+" shadowed by "+sn.Repo) {
				t.Errorf("%s: exemption %s not in the output:\n%s", res.Task, sn.Oracle, res.Text())
			}
		}
	}
	// The two the corpus actually has: an oracle fixture reusing the name
	// of a file the agent can already see in its workspace.
	want := map[string]string{
		"py-0010-ledger-fx-rounding": "fixture-bad/controls.csv <- fixtures/controls.csv",
		"ts-0004-buried-build-error": "fixture-schema/samples.json <- schema/samples.json",
	}
	for task, one := range want {
		if !strings.Contains(strings.Join(shadowed[task], "; "), one) {
			t.Errorf("%s: shadowed names %v, want %q", task, shadowed[task], one)
		}
	}
	for task, names := range shadowed {
		t.Logf("%s exempt: %s", task, strings.Join(names, "; "))
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
	// Valid twin: a contract whose FROM: span quotes prompt.md. Spans
	// quote the request, never gold.patch, so they cannot trip the gold
	// rule (bench-spec 2.4).
	request := "Make src/a.txt hold the corrected value instead of the placeholder.\n"
	if res := run(map[string]string{
		"prompt.md":   request,
		"contract.md": "<!-- canary: deadbeefcafef00d -->\nIN: src/**\nOUT: docs/**\nFROM: R1 \"" + strings.TrimSpace(request) + "\"\n",
	}); !res.OK() {
		t.Errorf("a contract quoting prompt.md must pass:\n%s", res.Text())
	}
	// Valid twin: an oracle name the workspace already carries is exempt,
	// and the exemption is disclosed rather than silent.
	res := run(map[string]string{
		"oracle/shared.json":   "{\"canary\": \"deadbeefcafef00d\"}\n",
		"repo/src/shared.json": "{}\n",
		"contract.md":          "<!-- canary: deadbeefcafef00d -->\nIN: src/**\nOUT: docs/**\nNOTE: see shared.json\n",
	})
	if !res.OK() {
		t.Errorf("a shadowed oracle name must be exempt:\n%s", res.Text())
	}
	if len(res.ShadowedNames) != 1 || res.ShadowedNames[0].Oracle != "shared.json" || res.ShadowedNames[0].Repo != "src/shared.json" {
		t.Errorf("shadowed_names %+v", res.ShadowedNames)
	}
	if !strings.Contains(res.Text(), "shared.json shadowed by src/shared.json") {
		t.Errorf("the exemption must show in the output:\n%s", res.Text())
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
		// The contract is staged into an arm that has gate, so it is
		// agent-visible and the leak scan reads it beside prompt.md
		// (docs/12 row 12).
		{"gold line in contract", map[string]string{
			"contract.md":         "<!-- canary: deadbeefcafef00d -->\nIN: src/**\nOUT: docs/**\nNOTE: the file must read fixed // please change the file so that it reads exactly this\n",
			"controls/gold.patch": "# canary: deadbeefcafef00d\ndiff --git a/src/a.txt b/src/a.txt\n--- a/src/a.txt\n+++ b/src/a.txt\n@@ -1 +1 @@\n-broken\n+fixed // please change the file so that it reads exactly this\n",
		}, cli.ExitContamination, "leak"},
		{"oracle name in contract", map[string]string{
			"oracle/hidden.json": "{\"canary\": \"deadbeefcafef00d\"}\n",
			"contract.md":        "<!-- canary: deadbeefcafef00d -->\nIN: src/**\nOUT: docs/**\nNOTE: see hidden.json\n",
		}, cli.ExitContamination, "leak"},
		// The exemption is by file name and only when the workspace holds
		// that exact name. A shared directory name does not widen it.
		{"oracle name in contract, only the directory is shared", map[string]string{
			"oracle/fixtures/hidden2.json": "{\"canary\": \"deadbeefcafef00d\"}\n",
			"repo/fixtures/other.txt":      "other\n",
			"contract.md":                  "<!-- canary: deadbeefcafef00d -->\nIN: src/**\nOUT: docs/**\nNOTE: see hidden2.json\n",
		}, cli.ExitContamination, "leak"},
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
