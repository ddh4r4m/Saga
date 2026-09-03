package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/harness/claude"
	"github.com/ddh4r4m/saga/internal/hookio"
)

type fakeLayer struct {
	name  string
	delay time.Duration
	out   *hookio.Output
	final *hookio.Output
}

func (f *fakeLayer) Name() string { return f.name }
func (f *fakeLayer) Run(ctx context.Context, in *hookio.Input) (*hookio.Output, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.out == nil {
		return hookio.Allow(f.name), nil
	}
	o := *f.out
	o.Layer = f.name
	return &o, nil
}
func (f *fakeLayer) Finalize(ctx context.Context, in *hookio.Input, merged *hookio.Output) error {
	f.final = merged
	return nil
}

// syncBuffer is a stderr sink safe against a chain goroutine that outlives
// the deadline path.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func run(t *testing.T, event string, stdin string, deadline time.Duration, layers ...hookio.Layer) (string, string, cli.Code) {
	t.Helper()
	var out bytes.Buffer
	errb := &syncBuffer{}
	e := &Entry{Harness: "claude-code", Parse: claude.Parse, Render: claude.Render, Layers: layers, Deadline: deadline, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: errb}
	code := e.Run(event)
	return out.String(), errb.String(), code
}

func singleJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(s)
	if strings.Count(trimmed, "\n") != 0 {
		t.Fatalf("stdout is not exactly one line: %q", s)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %q", s)
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.Decode(&m)
	if dec.More() {
		t.Fatalf("more than one JSON value on stdout: %q", s)
	}
	return m
}

const preTool = `{"session_id":"s1","hook_event_name":"PreToolUse","cwd":"/tmp","tool_name":"Bash","tool_input":{"command":"ls"},"tool_use_id":"tu1"}`

func TestAllowEmitsEmptyObject(t *testing.T) {
	out, _, code := run(t, hookio.EventPreToolUse, preTool, time.Second, &fakeLayer{name: "a"})
	m := singleJSON(t, out)
	if len(m) != 0 || code != cli.ExitOK {
		t.Errorf("allow: %v code %v", m, code)
	}
}

func TestDenyRendersPermissionDecision(t *testing.T) {
	deny := &fakeLayer{name: "guard", out: &hookio.Output{Decision: hookio.DecisionDeny, Reason: "no", Exit: int(cli.ExitRefusal)}}
	after := &fakeLayer{name: "after", out: &hookio.Output{AdditionalContext: []string{"should not run"}}}
	out, _, code := run(t, hookio.EventPreToolUse, preTool, time.Second, deny, after)
	m := singleJSON(t, out)
	hso := m["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "deny" || hso["permissionDecisionReason"] != "no" || code != cli.ExitRefusal {
		t.Errorf("deny: %v code %v", m, code)
	}
	if _, ok := hso["additionalContext"]; ok {
		t.Error("chain did not short-circuit on deny")
	}
}

func TestDeadlineFailsClosedOnDecidingEvents(t *testing.T) {
	slow := &fakeLayer{name: "slow", delay: 2 * time.Second}
	start := time.Now()
	out, errs, code := run(t, hookio.EventPreToolUse, preTool, 100*time.Millisecond, slow)
	if time.Since(start) > time.Second {
		t.Fatalf("deadline not enforced: %v", time.Since(start))
	}
	m := singleJSON(t, out)
	hso, _ := m["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "deny" || hso["permissionDecisionReason"] != DeadlineReason || code != cli.ExitRefusal {
		t.Errorf("deadline: %v code %v stderr %s", m, code, errs)
	}
	stop := `{"session_id":"s1","hook_event_name":"Stop","cwd":"/tmp","stop_hook_active":false,"last_assistant_message":"done"}`
	out, _, _ = run(t, hookio.EventStop, stop, 100*time.Millisecond, &fakeLayer{name: "slow", delay: 2 * time.Second})
	m = singleJSON(t, out)
	if m["decision"] != "block" || m["reason"] != DeadlineReason {
		t.Errorf("stop deadline: %v", m)
	}
}

