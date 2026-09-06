package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/trace"
)

// latencySession prepares a store and drives one session's worth of hook
// events through the composed entry, returning the session directory.
func latencySession(t *testing.T, session string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := runIn(t, root, "", "init"); code != cli.ExitOK {
		t.Fatalf("init: %s", errs)
	}
	common := `"session_id":"` + session + `","cwd":"` + root + `"`
	for _, ev := range []struct{ event, payload string }{
		{"SessionStart", `{` + common + `,"hook_event_name":"SessionStart","source":"startup"}`},
		{"UserPromptSubmit", `{` + common + `,"hook_event_name":"UserPromptSubmit","prompt":"add a retry helper"}`},
		{"PreToolUse", `{` + common + `,"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_use_id":"toolu_01"}`},
	} {
		if _, code := hookJSON(t, root, ev.event, ev.payload); code != cli.ExitOK {
			t.Fatalf("%s exited %v", ev.event, code)
		}
	}
	return root, filepath.Join(root, ".saga", "trace", "sessions", session)
}

// readEvents loads a session's chain events.
func readEvents(t *testing.T, dir string) []trace.Event {
	t.Helper()
	segs, err := trace.Segments(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []trace.Event
	for _, seg := range segs {
		raw, err := os.ReadFile(seg)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			var ev trace.Event
			if err := json.Unmarshal([]byte(l), &ev); err != nil {
				t.Fatalf("event %q: %v", l, err)
			}
			out = append(out, ev)
		}
	}
	return out
}

// TestHookEventsCarryTheirOwnWallTime (docs/12 commitment 7): every event
// a hook writes says what the invocation had spent when it was appended,
// and which steps had run by then. Without this the overhead table can
// only report a total; with it a slow invocation is attributable to a
// step.
func TestHookEventsCarryTheirOwnWallTime(t *testing.T) {
	_, dir := latencySession(t, "lat-1")
	events := readEvents(t, dir)
	if len(events) == 0 {
		t.Fatal("no events written")
	}
	for _, ev := range events {
		if ev.HookMS == nil {
			t.Errorf("seq %d (%s from %s) has no hook_ms", ev.Seq, ev.Type, ev.Source)
			continue
		}
		if *ev.HookMS < 0 {
			t.Errorf("seq %d hook_ms %d", ev.Seq, *ev.HookMS)
		}
		// The stamp is at or above the steps that had finished, because
		// it also carries the process start-up and the payload read.
		sum := 0
		for _, ms := range ev.HookSteps {
			sum += ms
		}
		if *ev.HookMS < sum {
			t.Errorf("seq %d hook_ms %d is below its step sum %d (%v)", ev.Seq, *ev.HookMS, sum, ev.HookSteps)
		}
	}
	// The tool_call event is written by trace's Finalize, after every
	// layer has run, so it names the steps that preceded it.
	var final *trace.Event
	for i := range events {
		if events[i].Type == trace.TypeToolCall {
			final = &events[i]
		}
	}
	if final == nil {
		t.Fatal("no tool_call event")
	}
	if _, ok := final.HookSteps["trace"]; !ok {
		t.Errorf("the finalized tool_call does not name the trace step: %v", final.HookSteps)
	}
	if _, ok := final.HookSteps["gate"]; !ok {
		t.Errorf("the finalized tool_call does not name the gate step: %v", final.HookSteps)
	}
}

// TestHookLatencySidecar: one line per invocation, outside the hash
// chain, holding the total the per-event stamps cannot carry. The chain
// stays eagerly written, so a hook that overruns its deadline still
// leaves its events behind.
func TestHookLatencySidecar(t *testing.T) {
	_, dir := latencySession(t, "lat-2")
	lines, err := trace.ReadLatency(filepath.Join(dir, trace.LatencyFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("%d sidecar lines, want 3: %+v", len(lines), lines)
	}
	want := []string{"SessionStart", "UserPromptSubmit", "PreToolUse"}
	for i, l := range lines {
		if l.Schema != hookio.LatencySchema {
			t.Errorf("line %d schema %q", i, l.Schema)
		}
		if l.Event != want[i] {
			t.Errorf("line %d event %q, want %q", i, l.Event, want[i])
		}
		if l.Session != "lat-2" || l.TS == "" {
			t.Errorf("line %d fields: %+v", i, l)
		}
		if l.TotalMS < 0 {
			t.Errorf("line %d total_ms %d", i, l.TotalMS)
		}
		if l.TimedOut {
			t.Errorf("line %d timed out", i)
		}
		if _, ok := l.Steps["trace"]; !ok {
			t.Errorf("line %d does not name the trace step: %v", i, l.Steps)
		}
		if l.Decision == "" {
			t.Errorf("line %d has no decision", i)
		}
	}
	// The sidecar is not part of the chain: every event still verifies.
	if _, errs, code := runIn(t, filepath.Dir(filepath.Dir(filepath.Dir(dir))), "", "trace", "verify"); code != cli.ExitOK && !strings.Contains(errs, "no sessions") {
		t.Errorf("trace verify after the sidecar: %v %s", code, errs)
	}
	// A total at or above the sum of that invocation's steps.
	for i, l := range lines {
		sum := 0
		for _, ms := range l.Steps {
			sum += ms
		}
		if l.TotalMS < sum {
			t.Errorf("line %d total_ms %d below its step sum %d", i, l.TotalMS, sum)
		}
	}
}

// TestSessionEventRecordsInjectedContext: the session event says what the
// step put in front of the model, so a run's injected tokens can be
// summed from the trace alone. Nothing injects on SessionStart today, so
// the honest value is zero, not absent.
func TestSessionEventRecordsInjectedContext(t *testing.T) {
	_, dir := latencySession(t, "lat-3")
	var found bool
	for _, ev := range readEvents(t, dir) {
		if ev.Type != trace.TypeSession {
			continue
		}
		found = true
		v, ok := ev.Body["context_tokens_est"]
		if !ok {
			t.Errorf("session event has no context_tokens_est: %v", ev.Body)
			continue
		}
		if n, ok := v.(float64); !ok || n != 0 {
			t.Errorf("context_tokens_est %v, want 0 (nothing injects on SessionStart)", v)
		}
	}
	if !found {
		t.Fatal("no session event")
	}
}
