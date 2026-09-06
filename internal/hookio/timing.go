package hookio

import (
	"sync"
	"sync/atomic"
	"time"
)

// LatencySchema is the id of one line of the hook-latency sidecar.
const LatencySchema = "saga.trace.hooklatency/1"

// Timing is the wall-clock accounting of one hook invocation: the time
// since the process started and the time each layer step took. A hook
// process serves exactly one invocation, so one Timing describes it all.
//
// The measurement is deliberately outside the evidence path. Per-event
// stamps (hook_ms, hook_steps on the envelope) tie a slow invocation to
// the step that was slow, and the invocation's own total is written to a
// sidecar that is not hash-chained: buffering chain events until the
// total is known would lose them exactly when a hook overruns its
// deadline, which is the case the overhead table exists to measure
// (docs/12 commitment 7).
type Timing struct {
	start time.Time

	mu    sync.Mutex
	steps map[string]int
}

// NewTiming starts an accounting at start, which is the earliest moment
// the process can name as its own beginning.
func NewTiming(start time.Time) *Timing {
	if start.IsZero() {
		start = time.Now()
	}
	return &Timing{start: start, steps: map[string]int{}}
}

// MS is the milliseconds elapsed since the process started.
func (t *Timing) MS() int {
	if t == nil {
		return 0
	}
	return int(time.Since(t.start) / time.Millisecond)
}

// Step records that a named step took d. A step that runs twice in one
// invocation (trace records, then finalizes) accumulates.
func (t *Timing) Step(name string, d time.Duration) {
	if t == nil || name == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps[name] += int(d / time.Millisecond)
}

// Steps copies the steps that have run so far; nil when none has.
func (t *Timing) Steps() map[string]int {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.steps) == 0 {
		return nil
	}
	out := make(map[string]int, len(t.steps))
	for k, v := range t.steps {
		out[k] = v
	}
	return out
}

// invocation is the Timing of the invocation this process serves. The
// layers write trace events through three different call sites (the
// recorder, gate and the claim step), and every one of them stamps from
// here rather than carrying a timer of its own.
var invocation atomic.Pointer[Timing]

// SetInvocation names the current invocation's Timing; nil clears it,
// which is what a test does when it is done.
func SetInvocation(t *Timing) {
	if t == nil {
		invocation.Store(nil)
		return
	}
	invocation.Store(t)
}

// Invocation is the current invocation's Timing, nil outside a hook.
func Invocation() *Timing { return invocation.Load() }

// LatencyLine is one record of the hook-latency sidecar
// (sessions/<id>/hook-latency.jsonl). It is not hash-chained and is
// never evidence: it is the measurement of the hook's own cost, written
// at the last point before the reply goes to the harness.
type LatencyLine struct {
	Schema  string `json:"schema"`
	TS      string `json:"ts"`
	Session string `json:"session"`
	Event   string `json:"event"`
	// TotalMS is the process's own wall time to the byte before the
	// reply; Steps names the steps that ran and what each cost.
	TotalMS int            `json:"total_ms"`
	Steps   map[string]int `json:"steps"`
	// Decision is the merged decision the harness was told.
	Decision string `json:"decision"`
	// TimedOut is true when the chain was abandoned at the deadline, in
	// which case Steps holds only the steps that had finished. These are
	// the fail-open cases of docs/12 section 9.
	TimedOut bool `json:"timed_out"`
}
