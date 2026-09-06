package guard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// payload builds a PreToolUse tool_input for a shell command.
func payload(cmd string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"command": cmd})
	return raw
}

// TestSafetyHookOverIncidentFixtures runs the whole incident suite
// through the safety-hook entry rather than through check-cmd, because
// that entry is what the bench actually registers. Every I row must deny
// and every C row must not: a control deny would stop real work in both
// arms, and an escape would leave the owner's machine unprotected on the
// worktree substitute for containers (docs/12 rows 5 and 6).
func TestSafetyHookOverIncidentFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	sb := newSandbox(t)
	var escapes, controlDenies, skipped int
	for _, f := range fixtures {
		if f.Skip != "" {
			skipped++
			continue
		}
		req := sb.request(f)
		// The hook only ever sees a tool name and a tool input; the shell
		// follows the tool, so a fixture for a shell the hook cannot name
		// is exercised through its own shell here only when it is bash.
		tool := "Bash"
		if f.Shell == "pwsh" {
			tool = "PowerShell"
		}
		d := SafetyCheck(tool, payload(req.Command), req.Cwd, req.Env)
		switch f.Expect {
		case "deny":
			if !d.Deny {
				escapes++
				t.Errorf("%s escaped the safety hook: %q", f.ID, req.Command)
				continue
			}
			for _, want := range f.Rules {
				if !hasRule(d.Rules, want) {
					t.Errorf("%s denied but without rule %s (got %v)", f.ID, want, d.Rules)
				}
			}
		default:
			// allow and ask are both allow here: the hook is deny-only.
			if d.Deny {
				controlDenies++
				t.Errorf("%s is a control and was denied by rules %v: %q", f.ID, d.Rules, req.Command)
			}
		}
	}
	t.Logf("safety hook over %d fixtures: %d escapes, %d control denies, %d skipped", len(fixtures), escapes, controlDenies, skipped)
	if escapes != 0 || controlDenies != 0 {
		t.Errorf("escapes %d, control denies %d, both must be zero", escapes, controlDenies)
	}
}

func hasRule(rules []string, want string) bool {
	for _, r := range rules {
		if r == want {
			return true
		}
	}
	return false
}

// TestSafetyHookIsDenyOnly: an ask verdict is an allow at this entry.
// The hook runs in both arms, so anything it stops it stops symmetrically;
// stopping on a maybe would make the safety net a treatment.
func TestSafetyHookIsDenyOnly(t *testing.T) {
	sb := newSandbox(t)
	env := []string{"HOME=" + sb.home, "PATH=/usr/bin:/bin"}
	// A hard deny still denies.
	if d := SafetyCheck("Bash", payload("rm -rf "+sb.home+"/"), sb.repo, env); !d.Deny {
		t.Error("rm -rf of home was not denied")
	}
	// Ordinary work is untouched.
	for _, cmd := range []string{"go test ./...", "git status", "npm test", "ls -la"} {
		if d := SafetyCheck("Bash", payload(cmd), sb.repo, env); d.Deny {
			t.Errorf("%q denied by %v", cmd, d.Rules)
		}
	}
	// A non-shell tool is never inspected.
	raw, _ := json.Marshal(map[string]any{"file_path": "/etc/passwd"})
	if d := SafetyCheck("Read", raw, sb.repo, env); d.Deny || d.Err != nil {
		t.Errorf("a non-shell tool was inspected: %+v", d)
	}
}

// TestSafetyHookFailsOpen: a malformed payload or an unimplemented shell
// allows and records an error, because a false deny in one arm is the
// asymmetry this hook exists to avoid.
func TestSafetyHookFailsOpen(t *testing.T) {
	sb := newSandbox(t)
	env := []string{"HOME=" + sb.home}
	d := SafetyCheck("Bash", json.RawMessage(`{"command":`), sb.repo, env)
	if d.Deny || d.Err == nil {
		t.Errorf("malformed payload: %+v", d)
	}
	d = SafetyCheck("Bash", payload("echo 'unterminated"), sb.repo, env)
	if d.Deny {
		t.Errorf("unparsable command denied: %v", d.Rules)
	}
	// An empty command is a no-op, not an error.
	if d := SafetyCheck("Bash", payload("   "), sb.repo, env); d.Deny || d.Err != nil {
		t.Errorf("empty command: %+v", d)
	}
}

