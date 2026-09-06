package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	claudecode "github.com/ddh4r4m/saga/adapters/claude-code"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/guard"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// ClaudeCode runs `claude -p` headless (bench-spec section 6.1) with a
// bench-generated minimal settings file: Saga hooks bound to
// `saga hook claude-code <event>` in a treatment arm, an explicit tool
// list that restores Grep and Glob (harness-facts C32), the task's
// permission mode and turn cap, and a fresh HOME and CLAUDE_CONFIG_DIR so
// the operator's own configuration is never read (section 3.1).
//
// Authentication: a fresh config dir holds no login (on macOS the OAuth
// credential lives in the Keychain under a service name scoped to the
// config dir; the 2026-09-05 smoke observed "Not logged in" with total
// cost 0), so the operator supplies CLAUDE_CODE_OAUTH_TOKEN (from
// `claude setup-token`) or ANTHROPIC_API_KEY in the environment; both
// pass through and are redacted in harness.json. The bench never reads
// the operator's ~/.claude.
//
// Arms: with Components empty the run is the bare control arm of section
// 4.2: no hooks, no .saga/, and a PATH shim `saga` that logs the reach to
// blocked.log and exits 127 (blocked_reach_attempts). With "gate" among
// the components the workspace gets .saga/ with the task's contract and
// the prompt as the request, the baseline red run with approval (a human
// act, so the bench must be launched from a human shell), `saga` on the
// agent's PATH and an owner-private approval store under the config dir.
type ClaudeCode struct {
	// Binary is the claude executable; "claude" on PATH by default.
	Binary string
	// SagaBinary is the saga executable the hooks call.
	SagaBinary string
	// Model is passed as --model when set.
	Model string
	// Tools overrides DefaultTools.
	Tools []string
	// Version, when set, skips executing `claude --version`.
	Version string
	// CorpusStore is the corpus approval store a gate arm consumes
	// (ADR 0010). Empty leaves SAGA_APPROVAL_DIR unset, which is the
	// bare arm and every non-gate use.
	CorpusStore string
}

// BenchHome is the root of the bench's own state outside any workspace:
// the stable saga links and the corpus approval stores. SAGA_HOME
// overrides it, which is how a test keeps off the real home.
func BenchHome() (string, error) {
	if h := os.Getenv("SAGA_HOME"); h != "" {
		return filepath.Join(h, "bench"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".saga", "bench"), nil
}

// BenchBinDir is where the gate arm's `saga` lives for a given binary:
// a directory named by the binary's own hash, so the approval
// identity's PATH component is stable across runs of one binary and
// different for another (ADR 0010 decision 1).
func BenchBinDir(sagaBinary string) (string, error) {
	sum := FileSHA256(sagaBinary)
	if sum == "" {
		return "", fmt.Errorf("bench bin dir: %s is unreadable", sagaBinary)
	}
	root, err := BenchHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bin", strings.TrimPrefix(sum, "sha256:")[:16]), nil
}

// LinkBenchBinary puts sagaBinary at BenchBinDir/saga, replacing a link
// that points elsewhere. It returns the directory to put on PATH.
func LinkBenchBinary(sagaBinary string) (string, error) {
	abs, err := filepath.Abs(sagaBinary)
	if err != nil {
		return "", err
	}
	dir, err := BenchBinDir(abs)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, "saga")
	if cur, err := os.Readlink(link); err == nil && cur == abs {
		return dir, nil
	}
	// Replace atomically: a link pointing at a moved binary must not
	// survive, and two runs may race here.
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(abs, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return dir, nil
}

// CorpusStoreDir is the approval store for one frozen task set
// (ADR 0010 decision 2). It is outside every workspace and 0700: the
// agent can neither read nor write it.
func CorpusStoreDir(taskSetHash string) (string, error) {
	h := strings.TrimPrefix(taskSetHash, "sha256:")
	if len(h) != 64 {
		return "", fmt.Errorf("corpus store: %q is not a task-set hash", taskSetHash)
	}
	root, err := BenchHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "approved", h), nil
}

// DefaultTools is the -p tool list; C32: Grep and Glob must be named.
var DefaultTools = []string{"Bash", "Read", "Edit", "Write", "MultiEdit", "Grep", "Glob"}

// Name implements Adapter.
func (c *ClaudeCode) Name() string { return "claude-code" }

func (c *ClaudeCode) binary() string {
	if c.Binary != "" {
		return c.Binary
	}
	return "claude"
}

func (c *ClaudeCode) tools() []string {
	if len(c.Tools) > 0 {
		return c.Tools
	}
	return DefaultTools
}

