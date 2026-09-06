package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
	schemapkg "github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

func runIn(t *testing.T, cwd, stdin string, args ...string) (string, string, cli.Code) {
	t.Helper()
	var out, errb bytes.Buffer
	// Started is what Main sets from the process start; the hook entries
	// measure their own wall time from it.
	app := &App{Version: "test", Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb, Cwd: cwd, Started: time.Now()}
	err := app.run(args)
	if err != nil {
		fmt.Fprintf(&errb, "saga: %v\n", err)
	}
	return out.String(), errb.String(), cli.CodeOf(err)
}

func hookJSON(t *testing.T, cwd, event, payload string) (map[string]any, cli.Code) {
	t.Helper()
	out, errs, code := runIn(t, cwd, payload, "hook", "claude-code", event)
	if errs != "" {
		t.Logf("%s stderr: %s", event, strings.TrimSpace(errs))
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("%s: stdout not one line: %q (stderr %s)", event, out, errs)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("%s: stdout not JSON: %q", event, out)
	}
	return m, code
}

func TestEndToEndClaudeCodeSession(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	if _, _, code := runIn(t, root, "", "init"); code != cli.ExitOK {
		t.Fatal("init")
	}
	if _, _, code := runIn(t, root, "", "init"); code != cli.ExitOK {
		t.Fatal("init twice")
	}
	// Limit the budget so the hard stop is reachable with the fixture.
	if _, errs, code := runIn(t, root, "", "trace", "budget", "--session-usd", "0.10"); code != cli.ExitOK {
		t.Fatalf("budget: %s", errs)
	}
	transcript := filepath.Join(root, "t.jsonl")
	fixture, _ := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "trace", "claude-transcript.jsonl"))
	// Start with an empty transcript; grow it as the session goes.
	os.WriteFile(transcript, nil, 0o600)
	common := `"session_id":"e2e-1","transcript_path":"` + transcript + `","cwd":"` + root + `"`

	m, code := hookJSON(t, root, "SessionStart", `{`+common+`,"hook_event_name":"SessionStart","source":"startup"}`)
	if len(m) != 0 || code != cli.ExitOK {
		t.Fatalf("SessionStart %v %v", m, code)
	}
	m, code = hookJSON(t, root, "UserPromptSubmit", `{`+common+`,"hook_event_name":"UserPromptSubmit","prompt":"add a retry helper, key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789"}`)
	if len(m) != 0 || code != cli.ExitOK {
		t.Fatalf("UserPromptSubmit %v %v", m, code)
	}
	m, code = hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_use_id":"toolu_01"}`)
	if len(m) != 0 || code != cli.ExitOK {
		t.Fatalf("PreToolUse %v %v", m, code)
	}
	// Forbidden command is denied by string match (contracts section 8).
	m, code = hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"saga trace budget --raise session"},"tool_use_id":"toolu_02"}`)
	hso, _ := m["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "deny" || code != cli.ExitRefusal {
		t.Fatalf("forbidden: %v %v", m, code)
	}
	m, code = hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"`+root+`/.saga/manifest.json"},"tool_use_id":"toolu_03"}`)
	hso, _ = m["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "deny" {
		t.Fatalf("forbidden edit: %v %v", m, code)
	}
	// First two model calls land in the transcript before PostToolUse.
	lines := strings.Split(strings.TrimRight(string(fixture), "\n"), "\n")
	os.WriteFile(transcript, []byte(strings.Join(lines[:4], "\n")+"\n"), 0o600)
	m, code = hookJSON(t, root, "PostToolUse", `{`+common+`,"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"stdout":"ok","stderr":"","interrupted":false,"isImage":false},"tool_use_id":"toolu_01","duration_ms":840}`)
	if code != cli.ExitOK {
		t.Fatalf("PostToolUse %v %v", m, code)
	}
	// The transcript so far holds one call: 0.0091 + 0.0706 + 0.091 + 0.0103 = 0.181 usd, above the 0.10 budget:
	// the hard crossing is reported as feedback here and denies the next tool call.
	hso, _ = m["hookSpecificOutput"].(map[string]any)
	if ctx, _ := hso["additionalContext"].(string); !strings.Contains(ctx, "saga trace: saga trace: budget session exhausted") && !strings.Contains(ctx, "budget session exhausted") {
		t.Fatalf("expected hard budget feedback, got %v", m)
	}
	m, code = hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"x"},"tool_use_id":"toolu_04"}`)
	hso, _ = m["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "deny" || !strings.Contains(hso["permissionDecisionReason"].(string), "budget session exhausted") || code != cli.ExitFinding {
		t.Fatalf("budget deny: %v %v", m, code)
	}
	os.WriteFile(transcript, fixture, 0o600)
	m, code = hookJSON(t, root, "Stop", `{`+common+`,"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Implemented the retry helper.\nDONE"}`)
	if m["decision"] != "block" || code != cli.ExitFinding {
		t.Fatalf("Stop under exhausted budget should block: %v %v", m, code)
	}
	for _, ev := range []string{"PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "SessionEnd"} {
		payload := `{` + common + `,"hook_event_name":"` + ev + `","trigger":"auto","compact_summary":"s","agent_id":"ag1","agent_type":"Explore","last_assistant_message":"pong","reason":"exit"}`
		if m, code := hookJSON(t, root, ev, payload); len(m) != 0 || code != cli.ExitOK {
			t.Fatalf("%s: %v %v", ev, m, code)
		}
	}

	// The record: chain verifies, events are the expected types, secrets masked, ledger priced.
	out, errs, code := runIn(t, root, "", "trace", "verify", "e2e-1")
	if code != cli.ExitOK {
		t.Fatalf("verify: %s %s", out, errs)
	}
	events, err := trace.ReadAll(trace.SessionDir(nil0(root), "e2e-1"))
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	want := "session turn tool_call tool_call tool_call tool_result model_call budget tool_call model_call model_call turn gate compaction compaction subagent subagent gate session"
	if got := strings.Join(types, " "); got != want {
		t.Errorf("event sequence\n got %s\nwant %s", got, want)
	}
	raw, _ := os.ReadFile(filepath.Join(trace.SessionDir(nil0(root), "e2e-1"), "events.000001.jsonl"))
	if strings.Contains(string(raw), "sk-ant-") {
		t.Error("secret reached the event log")
	}
	if events[1].MaskedCount == nil || *events[1].MaskedCount != 1 {
		t.Errorf("masked_count on the prompt turn: %v", events[1].MaskedCount)
	}
	if events[5].Body["for_seq"] != float64(3) {
		t.Errorf("tool_result for_seq %v", events[5].Body["for_seq"])
	}
	if events[3].Body["decision"] != "deny" || events[2].Body["decision"] != "allow" {
		t.Errorf("decisions recorded: %v %v", events[2].Body["decision"], events[3].Body["decision"])
	}
	rows, _ := trace.ReadLedger(trace.SessionDir(nil0(root), "e2e-1"))
	if len(rows) != 3 || rows[0].USD.Total == nil || rows[2].USD.Total != nil {
		t.Fatalf("ledger rows: %d", len(rows))
	}
	out, _, code = runIn(t, root, "", "trace", "ledger", "e2e-1", "--json")
	if code != cli.ExitOK || !strings.Contains(out, `"unpriced_calls": 1`) {
		t.Errorf("ledger json: %v %s", code, out)
	}
	out, _, code = runIn(t, root, "", "trace", "tail", "e2e-1", "--type", "model_call")
	if code != cli.ExitOK || strings.Count(out, "\n") != 3 {
		t.Errorf("tail: %v %s", code, out)
	}
	out, _, _ = runIn(t, root, "", "doctor", "--json")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil || rep["schema"] != "saga.doctor/1" {
		t.Errorf("doctor json: %v %s", err, out)
	}
	out, _, code = runIn(t, root, "", "install", "--harness", "claude-code", "--dry-run")
	if code != cli.ExitOK || !strings.Contains(out, "hook claude-code PreToolUse") {
		t.Errorf("install dry-run: %v %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Error("dry run created .claude")
	}
}

func TestHookWithoutStoreAllowsAndSaysSo(t *testing.T) {
	root := t.TempDir()
	m, code := hookJSON(t, root, "PreToolUse", `{"session_id":"s","hook_event_name":"PreToolUse","cwd":"`+root+`","tool_name":"Bash","tool_input":{"command":"ls"}}`)
	if len(m) != 0 || code != cli.ExitOK {
		t.Errorf("%v %v", m, code)
	}
}

func TestTamperedChainExitsIntegrity(t *testing.T) {
	root := t.TempDir()
	runIn(t, root, "", "init")
	hookJSON(t, root, "SessionStart", `{"session_id":"s2","hook_event_name":"SessionStart","cwd":"`+root+`","source":"startup"}`)
	hookJSON(t, root, "PreCompact", `{"session_id":"s2","hook_event_name":"PreCompact","cwd":"`+root+`","trigger":"manual"}`)
	seg := filepath.Join(trace.SessionDir(nil0(root), "s2"), "events.000001.jsonl")
	raw, _ := os.ReadFile(seg)
	os.WriteFile(seg, bytes.Replace(raw, []byte(`"trigger":"manual"`), []byte(`"trigger":"auto"`), 1), 0o600)
	_, errs, code := runIn(t, root, "", "trace", "verify", "s2")
	if code != cli.ExitIntegrity || !strings.Contains(errs, "seq 2") {
		t.Errorf("code %v stderr %s", code, errs)
	}
}

// nil0 opens the store at root for assertions.
func nil0(root string) *store.Store { return store.Open(root) }

// TestTraceClaimsCLI covers `saga trace claims` over a recorded session
// (human and --json output, --status exit per trace-spec 5.9) and over a
// bench run directory (source derived).
func TestTraceClaimsCLI(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}, {"config", "commit.gpgsign", "false"}} {
		gitIn(t, root, args...)
	}
	os.WriteFile(filepath.Join(root, "README.md"), []byte("x\n"), 0o644)
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "base")
	if _, _, code := runIn(t, root, "", "init"); code != cli.ExitOK {
		t.Fatal("init")
	}
	os.Remove(filepath.Join(root, ".saga", "config.toml"))
	common := `"session_id":"cl-1","cwd":"` + root + `"`
	hookJSON(t, root, "SessionStart", `{`+common+`,"hook_event_name":"SessionStart","source":"startup"}`)
	hookJSON(t, root, "UserPromptSubmit", `{`+common+`,"hook_event_name":"UserPromptSubmit","prompt":"fix it"}`)
	hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"pytest -q"},"tool_use_id":"tu1"}`)
	hookJSON(t, root, "PostToolUse", `{`+common+`,"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"pytest -q"},"tool_response":{"stdout":"===== 2 failed, 5 passed in 0.1s =====","stderr":"","interrupted":false,"isImage":false},"tool_use_id":"tu1","duration_ms":10}`)
	m, code := hookJSON(t, root, "Stop", `{`+common+`,"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"I ran `+"`pytest -q`"+` and the tests pass.\n\nDONE"}`)
	if m["decision"] != "block" || code != cli.ExitIntegrity {
		t.Fatalf("stop: %v %v", m, code)
	}
	out, errs, code := runIn(t, root, "", "trace", "claims", "cl-1", "--status")
	if code != cli.ExitIntegrity || !strings.Contains(out, "tests_pass   contradicted") || !strings.Contains(out, "verdict contradicted") {
		t.Fatalf("claims --status: %d %s %s", code, out, errs)
	}
	out, errs, code = runIn(t, root, "", "trace", "claims", "--session", "cl-1", "--json")
	if code != cli.ExitOK {
		t.Fatalf("claims --json: %d %s", code, errs)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if body["schema"] != "saga.trace.claims/1" || body["session"] != "cl-1" || body["trigger"] != "cli" || body["verdict"] != "contradicted" || body["claimed_done"] != true {
		t.Errorf("claims json: %v", body)
	}
	if err := schemapkg.ValidateBytes("saga.trace.claims/1", []byte(out)); err != nil {
		t.Errorf("schema: %v", err)
	}
	// A bench run directory: final_message.txt plus trace.jsonl, judged as derived.
	dir := filepath.Join(t.TempDir(), "run")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "final_message.txt"), []byte("Updated `src/a.py`.\n\nDONE"), 0o644)
	os.WriteFile(filepath.Join(dir, "workspace.diff"), []byte("diff --git a/src/a.py b/src/a.py\n--- a/src/a.py\n+++ b/src/a.py\n@@ -1 +1 @@\n-x\n+y\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "trace.jsonl"), nil, 0o644)
	out, errs, code = runIn(t, root, "", "trace", "claims", dir, "--json", "--status")
	if code != cli.ExitFinding {
		t.Fatalf("run dir: %d %s %s", code, out, errs)
	}
	body = map[string]any{}
	json.Unmarshal([]byte(out), &body)
	if body["trigger"] != "derived" || body["verdict"] != "unverified" || body["claimed_done"] != true {
		t.Errorf("run dir json: %v", body)
	}
}

// TestPostToolUseFailureRecordsResult closes the claims gap "a red last
// test run reads no_result": Claude Code fires PostToolUseFailure (not
// PostToolUse) when Bash fails, so the composed hook binds it and trace
// writes the tool_result with the exit status the failure text names.
// A later "tests pass" claim is then contradicted at Stop. The payload
// is synthesised from the documented shape (harness-facts C34).
func TestPostToolUseFailureRecordsResult(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}, {"config", "commit.gpgsign", "false"}} {
		gitIn(t, root, args...)
	}
	os.WriteFile(filepath.Join(root, "README.md"), []byte("x\n"), 0o644)
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "base")
	if _, _, code := runIn(t, root, "", "init"); code != cli.ExitOK {
		t.Fatal("init")
	}
	os.Remove(filepath.Join(root, ".saga", "config.toml"))
	common := `"session_id":"cf-1","cwd":"` + root + `"`
	hookJSON(t, root, "SessionStart", `{`+common+`,"hook_event_name":"SessionStart","source":"startup"}`)
	hookJSON(t, root, "UserPromptSubmit", `{`+common+`,"hook_event_name":"UserPromptSubmit","prompt":"fix it"}`)
	hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"pytest -q"},"tool_use_id":"tu1"}`)
	m, code := hookJSON(t, root, "PostToolUseFailure", `{`+common+`,"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"pytest -q"},"tool_use_id":"tu1","error":"Command failed with exit code 1\n===== 2 failed, 5 passed in 0.1s =====","is_interrupt":false,"duration_ms":25}`)
	if code != cli.ExitOK {
		t.Fatalf("PostToolUseFailure: %v %v", m, code)
	}
	// The tool_result exists, bound to the tool_call, with exit 1.
	raw, err := os.ReadFile(filepath.Join(root, ".saga", "trace", "sessions", "cf-1", "events.000001.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var ev trace.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != trace.TypeToolResult {
			continue
		}
		found = true
		if ev.Source != "hook:PostToolUseFailure" || ev.Body["exit"] != float64(1) || ev.Body["for_seq"] == nil || ev.Body["error"] != "Command failed with exit code 1" {
			t.Errorf("tool_result body: %v", ev.Body)
		}
	}
	if !found {
		t.Fatalf("no tool_result recorded:\n%s", raw)
	}
	m, code = hookJSON(t, root, "Stop", `{`+common+`,"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"I ran `+"`pytest -q`"+` and the tests pass.\n\nDONE"}`)
	if m["decision"] != "block" || code != cli.ExitIntegrity {
		t.Fatalf("stop: %v %v", m, code)
	}
	out, _, code := runIn(t, root, "", "trace", "claims", "cf-1", "--status")
	if code != cli.ExitIntegrity || !strings.Contains(out, "tests_pass   contradicted") || !strings.Contains(out, "status fail") {
		t.Fatalf("claims --status: %d %s", code, out)
	}
	// An interrupted tool records error "interrupted" and no exit parse.
	hookJSON(t, root, "PreToolUse", `{`+common+`,"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"sleep 100"},"tool_use_id":"tu2"}`)
	hookJSON(t, root, "PostToolUseFailure", `{`+common+`,"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"sleep 100"},"tool_use_id":"tu2","error":"","is_interrupt":true}`)
	raw, _ = os.ReadFile(filepath.Join(root, ".saga", "trace", "sessions", "cf-1", "events.000001.jsonl"))
	if !strings.Contains(string(raw), `"error":"interrupted"`) {
		t.Errorf("interrupted tool not recorded:\n%s", raw)
	}
}
