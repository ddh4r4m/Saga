// Package hook is the composed entry `saga hook <harness> <event>`
// (contracts section 1): it reads one JSON payload from stdin, runs the
// installed layers in the fixed order, merges their outputs, enforces its
// own deadline, and writes exactly one JSON object to stdout.
package hook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/hookio"
)

// MaxStdin bounds the payload read from the harness.
const MaxStdin = 32 << 20

// DeadlineReason is the fixed reason emitted on overrun (contracts
// section 1.1).
const DeadlineReason = "saga: deadline"

// MalformedReason is the fixed reason emitted when stdin is not a valid
// payload; the entry fails closed on deciding events.
const MalformedReason = "saga: malformed hook input"

// Entry is one configured composed hook.
type Entry struct {
	Harness string
	// Parse and Render are the harness adapter's translation functions.
	Parse  func(event string, raw []byte) (*hookio.Input, error)
	Render func(in *hookio.Input, out *hookio.Output) []byte
	// Layers run in order; a deny or block short-circuits.
	Layers   []hookio.Layer
	Deadline time.Duration
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	// Start is the earliest moment the process can name as its own
	// beginning; the zero value means "now", which undercounts by the
	// CLI's own start-up.
	Start time.Time
	// WriteLatency persists one sidecar line per invocation (docs/12
	// commitment 7). It runs at the last point before the reply goes to
	// the harness, and its failure is a note on stderr, never a change to
	// what the harness is told. nil records nothing.
	WriteLatency func(hookio.LatencyLine) error

	mu sync.Mutex
}

// errf writes one diagnostic line to stderr. It is serialised because the
// chain goroutine may still be running when the deadline path reports.
func (e *Entry) errf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Stderr != nil {
		fmt.Fprintf(e.Stderr, format+"\n", args...)
	}
}

// Run executes the entry for event and returns the process exit code.
// Whatever happens, exactly one JSON object is written to stdout.
func (e *Entry) Run(event string) cli.Code {
	if e.Stdin == nil {
		e.Stdin = os.Stdin
	}
	if e.Stdout == nil {
		e.Stdout = os.Stdout
	}
	if e.Deadline <= 0 {
		e.Deadline = 5 * time.Second
	}
	timing := hookio.NewTiming(e.Start)
	hookio.SetInvocation(timing)
	defer hookio.SetInvocation(nil)
	// latency writes the sidecar line and then the reply, in that order,
	// so the recorded total is the process's wall time to the byte
	// before the harness hears from us.
	latency := func(in *hookio.Input, out *hookio.Output, timedOut bool) {
		if e.WriteLatency == nil {
			return
		}
		line := hookio.LatencyLine{
			Schema: hookio.LatencySchema, TS: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			Event: event, TotalMS: timing.MS(), Steps: timing.Steps(), TimedOut: timedOut,
		}
		if line.Steps == nil {
			line.Steps = map[string]int{}
		}
		if in != nil {
			line.Session = in.SessionID
		}
		if out != nil {
			line.Decision = out.Decision
		}
		if err := e.WriteLatency(line); err != nil {
			e.errf("saga hook: latency: %v", err)
		}
	}
	emit := func(b []byte) {
		if len(b) == 0 {
			b = []byte("{}")
		}
		e.Stdout.Write(append(b, '\n'))
	}
	if !hookio.Known(event) {
		latency(nil, nil, false)
		emit(nil)
		e.errf("saga hook: unknown event %q", event)
		return cli.ExitUsage
	}
	raw, err := io.ReadAll(io.LimitReader(e.Stdin, MaxStdin))
	var in *hookio.Input
	if err == nil {
		in, err = e.Parse(event, raw)
	}
	if err != nil {
		e.errf("saga hook: %v", err)
		fallback := &hookio.Input{Harness: e.Harness, Event: event}
		out := hookio.Allow("hook")
		if hookio.Deciding(event) {
			out.Decision, out.Reason = denyFor(event), MalformedReason
		}
		latency(fallback, out, false)
		emit(e.Render(fallback, out))
		return cli.ExitUsage
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.Deadline)
	defer cancel()
	type result struct {
		out  *hookio.Output
		code cli.Code
	}
	done := make(chan result, 1)
	go func() {
		out, code := e.chain(ctx, in)
		done <- result{out, code}
	}()
	select {
	case r := <-done:
		latency(in, r.out, false)
		emit(e.Render(in, r.out))
		return r.code
	case <-ctx.Done():
		out := hookio.Allow("hook")
		if hookio.Deciding(event) {
			out.Decision, out.Reason = denyFor(event), DeadlineReason
		}
		e.errf("saga hook: %s overran the %s deadline; %s", event, e.Deadline, out.Decision)
		// The chain goroutine is abandoned here, so Steps holds only the
		// steps that had finished. That is the fail-open case of docs/12
		// section 9 and the table counts it as its own outcome.
		latency(in, out, true)
		emit(e.Render(in, out))
		if hookio.Deciding(event) {
			return cli.ExitRefusal
		}
		return cli.ExitOK
	}
}

