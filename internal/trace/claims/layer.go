package claims

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// SessionShare is trace's per-session injected budget (contracts
// section 7.3); the claim line is charged to it.
const SessionShare = 400

// Layer is the second trace step of the Stop chain (contracts section
// 1): after gate's `check --status`, it computes the claim verdict of
// the turn, writes the `gate` event with kind claim, and contributes the
// section 5.9 decision. On SubagentStop it records and never blocks.
type Layer struct {
	Store *store.Store
	// Gate is the gate layer of the same entry, for its Stop report and
	// decision (one write-tree per Stop, not two).
	Gate   *gate.Layer
	Stderr func(string)
}

// Name implements hookio.Layer.
func (l *Layer) Name() string { return "trace" }

func (l *Layer) note(format string, args ...any) {
	if l.Stderr != nil {
		l.Stderr(fmt.Sprintf(format, args...))
	}
}

// Run implements hookio.Layer.
func (l *Layer) Run(ctx context.Context, in *hookio.Input) (*hookio.Output, error) {
	out := hookio.Allow("trace")
	if l.Store == nil || !l.Store.Exists() || in.SessionID == "" {
		return out, nil
	}
	switch in.Event {
	case hookio.EventStop, hookio.EventSubagentStop:
	default:
		return out, nil
	}
	obs, err := l.Store.ReadObserved(in.SessionID)
	if err != nil {
		return out, cli.Wrap(cli.ExitEnvironment, "observed", err)
	}
	dir := trace.SessionDir(l.Store, in.SessionID)
	events, err := trace.ReadAll(dir)
	if err != nil {
		l.note("saga trace: claims: %v", err)
		events = nil
	}
	agent := "main"
	trigger := "stop"
	if in.Event == hookio.EventSubagentStop {
		trigger = "subagent_stop"
		if in.AgentID != "" {
			agent = in.AgentID
		}
	}
	ji := &Input{
		Final: in.LastAssistantMessage, FinalAvailable: in.LastAssistantMessage != "", Events: events, Turn: obs.Turn, Agent: agent,
		Root: l.Store.Root, BlobDir: filepath.Join(dir, "blobs"), Exists: ExistsIn(l.Store.Root),
		Source: "hook:" + in.Event, Trigger: trigger, Session: in.SessionID,
	}
	if ji.FinalAvailable {
		masked, _ := trace.NewMasker(obs.MaskSalt).MaskString(in.LastAssistantMessage)
		ji.FinalHash = canon.SHA256([]byte(masked))
	}
	// Gate's view from the same Stop: one write-tree per entry.
	var stop *gate.StopOutcome
	if l.Gate != nil {
		stop = l.Gate.LastStop()
	}
	rev := "HEAD"
	if stop != nil && stop.Loaded != nil && stop.Loaded.Contract != nil {
		rev = stop.Loaded.Base
		ji.Gate = GateViewOf(stop.Loaded, stop.Report)
		ji.DiffPaths, ji.DiffKnown = DiffPathsOf(l.Store.Root, stop.Loaded.Base)
	} else {
		ji.DiffPaths, ji.DiffKnown = DiffPathsOf(l.Store.Root, "")
	}
	cfg := LoadConfig(l.Store.Root, rev)
	ji.Mode, ji.Unverified = cfg.Mode, cfg.Unverified
	if in.Event == hookio.EventStop {
		for i := len(events) - 1; i >= 0; i-- {
			ev := events[i]
			if ev.Turn != obs.Turn {
				break
			}
			if ev.Type == trace.TypeGate && ev.Body["kind"] == "claim" && ev.Agent != "main" {
				ji.SubagentClaims = append([]int{ev.Seq}, ji.SubagentClaims...)
			}
		}
	}
	res := Judge(ji)
	body := res.Body(ji)

	if in.Event == hookio.EventSubagentStop {
		body["decision"] = "record"
		l.record(in, obs, agent, body)
		return out, nil
	}

	// Decision, block counter shared with gate, session share.
	gateBlocked := stop != nil && (stop.Decision == "block")
	gateReleased := stop != nil && stop.Decision == "release"
	if res.Decision == DecisionBlock {
		progress := claimProgress(stop, res)
		if gateReleased {
			body["decision"] = "release"
		} else if !gateBlocked {
			if in.StopHookActive && obs.GateProgress == progress {
				obs.GateBlocks++
			} else {
				obs.GateBlocks = 1
			}
			obs.GateProgress = progress
			if obs.GateBlocks > cfg.MaxBlocks {
				body["decision"], body["blocks"] = "release", obs.GateBlocks
				out.AdditionalContext = append(out.AdditionalContext, fmt.Sprintf("HANDOFF REQUIRED: %d Stop blocks without progress", cfg.MaxBlocks))
				l.record(in, obs, agent, body)
				l.drift(in, obs, "released", obs.Seq)
				return out, l.Store.WriteObserved(obs)
			}
			body["blocks"] = obs.GateBlocks
		} else {
			// Gate already counted this Stop; extend its progress hash
			// with the verdict so a reworded message is not progress.
			obs.GateProgress = progress
		}
		msg := res.Message
		left := SessionShare - obs.Tokens["trace"]
		if canon.TokensEstString(msg) > left {
			msg = fmt.Sprintf(Collapsed, res.Verdict, in.SessionID)
		}
		obs.Tokens["trace"] += canon.TokensEstString(msg)
		obs.OriginTokens["trace"] += canon.TokensEstString(msg)
		body["message_tokens_est"] = canon.TokensEstString(msg)
		out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, msg, hookExit(res)
	} else {
		if !gateBlocked && !gateReleased {
			// Progress: an allow or warn resets the shared counter only
			// when gate allowed too.
			obs.GateBlocks = 0
		}
		if res.Exit != 0 {
			out.Exit = hookExit(res)
		}
	}
	l.record(in, obs, agent, body)
	return out, l.Store.WriteObserved(obs)
}

