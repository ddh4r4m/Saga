package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/gate"
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
	// The bare arm carries exactly one hook: the deny-only safety hook of
	// docs/12 row 6, with the same command string the gate arm registers.
	bare := c.Settings(nil)["hooks"].(map[string]any)
	if len(bare) != 1 {
		t.Errorf("bare arm settings carry %d hook events: %v", len(bare), bare)
	}
	bareCmds := hookCommands(t, bare)
	if len(bareCmds) != 1 || bareCmds[0] != SafetyHookCommand("/opt/saga") {
		t.Errorf("bare arm hooks %v, want only %q", bareCmds, SafetyHookCommand("/opt/saga"))
	}
	if !containsCmd(hookCommands(t, hooks), bareCmds[0]) {
		t.Errorf("the gate arm does not register the same safety hook: %v", hookCommands(t, hooks))
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
	// The control arm's .saga is an unreadable sentinel, not the layout:
	// a reach errors instead of silently creating the store (4.2).
	fi, err := os.Stat(filepath.Join(ws, ".saga"))
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0 {
		t.Errorf("control arm sentinel: %v %v", fi, err)
	}
	if _, err := os.ReadDir(filepath.Join(ws, ".saga")); err == nil {
		t.Error("the control arm sentinel must not be readable")
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
	// The bare arm discloses its one hook as a deviation, with a reason,
	// rather than leaving it to be inferred from the block list.
	hks := out.Disclosure["hooks"].([]any)
	if len(hks) != 1 {
		t.Fatalf("%d hooks disclosed in the control arm: %v", len(hks), hks)
	}
	hk := hks[0].(map[string]any)
	if hk["role"] != "safety" || hk["deviation_from_bare"] != true || hk["deviation_reason"] == nil {
		t.Errorf("safety hook disclosure %v", hk)
	}
	if hk["event"] != "PreToolUse" || !strings.Contains(hk["command"].(string), "guard hook claude-code PreToolUse") {
		t.Errorf("safety hook disclosure %v", hk)
	}
	if blocks := out.Disclosure["blocks"].([]string); strings.Join(blocks, ",") != strings.Join(ControlBlocks, ",") {
		t.Errorf("blocks %v, want %v", blocks, ControlBlocks)
	}
	// Every 4.2 surface is accounted for, and the file surface says why
	// its open count is null on the worktree substitute.
	detail := out.Disclosure["blocks_detail"].([]any)
	seen := map[string]map[string]any{}
	for _, e := range detail {
		m := e.(map[string]any)
		seen[m["surface"].(string)] = m
	}
	for _, s := range []string{"cli_binary", "mcp_server", "hooks", "files", "prompt_text"} {
		if seen[s] == nil {
			t.Errorf("blocks_detail has no %s surface: %v", s, detail)
		}
	}
	if f := seen["files"]; f == nil || f["sentinel"] != "present" || f["sentinel_open_count"] != nil || f["sentinel_open_count_reason"] != "no audit log on host" {
		t.Errorf("files surface: %v", f)
	}
	// Cleanup restores and removes it, and is harmless a second time.
	for i := 0; i < 2; i++ {
		if err := RemoveSentinel(ws); err != nil {
			t.Fatalf("RemoveSentinel %d: %v", i, err)
		}
		if _, err := os.Stat(filepath.Join(ws, ".saga")); err == nil {
			t.Error("sentinel still present after cleanup")
		}
	}

	// Treatment arm with gate: .saga/ with contract and request, saga on
	// PATH, the approval store, the baseline check run through the saga
	// binary (a fake here that records its argv and exits 1 = unmet).
	// A real git workspace, as task.Stage gives the runner: the gate
	// reads its config from the base commit, so staging has to commit it
	// and there has to be a commit to amend.
	ws2 := filepath.Join(root, "ws2")
	os.MkdirAll(ws2, 0o755)
	gitInit(t, ws2)
	fake := filepath.Join(root, "fake-saga")
	argv := filepath.Join(root, "argv")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@\" > '"+argv+"'\necho \"$SAGA_APPROVAL_DIR\" >> '"+argv+"'\necho '{}'\nexit 1\n"), 0o755)
	c2 := &ClaudeCode{Binary: "/nonexistent/claude", SagaBinary: fake, Version: "2.1.259 (Claude Code)"}
	cfg2 := filepath.Join(root, "cfg2")
	staged := StagedPrompt(tk.Prompt(), []string{"gate"})
	// SAGA_HOME keeps the stable bin dir and the corpus store off the
	// real home (ADR 0010).
	t.Setenv("SAGA_HOME", filepath.Join(root, "saga-home"))
	out2, err := c2.Prepare(context.Background(), &PrepareInput{Task: tk, Workspace: ws2, ConfigDir: cfg2, Components: []string{"gate"}, Prompt: staged, Limits: Limits{WallS: 720, MaxTurns: 200, USD: 0.45}, Blocks: []string{}, TaskSetSHA256: testTaskSet})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".saga/config.toml", ".saga/contract.md", ".saga/request.md"} {
		if _, err := os.Stat(filepath.Join(ws2, f)); err != nil {
			t.Errorf("%s missing", f)
		}
	}
	// The treatment arm's .saga is the real store, never a sentinel, and
	// cleanup must leave it alone.
	if fi, err := os.Stat(filepath.Join(ws2, ".saga")); err != nil || fi.Mode().Perm() == 0 {
		t.Errorf("treatment .saga: %v %v", fi, err)
	}
	if err := RemoveSentinel(ws2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws2, ".saga", "contract.md")); err != nil {
		t.Error("cleanup removed the treatment arm's store")
	}
	if out2.Disclosure["blocks_detail"] != nil {
		t.Errorf("blocks_detail in a treatment arm: %v", out2.Disclosure["blocks_detail"])
	}
	if cfgToml, _ := os.ReadFile(filepath.Join(ws2, ".saga", "config.toml")); !strings.Contains(string(cfgToml), "[gate]\nrequire_red = false\n") {
		t.Errorf("arm B config.toml lacks require_red = false (docs/12 section 4):\n%s", cfgToml)
	}
	contract, _ := os.ReadFile(filepath.Join(ws2, ".saga", "contract.md"))
	want, _ := os.ReadFile(filepath.Join(tk.Dir, "contract.md"))
	if string(contract) != string(want) {
		t.Error("contract not the task's")
	}
	// The harness receives the staged prompt, but request.md is the bare
	// prompt.md the contract's REQUEST: hash was computed over; the two
	// differ, and gate row 12 rejects the contract when they are confused
	// (smoke 2026-09-06 excluded every arm B run this way).
	req, _ := os.ReadFile(filepath.Join(ws2, ".saga", "request.md"))
	if string(req) != tk.Prompt() {
		t.Error("request.md is not the bare prompt.md")
	}
	if string(req) == staged {
		t.Error("request.md is the staged prompt; the contract's REQUEST: hash would not match")
	}
	if out2.PromptHash != BytesSHA256([]byte(staged)) {
		t.Errorf("prompt_hash %s is not the staged prompt's", out2.PromptHash)
	}
	parsed, perr := gate.Parse(contract)
	if perr != nil {
		t.Fatalf("task contract: %v", perr)
	}
	if parsed.Request != "" && parsed.Request != BytesSHA256(req) {
		t.Errorf("contract REQUEST: %s does not hash request.md (%s)", parsed.Request, BytesSHA256(req))
	}
	// The corpus store of ADR 0010, outside every workspace and 0700, and
	// keyed by the task set rather than by the run.
	corpus, err := CorpusStoreDir(testTaskSet)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(corpus); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("corpus approval store: %v %v", fi, err)
	}
	// Outside every workspace: the agent can neither read it nor write
	// it. (It is under SAGA_HOME here, which the test points at a scratch
	// directory so the real home is untouched.)
	if strings.HasPrefix(corpus, ws2) || strings.HasPrefix(corpus, cfg2) {
		t.Errorf("the approval store is inside the workspace or its config dir: %s", corpus)
	}
	// The saga link is at the stable, binary-keyed path, not per run.
	binHome, err := BenchBinDir(fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(binHome, "saga")); err != nil {
		t.Errorf("saga not linked at the stable bench path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg2, binDir, "saga")); err == nil {
		t.Error("the per-run bin dir is still created; the identity's PATH must not vary per run")
	}
	// A run consumes approvals and never creates one: no --approve.
	got, _ := os.ReadFile(argv)
	if !strings.HasPrefix(string(got), "gate check --json\n"+corpus) {
		t.Errorf("baseline check argv/env:\n%s", got)
	}
	if strings.Contains(string(got), "--approve") {
		t.Errorf("a run approved its own baseline:\n%s", got)
	}
	// Eleven gate hooks plus the safety hook, which is the same entry the
	// bare arm carries: identical command string, so it cannot be the
	// treatment.
	hks2 := out2.Disclosure["hooks"].([]any)
	if len(hks2) != 12 {
		t.Errorf("%d hooks disclosed", len(hks2))
	}
	var safety int
	for _, e := range hks2 {
		m := e.(map[string]any)
		if m["role"] == "safety" {
			safety++
			if _, ok := m["deviation_from_bare"]; ok {
				t.Errorf("the gate arm called the safety hook a deviation: %v", m)
			}
			if m["command"] != SafetyHookCommand(fake) {
				t.Errorf("safety hook command %q, want %q", m["command"], SafetyHookCommand(fake))
			}
		} else if m["role"] != "gate" {
			t.Errorf("hook without a role: %v", m)
		}
	}
	if safety != 1 {
		t.Errorf("%d safety hooks in the gate arm, want 1", safety)
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
	// The treatment arm's environment is covered by
	// TestBenchBinDirIsStablePerBinary, which owns the stable bin dir and
	// the corpus store.
}

// testTaskSet is a task-set hash for tests; the real one comes from
// bench/tasks/TASKSET.sha256.
const testTaskSet = "sha256:" + "11" + "22334455667788990011223344556677889900112233445566778899001122"

// TestBenchBinDirIsStablePerBinary (ADR 0010 decision 1): the approval
// identity hashes PATH, so the bench's saga has to sit at the same place
// for every run of one binary, and at a different place for another.
// While it lived under the per-run config dir every run had its own
// identity, and that is what forced a human act into every run.
func TestBenchBinDirIsStablePerBinary(t *testing.T) {
	t.Setenv("SAGA_HOME", t.TempDir())
	a := filepath.Join(t.TempDir(), "saga-a")
	b := filepath.Join(t.TempDir(), "saga-b")
	if err := os.WriteFile(a, []byte("#!/bin/sh\necho a\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("#!/bin/sh\necho b\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	da1, err := LinkBenchBinary(a)
	if err != nil {
		t.Fatal(err)
	}
	da2, err := LinkBenchBinary(a)
	if err != nil {
		t.Fatal(err)
	}
	if da1 != da2 {
		t.Errorf("the same binary linked to two places: %s and %s", da1, da2)
	}
	db, err := LinkBenchBinary(b)
	if err != nil {
		t.Fatal(err)
	}
	if db == da1 {
		t.Errorf("a different binary reused the directory %s; a binary change must invalidate approvals", db)
	}
	// The link points at the binary, and re-linking a moved binary
	// replaces it rather than leaving a stale one.
	target, err := os.Readlink(filepath.Join(da1, "saga"))
	if err != nil {
		t.Fatal(err)
	}
	if abs, _ := filepath.Abs(a); target != abs {
		t.Errorf("link points at %s, want %s", target, abs)
	}
	// The env a gate arm runs with names that directory, and the corpus
	// store rather than the operator's own ~/.saga/approved.
	corpus, err := CorpusStoreDir(testTaskSet)
	if err != nil {
		t.Fatal(err)
	}
	c := &ClaudeCode{SagaBinary: a, CorpusStore: corpus}
	env := strings.Join(c.Env(t.TempDir()), "\n")
	if !strings.Contains(env, "PATH="+da1+string(os.PathListSeparator)) {
		t.Errorf("PATH does not lead with the stable dir:\n%s", env)
	}
	if !strings.Contains(env, "SAGA_APPROVAL_DIR="+corpus) {
		t.Errorf("the corpus store is not named:\n%s", env)
	}
	// Two Prepare-shaped calls with the same binary give the same PATH,
	// which is the property the approval identity depends on.
	if p1, p2 := c.Env(t.TempDir()), c.Env(t.TempDir()); pathOf(p1) != pathOf(p2) {
		t.Errorf("PATH varies per run:\n%s\n%s", pathOf(p1), pathOf(p2))
	}
}

// pathOf returns the PATH entry of an environment slice.
func pathOf(env []string) string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			return kv
		}
	}
	return ""
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

// streamArrayResult is a tool_result whose content is an array of text
// blocks, the shape the harness uses for multi-part results.
const streamArrayResult = `{"type":"system","subtype":"init","session_id":"s2","model":"claude-opus-5","tools":["Bash"],"permissionMode":"acceptEdits"}
{"type":"assistant","message":{"id":"m1","model":"claude-opus-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"pytest -q"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":false,"content":[{"type":"text","text":"3 passed"},{"type":"text","text":"in 0.4s"}]}]}}
{"type":"assistant","message":{"id":"m2","model":"claude-opus-5","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"src/a.py"}}]}}
{"type":"result","subtype":"success","is_error":false,"num_turns":2,"result":"Done.\n\nDONE"}
`

func TestParseStreamToolUses(t *testing.T) {
	// The duplicate assistant message of the shared fixture must not
	// duplicate the tool use, and is_error flows through to the result.
	r := ParseStream([]byte(stream))
	if len(r.ToolUses) != 1 {
		t.Fatalf("tool uses: %+v", r.ToolUses)
	}
	tu := r.ToolUses[0]
	if tu.ID != "t1" || tu.Name != "Bash" || tu.Input["command"] != "ls" {
		t.Errorf("tool use: %+v", tu)
	}
	if tu.Result == nil || !tu.Result.IsError || tu.Result.Text != "" {
		t.Errorf("result: %+v", tu.Result)
	}
	// A content array joins its text blocks with newlines; a tool use the
	// stream never answers keeps a nil result.
	r = ParseStream([]byte(streamArrayResult))
	if len(r.ToolUses) != 2 {
		t.Fatalf("tool uses: %+v", r.ToolUses)
	}
	if r.ToolUses[0].Result == nil || r.ToolUses[0].Result.Text != "3 passed\nin 0.4s" || r.ToolUses[0].Result.IsError {
		t.Errorf("array result: %+v", r.ToolUses[0].Result)
	}
	if r.ToolUses[1].Result != nil || r.ToolUses[1].Input["file_path"] != "src/a.py" {
		t.Errorf("unanswered tool use: %+v", r.ToolUses[1])
	}
	if r.ToolCalls[0].Error || r.ToolCalls[1].Error {
		t.Errorf("tool call errors: %+v", r.ToolCalls)
	}
}

// TestHarnessFailureIsInfra: Claude Code retries inside the harness and
// emits no retry event, so the bench sees only the terminal failure. A
// run the harness gave up on says nothing about the model and must be
// excluded rather than counted as a fail (bench-spec 3.3, docs/12 row 14).
func TestHarnessFailureIsInfra(t *testing.T) {
	for _, c := range []struct {
		subtype string
		isError bool
		want    bool
	}{
		{"error_during_execution", true, true},
		{"error_api", true, true},
		{"error_max_turns", true, false},      // a real outcome, not infra
		{"error_max_budget_usd", true, false}, // likewise
		{"success", false, false},
		{"", false, false},
		{"error_during_execution", false, false}, // not flagged an error
	} {
		if got := IsHarnessFailure(c.subtype, c.isError); got != c.want {
			t.Errorf("subtype %q isError %v: %v, want %v", c.subtype, c.isError, got, c.want)
		}
	}

	// End to end through Collect on a synthetic stream.
	dir := t.TempDir()
	native := filepath.Join(dir, "native.jsonl")
	line := `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"API Error: 529 overloaded"}` + "\n"
	if err := os.WriteFile(native, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &ClaudeCode{Version: "test"}
	out, err := c.Collect(context.Background(), &CollectInput{
		Workspace: dir, ConfigDir: dir, NativeLogPath: native,
		Seed: strings.Repeat("ab", 32), Run: &RunOutput{}, Prompt: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != "infra" {
		t.Errorf("outcome %q, want infra", out.Outcome)
	}
	if !strings.Contains(out.OutcomeReason, "error_during_execution") || !strings.Contains(out.OutcomeReason, "529") {
		t.Errorf("outcome reason %q must name the subtype and the error", out.OutcomeReason)
	}
}

// TestRetriesDisclosure: the block says whose retries they are.
func TestRetriesDisclosure(t *testing.T) {
	tk := fixtureTask(t)
	root := t.TempDir()
	c := &ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: "saga"}
	ws := filepath.Join(root, "ws")
	os.MkdirAll(ws, 0o755)
	out, err := c.Prepare(context.Background(), &PrepareInput{Task: tk, Workspace: ws, ConfigDir: filepath.Join(root, "cfg"), Blocks: []string{}, Limits: Limits{WallS: 60, MaxTurns: 5, USD: 1}})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := out.Disclosure["retries"].(map[string]any)
	if r == nil || r["policy"] != "harness-internal" || r["count"] != nil || r["count_reason"] == nil {
		t.Errorf("retries block %v", r)
	}
}

// hookCommands lists every command string in a settings hooks object.
func hookCommands(t *testing.T, hooks map[string]any) []string {
	t.Helper()
	var out []string
	for _, list := range hooks {
		for _, entry := range list.([]any) {
			for _, hk := range entry.(map[string]any)["hooks"].([]any) {
				out = append(out, hk.(map[string]any)["command"].(string))
			}
		}
	}
	sort.Strings(out)
	return out
}

func containsCmd(list []string, want string) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

// gitInit makes dir a git repository with one commit, the shape
// task.Stage leaves behind.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"}, {"commit", "-q", "--allow-empty", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}
