// Package claude translates Claude Code hook stdin and stdout to and from
// the harness-neutral hookio types. Translation only, no logic
// (ADR 0003, ADR 0008): field names come from harness-facts.md section 1
// and harness-probes.md.
package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ddh4r4m/saga/internal/hookio"
)

// Name is the harness id used in `saga hook claude-code <event>`.
const Name = "claude-code"

// Parse decodes one hook payload for event. It fails on anything that is
// not a JSON object or that names a different hook_event_name, so the
// entry can fail closed.
func Parse(event string, raw []byte) (*hookio.Input, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("claude-code hook input: %w", err)
	}
	if m == nil {
		return nil, errors.New("claude-code hook input: not a JSON object")
	}
	if name, _ := m["hook_event_name"].(string); name != "" && name != event {
		return nil, fmt.Errorf("claude-code hook input: hook_event_name %q, expected %q", name, event)
	}
	in := &hookio.Input{Harness: Name, Event: event, Raw: m}
	in.SessionID = str(m, "session_id")
	in.TranscriptPath = str(m, "transcript_path")
	in.Cwd = str(m, "cwd")
	in.PermissionMode = str(m, "permission_mode")
	if eff, ok := m["effort"].(map[string]any); ok {
		in.Effort = str(eff, "level")
	}
	in.ToolName = str(m, "tool_name")
	in.ToolUseID = str(m, "tool_use_id")
	in.ToolInput = rawOf(m, "tool_input")
	in.ToolResponse = rawOf(m, "tool_response")
	if d, ok := m["duration_ms"].(float64); ok {
		in.DurationMS = int(d)
	}
	in.ToolError = str(m, "error")
	in.Interrupted, _ = m["is_interrupt"].(bool)
	in.Prompt = str(m, "prompt")
	in.LastAssistantMessage = str(m, "last_assistant_message")
	in.StopHookActive, _ = m["stop_hook_active"].(bool)
	in.Source = str(m, "source")
	in.Trigger = str(m, "trigger")
	in.CompactSummary = str(m, "compact_summary")
	in.AgentID = str(m, "agent_id")
	in.AgentType = str(m, "agent_type")
	in.AgentTranscriptPath = str(m, "agent_transcript_path")
	if in.SessionID == "" && event != hookio.EventSessionStart {
		return nil, errors.New("claude-code hook input: session_id missing")
	}
	return in, nil
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func rawOf(m map[string]any, k string) json.RawMessage {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// Render encodes the merged output as the one JSON object Claude Code
// accepts for the event (gate-spec section 6.1, harness-facts C2 to C26).
// An allow with nothing to add renders as {}.
func Render(in *hookio.Input, out *hookio.Output) []byte {
	ctx := strings.Join(out.AdditionalContext, "\n")
	obj := map[string]any{}
	specific := map[string]any{"hookEventName": in.Event}
	switch in.Event {
	case hookio.EventPreToolUse:
		switch out.Decision {
		case hookio.DecisionDeny, hookio.DecisionBlock:
			specific["permissionDecision"] = "deny"
			specific["permissionDecisionReason"] = out.Reason
		case hookio.DecisionAsk:
			specific["permissionDecision"] = "ask"
			specific["permissionDecisionReason"] = out.Reason
		default:
			if len(out.UpdatedInput) > 0 {
				specific["permissionDecision"] = "allow"
				merged := map[string]any{}
				_ = json.Unmarshal(in.ToolInput, &merged)
				for k, v := range out.UpdatedInput {
					merged[k] = v
				}
				specific["updatedInput"] = merged
			}
		}
		if ctx != "" {
			specific["additionalContext"] = ctx
		}
	case hookio.EventPostToolUse, hookio.EventUserPromptSubmit:
		if out.Decision == hookio.DecisionBlock || out.Decision == hookio.DecisionDeny {
			obj["decision"] = "block"
			obj["reason"] = out.Reason
		}
		if ctx != "" {
			specific["additionalContext"] = ctx
		}
	case hookio.EventStop, hookio.EventSubagentStop:
		if out.Decision == hookio.DecisionBlock || out.Decision == hookio.DecisionDeny {
			obj["decision"] = "block"
			obj["reason"] = out.Reason
		}
		// On Stop, additionalContext keeps the conversation going through
		// the same loop protections as decision: block (harness-facts
		// C11), so it is never the way to end a turn. A release therefore
		// carries none: the gate layer sends its handoff line to stderr
		// and to the trace event instead. The 2026-09-06 smoke spent half
		// of one arm's cost on releases that read as continuations.
		if ctx != "" {
			specific["additionalContext"] = ctx
		}
	case hookio.EventSessionStart, hookio.EventSubagentStart, hookio.EventPostToolUseFailure:
		// PostToolUseFailure takes additionalContext only (harness-facts
		// C5); the tool already failed, there is nothing to decide.
		if ctx != "" {
			specific["additionalContext"] = ctx
		}
	default:
		// PreCompact never blocks (contracts section 1); PostCompact and
		// SessionEnd have no decision control.
	}
	if len(specific) > 1 {
		obj["hookSpecificOutput"] = specific
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return []byte("{}")
	}
	return b
}