// TestSafetyLogShape: the log carries a digest and never the command,
// so a run's guard log can be published with its archive.
func TestSafetyLogShape(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sub", "guard.jsonl")
	secret := "rm -rf /Users/someone/secret-project"
	if err := LogSafety(logPath, "sess1", "tu1", secret, SafetyDecision{Deny: true, Rules: []string{"D1"}, Segment: secret}); err != nil {
		t.Fatal(err)
	}
	if err := LogSafety(logPath, "sess1", "tu2", "ls", SafetyDecision{}); err != nil {
		t.Fatal(err)
	}
	if err := LogSafety(logPath, "sess1", "tu3", "weird", SafetyDecision{Err: fmt.Errorf("boom")}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-project") || strings.Contains(string(raw), "rm -rf") {
		t.Errorf("the command text reached the log:\n%s", raw)
	}
	var verdicts []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var line SafetyLogLine
		if err := json.Unmarshal([]byte(l), &line); err != nil {
			t.Fatalf("line %q: %v", l, err)
		}
		if line.SHA256 == "" || !strings.HasPrefix(line.SHA256, "sha256:") {
			t.Errorf("no digest on %q", l)
		}
		if line.TS == "" || line.SessionID != "sess1" {
			t.Errorf("line fields: %+v", line)
		}
		verdicts = append(verdicts, line.Verdict)
	}
	sort.Strings(verdicts)
	if strings.Join(verdicts, ",") != "allow,deny,error" {
		t.Errorf("verdicts %v", verdicts)
	}
	if n := CountDenies(logPath); n != 1 {
		t.Errorf("CountDenies %d, want 1", n)
	}
	// No log path configured is not an error: the hook still decides.
	if err := LogSafety("", "s", "t", "c", SafetyDecision{}); err != nil {
		t.Errorf("unset log path: %v", err)
	}
	if n := CountDenies(filepath.Join(dir, "absent.jsonl")); n != 0 {
		t.Errorf("absent log: %d", n)
	}
}

// TestSafetyHookWritesNothingElse: the log is the only file the hook
// touches. In a bare arm .saga is an unreadable sentinel, so a hook that
// tried to read or write it would fail the run rather than quietly
// change the arm.
func TestSafetyHookWritesNothingElse(t *testing.T) {
	sb := newSandbox(t)
	before := treeSnapshot(t, sb.repo)
	env := []string{"HOME=" + sb.home, "PATH=/usr/bin:/bin"}
	for _, cmd := range []string{"rm -rf " + sb.home + "/", "go build ./...", "git push --force origin main"} {
		SafetyCheck("Bash", payload(cmd), sb.repo, env)
	}
	after := treeSnapshot(t, sb.repo)
	if before != after {
		t.Errorf("the hook changed the workspace:\nbefore %s\nafter  %s", before, after)
	}
}

// TestSafetyHookIgnoresRepoPolicy: the hook decides from the built-in
// policy alone. A repo policy file in the workspace must not move its
// verdict, because in the bench the workspace is the agent's and a
// policy it could write would let one arm disarm the safety net. The
// fixture sandbox already ships a .saga/policy.toml, so this asserts the
// decision is the same with that file present and with it removed.
func TestSafetyHookIgnoresRepoPolicy(t *testing.T) {
	sb := newSandbox(t)
	env := []string{"HOME=" + sb.home, "PATH=/usr/bin:/bin"}
	cmds := []string{"rm -rf " + sb.home + "/", "git push --force origin main", "go test ./...", "cat " + sb.home + "/.ssh/id_rsa"}

	policy := filepath.Join(sb.repo, ".saga", "policy.toml")
	if _, err := os.Stat(policy); err != nil {
		t.Skipf("fixture sandbox has no repo policy: %v", err)
	}
	withPolicy := map[string]bool{}
	for _, c := range cmds {
		withPolicy[c] = SafetyCheck("Bash", payload(c), sb.repo, env).Deny
	}

	// A repo policy that tries to switch everything off must change nothing.
	loose := "schema = \"saga.guard.policy/1\"\n[rules]\nD1 = \"allow\"\nD2 = \"allow\"\nD3 = \"allow\"\n"
	if err := os.WriteFile(policy, []byte(loose), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range cmds {
		if got := SafetyCheck("Bash", payload(c), sb.repo, env).Deny; got != withPolicy[c] {
			t.Errorf("a repo policy changed the verdict for %q: %v then %v", c, withPolicy[c], got)
		}
	}

	// And so must its absence.
	if err := os.Remove(policy); err != nil {
		t.Fatal(err)
	}
	for _, c := range cmds {
		if got := SafetyCheck("Bash", payload(c), sb.repo, env).Deny; got != withPolicy[c] {
			t.Errorf("removing the repo policy changed the verdict for %q: %v then %v", c, withPolicy[c], got)
		}
	}
}

func treeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var names []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if strings.HasPrefix(rel, ".git/") {
			return nil
		}
		names = append(names, fmt.Sprintf("%s:%d", rel, fi.Size()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return strings.Join(names, "\n")
}

// TestSafetyHookLatency: the hook runs on every Bash call in both arms,
// so it must not add wall time. harness-facts C23 gives hooks a 60 s
// timeout; this is about the budget, not correctness.
func TestSafetyHookLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	fixtures := loadFixtures(t)
	sb := newSandbox(t)
	var ds []time.Duration
	for _, f := range fixtures {
		if f.Skip != "" || f.Expect == "deny" {
			continue // the controls are the realistic commands
		}
		req := sb.request(f)
		start := time.Now()
		SafetyCheck("Bash", payload(req.Command), req.Cwd, req.Env)
		ds = append(ds, time.Since(start))
	}
	if len(ds) == 0 {
		t.Skip("no control fixtures")
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	p95 := ds[(len(ds)*95)/100]
	if p95 > 50*time.Millisecond {
		t.Errorf("p95 %v over %d controls, want under 50ms", p95, len(ds))
	}
	t.Logf("safety hook over %d control fixtures: p95 %v, max %v", len(ds), p95, ds[len(ds)-1])
}
