package run

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/guard"
	"github.com/ddh4r4m/saga/internal/hookio"
)

// SafetyEvent is the event name the safety hook's own invocations are
// reported under. It is not a composed-hook event: in a bare arm it is
// the only hook there is (guard-spec 8.4.1).
const SafetyEvent = "safety"

// EventOverhead is one event type's measured cost in a run.
type EventOverhead struct {
	// N is the number of invocations of this event type.
	N int `json:"n"`
	// P50MS and P95MS are the invocations' own wall time. With one
	// invocation both are that invocation.
	P50MS int `json:"p50_ms"`
	P95MS int `json:"p95_ms"`
	// MaxMS is the slowest invocation, which is what a timeout argument
	// is made from.
	MaxMS int `json:"max_ms"`
	// TotalMS is the sum, which is what the wall share is made from.
	TotalMS int `json:"total_ms"`
	// TimedOut counts invocations the deadline abandoned; those fail open
	// (docs/12 section 9) and their steps are partial.
	TimedOut int `json:"timed_out"`
}

// Overhead is what the hooks cost this run (docs/12 commitment 7). It is
// null on an archive written before hook wall time was recorded; nothing
// is back-filled, because a number that was never measured cannot be
// reconstructed from what was.
type Overhead struct {
	// Invocations is every hook invocation of the run, composed and
	// safety together.
	Invocations int `json:"invocations"`
	// ByEvent is per event type, sorted by name in the rendered table.
	ByEvent map[string]EventOverhead `json:"by_event"`
	// WallMS is the sum over invocations: the run's hook wall time.
	WallMS int `json:"wall_ms"`
	// WallShare is WallMS over the run's own wall time; null when the run
	// has no wall time to divide by.
	WallShare *float64 `json:"wall_share"`
	// InjectedTokensEst is what the components put in front of the model:
	// the per-event estimates in the hook trace plus the fixed contract
	// sentence of the staged prompt. A bare arm's figure is 0, and it is
	// 0 because nothing injected, not because nothing was measured.
	InjectedTokensEst int `json:"injected_tokens_est"`
	// TimedOut is the run's total of abandoned invocations.
	TimedOut int `json:"timed_out"`
}

// OverheadInput is what one run's overhead is computed from. Every field
// may be empty: an arm without hooks has no latency lines and no hook
// trace, and its overhead is the safety hook alone.
type OverheadInput struct {
	// Latency is the composed hook's sidecar for this run's session.
	Latency []hookio.LatencyLine
	// Safety is the safety hook's own log lines.
	Safety []guard.SafetyLogLine
	// HookTraceJSONL is what the hooks wrote in the workspace; the
	// injected-token estimates are read from its event bodies.
	HookTraceJSONL []byte
	// WallS is the run's wall time, for the share.
	WallS float64
	// Components are the arm's Saga components; a gate arm's staged
	// prompt carries the contract sentence, which is injected text the
	// trace never sees.
	Components []string
}

// ComputeOverhead measures one run. It returns nil with a reason when
// nothing recorded wall time, which is every archive written before the
// recording existed.
func ComputeOverhead(in OverheadInput) (*Overhead, string) {
	byEvent := map[string][]int{}
	timedOut := map[string]int{}
	for _, l := range in.Latency {
		ev := l.Event
		if ev == "" {
			ev = "unknown"
		}
		byEvent[ev] = append(byEvent[ev], l.TotalMS)
		if l.TimedOut {
			timedOut[ev]++
		}
	}
	// The safety hook is in every arm and is the only hook a bare arm
	// has, so its own latency is that arm's whole hook overhead. A line
	// written before latency_ms existed is skipped rather than counted
	// as zero.
	for _, l := range in.Safety {
		if l.LatencyMS == nil {
			continue
		}
		byEvent[SafetyEvent] = append(byEvent[SafetyEvent], *l.LatencyMS)
	}
	if len(byEvent) == 0 {
		return nil, "hook wall time not recorded in this archive"
	}
	out := &Overhead{ByEvent: map[string]EventOverhead{}}
	for ev, ms := range byEvent {
		sort.Ints(ms)
		e := EventOverhead{N: len(ms), P50MS: pctl(ms, 50), P95MS: pctl(ms, 95), MaxMS: ms[len(ms)-1], TimedOut: timedOut[ev]}
		for _, v := range ms {
			e.TotalMS += v
		}
		out.ByEvent[ev] = e
		out.Invocations += e.N
		out.WallMS += e.TotalMS
		out.TimedOut += e.TimedOut
	}
	if in.WallS > 0 {
		share := float64(out.WallMS) / (in.WallS * 1000)
		out.WallShare = &share
	}
	out.InjectedTokensEst = injectedTokens(in.HookTraceJSONL)
	if adapter.HasComponent(in.Components, "gate") {
		// The contract sentence is staged into the prompt, so no hook
		// ever sees it and the trace cannot report it.
		out.InjectedTokensEst += canon.TokensEstString(adapter.ContractSentence)
	}
	return out, ""
}

// pctl is the nearest-rank percentile of a sorted slice.
func pctl(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	i := (len(sorted)*p + 99) / 100
	if i < 1 {
		i = 1
	}
	if i > len(sorted) {
		i = len(sorted)
	}
	return sorted[i-1]
}

// injectedTokens sums the per-event estimates a hook trace carries
// (trace-spec 3.5): what gate and the claim step put in front of the
// model, and what a session step put into additionalContext.
func injectedTokens(hookTrace []byte) int {
	total := 0
	for _, line := range strings.Split(string(hookTrace), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev struct {
			Body map[string]any `json:"body"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		for _, k := range []string{"message_tokens_est", "context_tokens_est"} {
			if v, ok := ev.Body[k].(float64); ok {
				total += int(v)
			}
		}
	}
	return total
}
