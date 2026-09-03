package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Observed is .saga/observed/session-<id>.json: the one turn counter of
// contracts section 3, the per-layer injected-token counters of section
// 7.3, the block counter, and the small amount of cross-invocation state
// the hook needs (the previous event hash and seq for the chain, the
// pending tool calls, the transcript cursor).
type Observed struct {
	Schema  string `json:"schema"`
	Session string `json:"session"`
	Turn    int    `json:"turn"`
	// HarnessTurnID is the harness's own turn id when present.
	HarnessTurnID string `json:"harness_turn_id,omitempty"`
	// Tokens is injected estimated tokens per layer this session.
	Tokens map[string]int `json:"tokens"`
	Blocks int            `json:"blocks"`
	// Seq is the last event seq written.
	Seq int `json:"seq"`
	// Prev is the hash of the last event written.
	Prev string `json:"prev"`
	// Segment is the current events file number.
	Segment int `json:"segment"`
	// MonoStart is the wall time (unix ns) the session started; mono_ns is
	// measured from it because a hook is a fresh process every call.
	MonoStart int64 `json:"mono_start_ns"`
	// Pending maps tool_use_id to the tool_call seq awaiting its result.
	Pending map[string]int `json:"pending"`
	// TranscriptOffset is the byte offset already consumed from the
	// harness transcript.
	TranscriptOffset int64 `json:"transcript_offset"`
	// SeenMessages are the provider message ids already turned into
	// model_call events (dedupe rule of harness-facts C29).
	SeenMessages []string `json:"seen_messages"`
	// CumUSD is the running priced total; CumUnpriced counts rows without a
	// price.
	CumUSD      float64 `json:"cum_usd"`
	CumUnpriced int     `json:"cum_unpriced"`
	// LastContext is the previous model_call's context size for
	// context_delta.
	LastContext int `json:"last_context"`
	// ToolOutputBytesTurn is the tool output byte count this turn.
	ToolOutputBytesTurn int `json:"tool_output_bytes_turn"`
	// BudgetCrossed remembers which thresholds already produced an event.
	BudgetCrossed map[string]bool `json:"budget_crossed"`
	// OriginTokens is estimated tokens added to the context since the last
	// model call, by attribution key (trace-spec section 3.5).
	OriginTokens map[string]int `json:"origin_tokens"`
	// MaskSalt is the per-session salt of mask placeholders.
	MaskSalt string `json:"mask_salt"`
}

// ObservedSchema is the schema id of the observed session file.
const ObservedSchema = "saga.observed.session/1"

// ObservedPath returns the path of the observed file for session id.
func (s *Store) ObservedPath(session string) string {
	return s.Path("observed", "session-"+session+".json")
}

// ReadObserved loads the observed state for a session, returning a fresh
// record when none exists.
func (s *Store) ReadObserved(session string) (*Observed, error) {
	o := &Observed{Schema: ObservedSchema, Session: session, Tokens: map[string]int{}, Pending: map[string]int{}, BudgetCrossed: map[string]bool{}, OriginTokens: map[string]int{}}
	p := s.ObservedPath(session)
	if err := CheckShape(p); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return o, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, o); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	if o.Tokens == nil {
		o.Tokens = map[string]int{}
	}
	if o.Pending == nil {
		o.Pending = map[string]int{}
	}
	if o.BudgetCrossed == nil {
		o.BudgetCrossed = map[string]bool{}
	}
	if o.OriginTokens == nil {
		o.OriginTokens = map[string]int{}
	}
	return o, nil
}

// WriteObserved stores the observed state atomically.
func (s *Store) WriteObserved(o *Observed) error {
	if err := os.MkdirAll(s.Path("observed"), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.ObservedPath(o.Session), raw, 0o600)
}
