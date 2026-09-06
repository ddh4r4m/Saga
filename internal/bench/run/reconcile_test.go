package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/trace"
)

// synthArchive writes a minimal archive whose runs carry their ledger
// beside them, the layout the 2026-09-05 smoke was preserved in.
func synthArchive(t *testing.T, runs []synthRun) string {
	t.Helper()
	dir := t.TempDir()
	m := Manifest{
		Schema: ManifestSchema, Created: "2026-09-06T00:00:00Z", Tier: "smoke",
		TaskSet: TaskSet{SHA256: "sha256:" + strings.Repeat("a", 64), Tasks: []TaskHash{}},
		Arms:    []Arm{{ID: "A", Components: []string{}}, {ID: "B", Components: []string{"gate"}}},
		Models:  []Model{{ID: "claude-sonnet-5"}}, Harnesses: []HarnessRef{{Name: "replay"}},
		K: 1, RunSeed: strings.Repeat("ab", 32), BootstrapSeed: 1,
		AbstainListSHA256: AbstainHash, Isolation: "worktree", Images: map[string]string{},
		Host: Host{OS: "darwin", Arch: "arm64"}, Budget: Budget{EstimateUSD: 1, CapUSD: 2},
	}
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var rows strings.Builder
	for _, r := range runs {
		row := Row{
			Schema: RowSchema, Manifest: "sha256:" + strings.Repeat("b", 64), Task: r.task, Model: "claude-sonnet-5",
			Harness: "replay", Arm: r.arm, I: 1, Sequence: 1, Seed: strings.Repeat("cd", 32),
			Outcome: "completed", Oracle: OracleRow{Tests: map[string]string{}, Regressed: []string{}, Integrity: "ok"},
			Usage: UsageFrom(r.harness), Compliance: []any{}, ToolSequence: nil, Artifacts: map[string]string{},
		}
		line, _ := json.Marshal(row)
		rows.WriteString(string(line) + "\n")

		rd := filepath.Join(dir, r.task, "claude-sonnet-5", "replay", r.arm, "1")
		if err := os.MkdirAll(rd, 0o755); err != nil {
			t.Fatal(err)
		}
		h := map[string]any{"schema": "saga.bench.harness/1"}
		if r.harnessUSD > 0 {
			h["harness_cost_usd"] = r.harnessUSD
		}
		hraw, _ := json.Marshal(h)
		if err := os.WriteFile(filepath.Join(rd, "harness.json"), hraw, 0o644); err != nil {
			t.Fatal(err)
		}
		if !r.noLedger {
			ld := filepath.Join(rd, "trace")
			if err := os.MkdirAll(ld, 0o755); err != nil {
				t.Fatal(err)
			}
			lr := trace.LedgerRow{Schema: "saga.trace.ledger/1", Session: "s-" + r.task, Usage: r.ledger}
			lraw, _ := json.Marshal(lr)
			if err := os.WriteFile(filepath.Join(ld, "ledger.jsonl"), append(lraw, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "rows.jsonl"), []byte(rows.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

type synthRun struct {
	task       string
	arm        string
	ledger     trace.Usage
	harness    trace.Usage
	harnessUSD float64
	noLedger   bool
}

// usd prices a usage vector at the pinned claude-sonnet-5 row, so the
// test states its expectations in tokens rather than in money.
func usd(t *testing.T, u trace.Usage) float64 {
	t.Helper()
	v, reason := trace.DefaultPrices().Cost("claude-sonnet-5", u)
	if v.Total == nil {
		t.Fatalf("unpriced: %s", reason)
	}
	return *v.Total
}

// TestReconcileRowsFooterAndSchema (docs/12 row 13): an exact session, a
// 4 percent session, a 6 percent session and an arm without hooks. The
// arm without a ledger is listed and excluded rather than counted a
// discrepancy, because its absence is by design.
func TestReconcileRowsFooterAndSchema(t *testing.T) {
	exact := trace.Usage{InputFresh: 100, CacheRead: 200000, CacheWrite1h: 5000, Output: 1000}
	base := usd(t, exact)

	// Scale the harness figure so the ledger prices 4 and 6 percent above it.
	four := trace.Usage{InputFresh: 100, CacheRead: 200000, CacheWrite1h: 5000, Output: 1000}
	six := four

	archive := synthArchive(t, []synthRun{
		{task: "t-exact", arm: "B", ledger: exact, harness: exact, harnessUSD: base},
		{task: "t-four", arm: "B", ledger: four, harness: four, harnessUSD: base / 1.04},
		{task: "t-six", arm: "B", ledger: six, harness: six, harnessUSD: base / 1.06},
		{task: "t-bare", arm: "A", harness: exact, harnessUSD: base, noLedger: true},
	})

	rep, err := Reconcile(archive, "", trace.DefaultPrices())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 4 {
		t.Fatalf("%d rows, want 4", len(rep.Rows))
	}
	by := map[string]ReconcileRow{}
	for _, r := range rep.Rows {
		by[r.Task] = r
	}
	if n := by["t-bare"].Note; n != "no ledger" {
		t.Errorf("arm without hooks: note %q, want \"no ledger\"", n)
	}
	if rep.Sessions != 3 {
		t.Errorf("sessions %d, want 3 (the ledger-less run is excluded)", rep.Sessions)
	}
	if rep.MaxTokenDelta != 0 {
		t.Errorf("max token delta %d, want 0", rep.MaxTokenDelta)
	}
	for task, want := range map[string]float64{"t-exact": 0, "t-four": 0.04, "t-six": 0.06} {
		got := by[task].CostError
		if got == nil {
			t.Errorf("%s: no cost error", task)
			continue
		}
		if diff := *got - want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s: cost error %.6f, want %.2f", task, *got, want)
		}
	}
	// The footer takes the worst session, and 6 percent fails the bar.
	if rep.MaxCostError == nil || *rep.MaxCostError < 0.059 {
		t.Errorf("max cost error %v, want about 0.06", rep.MaxCostError)
	}
	if rep.WithinToleranc {
		t.Error("a 6 percent session must not be within tolerance")
	}

	// Text names the excluded run and the footer.
	txt := rep.Text()
	if !strings.Contains(txt, "no ledger") || !strings.Contains(txt, "within 5%: no") {
		t.Errorf("text:\n%s", txt)
	}

	// JSON validates against saga.bench.reconcile/1.
	v, err := schema.Normalize(rep)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateID(ReconcileSchema, v); err != nil {
		t.Errorf("report invalid: %v", err)
	}

	// Dropping the failing session puts the archive back inside the bar.
	ok := synthArchive(t, []synthRun{
		{task: "t-exact", arm: "B", ledger: exact, harness: exact, harnessUSD: base},
		{task: "t-four", arm: "B", ledger: four, harness: four, harnessUSD: base / 1.04},
	})
	rep2, err := Reconcile(ok, "", trace.DefaultPrices())
	if err != nil {
		t.Fatal(err)
	}
	if !rep2.WithinToleranc {
		t.Errorf("4 percent is inside the 5 percent bar: %v", rep2.MaxCostError)
	}
}

// TestReconcileTokenDelta: a ledger that disagrees with the row's usage
// is reported per component, which is how the 2026-09-05 ts-0001
// sessions were found to have short usage in the row rather than a
// drifting ledger.
func TestReconcileTokenDelta(t *testing.T) {
	ledger := trace.Usage{InputFresh: 74, CacheRead: 990463, CacheWrite1h: 30709, Output: 14633}
	row := trace.Usage{InputFresh: 72, CacheRead: 950421, CacheWrite1h: 28971, Output: 14305}
	archive := synthArchive(t, []synthRun{
		{task: "t", arm: "B", ledger: ledger, harness: row, harnessUSD: usd(t, ledger)},
	})
	rep, err := Reconcile(archive, "", trace.DefaultPrices())
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Rows[0]
	if r.Delta["cache_read"] != 40042 || r.Delta["output"] != 328 || r.Delta["input_fresh"] != 2 {
		t.Errorf("delta %v", r.Delta)
	}
	if rep.MaxTokenDelta != 40042 {
		t.Errorf("max token delta %d, want 40042", rep.MaxTokenDelta)
	}
	// Cost still reconciles, because the harness's own figure matches the
	// ledger: the row's usage is the short one.
	if r.CostError == nil || *r.CostError > 1e-9 {
		t.Errorf("cost error %v, want ~0", r.CostError)
	}
	if !strings.Contains(rep.Text(), fmt.Sprint(40042)) {
		t.Error("the text does not surface the token delta")
	}
}
