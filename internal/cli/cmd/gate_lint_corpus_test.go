package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
)

// lintWorkspace stages one bench task the way adapter.stageGate does:
// the task's repo as a git workspace, `saga init`, then prompt.md as the
// request and contract.md as the contract. Nothing else about arm B is
// needed to lint, and nothing here touches the committed corpus.
func lintWorkspace(t *testing.T, tk *task.Task) string {
	t.Helper()
	ws := t.TempDir()
	if r, err := filepath.EvalSymlinks(ws); err == nil {
		ws = r
	}
	if out, err := exec.Command("cp", "-R", tk.RepoDir()+"/.", ws+"/").CombinedOutput(); err != nil {
		t.Fatalf("copy %s: %v %s", tk.ID, err, out)
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"}, {"add", "-A"}, {"commit", "-q", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v %s", args, tk.ID, err, out)
		}
	}
	if _, errs, code := runIn(t, ws, "", "init"); code != cli.ExitOK {
		t.Fatalf("saga init for %s: %v %s", tk.ID, code, errs)
	}
	for src, dst := range map[string]string{"prompt.md": "request.md", "contract.md": "contract.md"} {
		raw, err := os.ReadFile(filepath.Join(tk.Dir, src))
		if err != nil {
			t.Fatalf("%s %s: %v", tk.ID, src, err)
		}
		if err := os.WriteFile(filepath.Join(ws, ".saga", dst), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

// TestGateLintOnTheFrozenCorpus (docs/12 row 8): every contract in the
// frozen set lints at exit 0. Lint warnings are advisory by design, so
// the count is reported rather than asserted at zero; L-RED-NONE is
// expected on this corpus, because arm B runs with require_red = false
// and a gate green at baseline cannot show a red.
func TestGateLintOnTheFrozenCorpus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dirs, err := task.Find(filepath.Join("..", "..", "..", "bench", "tasks", "*"))
	if err != nil || len(dirs) == 0 {
		t.Skip("no corpus")
	}
	sort.Strings(dirs)
	if testing.Short() && len(dirs) > 5 {
		// Every eighth task, so the sample spans both languages.
		var sample []string
		for i := 0; i < len(dirs) && len(sample) < 5; i += len(dirs) / 5 {
			sample = append(sample, dirs[i])
		}
		dirs = sample
	}

	byRule := map[string]int{}
	total, linted := 0, 0
	for _, d := range dirs {
		tk, err := task.Load(d)
		if err != nil {
			t.Fatalf("load %s: %v", d, err)
		}
		ws := lintWorkspace(t, tk)
		out, errs, code := runIn(t, ws, "", "gate", "lint", "--json")
		if code != cli.ExitOK {
			t.Errorf("%s: gate lint exited %v: %s%s", tk.ID, code, out, errs)
			continue
		}
		var res struct {
			Warnings []gate.LintWarning `json:"warnings"`
			Count    int                `json:"count"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("%s: lint output %q: %v", tk.ID, out, err)
		}
		linted++
		total += res.Count
		for _, w := range res.Warnings {
			byRule[w.Rule]++
		}
	}
	if linted != len(dirs) {
		t.Errorf("%d of %d contracts linted", linted, len(dirs))
	}
	rules := make([]string, 0, len(byRule))
	for r := range byRule {
		rules = append(rules, r)
	}
	sort.Strings(rules)
	t.Logf("gate lint on %d contracts: exit 0 on all, %d warnings total", linted, total)
	for _, r := range rules {
		t.Logf("  %s: %d", r, byRule[r])
	}
	// Any rule other than L-RED-NONE is a contract worth looking at, so
	// it is named rather than folded into a total.
	for _, r := range rules {
		if r != gate.LintRedNone {
			t.Logf("  note: %s fires on this corpus (%d); advisory, not a failure", r, byRule[r])
		}
	}
}
