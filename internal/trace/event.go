// Package trace is the local, append-only record of what an agent session
// did and what it cost (docs/specs/trace-spec.md). This M0 step
// implements the event envelope and hash chain, segment rotation, the
// built-in masker, the price table and per-call ledger, pins, the budget
// crossings, the Claude Code transcript normaliser and `saga doctor`.
package trace

import (
	"fmt"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/schema"
)

// Schema is the envelope schema id.
const Schema = "saga.trace/1"

// Event types of the catalogue (contracts section 6).
const (
	TypeSession       = "session"
	TypeTurn          = "turn"
	TypeModelCall     = "model_call"
	TypeToolCall      = "tool_call"
	TypeToolResult    = "tool_result"
	TypeEdit          = "edit"
	TypeGate          = "gate"
	TypeGuard         = "guard"
	TypeMemInject     = "mem_inject"
	TypeCompaction    = "compaction"
	TypeSubagent      = "subagent"
	TypeBudget        = "budget"
	TypeDrift         = "drift"
	TypeCheckpoint    = "checkpoint"
	TypeCanary        = "canary"
	TypeRouteDecision = "route_decision"
)

// Types lists every event type in catalogue order.
var Types = []string{TypeSession, TypeTurn, TypeModelCall, TypeToolCall, TypeToolResult, TypeEdit, TypeGate, TypeGuard, TypeMemInject, TypeCompaction, TypeSubagent, TypeBudget, TypeDrift, TypeCheckpoint, TypeCanary, TypeRouteDecision}

// AttributionKeys is the closed set of contracts section 7.1 (without the
// open-ended mcp:<server> keys).
var AttributionKeys = []string{"harness", "user", "tool_results", "index", "mem.state", "mem.targeted", "mem.preamble", "mem.query", "shape", "gate", "guard", "trace", "route"}

// ZeroAttribution returns every key at 0.0; an uninstalled layer's share
// is 0.0, never absent.
func ZeroAttribution() map[string]float64 {
	m := make(map[string]float64, len(AttributionKeys))
	for _, k := range AttributionKeys {
		m[k] = 0
	}
	return m
}

// Event is the saga.trace/1 envelope (trace-spec section 2.1).
type Event struct {
	Schema      string         `json:"schema"`
	Seq         int            `json:"seq"`
	TS          string         `json:"ts"`
	MonoNS      int64          `json:"mono_ns"`
	Session     string         `json:"session"`
	Turn        int            `json:"turn"`
	Agent       string         `json:"agent"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	Body        map[string]any `json:"body"`
	Prev        string         `json:"prev"`
	Hash        string         `json:"hash,omitempty"`
	MaskedCount *int           `json:"masked_count,omitempty"`
	MergedInto  *int           `json:"merged_into,omitempty"`
	// HookMS is the milliseconds from the hook process starting to this
	// event being appended, and HookSteps what each layer step had cost
	// by then (trace-spec section 2.9). Both are absent outside a hook
	// invocation and on every archive written before the fields existed,
	// where they read as "not recorded" rather than as zero. The
	// invocation's own total is in the hook-latency sidecar, not here:
	// a number only known after every layer has run cannot be stamped on
	// an event that is already durable.
	HookMS    *int           `json:"hook_ms,omitempty"`
	HookSteps map[string]int `json:"hook_steps,omitempty"`
}

// TSFormat is the wall-clock format: UTC with millisecond precision.
const TSFormat = "2006-01-02T15:04:05.000Z"

// FormatTS renders t in TSFormat.
func FormatTS(t time.Time) string { return t.UTC().Format(TSFormat) }

// ComputeHash returns sha256 of the canonical JSON of the event with
// "hash" removed.
func (e *Event) ComputeHash() (string, error) {
	c := *e
	c.Hash = ""
	b, err := canon.JSON(c)
	if err != nil {
		return "", err
	}
	return canon.SHA256(b), nil
}

// Validate checks the envelope and the per-type body against the
// embedded schemas.
func (e *Event) Validate() error {
	v, err := schema.Normalize(e)
	if err != nil {
		return err
	}
	if err := schema.ValidateID(Schema, v); err != nil {
		return fmt.Errorf("event seq %d: %w", e.Seq, err)
	}
	body, err := schema.Normalize(e.Body)
	if err != nil {
		return err
	}
	if err := schema.ValidateFile(schema.BodyFile(e.Type), body); err != nil {
		return fmt.Errorf("event seq %d body: %w", e.Seq, err)
	}
	return nil
}
