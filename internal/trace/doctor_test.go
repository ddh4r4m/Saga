package trace

import (
	"path/filepath"
	"testing"

	"github.com/ddh4r4m/saga/internal/store"
)

// TestHooksFire: a registered hook is not a firing hook, so the check
// reads what a session actually recorded (docs/12 row 10). Each of the
// three events is required, and the message says which one is missing so
// a live probe can act on it.
func TestHooksFire(t *testing.T) {
	newStore := func(t *testing.T) *store.Store {
		t.Helper()
		dir := t.TempDir()
		if _, err := store.Init(dir); err != nil {
			t.Fatal(err)
		}
		return store.Open(dir)
	}
	write := func(t *testing.T, st *store.Store, session string, evs []Event) {
		t.Helper()
		w, err := OpenWriter(SessionDir(st, session), session, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		for i := range evs {
			if err := w.Append(&evs[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	call := Event{Turn: 1, Type: TypeToolCall, Source: "hook:PreToolUse", Body: map[string]any{"tool": "Bash", "args_hash": "sha256:" + zeros, "component": "harness", "cwd_rel": ".", "index_version": nil}}
	result := Event{Turn: 1, Type: TypeToolResult, Source: "hook:PostToolUse", Body: map[string]any{"for_seq": 1, "exit": 0, "error": nil, "result_hash": "sha256:" + zeros, "result_bytes": 0, "truncated": false, "wall_ms": 0, "served": "live"}}
	claim := Event{Turn: 1, Type: TypeGate, Source: "hook:Stop", Body: map[string]any{"kind": "claim", "verdict": "verified"}}

	for _, c := range []struct {
		name string
		evs  []Event
		ok   bool
		want string
	}{
		{"complete chain", []Event{call, result, claim}, true, "stop claim"},
		{"no tool_call", []Event{claim}, false, "PreToolUse hook did not fire"},
		{"no tool_result", []Event{call, claim}, false, "PostToolUse hook did not fire"},
		{"no stop claim", []Event{call, result}, false, "Stop hook did not fire"},
	} {
		st := newStore(t)
		write(t, st, "s-"+filepath.Base(c.name), c.evs)
		ok, detail := hooksFire(st, "")
		if ok != c.ok {
			t.Errorf("%s: ok %v, want %v (%s)", c.name, ok, c.ok, detail)
		}
		if !contains(detail, c.want) {
			t.Errorf("%s: detail %q must mention %q", c.name, detail, c.want)
		}
	}

	// No session at all is a clear failure, not a pass by absence.
	st := newStore(t)
	if ok, detail := hooksFire(st, ""); ok || !contains(detail, "no recorded session") {
		t.Errorf("empty store: %v %q", ok, detail)
	}
}

const zeros = "0000000000000000000000000000000000000000000000000000000000000000"

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
