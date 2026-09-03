package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/task"
)

func TestReplayTraceValid(t *testing.T) {
	tk := fixtureTask(t)
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	if err := task.Stage(context.Background(), tk, ws); err != nil {
		t.Skip("git unavailable:", err)
	}
	cfg := filepath.Join(root, "cfg")
	os.MkdirAll(cfg, 0o755)
	r := &Replay{Patch: "gold,broken-1"}
	seed := strings.Repeat("ab", 32)
	ro, err := r.Run(context.Background(), &RunInput{Task: tk, Workspace: ws, ConfigDir: cfg, Seed: seed, Index: 2})
	if err != nil {
		t.Fatal(err)
	}
	col, err := r.Collect(context.Background(), &CollectInput{Task: tk, Workspace: ws, ConfigDir: cfg, NativeLogPath: ro.NativeLogPath, Seed: seed, Run: ro})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(col.FinalMessage, "broken-1") {
		t.Errorf("index 2 should apply broken-1: %q", col.FinalMessage)
	}
	lines := strings.Split(strings.TrimSpace(string(col.TraceJSONL)), "\n")
	if len(lines) != 6 {
		t.Fatalf("%d trace events, want 6:\n%s", len(lines), col.TraceJSONL)
	}
	for _, want := range []string{`"type":"session"`, `"type":"turn"`, `"type":"tool_call"`, `"type":"tool_result"`} {
		if !strings.Contains(string(col.TraceJSONL), want) {
			t.Errorf("trace lacks %s", want)
		}
	}
}
