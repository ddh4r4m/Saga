package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/guard"
)

// preToolUse is a claude-code PreToolUse payload for a shell call.
func preToolUse(t *testing.T, cwd, tool, command, toolUseID string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id": "sess-hook", "cwd": cwd, "hook_event_name": "PreToolUse",
		"tool_name": tool, "tool_use_id": toolUseID,
		"tool_input": map[string]any{"command": command},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func guardHookLines(t *testing.T, path string) []guard.SafetyLogLine {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []guard.SafetyLogLine
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var line guard.SafetyLogLine
		if err := json.Unmarshal([]byte(l), &line); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		out = append(out, line)
	}
	return out
}

// TestGuardHookRendersDenyAndAllow: the entry the bench registers in both
// arms speaks the harness's PreToolUse JSON. A hard deny renders the
// documented deny object with the rule id in the reason; everything else
// renders {} and exits 0, because a hook that stopped work would be a
// treatment rather than a safety net (docs/12 row 6).
func TestGuardHookRendersDenyAndAllow(t *testing.T) {
	ws := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "guard.jsonl")
	t.Setenv(guard.LogEnv, logPath)

	// A destructive command is denied, and the reason names the rule.
	out, errs, code := runIn(t, ws, preToolUse(t, ws, "Bash", "rm -rf "+ws, "tu-deny"), "guard", "hook", "claude-code", "PreToolUse")
	if code != 0 {
		t.Fatalf("deny exited %v (stderr %s)", code, errs)
	}
	var m struct {
		Out struct {
			Event  string `json:"hookEventName"`
			Dec    string `json:"permissionDecision"`
			Reason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("deny output %q: %v", out, err)
	}
	if m.Out.Event != "PreToolUse" || m.Out.Dec != "deny" {
		t.Errorf("deny output %q", out)
	}
	if !strings.Contains(m.Out.Reason, "saga guard: D") {
		t.Errorf("reason does not name a rule: %q", m.Out.Reason)
	}

	// Ordinary work passes through untouched.
	for _, cmd := range []string{"go test ./...", "git status", "npm run build"} {
		out, _, code := runIn(t, ws, preToolUse(t, ws, "Bash", cmd, "tu-ok"), "guard", "hook", "claude-code", "PreToolUse")
		if code != 0 || strings.TrimSpace(out) != "{}" {
			t.Errorf("%q: code %v out %q", cmd, code, out)
		}
	}

	// A non-shell tool is never inspected.
	raw, _ := json.Marshal(map[string]any{
		"session_id": "sess-hook", "cwd": ws, "hook_event_name": "PreToolUse",
		"tool_name": "Read", "tool_use_id": "tu-read",
		"tool_input": map[string]any{"file_path": "/etc/passwd"},
	})
	if out, _, code := runIn(t, ws, string(raw), "guard", "hook", "claude-code", "PreToolUse"); code != 0 || strings.TrimSpace(out) != "{}" {
		t.Errorf("Read: code %v out %q", code, out)
	}

	lines := guardHookLines(t, logPath)
	if len(lines) != 5 {
		t.Fatalf("%d log lines, want 5: %+v", len(lines), lines)
	}
	if lines[0].Verdict != "deny" || lines[0].ToolUseID != "tu-deny" || lines[0].SessionID != "sess-hook" {
		t.Errorf("deny line %+v", lines[0])
	}
	if strings.Contains(fmt.Sprint(lines), ws) {
		t.Errorf("a command path reached the log: %+v", lines)
	}
	if n := guard.CountDenies(logPath); n != 1 {
		t.Errorf("CountDenies %d, want 1", n)
	}
}

// TestGuardHookFailsOpen: a payload the hook cannot read allows and logs
// an error. Denying on our own bug would stop the agent in whichever arm
// hit the bug first, which is the asymmetry this hook exists to avoid.
func TestGuardHookFailsOpen(t *testing.T) {
	ws := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "guard.jsonl")
	t.Setenv(guard.LogEnv, logPath)

	for _, payload := range []string{`{"session_id":`, `[]`, ``} {
		out, _, code := runIn(t, ws, payload, "guard", "hook", "claude-code", "PreToolUse")
		if code != 0 || strings.TrimSpace(out) != "{}" {
			t.Errorf("payload %q: code %v out %q", payload, code, out)
		}
	}
	// A well-formed payload for another event is not our business.
	other := `{"session_id":"s","cwd":"` + ws + `","hook_event_name":"Stop"}`
	if out, _, code := runIn(t, ws, other, "guard", "hook", "claude-code", "Stop"); code != 0 || strings.TrimSpace(out) != "{}" {
		t.Errorf("Stop: code %v out %q", code, out)
	}
	// An unparsable command allows with an error rather than a deny.
	if out, _, code := runIn(t, ws, preToolUse(t, ws, "Bash", "echo 'unterminated", "tu-bad"), "guard", "hook", "claude-code", "PreToolUse"); code != 0 || strings.TrimSpace(out) != "{}" {
		t.Errorf("unparsable: code %v out %q", code, out)
	}

	lines := guardHookLines(t, logPath)
	var errs int
	for _, l := range lines {
		if l.Verdict == "deny" {
			t.Errorf("fail-open path denied: %+v", l)
		}
		if l.Verdict == "error" {
			errs++
		}
	}
	if errs == 0 {
		t.Errorf("no error line was logged: %+v", lines)
	}
}

// TestGuardHookUnknownHarness: the entry names the harness it implements
// rather than silently allowing under one it does not.
func TestGuardHookUnknownHarness(t *testing.T) {
	ws := t.TempDir()
	if _, errs, code := runIn(t, ws, "{}", "guard", "hook", "codex", "PreToolUse"); code == 0 {
		t.Errorf("unknown harness exited 0: %s", errs)
	}
	if _, errs, code := runIn(t, ws, "{}", "guard", "hook", "claude-code"); code == 0 {
		t.Errorf("missing event exited 0: %s", errs)
	}
}
