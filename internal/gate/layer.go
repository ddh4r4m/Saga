package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// Token ceilings of gate-spec section 9.
const (
	CeilingPreTool = 150
	CeilingPost    = 200
	CeilingStop    = 400
	SessionShare   = 1000
)

// ProtectedPaths are ledger paths the agent's editor tools may not touch:
// the checker writes them (contracts section 8; gate-spec section 2.1).
var ProtectedPaths = []string{".saga/evidence/", ".saga/red/", ".saga/request.md"}

// forbidden are gate's rows in the shared agent-forbidden command list.
var forbidden = []string{"saga gate approve", "saga gate attest", "saga gate check --approve", "check --approve"}

// Layer is gate's step in the composed hook (section 6): translation of
// `status` and `guard-diff` into the harness envelope, never executing a
// CHECK: line.
type Layer struct {
	Store  *store.Store
	Stderr func(string)
}

// Name implements hookio.Layer.
func (l *Layer) Name() string { return "gate" }

func (l *Layer) note(format string, args ...any) {
	if l.Stderr != nil {
		l.Stderr(fmt.Sprintf(format, args...))
	}
}

// Run implements hookio.Layer.
func (l *Layer) Run(ctx context.Context, in *hookio.Input) (*hookio.Output, error) {
	out := hookio.Allow("gate")
	if l.Store == nil || !l.Store.Exists() {
		return out, nil
	}
	switch in.Event {
	case hookio.EventPreToolUse:
		return l.preTool(in, out)
	case hookio.EventPostToolUse:
		return l.postTool(in, out)
	case hookio.EventStop:
		return l.stop(in, out)
	}
	return out, nil
}

func (l *Layer) preTool(in *hookio.Input, out *hookio.Output) (*hookio.Output, error) {
	switch in.ToolName {
	case "Bash", "PowerShell":
		var ti struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		norm := strings.Join(strings.Fields(ti.Command), " ")
		for _, f := range forbidden {
			if strings.Contains(norm, f) {
				out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, "saga gate: "+f+" is a human act; ask the user to run it", int(cli.ExitRefusal)
				return out, nil
			}
		}
	case "Edit", "Write", "NotebookEdit", "MultiEdit":
		var ti struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		r, ok := rel(l.Store.Root, ti.FilePath)
		if ok {
			for _, p := range ProtectedPaths {
				if r == strings.TrimSuffix(p, "/") || strings.HasPrefix(r, p) {
					out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, "saga gate: "+r+" is written by the checker, not by the agent", int(cli.ExitRefusal)
					return out, nil
				}
			}
		}
		ld, err := LoadLite(l.Store.Root, l.Store)
		if err != nil || ld.Contract == nil {
			return out, nil
		}
		if f := PredictScope(GuardInput{Root: ld.Root, Contract: ld.Contract, Config: ld.Config}, ti.FilePath); f != nil {
			msg := fmt.Sprintf("saga gate: G-SCOPE %s %s", f.Path, f.Rule)
			if f.Detail != "" {
				msg += " (" + f.Detail + ")"
			}
			out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, l.cap(in, msg, CeilingPreTool), int(cli.ExitRefusal)
		}
	}
	return out, nil
}

func (l *Layer) postTool(in *hookio.Input, out *hookio.Output) (*hookio.Output, error) {
	switch in.ToolName {
	case "Edit", "Write", "NotebookEdit", "MultiEdit", "Bash":
	default:
		return out, nil
	}
	ld, err := LoadLite(l.Store.Root, l.Store)
	if err != nil || ld.Contract == nil {
		return out, nil
	}
	findings, err := GuardDiff(GuardInput{Root: ld.Root, Store: ld.Store, Base: ld.Base, Contract: ld.Contract, Config: ld.Config})
	if err != nil {
		l.note("saga gate: guard-diff: %v", err)
		return out, nil
	}
	var parts []string
	for _, f := range findings {
		if f.Blocks() {
			parts = append(parts, fmt.Sprintf("%s %s %s %s pre=%d post=%d", f.ID, f.Path, f.Hunk, f.Rule, f.Pre, f.Post))
		}
	}
	if len(parts) == 0 {
		return out, nil
	}
	msg := "saga gate: " + strings.Join(parts, "; ") + "; waive with `WAIVE: <guard> <path> <hunk> <reason>` in .saga/contract.md (G-LEDGER cannot be waived)"
	out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, msg, CeilingPost), int(cli.ExitRefusal)
	l.record(in, "guard_diff", map[string]any{"findings": len(parts), "decision": out.Decision})
	return out, nil
}

