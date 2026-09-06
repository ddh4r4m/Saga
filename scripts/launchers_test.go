package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runLauncher drives one wrapper with a scratch SAGA_HOME (so no
// approval on this machine can make the result depend on it) and
// returns its combined output and exit code.
func runLauncher(t *testing.T, script string, env []string, args ...string) (string, int) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{filepath.Join(root, "scripts", script)}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), append([]string{
		"SAGA_HOME=" + t.TempDir(),
		"OUT=" + t.TempDir(),
	}, env...)...)
	b, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if err != nil {
		if !asExit(err, &ee) {
			t.Fatalf("%s: %v\n%s", script, err, b)
		}
		code = ee.ExitCode()
	}
	return string(b), code
}

// TestPilotHeadIsAProvenanceRecord: the head is meant to be pasted
// whole, so every line a reader needs to reproduce the cell is on it,
// and it prints before any refusal rather than instead of one.
func TestPilotHeadIsAProvenanceRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("short: builds the binary")
	}
	needTools(t)
	out, code := runLauncher(t, "bench-pilot.sh", nil)
	for _, want := range []string{"commit:", "binary:    sha256:", "task set:  sha256:", "prereg:    sha256:", "path:      ", "estimate:  ", "budget:", "out:"} {
		if !strings.Contains(out, want) {
			t.Errorf("the head has no %q line:\n%s", want, out)
		}
	}
	// The pre-registered settings are the ones docs/12 fixes, and they
	// are in the head so a paste shows what was run.
	if !strings.Contains(out, "k=5") || !strings.Contains(out, "model claude-opus-5") {
		t.Errorf("the head does not state the pre-registered settings:\n%s", out)
	}
	if code == 0 {
		t.Error("an unapproved corpus with no budget must refuse")
	}
	if !strings.Contains(out, "nothing has been spent") {
		t.Errorf("the refusal does not say that nothing was spent:\n%s", out)
	}
}

// TestLaunchersRefuseOnEveryProblemAtOnce: an operator with an
// unapproved corpus and no budget should learn both in one attempt.
func TestLaunchersRefuseOnEveryProblemAtOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("short: builds the binary")
	}
	needTools(t)
	out, code := runLauncher(t, "bench-pilot.sh", nil)
	if !strings.Contains(out, "not approved for this binary") {
		t.Errorf("the approval problem is not reported:\n%s", out)
	}
	if !strings.Contains(out, "set BUDGET_USD") {
		t.Errorf("the budget problem is not reported in the same run:\n%s", out)
	}
	if !strings.Contains(out, "bash scripts/bench-approve.sh") {
		t.Errorf("the instruction that fixes the approval is missing:\n%s", out)
	}
	if code != 6 {
		t.Errorf("exit %d, want 6 (the approval is the first problem)", code)
	}
	// A budget below the estimate is refused on its own terms.
	out, code = runLauncher(t, "bench-pilot.sh", []string{"BUDGET_USD=1"})
	if !strings.Contains(out, "is below the estimate") {
		t.Errorf("a budget below the estimate was not refused:\n%s", out)
	}
	if code == 0 {
		t.Error("a budget below the estimate must refuse")
	}
}

// TestDevLauncherSelectsItsBatch: `bench-dev.sh 21-40` runs the second
// batch and not the first, which is the only thing the argument does.
func TestDevLauncherSelectsItsBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("short: builds the binary")
	}
	needTools(t)
	first, _ := runLauncher(t, "bench-dev.sh", nil, "1-20")
	second, _ := runLauncher(t, "bench-dev.sh", nil, "21-40")
	// The uncovered-task list names the batch, which is how the
	// selection is observable without a live run.
	if !strings.Contains(first, "py-0006") || strings.Contains(first, "py-0032") {
		t.Errorf("1-20 did not select the first batch:\n%s", tail(first))
	}
	if !strings.Contains(second, "ts-0038") || strings.Contains(second, "py-0006-contact-dedupe not approved") {
		t.Errorf("21-40 did not select the second batch:\n%s", tail(second))
	}
	// The dev run's own settings, not the pilot's.
	if !strings.Contains(second, "k=1") || !strings.Contains(second, "model sonnet") {
		t.Errorf("the dev launcher does not use its own settings:\n%s", tail(second))
	}
}

// TestLaunchersDoNotRunWithoutApproval: the point of the preflight is
// that no archive is created and no model is called.
func TestLaunchersDoNotRunWithoutApproval(t *testing.T) {
	if testing.Short() {
		t.Skip("short: builds the binary")
	}
	needTools(t)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "bench-pilot.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "SAGA_HOME="+t.TempDir(), "OUT="+out, "BUDGET_USD=1000")
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("the pilot ran without an approved corpus:\n%s", b)
	}
	for _, name := range []string{"archive", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s was created although the preflight refused", name)
		}
	}
	if !strings.Contains(string(b), "nothing has been spent") {
		t.Errorf("output:\n%s", b)
	}
}

// tail is the last few lines of an output, for a readable failure.
func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	return strings.Join(lines, "\n")
}
