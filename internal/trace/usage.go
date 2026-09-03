package trace

import (
	"encoding/json"
	"math"
)

// Usage is the contracts section 7.2 object: token counts by direction
// and cache state, identical in trace events, ledger rows and bench rows.
type Usage struct {
	InputFresh      int            `json:"input_fresh"`
	CacheRead       int            `json:"cache_read"`
	CacheWrite5m    int            `json:"cache_write_5m"`
	CacheWrite1h    int            `json:"cache_write_1h"`
	Output          int            `json:"output"`
	Reasoning       *int           `json:"reasoning"`
	ReasoningReason *string        `json:"reasoning_reason,omitempty"`
	SplitReason     *string        `json:"split_reason,omitempty"`
	Source          string         `json:"source"`
	Raw             map[string]any `json:"raw,omitempty"`
}

// ContextTokens is the total input of the call.
func (u Usage) ContextTokens() int {
	return u.InputFresh + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
}

// Growth is input_fresh plus cache writes, the quantity context_delta is
// computed from (trace-spec section 3.2).
func (u Usage) Growth() int { return u.InputFresh + u.CacheWrite5m + u.CacheWrite1h }

// CacheHitRatio is cache_read over total input; nil when there is no
// input.
func (u Usage) CacheHitRatio() *float64 {
	ctx := u.ContextTokens()
	if ctx == 0 {
		return nil
	}
	r := math.Round(float64(u.CacheRead)/float64(ctx)*1e6) / 1e6
	return &r
}

func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case int:
		return t
	}
	return 0
}

func strp(s string) *string { return &s }

// UsageFromAnthropic maps an Anthropic usage object (as found in the
// Claude Code transcript) onto Usage per trace-spec section 3.1.
// pinnedTTL names the bucket an unsplit cache_creation_input_tokens total
// falls into ("5m" or "1h").
func UsageFromAnthropic(raw map[string]any, source, pinnedTTL string) Usage {
	u := Usage{
		InputFresh:      intOf(raw["input_tokens"]),
		CacheRead:       intOf(raw["cache_read_input_tokens"]),
		Output:          intOf(raw["output_tokens"]),
		Reasoning:       nil,
		ReasoningReason: strp("anthropic bills thinking inside output"),
		Source:          source,
		Raw:             raw,
	}
	if cc, ok := raw["cache_creation"].(map[string]any); ok {
		u.CacheWrite5m = intOf(cc["ephemeral_5m_input_tokens"])
		u.CacheWrite1h = intOf(cc["ephemeral_1h_input_tokens"])
	} else {
		total := intOf(raw["cache_creation_input_tokens"])
		if pinnedTTL == "5m" {
			u.CacheWrite5m = total
		} else {
			u.CacheWrite1h = total
		}
		if total > 0 {
			u.SplitReason = strp("cache_creation split absent; total assigned to pinned ttl " + pinnedTTL)
		}
	}
	return u
}

// UsageFromOpenAI maps an OpenAI usage object onto Usage.
func UsageFromOpenAI(raw map[string]any, source string) Usage {
	cached := 0
	if d, ok := raw["input_tokens_details"].(map[string]any); ok {
		cached = intOf(d["cached_tokens"])
	}
	u := Usage{
		InputFresh: intOf(raw["input_tokens"]) - cached,
		CacheRead:  cached,
		Output:     intOf(raw["output_tokens"]),
		Source:     source,
		Raw:        raw,
	}
	if d, ok := raw["output_tokens_details"].(map[string]any); ok {
		r := intOf(d["reasoning_tokens"])
		u.Reasoning = &r
	} else {
		u.ReasoningReason = strp("output_tokens_details absent")
	}
	return u
}

// UsageFromGoogle maps a Gemini usageMetadata object onto Usage.
func UsageFromGoogle(raw map[string]any, source string) Usage {
	cached := intOf(raw["cached_content_token_count"])
	u := Usage{
		InputFresh: intOf(raw["prompt_token_count"]) - cached,
		CacheRead:  cached,
		Output:     intOf(raw["candidates_token_count"]),
		Source:     source,
		Raw:        raw,
	}
	if v, ok := raw["thoughts_token_count"]; ok {
		r := intOf(v)
		u.Reasoning = &r
	} else {
		u.ReasoningReason = strp("thoughts_token_count absent")
	}
	return u
}

// EstimatedUsage is the fallback when no usage source exists: tokens are
// ceil(bytes / 4) and the row is never reconciled.
func EstimatedUsage(inputBytes, outputBytes int) Usage {
	return Usage{
		InputFresh:      (inputBytes + 3) / 4,
		Output:          (outputBytes + 3) / 4,
		ReasoningReason: strp("estimated"),
		Source:          "estimated",
	}
}
