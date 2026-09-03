package trace

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/store"
)

// PinsSchema is the schema id of the pin record.
const PinsSchema = "saga.trace.pins/1"

// Pins is saga.trace.pins/1 (trace-spec section 4.1). Every value the
// adapter cannot observe is null with a sibling reason.
type Pins struct {
	Schema       string         `json:"schema"`
	Harness      map[string]any `json:"harness"`
	Model        map[string]any `json:"model"`
	Effort       map[string]any `json:"effort"`
	SystemPrompt map[string]any `json:"system_prompt"`
	Tools        map[string]any `json:"tools"`
	Cache        map[string]any `json:"cache"`
	PriceTable   *string        `json:"price_table"`
	Saga         map[string]any `json:"saga"`
}

// NewPins builds the pins a hook can observe on Claude Code without
// executing anything: harness name, settings hashes, effort from the hook
// input, the price table, and the saga version and components.
func NewPins(harness, version string, settingsHash, hooksHash *string, effort string, priceTable string, components []string) Pins {
	p := Pins{
		Schema: PinsSchema,
		Harness: map[string]any{
			"name": harness, "version": nil, "version_reason": "not in hook input; saga doctor records it",
			"binary_sha256": nil, "binary_sha256_reason": "not computed by the hook path",
			"settings_hash": settingsHash, "hooks_hash": hooksHash,
		},
		Model:        map[string]any{"requested": nil, "requested_reason": "not in hook input", "served": nil, "served_reason": "read from transcript per model_call", "fingerprint": nil, "fingerprint_reason": "anthropic does not expose one"},
		Effort:       map[string]any{"value": nil, "value_reason": "absent from hook input", "source": nil, "source_hash": nil},
		SystemPrompt: map[string]any{"sha256": nil, "sha256_reason": "not exposed by claude-code hooks; enable proxy"},
		Tools:        map[string]any{"sha256": nil, "sha256_reason": "not exposed by claude-code hooks", "count": nil},
		Cache:        map[string]any{"ttl_pinned": "1h", "ttl_observed": nil, "observed_at": nil},
		PriceTable:   &priceTable,
		Saga:         map[string]any{"version": version, "components": components},
	}
	if effort != "" {
		p.Effort["value"] = effort
		p.Effort["value_reason"] = nil
		p.Effort["source"] = "hook_input"
	}
	return p
}

// PinsPath is .saga/trace/pins/current.json.
func PinsPath(s *store.Store) string { return s.Path("trace", "pins", "current.json") }

// ReadPins loads the last observed pins; ok is false when none exist.
func ReadPins(s *store.Store) (Pins, bool, error) {
	var p Pins
	raw, err := os.ReadFile(PinsPath(s))
	if errors.Is(err, fs.ErrNotExist) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, false, err
	}
	return p, true, nil
}

// WritePins stores the current pins.
func WritePins(s *store.Store, p Pins) error {
	if err := os.MkdirAll(s.Path("trace", "pins"), 0o700); err != nil {
		return err
	}
	raw, err := canon.JSON(p)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(PinsPath(s), append(raw, '\n'), 0o600)
}

// ChangedKeys lists the top-level pin keys whose canonical JSON differs
// between two records, sorted.
func ChangedKeys(prev, cur Pins) []string {
	pm, _ := toMap(prev)
	cm, _ := toMap(cur)
	out := []string{}
	for k, v := range cm {
		if k == "schema" {
			continue
		}
		a, _ := canon.JSON(pm[k])
		b, _ := canon.JSON(v)
		if string(a) != string(b) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(raw, &m)
}
