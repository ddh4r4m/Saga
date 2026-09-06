package run

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/guard"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/schema"
)

// latency builds a sidecar line the way the composed hook writes one.
func latency(event string, totalMS int, timedOut bool) hookio.LatencyLine {
	return hookio.LatencyLine{
		Schema: hookio.LatencySchema, Session: "s", Event: event,
		TotalMS: totalMS, Steps: map[string]int{"trace": totalMS / 2}, TimedOut: timedOut,
	}
}

// hookTrace builds a hook-written trace whose events carry the injected
// token estimates of trace-spec 3.5.
func hookTrace(t *testing.T, bodies []map[string]any) []byte {
	t.Helper()
	var b strings.Builder
	for _, body := range bodies {
		raw, err := json.Marshal(map[string]any{"schema": "saga.trace/1", "type": "gate", "body": body})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func ms(n int) *int { return &n }

// TestComputeOverheadExact (docs/12 commitment 7): p50, p95, the wall
// share and the injected tokens are arithmetic over what the sidecar and
// the hook trace hold, not estimates. The safety hook's own invocations
// are counted under their own event name, because they are the only
// hook a bare arm has.
func TestComputeOverheadExact(t *testing.T) {
	in := OverheadInput{
		Latency: []hookio.LatencyLine{
			latency("PreToolUse", 10, false), latency("PreToolUse", 20, false),
			latency("PreToolUse", 30, false), latency("PreToolUse", 100, true),
			latency("Stop", 50, false),
		},
		Safety: []guard.SafetyLogLine{{LatencyMS: ms(4)}, {LatencyMS: ms(6)}, {LatencyMS: nil}},
		HookTraceJSONL: hookTrace(t, []map[string]any{
			{"kind": "stop", "message_tokens_est": 40},
			{"kind": "claim", "message_tokens_est": 12},
			{"phase": "start", "context_tokens_est": 0},
		}),
		WallS:      20,
		Components: []string{"gate"},
	}
	ov, why := ComputeOverhead(in)
	if ov == nil {
		t.Fatalf("no overhead: %s", why)
	}
	if ov.Invocations != 7 {
		t.Errorf("invocations %d, want 7 (5 composed, 2 safety with a latency)", ov.Invocations)
	}
	pre := ov.ByEvent["PreToolUse"]
	// Sorted 10, 20, 30, 100: nearest-rank p50 is the 2nd, p95 the 4th.
	if pre.N != 4 || pre.P50MS != 20 || pre.P95MS != 100 || pre.MaxMS != 100 {
		t.Errorf("PreToolUse %+v", pre)
	}
	if pre.TimedOut != 1 || ov.TimedOut != 1 {
		t.Errorf("timed out %d / %d, want 1 / 1", pre.TimedOut, ov.TimedOut)
	}
	if s := ov.ByEvent[SafetyEvent]; s.N != 2 || s.P50MS != 4 || s.MaxMS != 6 {
		t.Errorf("safety %+v (a line without latency_ms must be skipped, not counted zero)", s)
	}
	// 10+20+30+100+50 composed, 4+6 safety.
	if ov.WallMS != 220 {
		t.Errorf("wall_ms %d, want 220", ov.WallMS)
	}
	if ov.WallShare == nil || *ov.WallShare < 0.0109 || *ov.WallShare > 0.0111 {
		t.Errorf("wall_share %v, want 220/20000", ov.WallShare)
	}
	// 40 + 12 + 0 from the trace, plus the gate arm's contract sentence.
	want := 52 + canon.TokensEstString(adapter.ContractSentence)
	if ov.InjectedTokensEst != want {
		t.Errorf("injected_tokens_est %d, want %d", ov.InjectedTokensEst, want)
	}

	// A bare arm: the safety hook alone, and no contract sentence.
	bare, why := ComputeOverhead(OverheadInput{
		Safety: []guard.SafetyLogLine{{LatencyMS: ms(5)}}, WallS: 10,
	})
	if bare == nil {
		t.Fatalf("bare arm: %s", why)
	}
	if bare.Invocations != 1 || bare.InjectedTokensEst != 0 {
		t.Errorf("bare arm %+v: 0 injected tokens is a measurement, not an absence", bare)
	}
	if _, ok := bare.ByEvent[SafetyEvent]; !ok {
		t.Errorf("bare arm does not report the safety hook: %v", bare.ByEvent)
	}
}

// TestComputeOverheadNotRecorded: an archive from before the recording
// says so and is not back-filled with zeros, because a hook whose cost
// was never measured is not a free one.
func TestComputeOverheadNotRecorded(t *testing.T) {
	ov, why := ComputeOverhead(OverheadInput{WallS: 30, Components: []string{"gate"}})
	if ov != nil {
		t.Fatalf("overhead invented from nothing: %+v", ov)
	}
	if !strings.Contains(why, "not recorded") {
		t.Errorf("reason %q", why)
	}
	// A safety log whose lines predate latency_ms is equally not a zero.
	ov, why = ComputeOverhead(OverheadInput{Safety: []guard.SafetyLogLine{{Verdict: "allow"}}})
	if ov != nil || !strings.Contains(why, "not recorded") {
		t.Errorf("pre-latency safety lines: %+v %q", ov, why)
	}
}

// TestRowOverheadValidates: the run row carries the block or a null with
// a reason, and both shapes pass saga.bench.run/1.
func TestRowOverheadValidates(t *testing.T) {
	ov, _ := ComputeOverhead(OverheadInput{
		Latency: []hookio.LatencyLine{latency("Stop", 7, false)}, WallS: 1,
	})
	if ov == nil {
		t.Fatal("no overhead")
	}
	for _, r := range []Row{
		{Overhead: ov},
		{OverheadReason: strp("hook wall time not recorded in this archive")},
	} {
		row := minimalRow()
		row.Overhead, row.OverheadReason = r.Overhead, r.OverheadReason
		v, err := schema.Normalize(row)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateID(RowSchema, v); err != nil {
			t.Errorf("row invalid: %v", err)
		}
	}
}

// minimalRow is a row with every required field set, for schema checks.
func minimalRow() Row {
	return Row{
		Schema: RowSchema, Manifest: "sha256:" + strings.Repeat("a", 64), Task: "t", Model: "m",
		Harness: "replay", Arm: "A", I: 1, Sequence: 1, Seed: strings.Repeat("cd", 32),
		Outcome: "completed", Oracle: OracleRow{Tests: map[string]string{}, Regressed: []string{}, Integrity: "ok"},
		Compliance: []any{}, Artifacts: map[string]string{}, ToolSequence: []adapter.ToolCall{},
		Scan: task.ScanResult{Detectors: []string{}, ScopeViolations: []string{}},
	}
}
