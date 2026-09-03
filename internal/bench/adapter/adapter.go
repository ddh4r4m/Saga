// Package adapter defines the harness adapter contract of bench-spec
// section 6.1 (prepare, run, collect) and ships the built-in adapters:
// replay (applies a given patch, for testing the runner without a
// model), bare (the reference loop, not yet implemented) and
// claude-code (headless claude -p with a bench-generated settings file).
package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/trace"
)

// Limits are the section 3.3 per-run limits handed to the harness.
type Limits struct {
	WallS    float64 `json:"wall_s"`
	MaxTurns int     `json:"max_turns"`
	USD      float64 `json:"usd"`
}

// PrepareInput describes the run before the harness starts.
type PrepareInput struct {
	Task      *task.Task
	Workspace string
	ConfigDir string
	Arm       string
	Blocks    []string
	Seed      string
	Limits    Limits
	// SagaBinary is the saga executable hooks are bound to.
	SagaBinary string
}

// PrepareOutput carries the generated config hashes and the disclosure
// block as far as prepare can fill it.
type PrepareOutput struct {
	ConfigHash string
	PromptHash string
	ToolsHash  string
	Disclosure Disclosure
}

// RunInput starts the harness.
type RunInput struct {
	Task       *task.Task
	Workspace  string
	ConfigDir  string
	PromptPath string
	Seed       string
	// Index is the run number i within the cell, from 1.
	Index  int
	Limits Limits
	// Log receives the harness's stderr when non-nil.
	Log io.Writer
}

// RunOutput is what run returns.
type RunOutput struct {
	Exit          int
	NativeLogPath string
	TimedOut      bool
}

// CollectInput normalises the native log.
type CollectInput struct {
	Task          *task.Task
	Workspace     string
	ConfigDir     string
	NativeLogPath string
	Seed          string
	Run           *RunOutput
}

// ToolCall is one entry of the tool-call sequence in run.json.
type ToolCall struct {
	Tool     string `json:"tool"`
	ArgsHash string `json:"args_hash"`
	Error    bool   `json:"error"`
}

// CollectOutput is the normalised result.
type CollectOutput struct {
	Usage        trace.Usage
	Model        string
	Turns        int
	ToolCalls    int
	ToolSequence []ToolCall
	FinalMessage string
	// Outcome is the harness's view: completed, turn_cap, budget,
	// abandon, infra; the runner overrides with timeout and cost cap.
	Outcome       string
	OutcomeReason string
	// HarnessCostUSD is the harness's own cost figure when it reports
	// one; the bench prices usage itself and records both.
	HarnessCostUSD *float64
	TraceJSONL     []byte
	TranscriptPath string
	// Disclosure additions learnt at collect time (model id, version).
	Disclosure Disclosure
}

// Adapter is the section 6.1 contract.
type Adapter interface {
	Name() string
	Prepare(ctx context.Context, in *PrepareInput) (*PrepareOutput, error)
	Run(ctx context.Context, in *RunInput) (*RunOutput, error)
	Collect(ctx context.Context, in *CollectInput) (*CollectOutput, error)
}

// Disclosure is the saga.bench.harness/1 block (section 6.2) as nested
// maps; Set records null fields with a sibling <field>_reason.
type Disclosure map[string]any

// Block returns the named sub-object, creating it.
func (d Disclosure) Block(name string) map[string]any {
	if m, ok := d[name].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	d[name] = m
	return m
}

// Set stores v under key in block; a nil v is null with reason.
func Set(block map[string]any, key string, v any, reason string) {
	if v == nil {
		block[key] = nil
		if reason != "" {
			block[key+"_reason"] = reason
		}
		return
	}
	block[key] = v
	delete(block, key+"_reason")
}

// NewDisclosure returns a block with every required field present as
// null with a reason, for adapters to fill in what they know.
func NewDisclosure(harness string, limits Limits, blocks []string) Disclosure {
	d := Disclosure{"schema": "saga.bench.harness/1"}
	h := d.Block("harness")
	h["name"] = harness
	Set(h, "version", nil, "not determined by adapter")
	Set(h, "binary_sha256", nil, "not determined by adapter")
	m := d.Block("model")
	for _, k := range []string{"id", "snapshot", "fingerprint", "reasoning_effort", "temperature", "seed"} {
		Set(m, k, nil, "not exposed by harness")
	}
	m["seed_supported"] = false
	sp := d.Block("system_prompt")
	Set(sp, "sha256", nil, "harness does not expose its system prompt")
	sp["verbatim_archived"] = false
	tl := d.Block("tools")
	Set(tl, "sha256", nil, "tool definitions not exposed")
	tl["names"] = []string{}
	c := d.Block("context")
	Set(c, "window", nil, "not exposed by harness")
	Set(c, "compaction", nil, "not exposed by harness")
	Set(c, "compaction_threshold", nil, "not exposed by harness")
	Set(c, "rewind", nil, "not exposed by harness")
	p := d.Block("permissions")
	Set(p, "mode", nil, "not applicable")
	Set(p, "sandbox", nil, "not applicable")
	p["allow"] = []string{}
	p["deny"] = []string{}
	in := d.Block("instructions")
	Set(in, "CLAUDE.md_sha256", nil, "no CLAUDE.md in the workspace")
	Set(in, "AGENTS.md_sha256", nil, "no AGENTS.md in the workspace")
	d["hooks"] = []any{}
	d["mcp_servers"] = []string{}
	d["limits"] = map[string]any{"max_turns": limits.MaxTurns, "wall_s": limits.WallS, "usd": limits.USD}
	d["retries"] = map[string]any{"policy": "3x backoff 429/5xx", "count": 0}
	d["env_vars"] = map[string]string{}
	host := d.Block("host")
	host["os"] = runtime.GOOS
	host["arch"] = runtime.GOARCH
	Set(host, "image_digest", nil, "worktree isolation: no image")
	if blocks == nil {
		blocks = []string{}
	}
	d["blocks"] = blocks
	return d
}

// FileSHA256 returns "sha256:<hex>" of a file, or "" when unreadable.
func FileSHA256(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return BytesSHA256(raw)
}

// BytesSHA256 returns "sha256:<hex>" of b.
func BytesSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Merge copies src blocks over dst (field-wise inside objects).
func (d Disclosure) Merge(src Disclosure) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			dm := d.Block(k)
			for kk, vv := range sm {
				dm[kk] = vv
			}
			continue
		}
		d[k] = v
	}
}
