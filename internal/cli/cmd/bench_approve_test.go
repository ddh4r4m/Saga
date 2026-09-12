package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
)

// approveFixture builds a private one-task corpus with its own freeze
// file, so the committed corpus is never touched and the store is keyed
// by a hash no other test shares.
func approveFixture(t *testing.T) (corpusGlob, sagaBin, home string) {
	t.Helper()
	needCorpusTools(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	src := filepath.Join(repo, "bench", "tasks", "ts-0001-slug-collapse")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("task not present: %v", err)
	}
	root := t.TempDir()
	corpus := filepath.Join(root, "tasks")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := execCopy(src, corpus); err != nil {
		t.Fatalf("copy: %v %s", err, out)
	}
	corpusGlob = filepath.Join(corpus, "*")
	if _, errs, code := runIn(t, repo, "", "bench", "taskset", corpusGlob, "--write", filepath.Join(corpus, run.FrozenName)); code != cli.ExitOK {
		t.Fatalf("taskset: %v %s", code, errs)
	}
	// A saga that answers --version, records its argv and reports one
	// unapproved gate until it has "approved" it, which is enough to
	// drive the command's own logic without a real gate run.
	sagaBin = filepath.Join(root, "saga")
	home = filepath.Join(root, "home")
	return corpusGlob, sagaBin, home
}

func execCopy(src, dst string) ([]byte, error) {
	return exec.Command("cp", "-R", src, dst+"/").CombinedOutput()
}

// TestApproveCorpusRefusesUnderAnAgentShell (ADR 0010 decision 2):
// approving is the same human act `gate check --approve` is, refused for
// the same reason and with the same words. --check writes nothing and is
// allowed, which is what lets the launcher ask the question.
func TestApproveCorpusRefusesUnderAnAgentShell(t *testing.T) {
	corpusGlob, _, home := approveFixture(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	t.Setenv("SAGA_HOME", home)
	t.Setenv("SAGA_AGENT_SHELL", "1")

	_, errs, code := runIn(t, repo, "", "bench", "approve-corpus", corpusGlob)
	if code != cli.ExitRefusal {
		t.Errorf("approve-corpus under an agent shell exited %v, want %v: %s", code, cli.ExitRefusal, errs)
	}
	if !strings.Contains(errs, "human act") || !strings.Contains(errs, "SAGA_AGENT_SHELL") {
		t.Errorf("the refusal does not read like gate's own: %s", errs)
	}
	// --check writes nothing, so it is not a human act and is allowed to
	// report. It exits non-zero because nothing is approved yet.
	out, _, code := runIn(t, repo, "", "bench", "approve-corpus", "--check", corpusGlob)
	if code == cli.ExitRefusal {
		t.Errorf("--check was refused although it approves nothing: %s", out)
	}
}

// TestApproveCorpusRefusesAnUnfrozenSet: the store is keyed by the
// frozen task set, so approving a corpus that does not match its own
// freeze file would write records for a set nobody named.
func TestApproveCorpusRefusesAnUnfrozenSet(t *testing.T) {
	corpusGlob, _, home := approveFixture(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	t.Setenv("SAGA_HOME", home)
	dir := filepath.Dir(corpusGlob)
	prompt := filepath.Join(dir, "ts-0001-slug-collapse", "prompt.md")
	body, err := os.ReadFile(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, append(body, []byte("\nAn edit after the freeze.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errs, code := runIn(t, repo, "", "bench", "approve-corpus", "--check", corpusGlob)
	if code != cli.ExitIntegrity {
		t.Errorf("an unfrozen set exited %v, want %v: %s", code, cli.ExitIntegrity, errs)
	}
}

// TestCorpusStoreIsOutsideEveryWorkspaceAnd0700 (ADR 0010 decision 3):
// the agent must be unable to read or write the records that decide
// whether its own baseline was approved.
func TestCorpusStoreIsOutsideEveryWorkspaceAnd0700(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SAGA_HOME", home)
	set := "sha256:" + strings.Repeat("ab", 32)
	dir, err := adapter.CorpusStoreDir(set)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dir, filepath.Join(home, "bench", "approved")) {
		t.Errorf("store %s is not under the bench home", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("store mode %v, want 0700", fi.Mode().Perm())
	}
	// A hash that is not one is refused rather than making a directory
	// named after whatever was passed.
	for _, bad := range []string{"", "sha256:short", "../../etc"} {
		if _, err := adapter.CorpusStoreDir(bad); err == nil {
			t.Errorf("%q was accepted as a task-set hash", bad)
		}
	}
	// The env a gate arm runs with names this store and not the
	// operator's own ~/.saga/approved.
	c := &adapter.ClaudeCode{SagaBinary: "/nonexistent/saga", CorpusStore: dir}
	env := strings.Join(c.Env(t.TempDir()), "\n")
	if strings.Contains(env, gate.ApprovalEnv+"="+filepath.Join(home, ".saga", "approved")) {
		t.Errorf("a run pointed at the operator's personal approval store:\n%s", env)
	}
}

// TestApproveCorpusCheckKeysOnTheFrozenSetLikeTheRun is the preflight
// half of the 2026-09-13 defect: `bench-smoke`'s `--check` must name the
// same store the run will open. It does now because both call
// run.CorpusKey; before, `--check` read the freeze file's set line and
// the run hashed the tasks it had selected, so a batch of 20 out of the
// frozen 40 passed the preflight and then found an empty store.
func TestApproveCorpusCheckKeysOnTheFrozenSetLikeTheRun(t *testing.T) {
	needCorpusTools(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	root := t.TempDir()
	corpus := filepath.Join(root, "tasks")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ts-0001-slug-collapse", "py-0006-contact-dedupe"} {
		src := filepath.Join(repo, "bench", "tasks", id)
		if _, err := os.Stat(src); err != nil {
			t.Skipf("task %s not present: %v", id, err)
		}
		if out, err := execCopy(src, corpus); err != nil {
			t.Fatalf("copy %s: %v %s", id, err, out)
		}
	}
	all := filepath.Join(corpus, "*")
	one := filepath.Join(corpus, "ts-0001-slug-collapse")
	if _, errs, code := runIn(t, repo, "", "bench", "taskset", all, "--write", filepath.Join(corpus, run.FrozenName)); code != cli.ExitOK {
		t.Fatalf("taskset: %v %s", code, errs)
	}
	t.Setenv("SAGA_HOME", filepath.Join(root, "home"))

	frozen, err := run.ReadFrozen(filepath.Join(corpus, run.FrozenName))
	if err != nil {
		t.Fatal(err)
	}
	want := short16(frozen.Set)

	// The preflight over one task of the two.
	out, _, _ := runIn(t, repo, "", "bench", "approve-corpus", "--check", one)
	if !strings.Contains(out, "task set "+want) {
		t.Errorf("--check over a subset names %q; the frozen set is %s", lastLine(out), want)
	}
	// And over the whole set, the same name: the key does not move with
	// the selection.
	outAll, _, _ := runIn(t, repo, "", "bench", "approve-corpus", "--check", all)
	if !strings.Contains(outAll, "task set "+want) {
		t.Errorf("--check over the whole set names %q; the frozen set is %s", lastLine(outAll), want)
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
