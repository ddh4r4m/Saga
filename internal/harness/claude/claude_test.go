package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddh4r4m/saga/internal/hookio"
)

func TestParseFields(t *testing.T) {
	raw := `{"session_id":"abc","transcript_path":"/t.jsonl","cwd":"/p","hook_event_name":"PostToolUse","permission_mode":"acceptEdits","effort":{"level":"high"},"tool_name":"Read","tool_input":{"file_path":"x"},"tool_response":{"type":"text"},"tool_use_id":"tu","duration_ms":12}`
	in, err := Parse(hookio.EventPostToolUse, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in.SessionID != "abc" || in.TranscriptPath != "/t.jsonl" || in.Effort != "high" || in.ToolName != "Read" || in.ToolUseID != "tu" || in.DurationMS != 12 || string(in.ToolInput) != `{"file_path":"x"}` {
		t.Errorf("%+v", in)
	}
	if _, err := Parse(hookio.EventPreToolUse, []byte(raw)); err == nil {
		t.Error("event mismatch accepted")
	}
}

func TestRenderShapes(t *testing.T) {
	in := &hookio.Input{Event: hookio.EventPreToolUse, ToolInput: json.RawMessage(`{"command":"ls","description":"d"}`)}
	got := string(Render(in, &hookio.Output{Decision: hookio.DecisionAllow, UpdatedInput: map[string]any{"command": "saga guard exec -- ls"}}))
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"command":"saga guard exec -- ls","description":"d"}}}`
	if got != want {
		t.Errorf("updatedInput union:\n%s\n%s", got, want)
	}
	in = &hookio.Input{Event: hookio.EventStop}
	if got := string(Render(in, &hookio.Output{Decision: hookio.DecisionBlock, Reason: "r"})); got != `{"decision":"block","reason":"r"}` {
		t.Errorf("stop: %s", got)
	}
	if got := string(Render(&hookio.Input{Event: hookio.EventPreCompact}, &hookio.Output{Decision: hookio.DecisionBlock, Reason: "r"})); got != `{}` {
		t.Errorf("precompact: %s", got)
	}
	if got := string(Render(&hookio.Input{Event: hookio.EventSessionStart}, &hookio.Output{Decision: hookio.DecisionAllow, AdditionalContext: []string{"saga mem: state"}})); got != `{"hookSpecificOutput":{"additionalContext":"saga mem: state","hookEventName":"SessionStart"}}` {
		t.Errorf("session start: %s", got)
	}
}

// TestParsePostToolUseFailure: the failure event carries `error` and
// `is_interrupt` beside the tool fields (harness-facts C34). The payload
// is the live capture from P17 of scripts/harness-probes.sh on Claude
// Code 2.1.263, not a shape written from the documentation, so the test
// fails if the real event ever stops matching what the parser expects.
// Render honours additionalContext only.
func TestParsePostToolUseFailure(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "harness", "posttoolusefailure.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The capture carries no tool_response: the tool failed, so there was
	// no response to carry. It does carry prompt_id, permission_mode,
	// effort and duration_ms, which the documented shape did not name.
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, has := got["tool_response"]; has {
		t.Error("the capture carries tool_response; C34 says the failure event does not")
	}
	for _, k := range []string{"prompt_id", "permission_mode", "effort", "duration_ms", "error", "is_interrupt", "tool_use_id"} {
		if _, has := got[k]; !has {
			t.Errorf("the capture lost %s", k)
		}
	}
	in, err := Parse(hookio.EventPostToolUseFailure, raw)
	if err != nil {
		t.Fatal(err)
	}
	if in.ToolName != "Bash" || in.ToolUseID != "toolu_01CCBHcNhaJsBT7faWxmRxQi" || in.Interrupted || !json.Valid(in.ToolInput) || in.ToolError != "Exit code 3" || in.DurationMS != 40 {
		t.Errorf("parsed: %+v", in)
	}
	out := hookio.Allow("trace")
	out.Decision = hookio.DecisionBlock
	out.AdditionalContext = []string{"saga trace: budget"}
	var m map[string]any
	if err := json.Unmarshal(Render(in, out), &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["decision"]; has {
		t.Errorf("PostToolUseFailure has no decision control: %v", m)
	}
	if sp, _ := m["hookSpecificOutput"].(map[string]any); sp["additionalContext"] != "saga trace: budget" || sp["hookEventName"] != "PostToolUseFailure" {
		t.Errorf("render: %v", m)
	}
	if _, err := Parse(hookio.EventPostToolUseFailure, []byte(`{"session_id":"abc","hook_event_name":"PostToolUse"}`)); err == nil {
		t.Error("event name mismatch accepted")
	}
}
