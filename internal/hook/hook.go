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
	emit := func(b []byte) {
		if len(b) == 0 {
			b = []byte("{}")
		}
		e.Stdout.Write(append(b, '\n'))
	}
	if !hookio.Known(event) {
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
		emit(e.Render(in, r.out))
		return r.code
	case <-ctx.Done():
		out := hookio.Allow("hook")
		if hookio.Deciding(event) {
			out.Decision, out.Reason = denyFor(event), DeadlineReason
		}
		e.errf("saga hook: %s overran the %s deadline; %s", event, e.Deadline, out.Decision)
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
		out, err := l.Run(ctx, in)
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
		if err := f.Finalize(ctx, in, merged); err != nil {
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