func (l *Layer) stop(in *hookio.Input, out *hookio.Output) (*hookio.Output, error) {
	ld, err := Load(l.Store.Root, l.Store)
	if ld != nil && ld.Missing {
		if ld.TrackedAtHead {
			out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, "saga gate: contract deleted; restore .saga/contract.md", CeilingStop), int(cli.ExitRefusal)
			l.record(in, "stop", map[string]any{"decision": "block", "reason": "contract deleted"})
		}
		return out, nil
	}
	if err != nil {
		out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, "saga gate: contract invalid: run saga gate lint", CeilingStop), int(cli.CodeOf(err))
		l.record(in, "stop", map[string]any{"decision": "block", "reason": "contract invalid", "exit": int(cli.CodeOf(err))})
		return out, nil
	}
	rep, err := Status(ld, StatusOptions{})
	if err != nil {
		out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, "saga gate: contract invalid: run saga gate lint", CeilingStop), int(cli.CodeOf(err))
		return out, nil
	}
	obs, oerr := l.Store.ReadObserved(in.SessionID)
	if oerr != nil {
		return out, cli.Wrap(cli.ExitEnvironment, "observed", oerr)
	}
	body := map[string]any{
		"for_turn": obs.Turn, "progress_hash": rep.ProgressHash, "tree_hash": rep.TreeHash, "mode": rep.Mode, "exit": rep.Exit,
		"ids": ids(rep), "states": states(rep),
		// Reserved for trace's claim verdict (trace-spec section 5.9, M1).
		"claims": nil, "claim_verdict": nil, "claim_reason": "trace claims ship in M1",
	}
	if rep.Exit == 0 {
		obs.GateBlocks, obs.GateProgress = 0, rep.ProgressHash
		body["decision"] = "allow"
		l.record(in, "stop", body)
		return out, l.Store.WriteObserved(obs)
	}
	if in.StopHookActive && obs.GateProgress == rep.ProgressHash {
		obs.GateBlocks++
	} else {
		obs.GateBlocks = 1
	}
	obs.GateProgress = rep.ProgressHash
	if obs.GateBlocks > ld.Config.MaxBlocks {
		body["decision"], body["blocks"] = "release", obs.GateBlocks
		out.AdditionalContext = append(out.AdditionalContext, "HANDOFF REQUIRED: "+fmt.Sprint(ld.Config.MaxBlocks)+" Stop blocks without progress")
		l.record(in, "stop", body)
		return out, l.Store.WriteObserved(obs)
	}
	left := SessionShare - obs.Tokens["gate"]
	msg := rep.StopReason(left)
	if canon.TokensEstString(msg) > left {
		msg = fmt.Sprintf("saga gate: %d unmet; run saga gate status", rep.Summary.Unmet+rep.Summary.Unproven+rep.Summary.Manual)
	}
	obs.Tokens["gate"] += canon.TokensEstString(msg)
	rep.Budget = Budget{BytesEmitted: len(msg), TokensEst: canon.TokensEstString(msg), Ceiling: CeilingStop}
	out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, msg, rep.Exit
	body["decision"], body["blocks"] = "block", obs.GateBlocks
	l.record(in, "stop", body)
	return out, l.Store.WriteObserved(obs)
}

// cap enforces the per-event ceiling and the session share; the decision
// is unchanged, the text collapses.
func (l *Layer) cap(in *hookio.Input, msg string, ceiling int) string {
	obs, err := l.Store.ReadObserved(in.SessionID)
	if err != nil {
		return msg
	}
	left := SessionShare - obs.Tokens["gate"]
	if canon.TokensEstString(msg) > ceiling || canon.TokensEstString(msg) > left {
		msg = "saga gate: blocked; run saga gate status"
	}
	obs.Tokens["gate"] += canon.TokensEstString(msg)
	_ = l.Store.WriteObserved(obs)
	return msg
}

func ids(r *Report) []string {
	var out []string
	for _, g := range r.Gates {
		out = append(out, g.ID)
	}
	return out
}

func states(r *Report) []string {
	var out []string
	for _, g := range r.Gates {
		out = append(out, g.State)
	}
	return out
}

// record writes one `gate` trace event (contracts section 6).
func (l *Layer) record(in *hookio.Input, kind string, body map[string]any) {
	if in.SessionID == "" {
		return
	}
	obs, err := l.Store.ReadObserved(in.SessionID)
	if err != nil || obs.MaskSalt == "" {
		return
	}
	dir := trace.SessionDir(l.Store, in.SessionID)
	w, err := trace.OpenWriter(dir, in.SessionID, trace.NewMasker(obs.MaskSalt))
	if err != nil {
		return
	}
	defer w.Close()
	body["kind"] = kind
	if err := w.Append(&trace.Event{Type: trace.TypeGate, Source: "hook:" + in.Event, Turn: obs.Turn, Body: body}); err != nil {
		l.note("saga gate: trace: %v", err)
	}
}
