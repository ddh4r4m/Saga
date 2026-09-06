package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/trace"
)

// Replay applies a fixed patch instead of running a model. Patch is a
// control name resolved per task ("gold", "broken-1", "cheat-1"), "none"
// for the untouched baseline, "abandon" for an ABANDON terminal, or a
// path to a patch file; a comma-separated list cycles by run index
// ("gold,broken-1" applies gold on odd runs and broken-1 on even ones),
// which gives a cell a known pass count. It exists so the runner,
// grading, scans and reports are testable without a model.
type Replay struct {
	Patch string
}

func (r *Replay) choice(index int) string {
	parts := strings.Split(r.Patch, ",")
	if len(parts) == 0 || index <= 0 {
		return r.Patch
	}
	return strings.TrimSpace(parts[(index-1)%len(parts)])
}

// Name implements Adapter.
func (r *Replay) Name() string { return "replay" }

func (r *Replay) resolve(t *task.Task, index int) (path, label string) {
	p := r.choice(index)
	switch p {
	case "", "none", "baseline":
		return "", "none"
	case "abandon":
		return "", "abandon"
	}
	if strings.ContainsAny(p, "/\\.") {
		return p, filepath.Base(p)
	}
	return filepath.Join(t.Dir, "controls", p+".patch"), p
}

// Prepare implements Adapter.
func (r *Replay) Prepare(ctx context.Context, in *PrepareInput) (*PrepareOutput, error) {
	d := NewDisclosure("replay", in.Limits, in.Blocks)
	Set(d.Block("model"), "id", "replay", "")
	Set(d.Block("harness"), "version", "1", "")
	Set(d.Block("harness"), "binary_sha256", nil, "built into saga")
	d.Block("tools")["names"] = []string{"apply_patch"}
	Set(d.Block("tools"), "sha256", BytesSHA256([]byte("apply_patch")), "")
	Set(d.Block("system_prompt"), "sha256", nil, "replay has no model and no prompt")
	prompt := BytesSHA256([]byte(in.PromptOf()))
	cfg := BytesSHA256([]byte("replay:" + r.Patch))
	return &PrepareOutput{ConfigHash: cfg, PromptHash: prompt, ToolsHash: d.Block("tools")["sha256"].(string), Disclosure: d}, nil
}

// Run implements Adapter: applies the patch and writes a native log.
func (r *Replay) Run(ctx context.Context, in *RunInput) (*RunOutput, error) {
	path, label := r.resolve(in.Task, in.Index)
	log := map[string]any{"adapter": "replay", "patch": label, "applied": false}
	if path != "" {
		if err := task.Apply(ctx, in.Workspace, path); err != nil {
			log["error"] = err.Error()
		} else {
			log["applied"] = true
		}
	}
	native := filepath.Join(in.ConfigDir, "native.json")
	raw, _ := json.Marshal(log)
	if err := os.WriteFile(native, raw, 0o644); err != nil {
		return nil, err
	}
	return &RunOutput{Exit: 0, NativeLogPath: native}, nil
}

