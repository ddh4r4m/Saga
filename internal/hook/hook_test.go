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

// TestLatencyLineOnEveryPath (docs/12 commitment 7): the invocation's own
// wall time is written once, at the last point before the reply, on the
// ordinary path and on the deadline path alike. The deadline case is the
// one the overhead table most needs, because it is where a hook fails
// open (docs/12 section 9), and it is exactly the case a buffered chain
// would have lost.
func TestLatencyLineOnEveryPath(t *testing.T) {
	var lines []hookio.LatencyLine
	runWith := func(event, stdin string, deadline time.Duration, layers ...hookio.Layer) cli.Code {
		var out bytes.Buffer
		errb := &syncBuffer{}
		e := &Entry{
			Harness: "claude-code", Parse: claude.Parse, Render: claude.Render, Layers: layers,
			Deadline: deadline, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: errb,
			WriteLatency: func(l hookio.LatencyLine) error { lines = append(lines, l); return nil },
		}
		return e.Run(event)
	}

	// Ordinary path: the steps that ran are named and the total covers them.
	runWith(hookio.EventPreToolUse, preTool, time.Second, &fakeLayer{name: "a"}, &fakeLayer{name: "b", delay: 20 * time.Millisecond})
	if len(lines) != 1 {
		t.Fatalf("%d lines after one invocation", len(lines))
	}
	l := lines[0]
	if l.Schema != hookio.LatencySchema || l.Event != hookio.EventPreToolUse || l.Session != "s1" {
		t.Errorf("line %+v", l)
	}
	if l.TimedOut {
		t.Error("an invocation inside its deadline reported timed_out")
	}
	if l.Steps["b"] < 15 {
		t.Errorf("step b took %d ms, want about 20: %v", l.Steps["b"], l.Steps)
	}
	if l.TotalMS < l.Steps["a"]+l.Steps["b"] {
		t.Errorf("total %d below its steps %v", l.TotalMS, l.Steps)
	}
	if l.Decision != hookio.DecisionAllow {
		t.Errorf("decision %q", l.Decision)
	}

	// Deadline path: one line, marked, carrying only the finished steps.
	lines = nil
	code := runWith(hookio.EventPreToolUse, preTool, 50*time.Millisecond, &fakeLayer{name: "fast"}, &fakeLayer{name: "slow", delay: 2 * time.Second})
	if code != cli.ExitRefusal {
		t.Errorf("deadline exit %v", code)
	}
	if len(lines) != 1 {
		t.Fatalf("%d lines on the deadline path", len(lines))
	}
	l = lines[0]
	if !l.TimedOut {
		t.Error("the deadline path did not report timed_out")
	}
	if _, ok := l.Steps["slow"]; ok {
		t.Errorf("the abandoned step was counted: %v", l.Steps)
	}
	if l.Decision != hookio.DecisionDeny {
		t.Errorf("deadline decision %q, want deny", l.Decision)
	}

	// A malformed payload still costs the process time, and is measured.
	lines = nil
	runWith(hookio.EventPreToolUse, "{not json", time.Second, &fakeLayer{name: "a"})
	if len(lines) != 1 || lines[0].Decision != hookio.DecisionDeny {
		t.Errorf("malformed payload: %+v", lines)
	}
}
