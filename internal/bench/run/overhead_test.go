package run

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestInjectedTokensIgnoreTheModelsContext (2026-09-06 smoke 3, finding
// 2): a model_call event carries its own context_tokens_est, the size of
// the model's whole context at that call. Summing it read 115,675
// injected tokens for arm B on a run whose injection was the 32-token
// contract sentence. Only a gate event's message_tokens_est and a
// session event's context_tokens_est are injection.
func TestInjectedTokensIgnoreTheModelsContext(t *testing.T) {
	trace := hookTraceTyped(t, []typedEvent{
		{"model_call", map[string]any{"context_tokens_est": 21546}},
		{"model_call", map[string]any{"context_tokens_est": 193921}},
		{"session", map[string]any{"phase": "start", "context_tokens_est": 0}},
		{"gate", map[string]any{"kind": "stop", "decision": "allow"}},
	})
	ov, why := ComputeOverhead(OverheadInput{
		Latency:        []hookio.LatencyLine{latency("Stop", 5, false)},
		HookTraceJSONL: trace, WallS: 40, Components: []string{"gate"},
	})
	if ov == nil {
		t.Fatalf("no overhead: %s", why)
	}
	// The contract sentence and nothing else, which is what that run
	// actually injected.
	want := canon.TokensEstString(adapter.ContractSentence)
	if ov.InjectedTokensEst != want {
		t.Errorf("injected_tokens_est %d, want %d (the contract sentence alone)", ov.InjectedTokensEst, want)
	}
	// What a real injection looks like: the gate blocked and said so.
	trace = hookTraceTyped(t, []typedEvent{
		{"model_call", map[string]any{"context_tokens_est": 193921}},
		{"gate", map[string]any{"kind": "stop", "message_tokens_est": 63}},
		{"gate", map[string]any{"kind": "claim", "message_tokens_est": 11}},
		{"session", map[string]any{"phase": "start", "context_tokens_est": 4}},
	})
	ov, _ = ComputeOverhead(OverheadInput{
		Latency:        []hookio.LatencyLine{latency("Stop", 5, false)},
		HookTraceJSONL: trace, WallS: 40, Components: []string{"gate"},
	})
	if ov.InjectedTokensEst != 63+11+4+want {
		t.Errorf("injected_tokens_est %d, want %d", ov.InjectedTokensEst, 63+11+4+want)
	}
	// A bare arm counts nothing at all, contract sentence included.
	ov, _ = ComputeOverhead(OverheadInput{
		Latency: []hookio.LatencyLine{latency("Stop", 5, false)}, HookTraceJSONL: trace, WallS: 40,
	})
	if ov.InjectedTokensEst != 63+11+4 {
		t.Errorf("bare arm injected_tokens_est %d, want %d (no contract sentence)", ov.InjectedTokensEst, 63+11+4)
	}
}

// TestInjectedTokensOnTheArchivedRun re-reads the fourth smoke's own
// hook trace, which is where the wrong figure came from, and checks the
// fix against the number the notes give.
func TestInjectedTokensOnTheArchivedRun(t *testing.T) {
	path := filepath.Join("..", "..", "..", "bench", "results", "smoke-2026-09-06-3",
		"B", "ts-0001-slug-collapse", "sonnet", "claude-code", "B", "1", "hook-trace.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("archive not present: %v", err)
	}
	ov, why := ComputeOverhead(OverheadInput{
		Latency:        []hookio.LatencyLine{latency("Stop", 241, false)},
		HookTraceJSONL: raw, WallS: 40.3, Components: []string{"gate"},
	})
	if ov == nil {
		t.Fatalf("no overhead: %s", why)
	}
	want := canon.TokensEstString(adapter.ContractSentence)
	if ov.InjectedTokensEst != want {
		t.Errorf("ts-0001 B run 1 injected_tokens_est %d, want %d: the Stop step allowed at once, the claim block did not fire and SessionStart injected nothing, so the contract sentence is the whole injection", ov.InjectedTokensEst, want)
	}
	t.Logf("ts-0001 B run 1: injected_tokens_est %d (archived figure was 215499)", ov.InjectedTokensEst)
}

type typedEvent struct {
	kind string
	body map[string]any
}

// hookTraceTyped writes a hook trace whose events carry their real type,
// which is what the injected-token sum now keys on.
func hookTraceTyped(t *testing.T, evs []typedEvent) []byte {
	t.Helper()
	var b strings.Builder
	for _, e := range evs {
		raw, err := json.Marshal(map[string]any{"schema": "saga.trace/1", "type": e.kind, "body": e.body})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return []byte(b.String())
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
		Scan: task.ScanResult{Detectors: []string{}, ScopeViolations: []string{}, FixedPathEdits: []string{}},
	}
}
