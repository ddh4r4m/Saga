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

// TestLaunchersNameTheirTier: the first pilot launch of 2026-09-13
// passed the approval check and the budget guard and was then refused
// inside `bench run` with "the user tier caps at 20 usd", because no
// launcher passed --tier and the runner defaults to `user`. docs/12
// commitment 4 pre-registers the pilot as `dev`. The tier is asserted
// through the provenance head, which is the pasteable record of what a
// run was, and which would have shown `tier: user` on that launch.
//
// The argv `bench run` actually receives is not observable from a test:
// the wrapper builds the real binary and refuses at the preflight, and
// reaching the run line needs a corpus approved for those bytes, which
// is the owner's act. The head and the refusal below are what a test
// can hold.
func TestLaunchersNameTheirTier(t *testing.T) {
	if testing.Short() {
		t.Skip("short: builds the binary")
	}
	needTools(t)
	pilot, _ := runLauncher(t, "bench-pilot.sh", []string{"BUDGET_USD=65"})
	if !strings.Contains(pilot, "tier:      dev") {
		t.Errorf("the pilot head does not name the pre-registered tier:\n%s", tail(pilot))
	}
	dev, _ := runLauncher(t, "bench-dev.sh", []string{"BUDGET_USD=8"}, "1-20")
	if !strings.Contains(dev, "tier:      user") {
		t.Errorf("the dev head does not name its tier:\n%s", tail(dev))
	}
	// TIER reaches the dev wrapper, which pre-registers nothing.
	over, _ := runLauncher(t, "bench-dev.sh", []string{"BUDGET_USD=8", "TIER=publish"}, "1-20")
	if !strings.Contains(over, "tier:      publish") {
		t.Errorf("TIER did not reach the dev head:\n%s", tail(over))
	}
	// It does not reach the pilot, which does pre-register its tier: an
	// environment variable must not be able to run the pilot at a tier
	// docs/12 did not name.
	pinned, _ := runLauncher(t, "bench-pilot.sh", []string{"BUDGET_USD=65", "TIER=publish"})
	if !strings.Contains(pinned, "tier:      dev") {
		t.Errorf("TIER overrode the pre-registered pilot tier:\n%s", tail(pinned))
	}
}

// TestPreflightAppliesTheTierCap: the runner caps the `user` tier at 20
// usd (bench-spec 4.4) and the launcher used to know nothing about it,
// so a pilot-sized budget passed both launcher guards and died inside
// `bench run`. The preflight now applies the same cap, with the same
// words, alongside its other refusals.
func TestPreflightAppliesTheTierCap(t *testing.T) {
	if testing.Short() {
		t.Skip("short: builds the binary")
	}
	needTools(t)
	out, code := runLauncher(t, "bench-dev.sh", []string{"BUDGET_USD=65"}, "1-20")
	if !strings.Contains(out, "the user tier caps at 20 usd") {
		t.Errorf("a 65 usd budget on the user tier was not refused by the launcher:\n%s", tail(out))
	}
	if !strings.Contains(out, "TIER=dev") {
		t.Errorf("the refusal does not say how to proceed:\n%s", tail(out))
	}
	if code == 0 {
		t.Error("the launcher must refuse rather than hand the problem to the runner")
	}
	if !strings.Contains(out, "nothing has been spent") {
		t.Errorf("output:\n%s", tail(out))
	}
	// And the same budget on the pre-registered pilot tier passes this
	// particular guard: the pilot is refused for the corpus, not the cap.
	pilot, _ := runLauncher(t, "bench-pilot.sh", []string{"BUDGET_USD=65"})
	if strings.Contains(pilot, "the user tier caps at 20 usd") {
		t.Errorf("the dev-tier pilot was refused by the user cap:\n%s", tail(pilot))
	}
}

// TestTaskGlobsPartitionTheCorpus: `1-20` and `21-40` must select
// twenty tasks each, share none, and together be the whole frozen
// corpus. `*-002?-*` also matches `0020`, so until 2026-09-13 the
// second batch carried py-0020 as well and selected 21 tasks; the dev
// launcher's own output said "covered 0 of 21 tasks" and nobody read
// it. Fixed after the experiment closed, so the archives that ran the
// old globs are unaffected and record the set they ran.
func TestTaskGlobsPartitionTheCorpus(t *testing.T) {
	needTools(t)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	sel := func(selector string) map[string]bool {
		t.Helper()
		cmd := exec.Command("bash", "-c",
			`ROOT="$1"; . "$ROOT/scripts/bench-common.sh"; task_globs "$2"`, "bash", root, selector)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("task_globs %s: %v", selector, err)
		}
		got := map[string]bool{}
		for _, pattern := range strings.Split(strings.TrimSpace(string(out)), ",") {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				t.Fatalf("glob %q: %v", pattern, err)
			}
			for _, m := range matches {
				// Only real tasks: bench/tasks also holds helper dirs.
				if _, err := os.Stat(filepath.Join(m, "task.toml")); err == nil {
					got[filepath.Base(m)] = true
				}
			}
		}
		return got
	}

	first, second := sel("1-20"), sel("21-40")
	if len(first) != 20 {
		t.Errorf("1-20 selects %d tasks, want 20", len(first))
	}
	if len(second) != 20 {
		t.Errorf("21-40 selects %d tasks, want 20", len(second))
	}
	for id := range first {
		if second[id] {
			t.Errorf("%s is in both batches", id)
		}
	}
	// Together they are the whole corpus `all` selects, so no task can
	// be run twice or missed by running both batches.
	all := sel("all")
	union := map[string]bool{}
	for id := range first {
		union[id] = true
	}
	for id := range second {
		union[id] = true
	}
	if len(union) != len(all) {
		t.Errorf("the two batches cover %d tasks, `all` selects %d", len(union), len(all))
	}
	for id := range all {
		if !union[id] {
			t.Errorf("%s is in neither batch", id)
		}
	}
}
