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
	s := c.Settings([]string{"gate"})
	hooks := s["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok || len(hooks) < 8 {
		t.Errorf("hooks fragment incomplete: %d events", len(hooks))
	}
	if bare := c.Settings(nil)["hooks"].(map[string]any); len(bare) != 0 {
		t.Errorf("bare arm settings carry hooks: %v", bare)
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
	// Bare control arm: no .saga/, no hooks, the PATH shim block.
	out, err := c.Prepare(context.Background(), &PrepareInput{Task: tk, Workspace: ws, ConfigDir: filepath.Join(root, "cfg"), Limits: Limits{WallS: 720, MaxTurns: 200, USD: 0.45}, Blocks: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cfg", "settings.json")); err != nil {
		t.Error("settings.json not written")
	}
	if _, err := os.Stat(filepath.Join(ws, ".saga")); err == nil {
		t.Error(".saga present in the control arm workspace (bench-spec 4.2)")
	}
	if _, err := os.Stat(filepath.Join(root, "cfg", shimDir, "saga")); err != nil {
		t.Error("control arm has no PATH shim")
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
	if hooks := out.Disclosure["hooks"].([]any); len(hooks) != 0 {
		t.Errorf("%d hooks disclosed in the control arm", len(hooks))
	}
	if blocks := out.Disclosure["blocks"].([]string); len(blocks) != 1 || blocks[0] != "path-shim:saga" {
		t.Errorf("blocks %v", blocks)
	}

	// Treatment arm with gate: .saga/ with contract and request, saga on
	// PATH, the approval store, the baseline check run through the saga
	// binary (a fake here that records its argv and exits 1 = unmet).
	ws2 := filepath.Join(root, "ws2")
	os.MkdirAll(ws2, 0o755)
	fake := filepath.Join(root, "fake-saga")
	argv := filepath.Join(root, "argv")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@\" > '"+argv+"'\necho \"$SAGA_APPROVAL_DIR\" >> '"+argv+"'\necho '{}'\nexit 1\n"), 0o755)
	c2 := &ClaudeCode{Binary: "/nonexistent/claude", SagaBinary: fake, Version: "2.1.259 (Claude Code)"}
	cfg2 := filepath.Join(root, "cfg2")
	out2, err := c2.Prepare(context.Background(), &PrepareInput{Task: tk, Workspace: ws2, ConfigDir: cfg2, Components: []string{"gate"}, Limits: Limits{WallS: 720, MaxTurns: 200, USD: 0.45}, Blocks: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".saga/config.toml", ".saga/contract.md", ".saga/request.md"} {
		if _, err := os.Stat(filepath.Join(ws2, f)); err != nil {
			t.Errorf("%s missing", f)
		}
	}
	if cfgToml, _ := os.ReadFile(filepath.Join(ws2, ".saga", "config.toml")); !strings.Contains(string(cfgToml), "[gate]\nrequire_red = false\n") {
		t.Errorf("arm B config.toml lacks require_red = false (docs/12 section 4):\n%s", cfgToml)
	}
	contract, _ := os.ReadFile(filepath.Join(ws2, ".saga", "contract.md"))
	want, _ := os.ReadFile(filepath.Join(tk.Dir, "contract.md"))
	if string(contract) != string(want) {
		t.Error("contract not the task's")
	}
	if req, _ := os.ReadFile(filepath.Join(ws2, ".saga", "request.md")); string(req) != tk.Prompt() {
		t.Error("request is not the prompt")
	}
	if fi, err := os.Stat(filepath.Join(cfg2, approvedDir)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("approval store: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(cfg2, binDir, "saga")); err != nil {
		t.Error("saga not linked into the agent's PATH")
	}
	got, _ := os.ReadFile(argv)
	if !strings.HasPrefix(string(got), "gate check --approve --json\n"+filepath.Join(cfg2, approvedDir)) {
		t.Errorf("baseline check argv/env:\n%s", got)
	}
	if hooks := out2.Disclosure["hooks"].([]any); len(hooks) != 10 {
		t.Errorf("%d hooks disclosed", len(hooks))
	}
	if blocks := out2.Disclosure["blocks"].([]string); len(blocks) != 0 {
		t.Errorf("treatment arm blocks %v", blocks)
	}
}

func TestClaudeArmStaging(t *testing.T) {
	c := &ClaudeCode{SagaBinary: "/opt/saga"}
	cfg := t.TempDir()
	if err := c.stageShim(cfg); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(c.Env(cfg), "\n")
	if !strings.Contains(env, "PATH="+filepath.Join(cfg, shimDir)+string(os.PathListSeparator)) {
		t.Errorf("shim dir not first on PATH:\n%s", env)
	}
	if strings.Contains(env, "SAGA_APPROVAL_DIR=") {
		t.Errorf("control arm names an approval store")
	}
	if strings.Contains(env, "CLAUDE_CODE_ENTRYPOINT=") {
		t.Errorf("harness marker leaked into the bench environment")
	}
	shim, _ := os.ReadFile(filepath.Join(cfg, shimDir, "saga"))
	if !strings.Contains(string(shim), "exit 127") || !strings.Contains(string(shim), blockedLog) {
		t.Errorf("shim:\n%s", shim)
	}
	// Treatment: bin and approved dirs switch the environment.
	cfg2 := t.TempDir()
	os.MkdirAll(filepath.Join(cfg2, binDir), 0o755)
	os.WriteFile(filepath.Join(cfg2, binDir, "saga"), []byte("#!/bin/sh\n"), 0o755)
	env2 := strings.Join(c.Env(cfg2), "\n")
	if !strings.Contains(env2, "SAGA_APPROVAL_DIR="+filepath.Join(cfg2, approvedDir)) || !strings.Contains(env2, "PATH="+filepath.Join(cfg2, binDir)+string(os.PathListSeparator)) {
		t.Errorf("treatment env:\n%s", env2)
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