// Collect implements Adapter.
func (r *Replay) Collect(ctx context.Context, in *CollectInput) (*CollectOutput, error) {
	raw, err := os.ReadFile(in.NativeLogPath)
	if err != nil {
		return nil, err
	}
	var log struct {
		Patch   string `json:"patch"`
		Applied bool   `json:"applied"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(raw, &log); err != nil {
		return nil, err
	}
	out := &CollectOutput{
		Usage:     trace.Usage{Source: "replay", ReasoningReason: strp("replay has no model")},
		Model:     "replay",
		Turns:     1,
		ToolCalls: 1,
		Outcome:   "completed",
	}
	argsHash := BytesSHA256([]byte("patch:" + log.Patch))
	out.ToolSequence = []ToolCall{{Tool: "apply_patch", ArgsHash: argsHash, Error: log.Error != ""}}
	// The synthetic final message follows the gate-spec 10.3 convention
	// every bench prompt ends with (last line DONE or NOT-DONE), so the
	// trace claim detector reads it the way it reads a model's message.
	switch {
	case log.Patch == "abandon":
		// The synthetic abandon message names the task's required terms
		// as a contradiction and ends with the NOT-DONE marker; the
		// outcome comes from the shared detector, as in the live adapter.
		out.FinalMessage = "The task cannot be completed as stated: the requirements contradict each other."
		if in.Task.Terminal != nil && len(in.Task.Terminal.ReasonMustMention) > 0 {
			out.FinalMessage += " Contradiction between " + strings.Join(in.Task.Terminal.ReasonMustMention, " and ") + "."
		}
		out.FinalMessage += "\n\nNOT-DONE"
		out.ToolCalls = 0
		out.ToolSequence = nil
		contract, _ := os.ReadFile(filepath.Join(in.Workspace, ".saga", "contract.md"))
		if ab := DetectAbandon(out.FinalMessage, contract); ab != nil {
			out.Outcome, out.Abandon = "abandon", ab
			out.OutcomeReason = "ABANDON via " + ab.Source + ", reason class " + ab.ReasonClass
		}
	case log.Error != "":
		out.FinalMessage = fmt.Sprintf("Attempted to apply %s but it did not apply: %s.\n\nDONE", log.Patch, log.Error)
	case log.Patch == "none":
		// A zero-edit done: no tool call, nothing changed, still DONE.
		out.FinalMessage = "Reviewed the repository and made no changes.\n\nDONE"
		out.ToolCalls = 0
		out.ToolSequence = nil
	default:
		out.FinalMessage = fmt.Sprintf("Applied %s.\n\nDONE", log.Patch)
	}
	tr, err := replayTrace(in, out)
	if err != nil {
		return nil, fmt.Errorf("replay trace: %w", err)
	}
	out.StreamTraceJSONL = tr
	cfg := BytesSHA256([]byte("replay:" + r.Patch))
	pins := trace.NewPins("replay", "", &cfg, nil, "", "", nil)
	pins.Harness["version"], pins.Harness["version_reason"] = "1", nil
	pins.Harness["binary_sha256_reason"] = "built into saga"
	pins.Model["requested"], pins.Model["requested_reason"] = "replay", nil
	pins.Model["served"], pins.Model["served_reason"] = "replay", nil
	pins.Tools = map[string]any{"sha256": BytesSHA256([]byte("apply_patch")), "sha256_reason": nil, "count": 1}
	pins.PriceTable = nil
	out.Pins = &pins
	return out, nil
}

func strp(s string) *string { return &s }

// replayTrace emits a minimal saga.trace/1 chain for the replay run so
// the archive layout is exercised end to end.
func replayTrace(in *CollectInput, out *CollectOutput) ([]byte, error) {
	seed := in.Seed + "0000000000000000"
	e := newEmitter("bench-"+seed[:16], "cli", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cwd := BytesSHA256([]byte(in.Workspace))
	e.emit(0, trace.TypeSession, map[string]any{"phase": "start", "harness": "replay", "cwd_hash": cwd, "config_hash": BytesSHA256([]byte("replay")), "changed": []string{}})
	e.emit(1, trace.TypeTurn, map[string]any{"phase": "user", "prompt_hash": BytesSHA256([]byte(in.PromptOf())), "prompt_bytes": len(in.PromptOf())})
	for _, tc := range out.ToolSequence {
		call := e.emit(1, trace.TypeToolCall, map[string]any{"tool": tc.Tool, "args_hash": tc.ArgsHash, "component": "harness", "cwd_rel": ".", "index_version": nil})
		exit := 0
		var errStr any
		if tc.Error {
			exit = 1
			errStr = "patch did not apply"
		}
		e.emit(1, trace.TypeToolResult, map[string]any{"for_seq": call, "exit": exit, "error": errStr, "result_hash": BytesSHA256(nil), "result_bytes": 0, "truncated": false, "wall_ms": 0, "served": "live"})
	}
	e.emit(1, trace.TypeTurn, map[string]any{"phase": "assistant_end", "final_message_hash": BytesSHA256([]byte(out.FinalMessage)), "final_message_bytes": len(out.FinalMessage), "claimed_done": nil, "claimed_done_reason": "derived by saga trace claims over final_message.txt"})
	e.emit(1, trace.TypeSession, map[string]any{"phase": "end", "harness": "replay", "cwd_hash": cwd, "config_hash": BytesSHA256([]byte("replay")), "changed": []string{}})
	return e.done()
}