// SessionID derives the Claude session id (a UUID v4 string) from the
// run seed so the transcript path is known in advance.
func SessionID(seed string) string {
	h := []byte(seed)
	if len(h) < 32 {
		h = append(h, []byte(strings.Repeat("0", 32))...)
	}
	b := make([]byte, 16)
	for i := 0; i < 16; i++ {
		var v byte
		fmt.Sscanf(seed[i*2%len(seed):]+"00", "%02x", &v)
		b[i] = v
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// HasComponent reports whether name is among the arm's components.
func HasComponent(components []string, name string) bool {
	for _, c := range components {
		if c == name {
			return true
		}
	}
	return false
}

// Settings is the generated settings.json: Saga hooks when the arm has
// them, a permissive allow list for the bench tools, no co-author trailer.
func (c *ClaudeCode) Settings(components []string) map[string]any {
	saga := c.SagaBinary
	if saga == "" {
		saga = "saga"
	}
	var allow []string
	for _, t := range c.tools() {
		allow = append(allow, t)
	}
	hooks := map[string]any{}
	if HasComponent(components, "gate") {
		hooks = claudecode.Fragment(saga)["hooks"].(map[string]any)
	}
	// The deny-only safety hook of docs/12 row 6 is registered in every
	// arm with the same command string. Row 5 takes the worktree
	// substitute for containers, so the agent runs on the owner's
	// machine; a destructive command has to be refused by the same code
	// on both sides or the safety net becomes a treatment. It is not the
	// composed chain: it takes no snapshot, reads no policy and touches
	// nothing under .saga, so it works unchanged in a bare arm whose
	// .saga is an unreadable sentinel.
	var pre []any
	if list, ok := hooks[hookio.EventPreToolUse].([]any); ok {
		pre = list
	}
	pre = append(pre, map[string]any{"hooks": []any{map[string]any{
		"type": "command", "command": SafetyHookCommand(saga), "timeout": claudecode.Timeout,
	}}})
	hooks[hookio.EventPreToolUse] = pre
	return map[string]any{
		"hooks":               hooks,
		"permissions":         map[string]any{"allow": allow, "deny": []string{}},
		"includeCoAuthoredBy": false,
		"env":                 map[string]string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"},
	}
}

// SafetyHookCommand is the command string both arms register for the
// safety hook. It names the saga binary by absolute path, so it runs in
// the bare arm too: the PATH shim blocks the agent's reach for the
// component, not the bench's own safety net.
func SafetyHookCommand(binary string) string {
	return binary + " guard hook " + claudecode.Harness + " " + hookio.EventPreToolUse
}

// Per-run files under the config dir that Env reads back, so Prepare,
// Run and the baseline check agree on the environment without state.
const (
	shimDir    = "shim" // control arm: PATH shim `saga`
	binDir     = "bin"  // treatment arm: symlink to the saga binary (legacy; see BenchBinDir)
	blockedLog = "blocked.log"
	guardLog   = "guard.jsonl" // both arms: the safety hook's decision log
)

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// passthrough lists the operator environment variables the harness
// still needs (auth, network, locale); everything else is dropped.
var passthrough = []string{"PATH", "TMPDIR", "LANG", "SHELL", "USER", "LOGNAME", "TERM", "SSL_CERT_FILE", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy", "NODE_OPTIONS", "NODE_EXTRA_CA_CERTS"}

// Env is the environment for the harness process: a private HOME and
// CLAUDE_CONFIG_DIR under configDir plus the passthrough set and every
// ANTHROPIC_* and CLAUDE_CODE_* variable (credentials). The control
// arm's shim directory or the treatment arm's bin directory is put first
// on PATH when Prepare created it, and the treatment arm names its
// approval store. Sorted so the disclosure block is stable.
func (c *ClaudeCode) Env(configDir string) []string {
	env := map[string]string{
		"HOME":              filepath.Join(configDir, "home"),
		"CLAUDE_CONFIG_DIR": filepath.Join(configDir, "claude-config"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"DISABLE_AUTOUPDATER":                      "1",
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CI":                                       "1",
		// The safety hook appends here. It is set on the harness process
		// rather than in the settings `env` block because a hook is a
		// child of that process and inherits its environment, which needs
		// no harness fact to hold; the settings block's reach into hook
		// processes is not one of the pinned facts.
		guard.LogEnv: filepath.Join(configDir, guardLog),
	}
	for _, k := range passthrough {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	// PATH is composed, never inherited. The approval identity hashes the
	// whole of it (gate-spec 8), so an inherited PATH binds the corpus
	// approvals to the shell that gave them: the owner approved 101
	// records at 7d80087 and `--check` from another shell reported
	// covered 0 of 40 for the same binary and the same task set. See
	// BenchPath.
	delete(env, "PATH")
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_") || strings.HasPrefix(k, "LC_") {
			if _, set := env[k]; !set {
				env[k] = v
			}
		}
	}
	// CLAUDE_CODE_ENTRYPOINT is the harness's own marker for its children
	// (gate.AgentShellMarkers), not a credential; it must not leak into
	// the bench's shells.
	delete(env, "CLAUDE_CODE_ENTRYPOINT")
	env["PATH"] = strings.Join(c.BenchPath(configDir), string(os.PathListSeparator))
	// The corpus approval store a gate arm consumes; a run never writes
	// one (ADR 0010 decision 3).
	if c.CorpusStore != "" {
		env[gate.ApprovalEnv] = c.CorpusStore
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// Args builds the claude -p argument list for a run. The prompt is fed
// on stdin so prompt length never hits an argv limit.
func (c *ClaudeCode) Args(in *RunInput, settingsPath, sessionID string) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", fmt.Sprint(in.Limits.MaxTurns),
		"--permission-mode", in.Task.PermissionMode(),
		"--tools", strings.Join(c.tools(), ","),
		"--settings", settingsPath,
		"--session-id", sessionID,
		"--max-budget-usd", fmt.Sprintf("%.2f", in.Limits.USD),
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	return args
}

func (c *ClaudeCode) version(ctx context.Context) (string, error) {
	if c.Version != "" {
		return c.Version, nil
	}
	out, err := exec.CommandContext(ctx, c.binary(), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", c.binary(), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Prepare implements Adapter: writes the settings file, stages the arm
// (shim or .saga/ with the contract, request and baseline approval), and
// fills the disclosure block.
func (c *ClaudeCode) Prepare(ctx context.Context, in *PrepareInput) (*PrepareOutput, error) {
	if in.SagaBinary != "" && c.SagaBinary == "" {
		c.SagaBinary = in.SagaBinary
	}
	for _, d := range []string{filepath.Join(in.ConfigDir, "home"), filepath.Join(in.ConfigDir, "claude-config")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	settings, err := json.MarshalIndent(c.Settings(in.Components), "", "  ")
	if err != nil {
		return nil, err
	}
	settingsPath := filepath.Join(in.ConfigDir, "settings.json")
	if err := os.WriteFile(settingsPath, append(settings, '\n'), 0o644); err != nil {
		return nil, err
	}
	blocks := append([]string{}, in.Blocks...)
	withGate := HasComponent(in.Components, "gate")
	if withGate {
		if err := c.stageGate(ctx, in); err != nil {
			return nil, err
		}
	} else {
		if err := c.stageShim(in.ConfigDir); err != nil {
			return nil, err
		}
		if err := stageSentinel(in.Workspace); err != nil {
			return nil, err
		}
		blocks = append(blocks, ControlBlocks...)
	}
	version, err := c.version(ctx)
	if err != nil {
		return nil, err
	}
	d := NewDisclosure("claude-code", in.Limits, blocks)
	h := d.Block("harness")
	Set(h, "version", version, "")
	if p, err := exec.LookPath(c.binary()); err == nil {
		if sum := FileSHA256(p); sum != "" {
			Set(h, "binary_sha256", sum, "")
		} else {
			Set(h, "binary_sha256", nil, "binary unreadable")
		}
	} else {
		Set(h, "binary_sha256", nil, "binary not on PATH")
	}
	m := d.Block("model")
	if c.Model != "" {
		Set(m, "id", c.Model, "")
	} else {
		Set(m, "id", nil, "harness default model; filled from the stream-json init event at collect")
	}
	Set(m, "seed", nil, "claude -p has no seed parameter")
	m["seed_supported"] = false
	toolsCanon, _ := canon.JSON(c.tools())
	tl := d.Block("tools")
	tl["names"] = c.tools()
	Set(tl, "sha256", canon.SHA256(toolsCanon), "")
	p := d.Block("permissions")
	Set(p, "mode", in.Task.PermissionMode(), "")
	Set(p, "sandbox", "none", "")
	p["allow"] = c.Settings(in.Components)["permissions"].(map[string]any)["allow"]
	p["deny"] = []string{}
	ctxb := d.Block("context")
	Set(ctxb, "compaction", "auto", "")
	hooks := []any{}
	saga := c.SagaBinary
	sagaSum := FileSHA256(saga)
	for ev, list := range c.Settings(in.Components)["hooks"].(map[string]any) {
		for _, entry := range list.([]any) {
			for _, hk := range entry.(map[string]any)["hooks"].([]any) {
				cmd := hk.(map[string]any)["command"].(string)
				hm := map[string]any{"event": ev, "command": cmd, "role": "gate"}
				if cmd == SafetyHookCommand(saga) {
					// The one hook a bare arm carries. It is named as a
					// deviation there rather than left to be inferred from
					// the block list (bench-spec 4.2).
					hm["role"] = "safety"
					if !withGate {
						hm["deviation_from_bare"] = true
						hm["deviation_reason"] = "docs/12 row 5 takes the worktree substitute for containers, so the agent runs on the owner's machine; the deny-only safety hook runs with the same command string in the gate arm, so it cannot be the treatment"
					}
				}
				if sagaSum != "" {
					hm["script_sha256"] = sagaSum
				} else {
					hm["script_sha256"] = nil
					hm["script_sha256_reason"] = "saga binary path unresolved"
				}
				hooks = append(hooks, hm)
			}
		}
	}
	sort.Slice(hooks, func(i, j int) bool {
		a, b := hooks[i].(map[string]any), hooks[j].(map[string]any)
		if a["event"].(string) != b["event"].(string) {
			return a["event"].(string) < b["event"].(string)
		}
		// PreToolUse carries two entries in the gate arm, so the command
		// breaks the tie and the block stays byte-stable.
		return a["command"].(string) < b["command"].(string)
	})
	d["hooks"] = hooks
	envMap := map[string]string{}
	for _, kv := range c.Env(in.ConfigDir) {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_OAUTH") || k == "PATH" {
			v = "<redacted>"
		}
		envMap[k] = v
	}
	d["env_vars"] = envMap
	// The composed PATH, so a reader can see what the agent had and what
	// the approval identity was taken over (brief 2026-09-06 canonical
	// bench path). It is never the caller's inherited PATH.
	d["bench_path"] = c.BenchPath(in.ConfigDir)
	if !withGate {
		d["blocks_detail"] = BlocksDetail(nil, nil)
	} else {
		// Proof that the gate will read the protocol's config and not its
		// own defaults. Until 2026-09-06 it read the defaults in every
		// arm B run and nothing said so (dev run finding 1).
		// The corpus approval store this arm consumed, and who approved
		// each gate: a report can then say that no run approved anything
		// (ADR 0010 decision 5).
		d["approval_store"] = c.approvalStoreBlock(in.TaskSetSHA256)
		sha, present := GateConfigAtBase(ctx, in.Workspace)
		d["gate_config_present"] = present
		if present {
			Set(d, "gate_config_sha256", sha, "")
		} else {
			Set(d, "gate_config_sha256", nil, "no .saga/config.toml at the base commit; the gate would run on its defaults")
		}
	}
	// Claude Code retries inside the harness and emits no retry event in
	// stream-json, so the bench records the terminal failure only
	// (bench-spec 3.3, docs/12 row 14).
	d["retries"] = map[string]any{
		"policy":       "harness-internal",
		"count":        nil,
		"count_reason": "claude-code retries inside the harness and does not emit retry events in stream-json (checked on 2.1.263)",
	}
	d["config_hash"] = BytesSHA256(settings)
	d["prompt_hash"] = BytesSHA256([]byte(in.PromptOf()))
	return &PrepareOutput{ConfigHash: BytesSHA256(settings), PromptHash: BytesSHA256([]byte(in.PromptOf())), ToolsHash: canon.SHA256(toolsCanon), Disclosure: d}, nil
}

// ControlBlocks are the bench-spec 4.2 blocks applied in a bare arm, one
// per surface the component lives at: the CLI binary behind a PATH shim,
// MCP servers absent from the generated settings and the private config
// dir, no Saga hook but the deny-only safety hook of docs/12 row 6
// (registered identically in both arms, so it is not a treatment), and
// .saga present as an unreadable sentinel so a reach errors rather than
// silently creating the layout. The prompt
// surface needs no block: the bare arm's prompt is prompt.md plus the
// protocol sentence, and prompt_hash records it.
var ControlBlocks = []string{"path-shim:saga", "settings:no-saga-hooks-but-safety", "settings:no-mcp", "sentinel:.saga"}

// sentinelDir is the store path the bare arm blocks.
const sentinelDir = ".saga"

// stageSentinel creates the bare arm's unreadable .saga (bench-spec 4.2
// files row). A read or a write under it fails with EACCES, which is the
// signal the agent is meant to see; without it a `Write` to .saga/x
// would silently create the layout and the arm would stop being bare.
func stageSentinel(workspace string) error {
	p := filepath.Join(workspace, sentinelDir)
	if err := os.Mkdir(p, 0o000); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.Chmod(p, 0o000)
}

// RemoveSentinel restores and removes the bare arm's sentinel. It runs
// before the workspace diff and again on the way out, so it must be
// idempotent, and it never touches a real store: only a directory that
// is still unreadable and still empty is removed. A directory the agent
// somehow populated is left in place, readable, as evidence; task.Diff
// excludes .saga either way.
func RemoveSentinel(workspace string) error {
	p := filepath.Join(workspace, sentinelDir)
	fi, err := os.Lstat(p)
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0 {
		return nil
	}
	if err := os.Chmod(p, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(p)
	if err != nil || len(entries) > 0 {
		return err
	}
	return os.Remove(p)
}

// BlocksDetail is the disclosure's per-surface account of bench-spec 4.2
// in a bare arm: what blocks the surface and what counts the reaches.
// The worktree substitute of docs/12 row 5 has no sandbox audit log, so
// the sentinel reports its presence and a null open count with a reason
// rather than a number it cannot produce.
func BlocksDetail(shimHits, guardDenies *int) []any {
	cli := map[string]any{"surface": "cli_binary", "block": "path-shim:saga", "instrumentation": "blocked_reach_attempts", "count": nil}
	if shimHits != nil {
		cli["count"] = *shimHits
	} else {
		cli["count_reason"] = "counted at collect from the shim log"
	}
	hk := map[string]any{"surface": "hooks", "block": "settings:no-saga-hooks-but-safety", "instrumentation": "guard_denies", "count": nil,
		"block_reason": "the generated settings carry no Saga hook in a bare arm; the one exception is the deny-only safety hook of docs/12 row 6, registered with the same command string in both arms so it is never a treatment"}
	if guardDenies != nil {
		hk["count"] = *guardDenies
	} else {
		hk["count_reason"] = "counted at collect from the safety hook's log"
	}
	return []any{
		cli,
		map[string]any{"surface": "mcp_server", "block": "settings:no-mcp", "instrumentation": nil,
			"instrumentation_reason": "no MCP server is registered in the generated settings or the private config dir, so there is nothing to connect to and no attempt to log"},
		hk,
		map[string]any{"surface": "files", "block": "sentinel:.saga", "instrumentation": "sentinel",
			"sentinel": "present", "sentinel_open_count": nil, "sentinel_open_count_reason": "no audit log on host"},
		map[string]any{"surface": "prompt_text", "block": "none", "instrumentation": "prompt_hash",
			"block_reason": "the bare arm's prompt is prompt.md plus the protocol sentence; prompt_hash records it"},
	}
}

// commitStore amends the workspace's base commit to carry the staged
// .saga/config.toml and .saga/contract.md, forced past the store's own
// gitignore. The gate then reads at BASE: exactly what the bench staged,
// and `gate_config_present` in the disclosure proves it did.
func commitStore(ctx context.Context, ws string) error {
	git := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
		}
		return nil
	}
	if err := git("add", "-f", ".saga/config.toml", ".saga/contract.md"); err != nil {
		return err
	}
	// An amend, not a new commit: the contract's BASE: resolves to HEAD
	// and the task's own base commit stays the thing the diff is taken
	// against, so the graded diff is unchanged.
	return git("commit", "-q", "--amend", "--no-edit", "--allow-empty")
}

// GateConfigAtBase returns the gate config as the gate itself reads it,
// through `git show <rev>:.saga/config.toml`, and whether it was there.
// The disclosure carries both, so an arm that silently ran on defaults
// can never be graded as one that ran on the protocol's config.
func GateConfigAtBase(ctx context.Context, ws string) (sha string, present bool) {
	cmd := exec.CommandContext(ctx, "git", "show", "HEAD:.saga/config.toml")
	cmd.Dir = ws
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return "", false
	}
	return BytesSHA256(out), true
}

// stageShim writes the control arm's PATH shim (bench-spec 4.2): `saga`
// logs its arguments to blocked.log and exits 127.
func (c *ClaudeCode) stageShim(configDir string) error {
	dir := filepath.Join(configDir, shimDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	log := filepath.Join(configDir, blockedLog)
	script := "#!/bin/sh\n# saga bench control arm: the component is blocked here (bench-spec 4.2)\nprintf '%s\\n' \"saga $*\" >> '" + log + "'\necho 'saga: not available in this environment' >&2\nexit 127\n"
	return os.WriteFile(filepath.Join(dir, "saga"), []byte(script), 0o755)
}

// stageGate prepares the treatment arm: .saga/ in the workspace with the
// prompt as the request and the task contract, `saga` on the agent's
// PATH, an owner-private approval store, and the baseline `saga gate
// check --approve` so the reds are recorded and the CHECK: lines are
// approved before the agent starts (gate-spec 3.2; the approval is a
// human act and is refused inside an agent shell).
func (c *ClaudeCode) stageGate(ctx context.Context, in *PrepareInput) error {
	if err := c.stageGateFiles(ctx, in); err != nil {
		return err
	}
	return c.baselineCheck(ctx, in)
}

// stageGateFiles is everything a gate arm needs before the baseline
// check: the corpus store, the stable binary link, the store, the
// config, the request and the contract, all in the base commit. It is
// shared with `approve-corpus`, so the approval identity the owner
// records is exactly the one a run will present (ADR 0010).
func (c *ClaudeCode) stageGateFiles(ctx context.Context, in *PrepareInput) error {
	if c.SagaBinary == "" {
		return fmt.Errorf("gate arm: no saga binary")
	}
	sagaAbs, err := filepath.Abs(c.SagaBinary)
	if err != nil {
		return err
	}
	// The corpus approval store this run consumes. It is never the
	// operator's own ~/.saga/approved: without a task-set hash there is
	// no store to name, and a gate arm that silently read the operator's
	// personal approvals would be approving itself by accident.
	// `approve-corpus` sets the store itself, because it computes the
	// task-set hash from the freeze file; a run derives it from the
	// manifest. Either way it is never empty for a gate arm, and never
	// the operator's own ~/.saga/approved.
	if c.CorpusStore == "" {
		if in.TaskSetSHA256 == "" {
			return fmt.Errorf("gate arm: no task-set hash, so no corpus approval store (ADR 0010)")
		}
		corpus, err := CorpusStoreDir(in.TaskSetSHA256)
		if err != nil {
			return err
		}
		c.CorpusStore = corpus
	}
	if err := os.MkdirAll(c.CorpusStore, 0o700); err != nil {
		return err
	}

	// The stable directory of ADR 0010 decision 1, not a per-run one: the
	// approval identity hashes PATH, so a per-run directory made every
	// run's identity different and forced a human act into every run.
	if _, err := LinkBenchBinary(sagaAbs); err != nil {
		return err
	}
	if _, err := store.Init(in.Workspace); err != nil {
		return fmt.Errorf("saga init in workspace: %w", err)
	}
	st := store.Open(in.Workspace)
	// docs/12 section 4 arm table: arm B runs the task's contract with
	// require_red = false; regression gates green at baseline would
	// otherwise hold Stop at exit 5 for the whole run.
	cfgToml, err := os.ReadFile(st.Path("config.toml"))
	if err != nil {
		return fmt.Errorf("config.toml: %w", err)
	}
	if !bytes.Contains(cfgToml, []byte("require_red")) {
		cfgToml = bytes.Replace(cfgToml, []byte("[gate]\n"), []byte("[gate]\nrequire_red = false\n"), 1)
		if err := store.WriteFileAtomic(st.Path("config.toml"), cfgToml, 0o644); err != nil {
			return fmt.Errorf("config.toml: %w", err)
		}
	}
	man, _, _ := st.ReadManifest()
	for _, l := range []string{"trace", "gate"} {
		if !HasComponent(man.Layers, l) {
			man.Layers = append(man.Layers, l)
		}
	}
	if err := st.WriteManifest(man); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	// request.md is the bare prompt.md: the contract's REQUEST: hash and
	// FROM: spans trace to the task author's request, not to the bench's
	// staged sentences (which prompt_hash records). Staging the composed
	// prompt here detached every arm B contract at row 12 (smoke 2026-09-06).
	if err := store.WriteFileAtomic(st.Path("request.md"), []byte(in.Task.Prompt()), 0o644); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	contract, err := os.ReadFile(filepath.Join(in.Task.Dir, "contract.md"))
	if err != nil {
		return fmt.Errorf("task contract: %w", err)
	}
	if err := store.WriteFileAtomic(st.Path("contract.md"), contract, 0o644); err != nil {
		return fmt.Errorf("contract: %w", err)
	}
	// The gate reads its config and contract from the base commit, not
	// from the working tree: `gate.Load` calls `LoadConfig(root, base)`,
	// which is `git show <base>:.saga/config.toml`. `saga init` gitignores
	// .saga, so until now the staged `require_red = false` was in a file
	// no commit carried, `LoadConfig` fell back to the defaults with
	// Present false, and arm B ran with require_red ON and mode
	// "minimal" in every run to 2026-09-06, which is what held three of
	// the dev run's arm B runs at a block they could not clear. Reading
	// the working tree instead would remove the property that an agent
	// cannot loosen its own gate mid-run, so the fix is here: put the two
	// files in the base commit before the agent starts.
	if err := commitStore(ctx, in.Workspace); err != nil {
		return err
	}

	return nil
}

// baselineCheck runs the task's contract at the base tree: it must be
// red, and every gate must already carry a corpus approval.
func (c *ClaudeCode) baselineCheck(ctx context.Context, in *PrepareInput) error {
	sagaAbs, err := filepath.Abs(c.SagaBinary)
	if err != nil {
		return err
	}
	// Baseline red, in the agent's environment so the approval identity
	// (which includes PATH) matches the agent's checks. **No --approve**:
	// a run consumes the corpus approvals the owner gave once and never
	// creates one (ADR 0010 decision 3). Exit 4 means a gate has no
	// record, which is infra rather than a result: the arm did not run
	// the treatment the manifest names.
	cmd := exec.CommandContext(ctx, sagaAbs, "gate", "check", "--json")
	cmd.Dir = in.Workspace
	cmd.Env = c.Env(in.ConfigDir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	var ee *exec.ExitError
	code := 0
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return fmt.Errorf("baseline check: %w", err)
	}
	if err := os.WriteFile(filepath.Join(in.ConfigDir, "baseline.json"), out.Bytes(), 0o644); err != nil {
		return err
	}
	switch code {
	case 0:
		return fmt.Errorf("baseline check: every gate met before any work (task verify-task should have failed)")
	case 1, 5:
		return nil
	case int(cli.ExitApproval):
		return &NotPreApproved{Gates: approvalMissing(out.Bytes()), Store: c.CorpusStore}
	default:
		return fmt.Errorf("baseline check exited %d: %s", code, strings.TrimSpace(errb.String()))
	}
}

// NotPreApproved is the baseline check finding a gate with no corpus
// approval. The runner turns it into an infra outcome: the owner has
// not approved this task set for this binary, so the run would not be
// the treatment the manifest names (ADR 0010 decision 3).
type NotPreApproved struct {
	Gates []string
	Store string
}

func (e *NotPreApproved) Error() string {
	ids := strings.Join(e.Gates, " ")
	if ids == "" {
		ids = "unknown"
	}
	return "not pre-approved: " + ids + " (run `saga bench approve-corpus` from your terminal)"
}

// approvalMissing reads the gate ids the check reported as unapproved,
// from the per-gate `approval` field the status JSON already carries.
// Nothing in gate changes for this.
func approvalMissing(raw []byte) []string {
	var rep struct {
		Gates []struct {
			ID       string `json:"id"`
			Approval string `json:"approval"`
		} `json:"gates"`
	}
	if json.Unmarshal(raw, &rep) != nil {
		return nil
	}
	var out []string
	for _, g := range rep.Gates {
		if g.Approval == "missing" {
			out = append(out, g.ID)
		}
	}
	return out
}

// Run implements Adapter.
func (c *ClaudeCode) Run(ctx context.Context, in *RunInput) (*RunOutput, error) {
	prompt, err := os.ReadFile(in.PromptPath)
	if err != nil {
		return nil, err
	}
	settingsPath := filepath.Join(in.ConfigDir, "settings.json")
	sessionID := SessionID(in.Seed)
	native := filepath.Join(in.ConfigDir, "native.jsonl")
	out, err := os.Create(native)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	cmd := exec.CommandContext(ctx, c.binary(), c.Args(in, settingsPath, sessionID)...)
	cmd.Dir = in.Workspace
	cmd.Env = c.Env(in.ConfigDir)
	cmd.Stdin = bytes.NewReader(prompt)
	cmd.Stdout = out
	if in.Log != nil {
		cmd.Stderr = in.Log
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return cmd.Process.Kill()
	}
	err = cmd.Run()
	res := &RunOutput{NativeLogPath: native}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		res.Exit = ee.ExitCode()
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.TimedOut = true
		res.Exit = 124
	default:
		return res, err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
	}
	return res, nil
}

// StreamResult is what the stream-json parser extracts.
type StreamResult struct {
	Model        string
	Tools        []string
	Usage        trace.Usage
	UsageFrom    string
	NumTurns     int
	ToolCalls    []ToolCall
	FinalMessage string
	Subtype      string
	IsError      bool
	CostUSD      *float64
	SessionID    string
	PermMode     string
	// ClaudeCodeVersion is the init event's claude_code_version.
	ClaudeCodeVersion string
	// ServedModels lists the model of every assistant message, in order
	// (duplicates included); the pin record deduplicates.
	ServedModels []string
	// AssistantTexts are the text blocks of every assistant message in
	// order, so a bare terminal marker can be read against what the model
	// actually said earlier in the turn (2026-09-06 smoke, finding 3).
	AssistantTexts []string
	// APIKeySource is the init event's apiKeySource ("none" when not
	// logged in, the 2026-09-05 smoke's dry-run finding).
	APIKeySource string
	// ToolUses are the tool_use blocks in order with their arguments and
	// the matching tool_result, deduplicated on id like ToolCalls. They
	// are what StreamTrace synthesises the tool events from, so the claim
	// verifier reconciles against the harness's own stream in every arm
	// (docs/12 section 13, amendment of 2026-09-06).
	ToolUses []StreamToolUse
}

// StreamToolUse is one tool_use block of the stream with its result.
type StreamToolUse struct {
	ID    string
	Name  string
	Input map[string]any
	// Result is nil when the stream carries no tool_result for the id.
	Result *StreamToolResult
}

// StreamToolResult is the harness's tool_result for one tool use: the
// text content (a string, or the text blocks of a content array joined
// with newlines) and the is_error flag.
type StreamToolResult struct {
	Text    string
	IsError bool
}

// resultText renders a tool_result content field: a plain string, or the
// text fields of a content array joined with newlines.
func resultText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, blk := range t {
			b, _ := blk.(map[string]any)
			if s, ok := b["text"].(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ParseStream normalises a `--output-format stream-json` log. Usage is
// the result event's total when present (the harness's own accounting),
// otherwise the sum over assistant messages deduplicated on message id.
func ParseStream(raw []byte) StreamResult {
	var r StreamResult
	toolRes := map[string]*StreamToolResult{}
	toolByID := map[string]int{}
	seen := map[string]bool{}
	var summed trace.Usage
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev map[string]any
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch ev["type"] {
		case "system":
			if ev["subtype"] == "init" {
				if m, _ := ev["model"].(string); m != "" {
					r.Model = m
				}
				if tools, ok := ev["tools"].([]any); ok {
					for _, t := range tools {
						if s, ok := t.(string); ok {
							r.Tools = append(r.Tools, s)
						}
					}
				}
				r.SessionID, _ = ev["session_id"].(string)
				r.PermMode, _ = ev["permissionMode"].(string)
				r.ClaudeCodeVersion, _ = ev["claude_code_version"].(string)
				r.APIKeySource, _ = ev["apiKeySource"].(string)
			}
		case "assistant":
			msg, _ := ev["message"].(map[string]any)
			if msg == nil {
				continue
			}
			id, _ := msg["id"].(string)
			if m, _ := msg["model"].(string); m != "" {
				if r.Model == "" {
					r.Model = m
				}
				if id == "" || !seen[id] {
					r.ServedModels = append(r.ServedModels, m)
				}
			}
			if u, ok := msg["usage"].(map[string]any); ok && (id == "" || !seen[id]) {
				seen[id] = true
				uu := trace.UsageFromAnthropic(u, "claude-code:stream-json", "5m")
				summed.InputFresh += uu.InputFresh
				summed.CacheRead += uu.CacheRead
				summed.CacheWrite5m += uu.CacheWrite5m
				summed.CacheWrite1h += uu.CacheWrite1h
				summed.Output += uu.Output
			}
			content, _ := msg["content"].([]any)
			for _, blk := range content {
				b, _ := blk.(map[string]any)
				if b["type"] == "text" {
					if txt, _ := b["text"].(string); strings.TrimSpace(txt) != "" {
						r.AssistantTexts = append(r.AssistantTexts, txt)
					}
					continue
				}
				if b["type"] != "tool_use" {
					continue
				}
				name, _ := b["name"].(string)
				argsCanon, _ := canon.JSON(b["input"])
				tc := ToolCall{Tool: name, ArgsHash: canon.SHA256(argsCanon)}
				id, _ := b["id"].(string)
				if id != "" {
					if _, dup := toolByID[id]; dup {
						continue // the same message re-emitted with more blocks
					}
					toolByID[id] = len(r.ToolCalls)
				}
				input, _ := b["input"].(map[string]any)
				r.ToolCalls = append(r.ToolCalls, tc)
				r.ToolUses = append(r.ToolUses, StreamToolUse{ID: id, Name: name, Input: input})
			}
		case "user":
			msg, _ := ev["message"].(map[string]any)
			content, _ := msg["content"].([]any)
			for _, blk := range content {
				b, _ := blk.(map[string]any)
				if b["type"] != "tool_result" {
					continue
				}
				id, _ := b["tool_use_id"].(string)
				if id == "" {
					continue
				}
				if _, dup := toolRes[id]; dup {
					continue // a re-emitted user message repeats the result
				}
				isErr, _ := b["is_error"].(bool)
				toolRes[id] = &StreamToolResult{Text: resultText(b["content"]), IsError: isErr}
			}
		case "result":
			r.Subtype, _ = ev["subtype"].(string)
			r.IsError, _ = ev["is_error"].(bool)
			if n, ok := ev["num_turns"].(float64); ok {
				r.NumTurns = int(n)
			}
			if cost, ok := ev["total_cost_usd"].(float64); ok {
				r.CostUSD = &cost
			}
			if s, ok := ev["result"].(string); ok {
				r.FinalMessage = s
			}
			if u, ok := ev["usage"].(map[string]any); ok {
				r.Usage = trace.UsageFromAnthropic(u, "claude-code:result", "5m")
				r.UsageFrom = "result"
			}
		}
	}
	for id, i := range toolByID {
		res := toolRes[id]
		if res == nil {
			continue
		}
		r.ToolUses[i].Result = res
		r.ToolCalls[i].Error = res.IsError
	}
	if r.UsageFrom == "" {
		summed.Source = "claude-code:stream-json"
		summed.ReasoningReason = strp("anthropic bills thinking inside output")
		r.Usage = summed
		r.UsageFrom = "assistant-sum"
	}
	r.Usage.Raw = nil
	return r
}

// Collect implements Adapter.
func (c *ClaudeCode) Collect(ctx context.Context, in *CollectInput) (*CollectOutput, error) {
	raw, err := os.ReadFile(in.NativeLogPath)
	if err != nil {
		return nil, err
	}
	sr := ParseStream(raw)
	out := &CollectOutput{
		Usage: sr.Usage, Model: sr.Model, Turns: sr.NumTurns, ToolCalls: len(sr.ToolCalls), ToolSequence: sr.ToolCalls,
		FinalMessage: sr.FinalMessage, HarnessCostUSD: sr.CostUSD, Outcome: "completed", Disclosure: Disclosure{},
	}
	if out.Model == "" {
		out.Model = c.Model
	}
	switch {
	case sr.Subtype == "error_max_turns":
		out.Outcome = "turn_cap"
	case sr.Subtype == "error_max_budget_usd":
		out.Outcome = "budget"
	case in.Run != nil && in.Run.TimedOut:
		out.Outcome = "timeout"
	case sr.Subtype == "" && in.Run != nil && in.Run.Exit != 0:
		out.Outcome = "infra"
		out.OutcomeReason = fmt.Sprintf("claude exited %d without a result event", in.Run.Exit)
	case IsHarnessFailure(sr.Subtype, sr.IsError):
		// The harness exhausted its own retries: the run tells us nothing
		// about the model, so it is excluded rather than counted a fail.
		out.Outcome = "infra"
		out.OutcomeReason = "harness failure, subtype " + sr.Subtype + ": " + firstChars(sr.FinalMessage, 200)
	case sr.IsError:
		out.OutcomeReason = "result subtype " + sr.Subtype
	}
	// ABANDON terminal (gate-spec section 2, docs/12 section 2.2): the
	// staged contract's ABANDON: statement when the arm has one, else the
	// NOT-DONE last line; the same detector in every arm.
	if out.Outcome == "completed" {
		contract, _ := os.ReadFile(filepath.Join(in.Workspace, ".saga", "contract.md"))
		if ab := DetectAbandonWithHistory(sr.FinalMessage, contract, sr.AssistantTexts); ab != nil {
			out.Outcome, out.Abandon = "abandon", ab
			out.OutcomeReason = "ABANDON via " + ab.Source + ", reason class " + ab.ReasonClass
		}
	}
	sessionID := SessionID(in.Seed)
	if sr.SessionID != "" {
		sessionID = sr.SessionID
	}
	if matches, _ := filepath.Glob(filepath.Join(in.ConfigDir, "claude-config", "projects", "*", sessionID+".jsonl")); len(matches) > 0 {
		out.TranscriptPath = matches[0]
	}
	// Pins per run (trace-spec 4.1) from the stream plus the generated
	// settings file; the runner completes them from the disclosure.
	var settingsHash, hooksHash *string
	if raw, err := os.ReadFile(filepath.Join(in.ConfigDir, "settings.json")); err == nil {
		sh := canon.SHA256(raw)
		settingsHash = &sh
		var sm map[string]any
		if json.Unmarshal(raw, &sm) == nil {
			if hb, err := canon.JSON(sm["hooks"]); err == nil {
				hh := canon.SHA256(hb)
				hooksHash = &hh
			}
		}
	}
	// The events the claim verifier reconciles against, synthesised from
	// the harness's native stream by the same code in every arm.
	cfgHash := ""
	if settingsHash != nil {
		cfgHash = *settingsHash
	}
	streamTrace, err := StreamTrace(sr, in, cfgHash)
	if err != nil {
		return nil, fmt.Errorf("stream trace: %w", err)
	}
	out.StreamTraceJSONL = streamTrace
	// The trace the hooks wrote in the workspace, concatenated; archived
	// beside it and never read by the metric.
	st := store.Open(in.Workspace)
	dir := trace.SessionDir(st, sessionID)
	if segs, err := trace.Segments(dir); err == nil {
		for _, s := range segs {
			if b, err := os.ReadFile(s); err == nil {
				out.HookTraceJSONL = append(out.HookTraceJSONL, b...)
			}
		}
	}
	// The hook's own timing sidecar, beside the chain and outside it
	// (trace-spec 2.9). Absent in an arm without hooks.
	if b, err := os.ReadFile(filepath.Join(dir, trace.LatencyFile)); err == nil {
		out.HookLatencyJSONL = b
	}
	m := out.Disclosure.Block("model")
	if sr.Model != "" {
		Set(m, "id", sr.Model, "")
	}
	if len(sr.Tools) > 0 {
		tl := out.Disclosure.Block("tools")
		tl["names"] = sr.Tools
		tc, _ := canon.JSON(sr.Tools)
		Set(tl, "sha256", canon.SHA256(tc), "")
	}
	if sr.PermMode != "" {
		Set(out.Disclosure.Block("permissions"), "mode", sr.PermMode, "")
	}
	if sr.ClaudeCodeVersion != "" {
		Set(out.Disclosure.Block("harness"), "version", sr.ClaudeCodeVersion, "")
	}
	out.Pins = PinsFromStream(sr, c.Model, settingsHash, hooksHash, time.Now())
	// The bare arm is recognised by its own shim: the same signal Env
	// uses. Its per-surface block account carries the reach count.
	// The safety hook runs in every arm, so its log is read in every arm.
	// It is also the only hook a bare arm has, which makes its own wall
	// time that arm's whole hook overhead (docs/12 commitment 7).
	guardLogPath := filepath.Join(in.ConfigDir, guardLog)
	out.GuardDenies = guard.CountDenies(guardLogPath)
	out.SafetyLatencyMS = guard.Latencies(guardLogPath)
	if exists(filepath.Join(in.ConfigDir, shimDir, "saga")) {
		if raw, err := os.ReadFile(filepath.Join(in.ConfigDir, blockedLog)); err == nil {
			out.BlockedReachAttempts = len(strings.Split(strings.TrimRight(string(raw), "\n"), "\n"))
			if len(strings.TrimSpace(string(raw))) == 0 {
				out.BlockedReachAttempts = 0
			}
		}
		out.Disclosure["blocks_detail"] = BlocksDetail(&out.BlockedReachAttempts, &out.GuardDenies)
	}
	out.Disclosure["native_log"] = filepath.Base(in.NativeLogPath)
	if out.TranscriptPath != "" {
		out.Disclosure["transcript_path"] = out.TranscriptPath
	} else {
		out.Disclosure["transcript_path"] = nil
	}
	return out, nil
}

// IsHarnessFailure reports whether a result subtype names a failure of
// the harness or the provider rather than an outcome of the run. The
// turn and budget caps are outcomes and are excluded here; every other
// error_ subtype is the harness giving up, most often after its own
// retries on a 429 or 5xx (bench-spec 3.3, docs/12 row 14).
func IsHarnessFailure(subtype string, isError bool) bool {
	switch subtype {
	case "error_max_turns", "error_max_budget_usd", "":
		return false
	}
	return isError && strings.HasPrefix(subtype, "error_")
}

// firstChars caps a diagnostic on a rune boundary.
func firstChars(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	b := s[:n]
	for len(b) > 0 && !utf8.ValidString(b) {
		b = b[:len(b)-1]
	}
	return b + "..."
}

// ApproveCorpusInput is one task's pre-approval (ADR 0010 decision 2).
type ApproveCorpusInput struct {
	Task *task.Task
	// Workspace is a scratch directory the caller owns and deletes; it
	// is staged exactly as a gate-arm run stages one, so the approval
	// identity matches what a run will present.
	Workspace string
	// ConfigDir is a scratch config dir for the same reason.
	ConfigDir string
	// CorpusStore is the store the records are written to.
	CorpusStore string
	// Check only reports what is missing and approves nothing.
	Check bool
}

// ApproveCorpusResult is what one task's pre-approval did.
type ApproveCorpusResult struct {
	// Gates is the number of gates the task's contract declares.
	Gates int
	// Missing lists the gates with no record; empty means covered.
	Missing []string
	// Approved is the number of records written (0 in Check mode).
	Approved int
}

// ApproveCorpus stages one task the way a gate-arm run does and, unless
// Check is set, runs `saga gate check --approve` against the corpus
// store. It is called from a human act: the caller refuses under an
// agent shell before reaching here, exactly as `gate check --approve`
// does for itself.
func (c *ClaudeCode) ApproveCorpus(ctx context.Context, in *ApproveCorpusInput) (*ApproveCorpusResult, error) {
	if err := task.Stage(ctx, in.Task, in.Workspace); err != nil {
		return nil, err
	}
	// The same staging a run gets, so the identity is the same one a run
	// will present: the contract and config in the base commit, the
	// stable binary directory on PATH, the corpus store named.
	c.CorpusStore = in.CorpusStore
	prep := &PrepareInput{
		Task: in.Task, Workspace: in.Workspace, ConfigDir: in.ConfigDir,
		Components: []string{"gate"}, SagaBinary: c.SagaBinary,
	}
	if err := c.stageGateFiles(ctx, prep); err != nil {
		return nil, err
	}
	args := []string{"gate", "check", "--json"}
	if !in.Check {
		args = []string{"gate", "check", "--approve", "--json"}
	}
	sagaAbs, err := filepath.Abs(c.SagaBinary)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, sagaAbs, args...)
	cmd.Dir = in.Workspace
	cmd.Env = c.Env(in.ConfigDir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		return nil, fmt.Errorf("%s: %w", strings.Join(args, " "), err)
	}
	var rep struct {
		Gates []struct {
			ID       string `json:"id"`
			Approval string `json:"approval"`
		} `json:"gates"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &rep); jerr != nil {
		return nil, fmt.Errorf("%s: unreadable output: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()))
	}
	res := &ApproveCorpusResult{Gates: len(rep.Gates), Missing: []string{}}
	for _, g := range rep.Gates {
		if g.Approval == "missing" {
			res.Missing = append(res.Missing, g.ID)
		}
	}
	if !in.Check {
		res.Approved = res.Gates - len(res.Missing)
	}
	return res, nil
}

// approvalStoreBlock describes the corpus store a gate arm consumed:
// which frozen task set it belongs to, a hash of the directory path
// (never the path, which names the operator's home), and the approvals
// found there, by whom and when. A run writes none of these.
func (c *ClaudeCode) approvalStoreBlock(taskSet string) map[string]any {
	out := map[string]any{"kind": "corpus", "taskset_sha256": taskSet, "created_by_run": false}
	if c.CorpusStore == "" {
		out["dir_sha256"] = nil
		out["dir_sha256_reason"] = "no corpus store for this arm"
		return out
	}
	out["dir_sha256"] = BytesSHA256([]byte(c.CorpusStore))
	entries, err := os.ReadDir(c.CorpusStore)
	if err != nil {
		out["approvals"] = []any{}
		out["approvals_reason"] = "store unreadable: " + err.Error()
		return out
	}
	approvals := []any{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(c.CorpusStore, e.Name()))
		if err != nil {
			continue
		}
		var a struct {
			Gate       string `json:"gate"`
			By         string `json:"by"`
			ApprovedAt string `json:"approved_at"`
		}
		if json.Unmarshal(raw, &a) != nil {
			continue
		}
		approvals = append(approvals, map[string]any{
			"gate": a.Gate, "approved_by": a.By, "approved_at": a.ApprovedAt,
		})
	}
	sort.Slice(approvals, func(i, j int) bool {
		return approvals[i].(map[string]any)["gate"].(string) < approvals[j].(map[string]any)["gate"].(string)
	})
	out["approvals"] = approvals
	return out
}

// SystemPathDirs are the system directories the composed PATH ends
// with. They are fixed rather than inherited so a caller's shell cannot
// move them.
var SystemPathDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}

// BenchPath composes the PATH a bench run and an approval both use. The
// approval identity hashes the whole of PATH (gate-spec 8), so
// inheriting it bound the corpus approvals to the shell that gave them:
// the owner approved 101 records and `approve-corpus --check` from
// another shell found none of them, for the same binary and the same
// task set.
//
// The list is, in order: the control arm's shim directory when this
// config dir has one (the bare arm never enters an identity), the
// stable bench bin directory for this binary (ADR 0010 decision 1), the
// directory `node` resolves to, the directory `python3` resolves to,
// and the four system directories. Duplicates are dropped, keeping the
// first. `claude` is not looked up here: the launcher passes its
// absolute path, so the agent's PATH never has to name it.
//
// A toolchain that moves changes the identity, the run reads `not
// pre-approved` and the message names the path. That is the correct
// failure: a task graded with a different `node` is a different cell.
func (c *ClaudeCode) BenchPath(configDir string) []string {
	var dirs []string
	add := func(d string) {
		if d == "" {
			return
		}
		for _, seen := range dirs {
			if seen == d {
				return
			}
		}
		dirs = append(dirs, d)
	}
	if configDir != "" && exists(filepath.Join(configDir, shimDir, "saga")) {
		add(filepath.Join(configDir, shimDir))
	}
	if c.SagaBinary != "" {
		if d, err := BenchBinDir(c.SagaBinary); err == nil {
			add(d)
		}
	}
	for _, tool := range ToolchainTools {
		if p, err := exec.LookPath(tool); err == nil {
			if abs, err := filepath.Abs(p); err == nil {
				add(filepath.Dir(abs))
			}
		}
	}
	for _, d := range SystemPathDirs {
		add(d)
	}
	return dirs
}

// ToolchainTools are the interpreters the corpus needs on PATH; their
// directories are part of the composed PATH and so of the approval
// identity.
var ToolchainTools = []string{"node", "python3"}