func TestDeadlineAllowsOnNonDecidingEvents(t *testing.T) {
	post := `{"session_id":"s1","hook_event_name":"PostToolUse","cwd":"/tmp","tool_name":"Bash","tool_input":{},"tool_response":{"stdout":""},"tool_use_id":"tu1"}`
	out, _, code := run(t, hookio.EventPostToolUse, post, 50*time.Millisecond, &fakeLayer{name: "slow", delay: time.Second})
	m := singleJSON(t, out)
	if len(m) != 0 || code != cli.ExitOK {
		t.Errorf("post deadline: %v code %v", m, code)
	}
}

func TestMalformedStdinFailsClosed(t *testing.T) {
	for _, bad := range []string{"", "not json", "[1,2]", `{"hook_event_name":"Stop","session_id":"x"}`, `{"tool_name":"Bash"}`} {
		out, _, code := run(t, hookio.EventPreToolUse, bad, time.Second, &fakeLayer{name: "a"})
		m := singleJSON(t, out)
		hso, _ := m["hookSpecificOutput"].(map[string]any)
		if hso["permissionDecision"] != "deny" || hso["permissionDecisionReason"] != MalformedReason || code != cli.ExitUsage {
			t.Errorf("malformed %q: %v code %v", bad, m, code)
		}
	}
	// Non-deciding events still emit one object and exit 2.
	out, _, code := run(t, hookio.EventSessionEnd, "nope", time.Second, &fakeLayer{name: "a"})
	if m := singleJSON(t, out); len(m) != 0 || code != cli.ExitUsage {
		t.Errorf("malformed non-deciding: %v %v", m, code)
	}
}

func TestUnknownEvent(t *testing.T) {
	out, _, code := run(t, "Banana", preTool, time.Second)
	if m := singleJSON(t, out); len(m) != 0 || code != cli.ExitUsage {
		t.Errorf("%v %v", m, code)
	}
}

func TestFinalizeReceivesMergedDecisionAndContextIsPrefixed(t *testing.T) {
	tr := &fakeLayer{name: "trace", out: &hookio.Output{AdditionalContext: []string{"budget at 80%"}}}
	gate := &fakeLayer{name: "gate", out: &hookio.Output{Decision: hookio.DecisionDeny, Reason: "G1 unmet", Exit: int(cli.ExitFinding)}}
	out, _, code := run(t, hookio.EventPreToolUse, preTool, time.Second, tr, gate)
	m := singleJSON(t, out)
	hso := m["hookSpecificOutput"].(map[string]any)
	if hso["additionalContext"] != "saga trace: budget at 80%" || hso["permissionDecision"] != "deny" {
		t.Errorf("%v", m)
	}
	if code != cli.ExitFinding {
		t.Errorf("code %v", code)
	}
	if tr.final == nil || tr.final.Decision != hookio.DecisionDeny {
		t.Errorf("finalize did not see the merged decision: %+v", tr.final)
	}
}

func TestUpdatedInputConflictIsCompositionBug(t *testing.T) {
	a := &fakeLayer{name: "a", out: &hookio.Output{UpdatedInput: map[string]any{"command": "x"}}}
	b := &fakeLayer{name: "b", out: &hookio.Output{UpdatedInput: map[string]any{"command": "y"}}}
	out, _, code := run(t, hookio.EventPreToolUse, preTool, time.Second, a, b)
	m := singleJSON(t, out)
	hso := m["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "deny" || code != cli.ExitUsage {
		t.Errorf("%v %v", m, code)
	}
}

func TestPreCompactNeverBlocks(t *testing.T) {
	pre := `{"session_id":"s1","hook_event_name":"PreCompact","cwd":"/tmp","trigger":"auto"}`
	blocker := &fakeLayer{name: "x", out: &hookio.Output{Decision: hookio.DecisionBlock, Reason: "no", Exit: 3}}
	out, _, code := run(t, hookio.EventPreCompact, pre, time.Second, blocker)
	if m := singleJSON(t, out); len(m) != 0 || code != cli.ExitOK {
		t.Errorf("%v %v", m, code)
	}
}
