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

	claudecode "github.com/ddh4r4m/saga/adapters/claude-code"
	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// ClaudeCode runs `claude -p` headless (bench-spec section 6.1) with a
// bench-generated minimal settings file: Saga hooks bound to
// `saga hook claude-code <event>`, an explicit tool list that restores
// Grep and Glob (harness-facts C32), the task's permission mode and turn
// cap, and a fresh HOME and CLAUDE_CONFIG_DIR so the operator's own
// configuration is never read (section 3.1).
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

// Settings is the generated settings.json: Saga hooks, a permissive
// allow list for the bench tools, no co-author trailer.
func (c *ClaudeCode) Settings() map[string]any {
	saga := c.SagaBinary
	if saga == "" {
		saga = "saga"
	}
	var allow []string
	for _, t := range c.tools() {
		allow = append(allow, t)
	}
	return map[string]any{
		"hooks":               claudecode.Fragment(saga)["hooks"],
		"permissions":         map[string]any{"allow": allow, "deny": []string{}},
		"includeCoAuthoredBy": false,
		"env":                 map[string]string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"},
	}
}

// passthrough lists the operator environment variables the harness
// still needs (auth, network, locale); everything else is dropped.
var passthrough = []string{"PATH", "TMPDIR", "LANG", "SHELL", "USER", "LOGNAME", "TERM", "SSL_CERT_FILE", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy", "NODE_OPTIONS", "NODE_EXTRA_CA_CERTS"}

// Env is the environment for the harness process: a private HOME and
// CLAUDE_CONFIG_DIR under configDir plus the passthrough set and every
// ANTHROPIC_* and CLAUDE_CODE_* variable (credentials). Sorted so the
// disclosure block is stable.
func (c *ClaudeCode) Env(configDir string) []string {
	env := map[string]string{
		"HOME":              filepath.Join(configDir, "home"),
		"CLAUDE_CONFIG_DIR": filepath.Join(configDir, "claude-config"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"DISABLE_AUTOUPDATER":                      "1",
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CI":                                       "1",
	}
	for _, k := range passthrough {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_") || strings.HasPrefix(k, "LC_") {
			if _, set := env[k]; !set {
				env[k] = v
			}
		}
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

// Prepare implements Adapter: writes the settings file, initialises
// .saga/ in the workspace for the hooks, and fills the disclosure block.
func (c *ClaudeCode) Prepare(ctx context.Context, in *PrepareInput) (*PrepareOutput, error) {
	if in.SagaBinary != "" && c.SagaBinary == "" {
		c.SagaBinary = in.SagaBinary
	}
	for _, d := range []string{filepath.Join(in.ConfigDir, "home"), filepath.Join(in.ConfigDir, "claude-config")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	settings, err := json.MarshalIndent(c.Settings(), "", "  ")
	if err != nil {
		return nil, err
	}
	settingsPath := filepath.Join(in.ConfigDir, "settings.json")
	if err := os.WriteFile(settingsPath, append(settings, '\n'), 0o644); err != nil {
		return nil, err
	}
	if _, err := store.Init(in.Workspace); err != nil {
		return nil, fmt.Errorf("saga init in workspace: %w", err)
	}
	version, err := c.version(ctx)
	if err != nil {
		return nil, err
	}
	d := NewDisclosure("claude-code", in.Limits, in.Blocks)
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
	p["allow"] = c.Settings()["permissions"].(map[string]any)["allow"]
	p["deny"] = []string{}
	ctxb := d.Block("context")
	Set(ctxb, "compaction", "auto", "")
	var hooks []any
	saga := c.SagaBinary
	sagaSum := FileSHA256(saga)
	for ev, list := range c.Settings()["hooks"].(map[string]any) {
		for _, entry := range list.([]any) {
			for _, hk := range entry.(map[string]any)["hooks"].([]any) {
				cmd := hk.(map[string]any)["command"].(string)
				hm := map[string]any{"event": ev, "command": cmd}
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
		return hooks[i].(map[string]any)["event"].(string) < hooks[j].(map[string]any)["event"].(string)
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
	d["config_hash"] = BytesSHA256(settings)
	d["prompt_hash"] = BytesSHA256([]byte(in.Task.Prompt()))
	return &PrepareOutput{ConfigHash: BytesSHA256(settings), PromptHash: BytesSHA256([]byte(in.Task.Prompt())), ToolsHash: canon.SHA256(toolsCanon), Disclosure: d}, nil
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
}

// ParseStream normalises a `--output-format stream-json` log. Usage is
// the result event's total when present (the harness's own accounting),
// otherwise the sum over assistant messages deduplicated on message id.
func ParseStream(raw []byte) StreamResult {
	var r StreamResult
	toolErr := map[string]bool{}
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
			}
		case "assistant":
			msg, _ := ev["message"].(map[string]any)
			if msg == nil {
				continue
			}
			if m, _ := msg["model"].(string); m != "" && r.Model == "" {
				r.Model = m
			}
			id, _ := msg["id"].(string)
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
				if b["type"] != "tool_use" {
					continue
				}
				name, _ := b["name"].(string)
				argsCanon, _ := canon.JSON(b["input"])
				tc := ToolCall{Tool: name, ArgsHash: canon.SHA256(argsCanon)}
				if id, _ := b["id"].(string); id != "" {
					if _, dup := toolByID[id]; dup {
						continue // the same message re-emitted with more blocks
					}
					toolByID[id] = len(r.ToolCalls)
				}
				r.ToolCalls = append(r.ToolCalls, tc)
			}
		case "user":
			msg, _ := ev["message"].(map[string]any)
			content, _ := msg["content"].([]any)
			for _, blk := range content {
				b, _ := blk.(map[string]any)
				if b["type"] != "tool_result" {
					continue
				}
				if isErr, _ := b["is_error"].(bool); isErr {
					if id, _ := b["tool_use_id"].(string); id != "" {
						toolErr[id] = true
					}
				}
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
		if toolErr[id] {
			r.ToolCalls[i].Error = true
		}
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
	case sr.IsError:
		out.OutcomeReason = "result subtype " + sr.Subtype
	}
	sessionID := SessionID(in.Seed)
	if sr.SessionID != "" {
		sessionID = sr.SessionID
	}
	if matches, _ := filepath.Glob(filepath.Join(in.ConfigDir, "claude-config", "projects", "*", sessionID+".jsonl")); len(matches) > 0 {
		out.TranscriptPath = matches[0]
	}
	// The trace the hooks wrote in the workspace, concatenated.
	st := store.Open(in.Workspace)
	dir := trace.SessionDir(st, sessionID)
	if segs, err := trace.Segments(dir); err == nil {
		for _, s := range segs {
			if b, err := os.ReadFile(s); err == nil {
				out.TraceJSONL = append(out.TraceJSONL, b...)
			}
		}
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
	out.Disclosure["native_log"] = filepath.Base(in.NativeLogPath)
	if out.TranscriptPath != "" {
		out.Disclosure["transcript_path"] = out.TranscriptPath
	} else {
		out.Disclosure["transcript_path"] = nil
	}
	return out, nil
}
