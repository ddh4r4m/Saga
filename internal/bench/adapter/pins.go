package adapter

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/trace"
)

// PinsFromStream builds the trace-spec section 4.1 pin record of one
// claude -p run from its stream-json log: harness version from the init
// event's claude_code_version, the model requested on the command line
// and the model(s) served (init event, then every assistant message),
// the tool list hash, and the cache TTL observed from the result usage's
// cache_creation split (harness-facts C29). Effort and the system prompt
// are not in the stream and stay null with a reason. The settings hash
// is the bench-generated settings file's.
func PinsFromStream(sr StreamResult, requested string, settingsHash, hooksHash *string, observedAt time.Time) *trace.Pins {
	pp := trace.NewPins("claude-code", "", settingsHash, hooksHash, "", "", nil)
	p := &pp
	p.Saga = map[string]any{"version": nil, "components": []string{}}
	h := p.Harness
	if sr.ClaudeCodeVersion != "" {
		h["version"], h["version_reason"] = sr.ClaudeCodeVersion, nil
	} else {
		h["version_reason"] = "no claude_code_version in the stream-json init event"
	}
	h["binary_sha256_reason"] = "filled from the disclosure block when Prepare resolved the binary"
	m := p.Model
	if requested != "" {
		m["requested"], m["requested_reason"] = requested, nil
	} else {
		m["requested"], m["requested_reason"] = nil, "harness default model; no --model on the command line"
	}
	served := servedModels(sr)
	switch len(served) {
	case 0:
		m["served"], m["served_reason"] = nil, "no model in the stream-json init event or assistant messages"
	case 1:
		m["served"], m["served_reason"] = served[0], nil
	default:
		m["served"], m["served_reason"] = strings.Join(served, ","), fmt.Sprintf("%d distinct models served in one run", len(served))
	}
	p.Effort = map[string]any{"value": nil, "value_reason": "not in stream-json; hook input carries effort.level only in a hooked arm", "source": nil, "source_hash": nil}
	p.SystemPrompt = map[string]any{"sha256": nil, "sha256_reason": "not exposed by claude -p stream-json"}
	if len(sr.Tools) > 0 {
		tc, _ := canon.JSON(sr.Tools)
		p.Tools = map[string]any{"sha256": canon.SHA256(tc), "sha256_reason": nil, "count": len(sr.Tools)}
	} else {
		p.Tools = map[string]any{"sha256": nil, "sha256_reason": "no tools list in the stream-json init event", "count": nil}
	}
	p.Cache = map[string]any{"ttl_pinned": "1h", "ttl_observed": nil, "observed_at": nil}
	if ttl := observedTTL(sr.Usage); ttl != "" {
		p.Cache["ttl_observed"] = ttl
		p.Cache["observed_at"] = observedAt.UTC().Format(time.RFC3339)
	}
	p.PriceTable = nil
	return p
}

// servedModels lists the distinct models the stream named, in order of
// first appearance: the init event's model, then each assistant
// message's.
func servedModels(sr StreamResult) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range append([]string{sr.Model}, sr.ServedModels...) {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// observedTTL reads the cache TTL the harness actually wrote at from
// the usage split: writes only at 1h, only at 5m, or both ("mixed").
// Without any cache write there is no observation.
func observedTTL(u trace.Usage) string {
	switch {
	case u.CacheWrite1h > 0 && u.CacheWrite5m == 0:
		return "1h"
	case u.CacheWrite5m > 0 && u.CacheWrite1h == 0:
		return "5m"
	case u.CacheWrite5m > 0 && u.CacheWrite1h > 0:
		return "mixed"
	}
	return ""
}

// CompletePins fills what the adapter could not observe at collect time
// from the disclosure block (harness version and binary hash from
// Prepare) and the bench (saga version and the arm's components).
func CompletePins(p *trace.Pins, d Disclosure, sagaVersion string, components []string) {
	if p == nil {
		return
	}
	if hb, ok := d["harness"].(map[string]any); ok {
		if p.Harness["version"] == nil {
			if v, ok := hb["version"].(string); ok && v != "" {
				p.Harness["version"], p.Harness["version_reason"] = v, nil
			}
		}
		if sum, ok := hb["binary_sha256"].(string); ok && sum != "" {
			p.Harness["binary_sha256"], p.Harness["binary_sha256_reason"] = sum, nil
		} else if r, ok := hb["binary_sha256_reason"].(string); ok {
			p.Harness["binary_sha256_reason"] = r
		}
	}
	if components == nil {
		components = []string{}
	}
	var v any
	if sagaVersion != "" {
		v = sagaVersion
	}
	p.Saga = map[string]any{"version": v, "components": components}
}

var versionDigits = regexp.MustCompile(`\d`)

// ModelComparable applies the trace-spec section 4.2 first row to a
// pin record: the run is comparable when the served model is the
// requested one. A requested alias with no version digits ("sonnet",
// "opus") matches a served id that contains it as a hyphenated token
// ("claude-sonnet-5"); an exact id must be served exactly; more than one
// served model is a fallback mid-run. No requested model (harness
// default) or no served model is comparable with nothing to compare
// against, which the report shows as a null requested id.
func ModelComparable(p *trace.Pins) (bool, string) {
	if p == nil {
		return true, ""
	}
	req, _ := p.Model["requested"].(string)
	served, _ := p.Model["served"].(string)
	if req == "" || served == "" {
		return true, ""
	}
	list := strings.Split(served, ",")
	if len(list) > 1 {
		return false, "served models changed during the run: " + served
	}
	s := list[0]
	if s == req {
		return true, ""
	}
	if !versionDigits.MatchString(req) {
		for _, tok := range strings.Split(s, "-") {
			if tok == req {
				return true, ""
			}
		}
	}
	return false, fmt.Sprintf("served model %s differs from requested %s (trace-spec 4.2, gemini-cli #28859 class)", s, req)
}
