package adapter

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/trace"
)

// InlineCap is the largest canonical payload a synthesised event carries
// inline; beyond it the arguments are recorded by hash only and a result
// is truncated (trace-spec section 2.5).
const InlineCap = 64 * 1024

// emitter builds a validated, hash-chained saga.trace/1 log offline: one
// event per emit call, timestamps derived from the sequence number so
// the same input always produces the same bytes. The first validation or
// hashing error is kept and returned by done; nothing is emitted after
// it that would depend on the dropped event's hash.
type emitter struct {
	session string
	source  string
	start   time.Time
	prev    string
	seq     int
	buf     []byte
	err     error
}

func newEmitter(session, source string, start time.Time) *emitter {
	return &emitter{session: session, source: source, start: start, prev: canon.Genesis}
}

// emit appends one event and returns its seq.
func (e *emitter) emit(turn int, typ string, body map[string]any) int {
	e.seq++
	ev := trace.Event{Schema: trace.Schema, Seq: e.seq, TS: trace.FormatTS(e.start.Add(time.Duration(e.seq) * time.Millisecond)), MonoNS: int64(e.seq) * 1e6,
		Session: e.session, Turn: turn, Agent: "main", Type: typ, Source: e.source, Body: body, Prev: e.prev}
	h, err := ev.ComputeHash()
	if err == nil {
		ev.Hash = h
		err = ev.Validate()
	}
	if err != nil {
		if e.err == nil {
			e.err = err
		}
		return e.seq
	}
	line, _ := canon.JSON(ev)
	e.buf = append(e.buf, line...)
	e.buf = append(e.buf, '\n')
	e.prev = h
	return e.seq
}

func (e *emitter) done() ([]byte, error) { return e.buf, e.err }

// reExitCode reads the exit status a harness error text names.
var reExitCode = regexp.MustCompile(`(?i)exit code (\d+)`)

// capText cuts s to about max bytes, keeping the head and, above all,
// the tail: a test runner prints its summary last (pytest's "N passed",
// node's "pass N", go test's ok and FAIL lines) and that summary is what
// ParseSummary reads, so a head-only cut would drop a chatty run's
// tests_pass to unverified. A quarter of the budget goes to the head,
// the rest to the tail, with one marker line naming the elided bytes
// between them; both cuts land on a rune boundary. The caller keeps
// result_hash and result_bytes over the full text.
func capText(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	head := s[:max/4]
	for len(head) > 0 && !utf8.ValidString(head) {
		head = head[:len(head)-1]
	}
	tail := s[len(s)-(max-max/4):]
	for len(tail) > 0 && !utf8.ValidString(tail) {
		tail = tail[1:]
	}
	return head + fmt.Sprintf("\n[saga: %d bytes elided]\n", len(s)-len(head)-len(tail)) + tail, true
}

// StreamTrace synthesises the run's saga.trace/1 chain from the
// harness's own `--output-format stream-json` log: the session start,
// the user turn, one tool_call and tool_result per tool use, and the
// final turn. It is the log the derived claim event is reconciled
// against in every arm, so a bare arm with no hooks sees exactly the
// tool observations a treatment arm sees (docs/12 section 13, amendment
// of 2026-09-06). settingsHash is the generated settings file's hash, or
// "" when it could not be read.
func StreamTrace(sr StreamResult, in *CollectInput, settingsHash string) ([]byte, error) {
	session := sr.SessionID
	if session == "" {
		session = SessionID(in.Seed)
	}
	e := newEmitter(session, "stream-json", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cwd := BytesSHA256([]byte(in.Workspace))
	var cfg any
	if settingsHash != "" {
		cfg = settingsHash
	}
	e.emit(0, trace.TypeSession, map[string]any{"phase": "start", "harness": "claude-code", "cwd_hash": cwd, "config_hash": cfg, "changed": []string{}})
	prompt := in.PromptOf()
	e.emit(1, trace.TypeTurn, map[string]any{"phase": "user", "prompt_hash": BytesSHA256([]byte(prompt)), "prompt_bytes": len(prompt)})
	for _, tu := range sr.ToolUses {
		argsCanon, err := canon.JSON(tu.Input)
		if err != nil {
			return e.buf, err
		}
		body := map[string]any{"tool": tu.Name, "args_hash": canon.SHA256(argsCanon), "component": "harness", "cwd_rel": ".", "index_version": nil, "tool_use_id": nilIfEmpty(tu.ID)}
		if tu.Input != nil && len(argsCanon) <= InlineCap {
			body["args_inline"] = tu.Input
		}
		call := e.emit(1, trace.TypeToolCall, body)
		// A tool use the stream never answers gets no tool_result; the
		// reconciler reports no_result for it.
		if tu.Result == nil {
			continue
		}
		exit, errStr := 0, any(nil)
		if tu.Result.IsError {
			exit, errStr = 1, "tool_error"
			if m := reExitCode.FindStringSubmatch(tu.Result.Text); m != nil {
				n := 0
				for _, c := range m[1] {
					n = n*10 + int(c-'0')
				}
				exit = n
			}
		}
		inline, cut := capText(tu.Result.Text, InlineCap)
		e.emit(1, trace.TypeToolResult, map[string]any{"for_seq": call, "exit": exit, "error": errStr,
			"result_hash": BytesSHA256([]byte(tu.Result.Text)), "result_bytes": len(tu.Result.Text),
			"result_inline": inline, "truncated": cut, "wall_ms": 0, "served": "live"})
	}
	// Phase assistant_end is the schema's name for the final turn event
	// (schema/trace/1/turn.json); it carries the final message.
	e.emit(1, trace.TypeTurn, map[string]any{"phase": "assistant_end",
		"final_message_hash": BytesSHA256([]byte(sr.FinalMessage)), "final_message_bytes": len(sr.FinalMessage),
		"claimed_done": nil, "claimed_done_reason": "derived by saga trace claims over final_message.txt"})
	return e.done()
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