func denyFor(event string) string {
	if event == hookio.EventPreToolUse {
		return hookio.DecisionDeny
	}
	return hookio.DecisionBlock
}

// chain runs the layers and merges under contracts section 1.1.
func (e *Entry) chain(ctx context.Context, in *hookio.Input) (*hookio.Output, cli.Code) {
	merged := hookio.Allow("hook")
	var codes []cli.Code
	var finalizers []hookio.Finalizer
	for _, l := range e.Layers {
		if f, ok := l.(hookio.Finalizer); ok {
			finalizers = append(finalizers, f)
		}
		stepStart := time.Now()
		out, err := l.Run(ctx, in)
		// A step the deadline cut short did not finish, and the reply has
		// already gone without it, so it is not this invocation's cost.
		// Recording it would also race the main goroutine's read of the
		// sidecar line.
		if ctx.Err() == nil {
			e.Timing().Step(l.Name(), time.Since(stepStart))
		}
		if err != nil {
			e.errf("saga %s: %v", l.Name(), err)
			codes = append(codes, cli.CodeOf(err))
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
		}
		if out == nil {
			continue
		}
		if out.Layer == "" {
			out.Layer = l.Name()
		}
		stop, conflict := merged.Merge(out)
		if out.Exit != 0 {
			codes = append(codes, cli.Code(out.Exit))
		}
		if conflict != "" {
			e.errf("saga hook: two layers rewrote %q", conflict)
			merged.Decision, merged.Reason = denyFor(in.Event), "saga: composition bug on "+conflict
			codes = append(codes, cli.ExitUsage)
			break
		}
		// On Stop every layer runs: block wins, gate's reason first and
		// trace's claim line second (contracts section 1).
		if stop && in.Event != hookio.EventStop {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	for _, f := range finalizers {
		stepStart := time.Now()
		name := "finalize"
		if l, ok := f.(hookio.Layer); ok {
			name = l.Name() + ":finalize"
		}
		err := f.Finalize(ctx, in, merged)
		if ctx.Err() == nil {
			e.Timing().Step(name, time.Since(stepStart))
		}
		if err != nil {
			e.errf("saga hook: finalize: %v", err)
			codes = append(codes, cli.CodeOf(err))
		}
	}
	// PreCompact never blocks and always exits 0 (contracts section 1).
	if in.Event == hookio.EventPreCompact {
		merged.Decision = hookio.DecisionAllow
		merged.Reason = ""
		return merged, cli.ExitOK
	}
	if merged.Decision == hookio.DecisionAllow && !hasNonZero(codes) {
		return merged, cli.ExitOK
	}
	code := cli.Precedence(codes...)
	if code == cli.ExitOK && merged.Decision != hookio.DecisionAllow {
		code = cli.ExitRefusal
	}
	return merged, code
}

func hasNonZero(codes []cli.Code) bool {
	for _, c := range codes {
		if c != cli.ExitOK {
			return true
		}
	}
	return false
}

// Timing is the invocation's accounting, set by Run. It is read through
// the process-wide handle so the layers that write trace events through
// their own writers stamp the same numbers the sidecar reports.
func (e *Entry) Timing() *hookio.Timing { return hookio.Invocation() }
