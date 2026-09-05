package run

import (
	"encoding/json"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/trace"
	"github.com/ddh4r4m/saga/internal/trace/claims"
)

// AbstainList is the versioned abstention pattern list of bench-spec
// section 5.4 (abstain.txt), shipped by trace beside claims.txt so the
// bench and the Stop hook apply one definition byte for byte
// (trace-spec section 5.6). Both hashes are part of the manifest.
var AbstainList = claims.AbstainList()

// AbstainHash is sha256 of abstain.txt.
var AbstainHash = claims.AbstainListHash

// ClaimsList is claims.txt, verbatim.
var ClaimsList = claims.ClaimsList()

// ClaimsHash is sha256 of claims.txt.
var ClaimsHash = claims.ClaimsListHash

// Judgement is what the bench copies from the derived claim event into
// the run row (docs/12 section 2.1 rule 1): claimed_done is never
// recomputed by the bench.
type Judgement struct {
	ClaimedDone       *bool
	ClaimedDoneReason string
	Structural        *bool
	Verdict           *string
	Claims            int
	// Event is the `gate` event body with kind claim, source derived.
	Event map[string]any
}

// JudgeRun computes the derived claim verdict of a run from what the
// archive holds (final message, trace, workspace diff) plus the
// workspace when it still exists, identically in every arm.
func JudgeRun(rd claims.RunDirInput) (*Judgement, error) {
	in, err := claims.FromRun(rd)
	if err != nil {
		return nil, err
	}
	res := claims.Judge(in)
	j := &Judgement{ClaimedDone: res.Detection.ClaimedDone, ClaimedDoneReason: res.Detection.Reason, Structural: res.Detection.Structural, Claims: len(res.Claims), Event: res.Body(in)}
	if res.Verdict != "" {
		v := res.Verdict
		j.Verdict = &v
	}
	if j.ClaimedDone != nil {
		j.ClaimedDoneReason = "trace claim event (" + j.ClaimedDoneReason + ", claims.txt " + ClaimsHash[:23] + ")"
	} else {
		j.ClaimedDoneReason = "trace claim event: " + j.ClaimedDoneReason
	}
	return j, nil
}

// AppendDerived appends the derived claim event to a portable trace,
// continuing its hash chain (or starting one when the trace is empty),
// and returns the new bytes.
func AppendDerived(traceJSONL []byte, session string, turn int, body map[string]any) ([]byte, error) {
	events, err := claims.ReadTraceJSONL(traceJSONL)
	if err != nil {
		return traceJSONL, err
	}
	ev := trace.Event{Schema: trace.Schema, Seq: 1, Session: session, Turn: turn, Agent: "main", Type: trace.TypeGate, Source: "derived", Body: body, Prev: canon.Genesis}
	if len(events) > 0 {
		last := events[len(events)-1]
		ev.Seq, ev.Session, ev.Prev, ev.MonoNS = last.Seq+1, last.Session, last.Hash, last.MonoNS+1
		if turn == 0 {
			ev.Turn = last.Turn
		}
	}
	if ev.Session == "" {
		ev.Session = "bench"
	}
	ev.TS = trace.FormatTS(time.Now())
	h, err := ev.ComputeHash()
	if err != nil {
		return traceJSONL, err
	}
	ev.Hash = h
	if err := ev.Validate(); err != nil {
		return traceJSONL, err
	}
	line, err := canon.JSON(ev)
	if err != nil {
		return traceJSONL, err
	}
	out := append([]byte{}, traceJSONL...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(append(out, line...), '\n'), nil
}

// verdictOf reads the claim verdict back from a run row's trace, for
// callers that only have the archive.
func verdictOf(raw []byte) *string {
	events, err := claims.ReadTraceJSONL(raw)
	if err != nil {
		return nil
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == trace.TypeGate && events[i].Body["kind"] == "claim" && events[i].Source == "derived" {
			if v, ok := events[i].Body["verdict"].(string); ok {
				return &v
			}
			return nil
		}
	}
	return nil
}

var _ = json.Marshal
