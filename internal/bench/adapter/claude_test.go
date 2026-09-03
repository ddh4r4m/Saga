package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/schema"
)

func fixtureTask(t *testing.T) *task.Task {
	t.Helper()
	dirs, err := task.Find(filepath.Join("..", "..", "..", "bench", "tasks", "ts-0001-slug-collapse"))
	if err != nil || len(dirs) == 0 {
		t.Skip("corpus not found")
	}
	tk, err := task.Load(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestClaudeArgsAndEnv(t *testing.T) {
	tk := fixtureTask(t)
	c := &ClaudeCode{SagaBinary: "/opt/saga", Model: "claude-opus-5"}
	in := &RunInput{Task: tk, Limits: Limits{WallS: 720, MaxTurns: 200, USD: 0.45}, Seed: strings.Repeat("ab", 32)}
	args := c.Args(in, "/cfg/settings.json", SessionID(in.Seed))
	want := []string{"-p", "--output-format", "stream-json", "--verbose", "--max-turns", "200", "--permission-mode", "acceptEdits", "--tools", "Bash,Read,Edit,Write,MultiEdit,Grep,Glob", "--settings", "/cfg/settings.json", "--session-id", SessionID(in.Seed), "--max-budget-usd", "0.45", "--model", "claude-opus-5"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("args\n got %q\nwant %q", args, want)
	}
	for _, must := range []string{"Grep", "Glob", "Read"} {
		if !strings.Contains(args[9], must) {
			t.Errorf("tools missing %s (harness-facts C32)", must)
		}
	}
	id := SessionID(in.Seed)
	if len(id) != 36 || id[14] != '4' || id != SessionID(in.Seed) {
		t.Errorf("session id %q", id)
	}
	env := c.Env("/cfg")
	joined := strings.Join(env, "\n")
	for _, must := range []string{"HOME=/cfg/home", "CLAUDE_CONFIG_DIR=/cfg/claude-config", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"} {
		if !strings.Contains(joined, must) {
			t.Errorf("env missing %s", must)
		}
	}
	for i := 1; i < len(env); i++ {
		if env[i-1] > env[i] {
			t.Errorf("env not sorted at %d", i)
		}
	}
	s := c.Settings()
	hooks := s["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok || len(hooks) < 8 {
		t.Errorf("hooks fragment incomplete: %d events", len(hooks))
	}
	cmd := hooks["Stop"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if cmd != "/opt/saga hook claude-code Stop" {
		t.Errorf("hook command %q", cmd)
	}
}

func TestClaudePrepareDisclosure(t *testing.T) {
	tk := fixtureTask(t)
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	os.MkdirAll(ws, 0o755)
	c := &ClaudeCode{Binary: "/nonexistent/claude", SagaBinary: "/nonexistent/saga", Version: "2.1.259 (Claude Code)"}
	out, err := c.Prepare(context.Background(), &PrepareInput{Task: tk, Workspace: ws, ConfigDir: filepath.Join(root, "cfg"), Limits: Limits{WallS: 720, MaxTurns: 200, USD: 0.45}, Blocks: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cfg", "settings.json")); err != nil {
		t.Error("settings.json not written")
	}
	if _, err := os.Stat(filepath.Join(ws, ".saga", "config.toml")); err != nil {
		t.Error(".saga not initialised in the workspace")
	}
	v, err := schema.Normalize(out.Disclosure)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateID("saga.bench.harness/1", v); err != nil {
		t.Errorf("disclosure invalid: %v", err)
	}
	h := out.Disclosure.Block("harness")
	if h["version"] != "2.1.259 (Claude Code)" || h["binary_sha256"] != nil || h["binary_sha256_reason"] == nil {
		t.Errorf("harness block %v", h)
	}
	if hooks := out.Disclosure["hooks"].([]any); len(hooks) != 10 {
		t.Errorf("%d hooks disclosed", len(hooks))
	}
}

const stream = `{"type":"system","subtype":"init","session_id":"s1","model":"claude-opus-5","tools":["Bash","Read","Grep","Glob"],"permissionMode":"acceptEdits"}
{"type":"assistant","message":{"id":"m1","model":"claude-opus-5","usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":20},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":true}]}}
{"type":"assistant","message":{"id":"m1","model":"claude-opus-5","usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":20},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"assistant","message":{"id":"m2","model":"claude-opus-5","usage":{"input_tokens":5,"cache_read_input_tokens":160,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":7},"output_tokens":30},"content":[{"type":"text","text":"done"}]}}
{"type":"result","subtype":"success","is_error":false,"num_turns":2,"total_cost_usd":0.0123,"result":"Fixed the slug function. Done.","usage":{"input_tokens":15,"cache_read_input_tokens":260,"cache_creation_input_tokens":57,"output_tokens":50}}
`

func TestParseStream(t *testing.T) {
	r := ParseStream([]byte(stream))
	if r.Model != "claude-opus-5" || r.NumTurns != 2 || r.CostUSD == nil || *r.CostUSD != 0.0123 || r.FinalMessage != "Fixed the slug function. Done." {
		t.Errorf("result: %+v", r)
	}
	if r.UsageFrom != "result" || r.Usage.InputFresh != 15 || r.Usage.CacheRead != 260 || r.Usage.CacheWrite5m != 57 || r.Usage.Output != 50 {
		t.Errorf("usage: %+v", r.Usage)
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Tool != "Bash" || !r.ToolCalls[0].Error || r.ToolCalls[0].ArgsHash == "" {
		t.Errorf("tool calls: %+v", r.ToolCalls)
	}
	if len(r.Tools) != 4 || r.PermMode != "acceptEdits" {
		t.Errorf("init: %+v", r)
	}
	// Without a result event the assistant messages are summed, deduplicated on id.
	noResult := strings.Join(strings.Split(stream, "\n")[:5], "\n")
	r = ParseStream([]byte(noResult))
	if r.UsageFrom != "assistant-sum" || r.Usage.InputFresh != 15 || r.Usage.CacheRead != 260 || r.Usage.CacheWrite5m != 50 || r.Usage.CacheWrite1h != 7 || r.Usage.Output != 50 {
		t.Errorf("summed usage: %+v", r.Usage)
	}
}
