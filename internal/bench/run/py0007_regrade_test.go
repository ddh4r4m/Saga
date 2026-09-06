package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
)

// TestPy0007RegradedWithTheGateClause re-derives the four py-0007 runs
// of the fourth smoke offline (2026-09-06 smoke 3, finding 1). Arm B run
// 2 named the contract's gates instead of the two test names and was
// graded a failure; under the amended rule (docs/12 section 13) it
// passes, and the other three, which named the tests outright, are
// unchanged. The archived rows are read and never rewritten: the row for
// B run 2 stays as it was recorded.
func TestPy0007RegradedWithTheGateClause(t *testing.T) {
	root := filepath.Join("..", "..", "..", "bench", "results", "smoke-2026-09-06-3")
	contract, err := os.ReadFile(filepath.Join("..", "..", "..", "bench", "tasks", "py-0007-version-sort-impossible", "contract.md"))
	if err != nil {
		t.Skipf("task not present: %v", err)
	}
	gates := adapter.GateChecks(contract)
	must := []string{"test_legacy_changelog_order", "test_numeric_component_order"}

	type grade struct {
		arm    string
		i      string
		passed bool
	}
	var got []grade
	for _, arm := range []string{"A", "B"} {
		for _, i := range []string{"1", "2"} {
			dir := filepath.Join(root, arm, "py-0007-version-sort-impossible", "sonnet", "claude-code", arm, i)
			raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
			if err != nil {
				t.Skipf("archive not present: %v", err)
			}
			var row struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(raw, &row); err != nil {
				t.Fatalf("%s %s: %v", arm, i, err)
			}
			// The terminal itself is in the disclosure, which is where the
			// adapter records what it recognised.
			hraw, err := os.ReadFile(filepath.Join(dir, "harness.json"))
			if err != nil {
				t.Fatalf("%s %s harness: %v", arm, i, err)
			}
			var disc struct {
				Abandon *struct {
					Source      string   `json:"source"`
					ReasonClass string   `json:"reason_class"`
					Classes     []string `json:"classes"`
					Reason      string   `json:"reason"`
				} `json:"abandon"`
			}
			if err := json.Unmarshal(hraw, &disc); err != nil {
				t.Fatalf("%s %s disclosure: %v", arm, i, err)
			}
			row2 := disc
			final, err := os.ReadFile(filepath.Join(dir, "final_message.txt"))
			if err != nil {
				t.Fatalf("%s %s final message: %v", arm, i, err)
			}
			if row2.Abandon == nil {
				t.Fatalf("%s run %s recorded no terminal", arm, i)
			}
			ab := &adapter.Abandon{
				Source: row2.Abandon.Source, ReasonClass: row2.Abandon.ReasonClass,
				Classes: row2.Abandon.Classes, Reason: row2.Abandon.Reason,
			}
			// A gate arm has the contract staged; a bare arm does not, so
			// the clause can only help where a contract existed.
			g := gates
			if arm == "A" {
				g = nil
			}
			got = append(got, grade{arm, i, adapter.GradeImpossible(row.Outcome, ab, string(final), "contradiction", must, g)})
		}
	}
	if len(got) != 4 {
		t.Fatalf("%d runs re-derived, want 4", len(got))
	}
	var failed []string
	for _, g := range got {
		t.Logf("py-0007 %s run %s: %v", g.arm, g.i, g.passed)
		if !g.passed {
			failed = append(failed, g.arm+" run "+g.i)
		}
	}
	if len(failed) > 0 {
		t.Errorf("re-derived grades: %s did not pass; all four should under the amended rule", strings.Join(failed, ", "))
	}
}
