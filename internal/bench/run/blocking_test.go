package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/guard"
)

// fakeAgent tries every reach path of bench-spec 4.2 in order and
// records what each one did. It runs with the prepared arm's own
// environment and working directory, so PATH, HOME and CLAUDE_CONFIG_DIR
// are exactly what the harness would have seen.
const fakeAgent = `#!/bin/sh
echo "== saga gate check"
saga gate check >/dev/null 2>&1; echo "exit $?"
echo "== saga --version"
saga --version >/dev/null 2>&1; echo "exit $?"
echo "== cat .saga/contract.md"
cat .saga/contract.md >/dev/null 2>&1; echo "exit $?"
echo "== write .saga/probe"
{ mkdir -p .saga && echo x > .saga/probe; } >/dev/null 2>&1; echo "exit $?"
echo "== ls .saga"
ls .saga >/dev/null 2>&1; echo "exit $?"
`

// exits parses the fake agent's transcript into one exit code per probe.
func exits(t *testing.T, out string) []int {
	t.Helper()
	var codes []int
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "exit ") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(line, "exit %d", &n); err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		codes = append(codes, n)
	}
	return codes
}

// prepArm stages a workspace, prepares the arm and returns the workspace,
// the config dir and the prepare output.
func prepArm(t *testing.T, tk *task.Task, c *adapter.ClaudeCode, components []string) (ws, cfg string, out *adapter.PrepareOutput) {
	t.Helper()
	root := t.TempDir()
	ws, cfg = filepath.Join(root, "ws"), filepath.Join(root, "cfg")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := task.Stage(context.Background(), tk, ws); err != nil {
		t.Fatal(err)
	}
	prompt := adapter.StagedPrompt(tk.Prompt(), components)
	// SAGA_HOME keeps the stable bin dir and the corpus approval store of
	// ADR 0010 off the real home; the task-set hash keys the store.
	t.Setenv("SAGA_HOME", filepath.Join(root, "saga-home"))
	set := "sha256:" + strings.Repeat("ab", 32)
	// The store is the owner's, and Prepare no longer creates it (a run
	// that created its store turned a wrong key into an empty directory,
	// 2026-09-13), so the test stands in for the approval.
	if store, err := adapter.CorpusStoreDir(set); err == nil {
		if err := os.MkdirAll(store, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	out, err := c.Prepare(context.Background(), &adapter.PrepareInput{
		Task: tk, Workspace: ws, ConfigDir: cfg, Components: components, Prompt: prompt,
		Blocks: []string{}, Limits: adapter.Limits{WallS: 60, MaxTurns: 5, USD: 1},
		FrozenSetSHA256: set,
	})
	if err != nil {
		t.Fatalf("prepare %v: %v", components, err)
	}
	return ws, cfg, out
}

// runAgent executes the fake agent in ws with the arm's environment.
func runAgent(t *testing.T, c *adapter.ClaudeCode, ws, cfg string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(script, []byte(fakeAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", script)
	cmd.Dir = ws
	cmd.Env = c.Env(cfg, "")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("fake agent: %v\n%s", err, buf.String())
	}
	return buf.String()
}

// TestControlArmBlocking is the bench-spec 10.2 blocking test: a fake
// agent tries every reach path of 4.2 in the bare arm, every attempt is
// logged or fails, none succeeds, and blocked_reach_attempts equals the
// number of CLI reaches. The same agent in a gate arm succeeds, so the
// block is arm-specific and not an accident of the environment.
func TestControlArmBlocking(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tk := loadTasks(t, "ts-0001-slug-collapse")[0]
	c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: fakeSaga(t)}

	ws, cfg, prep := prepArm(t, tk, c, nil)
	got := exits(t, runAgent(t, c, ws, cfg))
	// gate check, --version, cat, write, ls: every one refused.
	if len(got) != 5 {
		t.Fatalf("%d probes: %v", len(got), got)
	}
	for i, code := range got {
		if code == 0 {
			t.Errorf("probe %d succeeded in the bare arm", i)
		}
	}
	if got[0] != 127 || got[1] != 127 {
		t.Errorf("the shim must exit 127, got %v", got[:2])
	}
	// The whole bare environment against the allowlist of bench-spec 4.2,
	// here as well as in TestBareArmEnvHasNoGateVariables, so the arm the
	// blocking test drives is the one the assertion covers.
	for _, kv := range c.Env(cfg, prep.CorpusStore) {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_") || strings.HasPrefix(k, "LC_") {
			continue
		}
		if !bareEnvAllowed[k] {
			t.Errorf("the bare arm's environment carries %s, which bench-spec 4.2 does not allow", k)
		}
	}
	// The two CLI reaches are logged, and nothing was created under .saga.
	log, err := os.ReadFile(filepath.Join(cfg, "blocked.log"))
	if err != nil {
		t.Fatalf("blocked.log: %v", err)
	}
	if n := len(strings.Split(strings.TrimRight(string(log), "\n"), "\n")); n != 2 {
		t.Errorf("%d shim log lines, want 2:\n%s", n, log)
	}
	if err := os.Chmod(filepath.Join(ws, ".saga"), 0o700); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Join(ws, ".saga")); err != nil || len(entries) != 0 {
		t.Errorf("sentinel contents %v %v", entries, err)
	}
	if err := os.Chmod(filepath.Join(ws, ".saga"), 0o000); err != nil {
		t.Fatal(err)
	}
	// The generated settings register neither hooks nor an MCP server.
	raw, err := os.ReadFile(filepath.Join(cfg, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	// The bare arm carries exactly one hook and it is the safety hook of
	// docs/12 row 6: no Saga component hook, and nothing else.
	h, _ := settings["hooks"].(map[string]any)
	if len(h) != 1 {
		t.Errorf("%d hook events in the bare arm's settings: %v", len(h), h)
	}
	bareCmds := settingsHookCommands(t, settings)
	wantCmd := adapter.SafetyHookCommand(c.SagaBinary)
	if len(bareCmds) != 1 || bareCmds[0] != wantCmd {
		t.Errorf("bare arm hooks %v, want only %q", bareCmds, wantCmd)
	}
	if _, ok := settings["mcpServers"]; ok {
		t.Error("mcpServers in the bare arm's settings")
	}
	// The safety hook denies a destructive command here, and says which
	// rule denied it. This is the hook the harness would invoke: the
	// command string out of the arm's own settings, run with the arm's
	// own environment and working directory.
	if reason := fireSafetyHook(t, c, cfg, ws, "rm -rf "+ws); !strings.Contains(reason, "saga guard: D") {
		t.Errorf("rm -rf of the workspace root was not denied in the bare arm: %q", reason)
	}
	if reason := fireSafetyHook(t, c, cfg, ws, "go test ./..."); reason != "" {
		t.Errorf("ordinary work denied in the bare arm: %q", reason)
	}
	if n := guard.CountDenies(filepath.Join(cfg, "guard.jsonl")); n != 1 {
		t.Errorf("%d denies logged in the bare arm, want 1", n)
	}
	// Every surface is named in the disclosure's blocks list.
	blocks, _ := prep.Disclosure["blocks"].([]string)
	for _, want := range ControlBlocks {
		if !adapter.HasComponent(blocks, want) {
			t.Errorf("block %q missing from the disclosure: %v", want, blocks)
		}
	}
	// blocked_reach_attempts is the shim count, read the way Collect does.
	col := collectBare(t, c, tk, ws, cfg)
	if col.BlockedReachAttempts != 2 {
		t.Errorf("blocked_reach_attempts %d, want 2", col.BlockedReachAttempts)
	}
	if col.GuardDenies != 1 {
		t.Errorf("guard_denies %d, want 1", col.GuardDenies)
	}
	// The sentinel does not change the graded diff: removing it leaves a
	// tree byte-identical to one that never carried it.
	if err := adapter.RemoveSentinel(ws); err != nil {
		t.Fatal(err)
	}
	withSentinel, err := task.Diff(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(t.TempDir(), "plain")
	if err := task.Stage(context.Background(), tk, plain); err != nil {
		t.Fatal(err)
	}
	without, err := task.Diff(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withSentinel, without) {
		t.Errorf("the sentinel changed the diff:\n%s\n---\n%s", withSentinel, without)
	}

	// Treatment arm: the same agent reaches the CLI and the store.
	ws2, cfg2, _ := prepArm(t, tk, c, []string{"gate"})
	got2 := exits(t, runAgent(t, c, ws2, cfg2))
	if len(got2) != 5 {
		t.Fatalf("%d probes: %v", len(got2), got2)
	}
	if got2[1] != 0 {
		t.Errorf("saga --version exit %d in the gate arm", got2[1])
	}
	if got2[2] != 0 {
		t.Errorf("cat .saga/contract.md exit %d in the gate arm", got2[2])
	}
	if _, err := os.Stat(filepath.Join(cfg2, "blocked.log")); err == nil {
		t.Error("the gate arm must have no shim log")
	}
	// The safety hook is the same entry with the same command string in
	// the gate arm, and it denies the same command. That identity is what
	// keeps it a safety net rather than part of the treatment.
	raw2, err := os.ReadFile(filepath.Join(cfg2, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings2 map[string]any
	if err := json.Unmarshal(raw2, &settings2); err != nil {
		t.Fatal(err)
	}
	gateCmds := settingsHookCommands(t, settings2)
	if !contains(gateCmds, wantCmd) {
		t.Errorf("the gate arm does not register the safety hook %q: %v", wantCmd, gateCmds)
	}
	if reason := fireSafetyHook(t, c, cfg2, ws2, "rm -rf "+ws2); !strings.Contains(reason, "saga guard: D") {
		t.Errorf("rm -rf of the workspace root was not denied in the gate arm: %q", reason)
	}
	if n := guard.CountDenies(filepath.Join(cfg2, "guard.jsonl")); n != 1 {
		t.Errorf("%d denies logged in the gate arm, want 1", n)
	}
}

// settingsHookCommands lists every hook command in a settings object.
func settingsHookCommands(t *testing.T, settings map[string]any) []string {
	t.Helper()
	var out []string
	hooks, _ := settings["hooks"].(map[string]any)
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

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// fireSafetyHook invokes the arm's registered PreToolUse hook the way the
// harness would: the command string out of the arm's own settings, the
// arm's own environment and working directory, and a PreToolUse payload
// on stdin. It returns the deny reason, or "" when the hook allowed.
func fireSafetyHook(t *testing.T, c *adapter.ClaudeCode, cfg, ws, command string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(cfg, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	var hookCmd string
	for _, s := range settingsHookCommands(t, settings) {
		if strings.Contains(s, " guard hook ") {
			hookCmd = s
		}
	}
	if hookCmd == "" {
		t.Fatal("no safety hook in the arm's settings")
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": "blocking-test", "cwd": ws, "hook_event_name": "PreToolUse",
		"tool_name": "Bash", "tool_use_id": "tu", "tool_input": map[string]any{"command": command},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", hookCmd)
	cmd.Dir = ws
	cmd.Env = c.Env(cfg, "")
	cmd.Stdin = bytes.NewReader(payload)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("safety hook %q: %v\n%s", hookCmd, err, errb.String())
	}
	var res struct {
		Out struct {
			Decision string `json:"permissionDecision"`
			Reason   string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("safety hook output %q: %v", out.String(), err)
	}
	if res.Out.Decision != "deny" {
		return ""
	}
	return res.Out.Reason
}

// fakeSaga stands in for the CLI. The bare arm never reaches it (the
// PATH shim answers first, which is the point of the test) and the gate
// arm needs only a binary that answers --version and returns an unmet
// baseline, because `gate check --approve` is a human act that refuses
// under an agent shell (docs/12 row 7) and cannot run inside `go test`.
// `guard` is the exception: it is delegated to a real build, so the
// safety hook the arms register is the real classifier and the command
// string under test is the one the settings actually carry.
func fakeSaga(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "saga")
	real := buildSaga(t)
	script := "#!/bin/sh\ncase \"$1\" in\n  --version) echo 'saga 0.0.0-test'; exit 0;;\n  guard) exec '" + real + "' \"$@\";;\nesac\necho '{}'\nexit 1\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// buildSaga compiles the CLI once per test binary.
var sagaBuild struct {
	path string
	err  error
	once sync.Once
}

func buildSaga(t *testing.T) string {
	t.Helper()
	sagaBuild.once.Do(func() {
		dir, err := os.MkdirTemp("", "saga-build")
		if err != nil {
			sagaBuild.err = err
			return
		}
		out := filepath.Join(dir, "saga")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/saga")
		cmd.Dir = repoRootOf(t)
		if b, err := cmd.CombinedOutput(); err != nil {
			sagaBuild.err = fmt.Errorf("go build: %v\n%s", err, b)
			return
		}
		sagaBuild.path = out
	})
	if sagaBuild.err != nil {
		t.Fatal(sagaBuild.err)
	}
	return sagaBuild.path
}

// repoRootOf finds the module root from this test file's location.
func repoRootOf(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// collectBare runs the adapter's Collect over an empty native log, which
// is the path blocked_reach_attempts and blocks_detail are filled on.
func collectBare(t *testing.T, c *adapter.ClaudeCode, tk *task.Task, ws, cfg string) *adapter.CollectOutput {
	t.Helper()
	native := filepath.Join(cfg, "native.jsonl")
	if err := os.WriteFile(native, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := c.Collect(context.Background(), &adapter.CollectInput{
		Task: tk, Workspace: ws, ConfigDir: cfg, NativeLogPath: native,
		Seed: strings.Repeat("ab", 32), Run: &adapter.RunOutput{}, Prompt: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := col.Disclosure["blocks_detail"].([]any)
	if len(detail) != 5 {
		t.Fatalf("blocks_detail has %d surfaces", len(detail))
	}
	if m := detail[0].(map[string]any); m["surface"] != "cli_binary" || m["count"] != col.BlockedReachAttempts {
		t.Errorf("cli_binary surface: %v", m)
	}
	var hooksSurface map[string]any
	for _, e := range detail {
		if m := e.(map[string]any); m["surface"] == "hooks" {
			hooksSurface = m
		}
	}
	if hooksSurface == nil || hooksSurface["block"] != "settings:no-saga-hooks-but-safety" || hooksSurface["count"] != col.GuardDenies {
		t.Errorf("hooks surface: %v (guard_denies %d)", hooksSurface, col.GuardDenies)
	}
	return col
}

// bareEnvAllowed is every variable the bare arm's environment may carry
// (bench-spec 4.2): the private HOME and config dir, the harness's own
// switches, the safety hook's log, the operator's passthrough set and
// the credentials. Nothing named for a Saga component belongs here.
var bareEnvAllowed = map[string]bool{
	"HOME": true, "CLAUDE_CONFIG_DIR": true, "PATH": true,
	"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": true, "DISABLE_AUTOUPDATER": true,
	"DISABLE_TELEMETRY": true, "DISABLE_ERROR_REPORTING": true, "CI": true,
	// The deny-only safety hook of docs/12 row 6 runs in both arms and
	// writes its decisions here, so it is not a treatment surface.
	"SAGA_GUARD_LOG": true,
	"TMPDIR":         true, "LANG": true, "SHELL": true, "USER": true, "LOGNAME": true,
	"TERM": true, "SSL_CERT_FILE": true, "HTTPS_PROXY": true, "HTTP_PROXY": true,
	"NO_PROXY": true, "https_proxy": true, "http_proxy": true, "no_proxy": true,
	"NODE_OPTIONS": true, "NODE_EXTRA_CA_CERTS": true,
}

// TestBareArmEnvHasNoGateVariables (bench-spec 4.2): a gate arm's
// Prepare must leave nothing behind that reaches the next bare arm.
// `RunArms` shares one *ClaudeCode across arms, and until 2026-09-13 the
// corpus store was a field on it, so `SAGA_APPROVAL_DIR` appeared in
// every arm A environment after the first arm B Prepare: absent on the
// first task of the pilot and present on the other nineteen. The store
// is now a value carried from Prepare to Run, so the order of the arms
// cannot change what the bare arm sees.
func TestBareArmEnvHasNoGateVariables(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tk := loadTasks(t, "ts-0001-slug-collapse")[0]
	c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: fakeSaga(t)}

	// The gate arm first, which is the order that used to poison the next
	// bare arm, and the same adapter for both.
	_, _, gatePrep := prepArm(t, tk, c, []string{"gate"})
	if gatePrep.CorpusStore == "" {
		t.Fatal("the gate arm resolved no corpus store, so this test proves nothing")
	}
	_, bareCfg, barePrep := prepArm(t, tk, c, nil)
	if barePrep.CorpusStore != "" {
		t.Errorf("the bare arm carries a corpus store: %q", barePrep.CorpusStore)
	}

	env := c.Env(bareCfg, barePrep.CorpusStore)
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k == gate.ApprovalEnv {
			t.Errorf("the bare arm's environment names an approval store: %s=%s", k, v)
		}
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_") || strings.HasPrefix(k, "LC_") {
			continue
		}
		if !bareEnvAllowed[k] {
			t.Errorf("the bare arm's environment carries %s, which bench-spec 4.2 does not allow", k)
		}
	}
	// And the gate arm still has its own store, so the fix did not simply
	// remove the variable from both arms.
	gateEnv := strings.Join(c.Env(bareCfg, gatePrep.CorpusStore), "\n")
	if !strings.Contains(gateEnv, gate.ApprovalEnv+"="+gatePrep.CorpusStore) {
		t.Error("the gate arm's environment does not name its corpus store")
	}
}
