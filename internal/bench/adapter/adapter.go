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
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/trace"
)

// Limits are the section 3.3 per-run limits handed to the harness.
type Limits struct {
	WallS    float64 `json:"wall_s"`
	MaxTurns int     `json:"max_turns"`
	USD      float64 `json:"usd"`
}

// ProtocolSentence is the fixed instruction every task prompt ends with
// in every arm (docs/12 section 2.1 rule 3, gate-spec section 10.3): it
// is what the structural claim marker reads.
const ProtocolSentence = "When you stop, the final line of your last message must be exactly DONE or NOT-DONE."

// ContractSentence is the one extra sentence of a gate arm (docs/12
// section 4): it names the file and the command, not a behaviour.
const ContractSentence = "A contract at .saga/contract.md declares the checks this task must satisfy; run saga gate check to verify them before you stop."

// StagedPrompt is the prompt the harness receives: prompt.md, the
// contract sentence when the arm has gate, then the protocol sentence
// last in every arm.
func StagedPrompt(prompt string, components []string) string {
	p := strings.TrimRight(prompt, "\n")
	if HasComponent(components, "gate") {
		p += "\n\n" + ContractSentence
	}
	return p + "\n\n" + ProtocolSentence + "\n"
}

// PrepareInput describes the run before the harness starts.
type PrepareInput struct {
	Task      *task.Task
	Workspace string
	// Prompt is the staged prompt text (StagedPrompt); empty means the
	// bare prompt.md.
	Prompt    string
	ConfigDir string
	Arm       string
	// Components are the Saga components of the arm (bench-spec 4.3);
	// empty is the bare control arm, blocked per 4.2.
	Components []string
	Blocks     []string
	Seed       string
	Limits     Limits
	// SagaBinary is the saga executable hooks are bound to.
	SagaBinary string
	// FrozenSetSHA256 names the frozen corpus this run belongs to: the
	// `set` line of TASKSET.sha256, from run.CorpusKey, and never the
	// hash over the tasks one invocation selected. A gate arm's approval
	// store is keyed by it (ADR 0010 decision 2). Empty leaves a gate arm
	// without a store, which the adapter refuses rather than falling back
	// to the operator's own ~/.saga/approved. The name says "frozen set"
	// because passing the per-run selection here is exactly the defect of
	// the 2026-09-13 dev run: a batch of 20 out of the frozen 40 keyed a
	// store of its own and found none of the owner's 202 records.
	FrozenSetSHA256 string
	// RunTaskSetSHA256 is the hash over the tasks this invocation
	// selected. It is disclosure only, never a store key: the archive
	// carries both so a divergence is visible instead of silent.
	RunTaskSetSHA256 string
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
	// Prompt is the staged prompt the harness received.
	Prompt string
}

// PromptOf returns the staged prompt of a prepare input, or the bare one.
func (in *PrepareInput) PromptOf() string {
	if in.Prompt != "" {
		return in.Prompt
	}
	return in.Task.Prompt()
}

// PromptOf returns the staged prompt of a collect input, or the bare one.
func (in *CollectInput) PromptOf() string {
	if in.Prompt != "" {
		return in.Prompt
	}
	return in.Task.Prompt()
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
	// Abandon is the recognised ABANDON terminal when Outcome is
	// "abandon" (DetectAbandon, identical in every arm).
	Abandon *Abandon
	// Pins is the trace-spec section 4.1 pin record the adapter observed
	// for this run (model requested and served, harness version, tools
	// hash, cache TTL observation); nil when the adapter has none.
	Pins *trace.Pins
	// HarnessCostUSD is the harness's own cost figure when it reports
	// one; the bench prices usage itself and records both.
	HarnessCostUSD *float64
	// BlockedReachAttempts counts control-arm reaches for the component
	// (bench-spec 4.2), from the PATH shim's log.
	BlockedReachAttempts int
	// GuardDenies counts the safety hook's denials (docs/12 row 6), from
	// its log. It is filled in every arm, because the hook runs in every
	// arm; a non-zero figure on either side is a fact about the run, not
	// about the component.
	GuardDenies int
	// StreamTraceJSONL is the saga.trace/1 chain synthesised from the
	// harness's own native log; it is what the derived claim event is
	// reconciled against, in every arm (docs/12 section 13, amendment of
	// 2026-09-06), and what the archive keeps as trace.jsonl.
	StreamTraceJSONL []byte
	// HookTraceJSONL is what Saga's hooks wrote in the workspace, nil in
	// an arm without hooks; it is archived as hook-trace.jsonl and never
	// feeds a metric.
	HookTraceJSONL []byte
	// HookLatencyJSONL is the composed hook's own timing sidecar for this
	// run, nil in an arm without hooks. It is archived as
	// hook-latency.jsonl and is what the overhead table reads (docs/12
	// commitment 7); it is not hash-chained and is never evidence.
	HookLatencyJSONL []byte
	// SafetyLatencyMS is one entry per safety-hook invocation that
	// recorded its own wall time. The safety hook runs in every arm, so
	// this is filled in every arm.
	SafetyLatencyMS []int
	TranscriptPath  string
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

// Merge copies src blocks over dst (field-wise inside objects); a
// non-null value drops the stale <field>_reason it replaces.
func (d Disclosure) Merge(src Disclosure) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			dm := d.Block(k)
			for kk, vv := range sm {
				dm[kk] = vv
				if vv != nil && !strings.HasSuffix(kk, "_reason") {
					if _, has := sm[kk+"_reason"]; !has {
						delete(dm, kk+"_reason")
					}
				}
			}
			continue
		}
		d[k] = v
	}
}
