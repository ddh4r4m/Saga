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
	lines := strings.Split(strings.TrimSpace(string(col.StreamTraceJSONL)), "\n")
	if len(lines) != 6 {
		t.Fatalf("%d trace events, want 6:\n%s", len(lines), col.StreamTraceJSONL)
	}
	for _, want := range []string{`"type":"session"`, `"type":"turn"`, `"type":"tool_call"`, `"type":"tool_result"`} {
		if !strings.Contains(string(col.StreamTraceJSONL), want) {
			t.Errorf("trace lacks %s", want)
		}
	}
}

// TestStagedPromptInBothArms: the DONE/NOT-DONE instruction ends the prompt
// identically in every arm; a gate arm carries the one contract sentence
// before it; both adapters disclose the staged prompt's hash.
func TestStagedPromptInBothArms(t *testing.T) {
	bare := StagedPrompt("Fix the thing.\n", nil)
	gated := StagedPrompt("Fix the thing.\n", []string{"gate"})
	for _, p := range []string{bare, gated} {
		if !strings.HasSuffix(p, ProtocolSentence+"\n") || !strings.HasPrefix(p, "Fix the thing.\n\n") {
			t.Errorf("staged prompt: %q", p)
		}
	}
	if strings.Contains(bare, ContractSentence) || !strings.Contains(gated, ContractSentence) {
		t.Errorf("contract sentence placement: bare %q gated %q", bare, gated)
	}
	if strings.Index(gated, ContractSentence) > strings.Index(gated, ProtocolSentence) {
		t.Error("protocol sentence must be last")
	}
	tk := fixtureTask(t)
	root := t.TempDir()
	cfg := filepath.Join(root, "cfg")
	os.MkdirAll(cfg, 0o755)
	os.MkdirAll(filepath.Join(root, "ws"), 0o755)
	staged := StagedPrompt(tk.Prompt(), nil)
	in := &PrepareInput{Task: tk, Workspace: filepath.Join(root, "ws"), ConfigDir: cfg, Prompt: staged, Limits: Limits{WallS: 60, MaxTurns: 5, USD: 1}}
	ro, err := (&Replay{Patch: "gold"}).Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want := BytesSHA256([]byte(staged))
	if ro.PromptHash != want || ro.Disclosure["prompt_hash"] != nil && ro.Disclosure["prompt_hash"] != want {
		t.Errorf("replay prompt hash %s, want %s", ro.PromptHash, want)
	}
	c := &ClaudeCode{Version: "test", SagaBinary: "saga"}
	co, err := c.Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if co.PromptHash != want || co.Disclosure["prompt_hash"] != want {
		t.Errorf("claude prompt hash %s, want %s", co.PromptHash, want)
	}
}
