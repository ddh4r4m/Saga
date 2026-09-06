// Package scripts holds the dry run for the owner-run launchers. The
// launchers themselves spend money on a live model; this test exercises
// their logic against a stub harness so a broken launcher is found here
// rather than at the owner's terminal.
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func needTools(t *testing.T) {
	t.Helper()
	for _, b := range []string{"bash", "python3", "git", "go"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s not on PATH", b)
		}
	}
}

// runProbes drives scripts/harness-probes.sh against the stub harness.
func runProbes(t *testing.T, env ...string) (string, int) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "harness-probes.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"DRY_RUN=1",
		"CLAUDE_BIN="+filepath.Join(root, "scripts", "testdata", "stub-claude"),
		"OUT="+out,
	)
	cmd.Env = append(cmd.Env, env...)
	b, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if err != nil {
		if !asExit(err, &ee) {
			t.Fatalf("harness-probes.sh: %v\n%s", err, b)
		}
		code = ee.ExitCode()
	}
	return string(b), code
}

func asExit(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// TestHarnessProbesDryRun: the launcher must work before the owner runs
// it against a live model. The stub answers P9 whichever way the knob
// says, so both verdicts are walked; the stub proves nothing about
// Claude Code itself, only that the launcher reads its own evidence
// correctly.
func TestHarnessProbesDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needTools(t)

	// A harness that honours the JSON deny on a non-zero hook exit.
	out, code := runProbes(t, "STUB_HONOUR_DENY=1")
	for _, want := range []string{"P9 PASS", "P10 hooks_fire PASS", "uninstall --dry-run PASS", "P17 PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("honouring stub: missing %q in:\n%s", want, out)
		}
	}
	if code != 0 {
		t.Errorf("exit %d, want 0 when every probe passes", code)
	}
	// The hook chain really ran: a registered hook is not a firing hook,
	// and this is the distinction P10 exists to make.
	if !strings.Contains(out, "1 tool_call, 1 tool_result") || !strings.Contains(out, "stop claim") {
		t.Errorf("P10 did not observe the whole chain:\n%s", out)
	}

	// A harness that fails open on a non-zero hook exit: the launcher
	// must call that FAIL and say the exit-2 path is the one to use.
	out, code = runProbes(t)
	if !strings.Contains(out, "P9 FAIL") || !strings.Contains(out, "fail-open on exit 1") {
		t.Errorf("failing-open stub: P9 should FAIL:\n%s", out)
	}
	if code != 1 {
		t.Errorf("exit %d, want 1 when a probe fails", code)
	}

	// The proposed harness-facts row is written for saga to read, and the
	// launcher never edits harness-facts itself.
	if !strings.Contains(out, "harness-facts-proposed.md") {
		t.Errorf("no proposed harness-facts row:\n%s", out)
	}
}

// TestLaunchersRefuseInsideAnAgent: both launchers are human acts. The
// smoke launcher's baseline approval is refused inside an agent shell,
// and the probes spend money, so neither may run from a harness session.
func TestLaunchersRefuseInsideAnAgent(t *testing.T) {
	needTools(t)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"harness-probes.sh", "bench-smoke.sh"} {
		cmd := exec.Command("bash", filepath.Join(root, "scripts", script))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CLAUDECODE=1", "OUT="+t.TempDir())
		b, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s ran inside an agent shell:\n%s", script, b)
			continue
		}
		if !strings.Contains(string(b), "plain terminal") {
			t.Errorf("%s: refusal should name the reason:\n%s", script, b)
		}
	}
}
