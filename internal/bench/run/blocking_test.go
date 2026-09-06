package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
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
	out, err := c.Prepare(context.Background(), &adapter.PrepareInput{
		Task: tk, Workspace: ws, ConfigDir: cfg, Components: components, Prompt: prompt,
		Blocks: []string{}, Limits: adapter.Limits{WallS: 60, MaxTurns: 5, USD: 1},
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
	cmd.Env = c.Env(cfg)
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
	if h, _ := settings["hooks"].(map[string]any); len(h) != 0 {
		t.Errorf("hooks in the bare arm's settings: %v", h)
	}
	if _, ok := settings["mcpServers"]; ok {
		t.Error("mcpServers in the bare arm's settings")
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
}

// fakeSaga stands in for the CLI. The bare arm never reaches it (the
// PATH shim answers first, which is the point of the test) and the gate
// arm needs only a binary that answers --version and returns an unmet
// baseline, because `gate check --approve` is a human act that refuses
// under an agent shell (docs/12 row 7) and cannot run inside `go test`.
func fakeSaga(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "saga")
	script := "#!/bin/sh\ncase \"$1\" in --version) echo 'saga 0.0.0-test'; exit 0;; esac\necho '{}'\nexit 1\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
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
	return col
}
