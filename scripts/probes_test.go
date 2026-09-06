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
// verdicts pulls the launcher's own verdict lines out of its output, so
// a dry run's result is readable in `go test -v` without the evidence
// paths that dominate the log.
func verdicts(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "P9 ") || strings.HasPrefix(l, "P10 ") || strings.HasPrefix(l, "P17 ") {
			keep = append(keep, "  "+l)
		}
	}
	return strings.Join(keep, "\n")
}

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
	t.Logf("honouring stub, exit %d:\n%s", code, verdicts(out))
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
	t.Logf("failing-open stub, exit %d:\n%s", code, verdicts(out))
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

// TestProbesRefuseToReadAnEmptyTranscript is the 2026-09-06 defect: the
// owner's run passed a session id that was not a UUID, every claude
// invocation exited on its arguments, every native.jsonl was empty, and
// the launcher still printed one PASS and one FAIL. A verdict read off
// a transcript that does not exist is worse than no verdict, so every
// probe must now degrade to INCONCLUSIVE and quote what the harness
// said. STUB_BAD_SESSION makes the stub refuse whatever id it is given,
// which reproduces the failure whatever the launcher passes.
func TestProbesRefuseToReadAnEmptyTranscript(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needTools(t)
	out, code := runProbes(t, "STUB_BAD_SESSION=1", "STUB_HONOUR_DENY=1")
	t.Logf("refused session ids, exit %d:\n%s", code, verdicts(out))
	for _, want := range []string{"P9 INCONCLUSIVE", "P10 hooks_fire INCONCLUSIVE", "P17 INCONCLUSIVE"} {
		if !strings.Contains(out, want) {
			t.Errorf("a probe read a verdict off an empty transcript: missing %q in:\n%s", want, out)
		}
	}
	// The harness's own first line is quoted, so the owner sees the cause
	// rather than a bare "inconclusive".
	if !strings.Contains(out, "Invalid session ID") {
		t.Errorf("the cause is not quoted:\n%s", out)
	}
	for _, never := range []string{"P9 PASS", "P9 FAIL", "P10 hooks_fire PASS", "P10 hooks_fire FAIL", "P17 PASS"} {
		if strings.Contains(out, never) {
			t.Errorf("%q survived an empty transcript:\n%s", never, out)
		}
	}
	if code != 2 {
		t.Errorf("exit %d, want 2 (inconclusive)", code)
	}
}

// TestLauncherPassesAUUIDSessionID: the stub rejects a non-UUID id
// exactly as the real binary does, so a launcher that builds its own id
// out of a probe name cannot pass a dry run again.
func TestLauncherPassesAUUIDSessionID(t *testing.T) {
	needTools(t)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(root, "scripts", "testdata", "stub-claude")
	// The stub writes its transcript beside CLAUDE_CONFIG_DIR, so it is
	// pointed at a scratch dir: a test must not leave a file in the repo.
	scratch := t.TempDir()
	run := func(id string) ([]byte, error) {
		cmd := exec.Command("bash", stub, "-p", "--session-id", id)
		cmd.Dir = scratch
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+scratch)
		cmd.Stdin = strings.NewReader("")
		return cmd.CombinedOutput()
	}
	// What the launcher used to pass.
	b, err := run("p9-json-exit1-0000-0000-0000-000000000000")
	if err == nil {
		t.Errorf("the stub accepted a non-UUID session id:\n%s", b)
	}
	if !strings.Contains(string(b), "Invalid session ID") {
		t.Errorf("the stub's refusal does not match the real binary's: %s", b)
	}
	// What it passes now.
	b, err = run("3f2a9c14-8b7d-4e21-9f60-1c2d3e4f5a6b")
	if err != nil {
		t.Errorf("the stub rejected a valid UUID: %v\n%s", err, b)
	}
	if !strings.Contains(string(b), `"subtype":"init"`) || !strings.Contains(string(b), `"type":"result"`) {
		t.Errorf("a valid id produced no usable transcript:\n%s", b)
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
