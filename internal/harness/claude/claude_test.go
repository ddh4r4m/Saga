package claude

import (
	"encoding/json"
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
// `is_interrupt` beside the tool fields (harness-facts C34; the payload
// is synthesised from the documented shape, not an archived capture);
// Render honours additionalContext only.
func TestParsePostToolUseFailure(t *testing.T) {
	raw := `{"session_id":"abc","cwd":"/p","hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"pytest -q"},"tool_use_id":"tu","error":"Command failed with exit code 1\n2 failed, 5 passed","is_interrupt":false,"duration_ms":40}`
	in, err := Parse(hookio.EventPostToolUseFailure, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in.ToolName != "Bash" || in.ToolUseID != "tu" || in.Interrupted || !json.Valid(in.ToolInput) || in.ToolError != "Command failed with exit code 1\n2 failed, 5 passed" || in.DurationMS != 40 {
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