// hookExit is the entry's exit for the verdict: the section 5.9 code,
// except that a missing final message is recorded as 6 in the event and
// treated as the unverified row (exit 1) in the chain, so it never
// outranks gate's own code.
func hookExit(res *Result) int {
	if res.FinalUnavailable {
		return int(cli.ExitFinding)
	}
	return res.Exit
}

// claimProgress extends gate's progress hash with the claim verdict
// (gate-spec section 6).
func claimProgress(stop *gate.StopOutcome, res *Result) string {
	base := ""
	if stop != nil && stop.Report != nil {
		base = stop.Report.ProgressHash
	}
	s := base + "\nclaim\x00" + res.Verdict
	for _, c := range res.Claims {
		s += "\n" + c.Kind + "\x00" + c.Verdict + "\x00" + c.Reason + "\x00" + c.Path + "\x00" + c.Command
	}
	return canon.SHA256([]byte(s))
}

// record writes the claim event (a `gate` event, kind claim) with source
// hook:<event>.
func (l *Layer) record(in *hookio.Input, obs *store.Observed, agent string, body map[string]any) {
	if obs.MaskSalt == "" {
		obs.MaskSalt = canon.ULID()
	}
	dir := trace.SessionDir(l.Store, in.SessionID)
	w, err := trace.OpenWriter(dir, in.SessionID, trace.NewMasker(obs.MaskSalt))
	if err != nil {
		l.note("saga trace: claims: %v", err)
		return
	}
	defer w.Close()
	if err := w.Append(&trace.Event{Type: trace.TypeGate, Source: "hook:" + in.Event, Turn: obs.Turn, Agent: agent, Body: body}); err != nil {
		l.note("saga trace: claims: %v", err)
		return
	}
	obs.Seq, obs.Prev = w.Seq(), w.Prev()
}

// drift writes the release event so the bench can count it.
func (l *Layer) drift(in *hookio.Input, obs *store.Observed, action string, evidence int) {
	dir := trace.SessionDir(l.Store, in.SessionID)
	w, err := trace.OpenWriter(dir, in.SessionID, trace.NewMasker(obs.MaskSalt))
	if err != nil {
		return
	}
	defer w.Close()
	body := map[string]any{"signal": "claim", "action": action, "evidence": []int{evidence}}
	if err := w.Append(&trace.Event{Type: trace.TypeDrift, Source: "hook:" + in.Event, Turn: obs.Turn, Body: body}); err == nil {
		obs.Seq, obs.Prev = w.Seq(), w.Prev()
	}
}
