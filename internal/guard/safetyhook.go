package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogEnv names the file every safety-hook decision is appended to. The
// bench sets it in the generated settings' env block, which Claude Code
// passes to hook processes; without it the hook still decides and simply
// logs nothing.
const LogEnv = "SAGA_GUARD_LOG"

// SafetyDecision is what the safety hook concluded about one tool call.
type SafetyDecision struct {
	// Deny is true only for a hard-deny rule (D1 to D11). An ask verdict
	// is an allow here: this entry point is deny-only, because a hook
	// that stopped work in one arm and not the other would be a
	// treatment, and it runs identically in both.
	Deny bool
	// Rules are the rule ids that fired, and Segment the offending
	// command segment's text for the reason line.
	Rules   []string
	Segment string
	// Err is set when the hook could not decide. The caller allows: a
	// false deny is worse than a missed one here, because the classifier
	// is a safety net over a machine the owner already trusts, not the
	// mechanism under test.
	Err error
}

// Reason is the permissionDecisionReason text for a deny.
func (d SafetyDecision) Reason() string {
	r := "saga guard: " + strings.Join(d.Rules, ",")
	if d.Segment != "" {
		r += ": " + clip(d.Segment, 160)
	}
	return r
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SafetyCheck runs the classifier over one PreToolUse payload in
// deny-only mode: no snapshot, no policy file, nothing read or written
// under .saga. toolName gates it to shell tools; anything else allows
// untouched.
func SafetyCheck(toolName string, toolInput json.RawMessage, cwd string, env []string) SafetyDecision {
	switch toolName {
	case "Bash", "PowerShell":
	default:
		return SafetyDecision{}
	}
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(toolInput, &in); err != nil {
		return SafetyDecision{Err: fmt.Errorf("tool_input: %w", err)}
	}
	if strings.TrimSpace(in.Command) == "" {
		return SafetyDecision{}
	}
	shell := "bash"
	if toolName == "PowerShell" {
		shell = "pwsh"
	}
	d, err := Check(Request{
		Command: in.Command, Shell: shell, Cwd: cwd, Env: env,
		Policy: DefaultPolicy(), RepoRoot: cwd,
	})
	if err != nil {
		// A shell the parser does not implement, or an unparsable
		// command: allow and say so. Failing closed here would deny work
		// on one machine and not another.
		return SafetyDecision{Err: err}
	}
	if d.Decision != Deny {
		return SafetyDecision{}
	}
	out := SafetyDecision{Deny: true, Rules: d.Rules}
	for _, s := range d.Segments {
		if s.Verdict == Deny {
			out.Segment = s.Raw
			if s.Rule != "" && len(out.Rules) == 0 {
				out.Rules = []string{s.Rule}
			}
			break
		}
	}
	if len(out.Rules) == 0 {
		out.Rules = []string{"deny"}
	}
	return out
}

// SafetyLogLine is one appended record. The command itself is never
// written: only its digest, so a log can be published with the archive.
type SafetyLogLine struct {
	TS        string   `json:"ts"`
	SessionID string   `json:"session_id"`
	ToolUseID string   `json:"tool_use_id"`
	Verdict   string   `json:"verdict"`
	Rules     []string `json:"rules"`
	SHA256    string   `json:"sha256"`
	Error     string   `json:"error,omitempty"`
	// LatencyMS is the hook process's own wall time to the byte before
	// its reply. The safety hook runs in both arms on every Bash call, so
	// it is part of the measured overhead of docs/12 commitment 7, and it
	// is the only hook arm A carries. Absent on lines written before the
	// field existed, where it reads as not recorded.
	LatencyMS *int `json:"latency_ms,omitempty"`
}

// LogSafety appends one decision to the file named by SAGA_GUARD_LOG.
// A missing variable is not an error: the hook decides either way.
func LogSafety(path, sessionID, toolUseID, command string, d SafetyDecision) error {
	return LogSafetyAt(path, sessionID, toolUseID, command, d, nil)
}

// LogSafetyAt is LogSafety with the invocation's own wall time. started
// is the process start; nil leaves latency_ms absent.
func LogSafetyAt(path, sessionID, toolUseID, command string, d SafetyDecision, started *time.Time) error {
	if path == "" {
		return nil
	}
	verdict := "allow"
	switch {
	case d.Err != nil:
		verdict = "error"
	case d.Deny:
		verdict = "deny"
	}
	sum := sha256.Sum256([]byte(command))
	line := SafetyLogLine{
		TS: time.Now().UTC().Format(time.RFC3339Nano), SessionID: sessionID, ToolUseID: toolUseID,
		Verdict: verdict, Rules: d.Rules, SHA256: "sha256:" + hex.EncodeToString(sum[:]),
	}
	if line.Rules == nil {
		line.Rules = []string{}
	}
	if d.Err != nil {
		line.Error = d.Err.Error()
	}
	if started != nil && !started.IsZero() {
		ms := int(time.Since(*started) / time.Millisecond)
		line.LatencyMS = &ms
	}
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

// CountDenies reads a safety log and returns the number of deny lines,
// for the run row's guard_denies.
func CountDenies(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var line SafetyLogLine
		if json.Unmarshal([]byte(l), &line) == nil && line.Verdict == "deny" {
			n++
		}
	}
	return n
}

// Latencies reads a safety log and returns the wall time of each
// invocation that recorded one, for the bench's overhead table. A line
// written before latency_ms existed is skipped rather than counted as
// zero.
func Latencies(path string) []int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []int
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var line SafetyLogLine
		if json.Unmarshal([]byte(l), &line) == nil && line.LatencyMS != nil {
			out = append(out, *line.LatencyMS)
		}
	}
	return out
}
