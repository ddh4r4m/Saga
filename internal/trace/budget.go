package trace

import (
	"fmt"

	"github.com/ddh4r4m/saga/internal/store"
)

// Crossing is a budget threshold crossed this call (trace-spec section
// 3.6).
type Crossing struct {
	// Level is soft, compact or hard.
	Level string
	// Action is warn, compact or stop.
	Action string
	Limit  float64
	Value  float64
	// Message is the fixed one-liner for the harness, at most 60 est.
	// tokens.
	Message string
}

// CheckBudget evaluates cumulative spend against [trace.budget]. It
// returns the highest crossing not yet reported (per crossed) and marks
// it; nil when nothing new was crossed or no budget is set.
func CheckBudget(cfg store.BudgetConfig, cumUSD float64, crossed map[string]bool) *Crossing {
	if cfg.SessionUSD == nil || *cfg.SessionUSD <= 0 {
		return nil
	}
	limit := *cfg.SessionUSD
	pct := cumUSD / limit * 100
	type level struct {
		name   string
		pct    int
		action string
	}
	levels := []level{{"hard", cfg.HardPct, "stop"}, {"compact", cfg.CompactPct, "compact"}, {"soft", cfg.SoftPct, "warn"}}
	for _, l := range levels {
		if l.pct <= 0 || pct < float64(l.pct) {
			continue
		}
		if crossed[l.name] {
			return nil
		}
		crossed[l.name] = true
		if l.name == "hard" && cfg.HardAction == "warn-only" {
			l.action = "warn"
		}
		c := &Crossing{Level: l.name, Action: l.action, Limit: limit, Value: cumUSD}
		switch l.name {
		case "hard":
			c.Message = HardStopMessage("session", cumUSD, limit)
		case "compact":
			c.Message = fmt.Sprintf("saga trace: budget session at %.0f%% (%.2f/%.2f); consider /compact", pct, cumUSD, limit)
		default:
			c.Message = fmt.Sprintf("saga trace: budget session at %.0f%% (%.2f/%.2f)", pct, cumUSD, limit)
		}
		return c
	}
	return nil
}

// HardStopMessage is the fixed deny and block text of trace-spec section
// 3.6.
func HardStopMessage(scope string, value, limit float64) string {
	return fmt.Sprintf("saga trace: budget %s exhausted (%.2f/%.2f); run saga trace budget --raise", scope, value, limit)
}

// Exhausted reports whether the hard threshold is currently exceeded with
// hard_action = stop, which denies the next tool call and blocks Stop
// until acknowledged.
func Exhausted(cfg store.BudgetConfig, cumUSD float64) bool {
	if cfg.SessionUSD == nil || *cfg.SessionUSD <= 0 || cfg.HardAction != "stop" || cfg.HardPct <= 0 {
		return false
	}
	return cumUSD / *cfg.SessionUSD * 100 >= float64(cfg.HardPct)
}
