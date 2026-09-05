// Package hookio defines the harness-neutral hook input and output that
// the composed entry (internal/hook) passes to every layer. Harness
// adapters (internal/harness/*) translate to and from these types and
// contain no other logic (ADR 0003, ADR 0008).
package hookio

import (
	"context"
	"encoding/json"
)

// Event names are the Claude Code names; every adapter maps its own
// vocabulary onto them (contracts section 1).
const (
	EventSessionStart     = "SessionStart"
	EventSessionEnd       = "SessionEnd"
	EventUserPromptSubmit = "UserPromptSubmit"
	EventPreToolUse       = "PreToolUse"
	EventPostToolUse      = "PostToolUse"
	// EventPostToolUseFailure fires instead of PostToolUse when the tool
	// failed (harness-facts C1, C34): the only record of a failed Bash
	// command's output, so trace binds it to write the tool_result.
	EventPostToolUseFailure = "PostToolUseFailure"
	EventStop               = "Stop"
	EventSubagentStart      = "SubagentStart"
	EventSubagentStop       = "SubagentStop"
	EventPreCompact         = "PreCompact"
	EventPostCompact        = "PostCompact"
)

// Events lists every event the entry binds, in a fixed order.
var Events = []string{
	EventSessionStart, EventSessionEnd, EventUserPromptSubmit, EventPreToolUse, EventPostToolUse, EventPostToolUseFailure,
	EventStop, EventSubagentStart, EventSubagentStop, EventPreCompact, EventPostCompact,
}

// Known reports whether name is a bound event.
func Known(name string) bool {
	for _, e := range Events {
		if e == name {
			return true
		}
	}
	return false
}

// Deciding reports whether a layer may deny or block on this event, which
// is when a deadline overrun must fail closed (contracts section 1.1).
func Deciding(event string) bool {
	return event == EventPreToolUse || event == EventStop
}

// Input is the harness-neutral view of one hook invocation.
type Input struct {
	Harness        string
	Event          string
	SessionID      string
	TranscriptPath string
	Cwd            string
	PermissionMode string
	Effort         string
	// Tool events.
	ToolName     string
	ToolUseID    string
	ToolInput    json.RawMessage
	ToolResponse json.RawMessage
	DurationMS   int
	// ToolError is the failure text of a PostToolUseFailure event;
	// Interrupted is its is_interrupt flag (the user cancelled the tool).
	ToolError   string
	Interrupted bool
	// Prompt and stop events.
	Prompt               string
	LastAssistantMessage string
	StopHookActive       bool
	// Session and compaction events.
	Source         string
	Trigger        string
	CompactSummary string
	// Sub-agent events.
	AgentID             string
	AgentType           string
	AgentTranscriptPath string
	// Raw is the original payload for fields a layer needs that the
	// neutral view does not carry.
	Raw map[string]any
}

// Decision values, in merge precedence order deny > block > ask > allow.
const (
	DecisionAllow = "allow"
	DecisionAsk   = "ask"
	DecisionBlock = "block"
	DecisionDeny  = "deny"
)

func rank(d string) int {
	switch d {
	case DecisionDeny:
		return 3
	case DecisionBlock:
		return 2
	case DecisionAsk:
		return 1
	}
	return 0
}

// Output is one layer's contribution, or the merged result.
type Output struct {
	Layer    string
	Decision string
	// Reason is the text the harness shows for a deny or block.
	Reason string
	// AdditionalContext blocks are concatenated in layer order, each
	// prefixed "saga <layer>:".
	AdditionalContext []string
	// UpdatedInput fields are unioned over the original tool_input.
	UpdatedInput map[string]any
	// Exit is the code this layer asks for; the entry applies the
	// contracts section 4 precedence.
	Exit int
}

// Allow is the empty allow output for layer.
func Allow(layer string) *Output { return &Output{Layer: layer, Decision: DecisionAllow} }

// Merge folds next into m under the contracts section 1.1 rules and
// reports whether next short-circuits the chain (a deny or block).
func (m *Output) Merge(next *Output) (stop bool, conflict string) {
	if next == nil {
		return false, ""
	}
	if m.Decision == "" {
		m.Decision = DecisionAllow
	}
	if rank(next.Decision) > rank(m.Decision) {
		m.Decision = next.Decision
	}
	if next.Reason != "" {
		if m.Reason != "" {
			m.Reason += " "
		}
		m.Reason += next.Reason
	}
	for _, c := range next.AdditionalContext {
		m.AdditionalContext = append(m.AdditionalContext, "saga "+next.Layer+": "+c)
	}
	for k, v := range next.UpdatedInput {
		if m.UpdatedInput == nil {
			m.UpdatedInput = map[string]any{}
		}
		if _, dup := m.UpdatedInput[k]; dup {
			conflict = k
		}
		m.UpdatedInput[k] = v
	}
	if next.Exit != 0 {
		m.Exit = next.Exit
	}
	return next.Decision == DecisionDeny || next.Decision == DecisionBlock, conflict
}

// Layer is one installed component in the composed chain.
type Layer interface {
	// Name is the manifest name of the layer.
	Name() string
	// Run handles one event. It must honour ctx.
	Run(ctx context.Context, in *Input) (*Output, error)
}

// Finalizer is implemented by a layer that records the merged decision
// after the chain has run (trace's second step in contracts section 1).
type Finalizer interface {
	Finalize(ctx context.Context, in *Input, merged *Output) error
}
