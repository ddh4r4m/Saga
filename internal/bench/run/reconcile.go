package run

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/trace"
)

// ReconcileSchema is the id of the reconciliation report.
const ReconcileSchema = "saga.bench.reconcile/1"

// Tolerance is the docs/12 row 13 bar: the ledger must price within 5
// percent of the harness's own total_cost_usd.
const Tolerance = 0.05

// ReconcileRow is one run's comparison of the trace ledger against the
// harness's own accounting (trace-spec section 3.4, docs/12 row 13).
type ReconcileRow struct {
	Task    string `json:"task"`
	Arm     string `json:"arm"`
	I       int    `json:"i"`
	Session string `json:"session"`
	// Ledger is the sum over the session's ledger rows; Harness is what
	// the run row recorded from the harness's own usage object.
	Ledger  trace.Usage `json:"ledger"`
	Harness trace.Usage `json:"harness"`
	// Delta is Ledger minus Harness, per component.
	Delta map[string]int `json:"delta"`
	// LedgerUSD prices the ledger usage from the pinned table;
	// HarnessUSD is the harness's own figure.
	LedgerUSD  *float64 `json:"ledger_usd"`
	HarnessUSD *float64 `json:"harness_usd"`
	// Ratio is LedgerUSD / HarnessUSD, and CostError its distance from 1.
	Ratio     *float64 `json:"ratio"`
	CostError *float64 `json:"cost_error"`
	// Note is "no ledger" for a run whose arm has no hooks, which is
	// excluded from the footer rather than counted as a failure.
	Note string `json:"note,omitempty"`
}

// ReconcileReport is saga.bench.reconcile/1.
type ReconcileReport struct {
	Schema         string         `json:"schema"`
	Archive        string         `json:"archive"`
	PriceTable     string         `json:"price_table"`
	Rows           []ReconcileRow `json:"rows"`
	Sessions       int            `json:"sessions"`
	MaxTokenDelta  int            `json:"max_token_delta"`
	MaxCostError   *float64       `json:"max_cost_error"`
	Tolerance      float64        `json:"tolerance"`
	WithinToleranc bool           `json:"within_tolerance"`
}

// sumLedger adds every row of a session's ledger.jsonl.
func sumLedger(dir string) (trace.Usage, int, error) {
	rows, err := trace.ReadLedger(dir)
	if err != nil {
		return trace.Usage{}, 0, err
	}
	var u trace.Usage
	for _, r := range rows {
		u.InputFresh += r.Usage.InputFresh
		u.CacheRead += r.Usage.CacheRead
		u.CacheWrite5m += r.Usage.CacheWrite5m
		u.CacheWrite1h += r.Usage.CacheWrite1h
		u.Output += r.Usage.Output
	}
	return u, len(rows), nil
}

// findLedger locates a run's ledger. Two layouts are supported: the
// kept workspace root the runner leaves behind
// (<root>/<task>/<i>/ws/.saga/trace/sessions/<s>/), and a ledger copied
// into the archive beside the run (<run-dir>/trace/), which is how the
// 2026-09-05 smoke was preserved. An archive that carries its own ledger
// reconciles with no workspace root at all.
func findLedger(keptRoot, task string, i int, session, archiveRunDir string) string {
	if archiveRunDir != "" {
		if p := filepath.Join(archiveRunDir, "trace"); dirHasLedger(p) {
			return p
		}
	}
	if keptRoot == "" {
		return ""
	}
	base := filepath.Join(keptRoot, task, fmt.Sprint(i), "ws", ".saga", "trace", "sessions")
	if session != "" {
		if p := filepath.Join(base, session); dirHasLedger(p) {
			return p
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if p := filepath.Join(base, e.Name()); dirHasLedger(p) {
			return p
		}
	}
	return ""
}

func dirHasLedger(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "ledger.jsonl"))
	return err == nil && fi.Size() > 0
}

// Reconcile compares every run's ledger with the harness's own
// accounting. keptRoot is the runner's `--keep` workspace root; a run
// with no ledger (an arm without hooks) is listed and excluded from the
// footer, because the absence is by design rather than a discrepancy.
func Reconcile(archive, keptRoot string, prices *trace.PriceTable) (*ReconcileReport, error) {
	_, _, rows, err := ReadArchive(archive)
	if err != nil {
		return nil, err
	}
	rep := &ReconcileReport{Schema: ReconcileSchema, Archive: archive, Tolerance: Tolerance, Rows: []ReconcileRow{}}
	if prices != nil {
		rep.PriceTable = prices.Hash
	}
	for _, r := range rows {
		out := ReconcileRow{Task: r.Task, Arm: r.Arm, I: r.I, Harness: usageOf(r), Delta: map[string]int{}}
		out.Session = sessionOf(archive, r)
		dir := findLedger(keptRoot, r.Task, r.I, out.Session, runDir(archive, r))
		if dir == "" {
			out.Note = "no ledger"
			rep.Rows = append(rep.Rows, out)
			continue
		}
		led, n, err := sumLedger(dir)
		if err != nil || n == 0 {
			out.Note = "no ledger"
			rep.Rows = append(rep.Rows, out)
			continue
		}
		out.Ledger = led
		out.Delta = map[string]int{
			"input_fresh":    led.InputFresh - out.Harness.InputFresh,
			"cache_read":     led.CacheRead - out.Harness.CacheRead,
			"cache_write_5m": led.CacheWrite5m - out.Harness.CacheWrite5m,
			"cache_write_1h": led.CacheWrite1h - out.Harness.CacheWrite1h,
			"output":         led.Output - out.Harness.Output,
		}
		if prices != nil && r.Model != "" {
			if usd, _ := prices.Cost(r.Model, led); usd.Total != nil {
				v := *usd.Total
				out.LedgerUSD = &v
			}
		}
		if h := harnessCost(archive, r); h != nil {
			out.HarnessUSD = h
		}
		if out.LedgerUSD != nil && out.HarnessUSD != nil && *out.HarnessUSD > 0 {
			ratio := *out.LedgerUSD / *out.HarnessUSD
			cerr := math.Abs(ratio - 1)
			out.Ratio, out.CostError = &ratio, &cerr
		}
		rep.Rows = append(rep.Rows, out)
	}
	for _, r := range rep.Rows {
		if r.Note != "" {
			continue
		}
		rep.Sessions++
		for _, d := range r.Delta {
			if a := abs(d); a > rep.MaxTokenDelta {
				rep.MaxTokenDelta = a
			}
		}
		if r.CostError != nil && (rep.MaxCostError == nil || *r.CostError > *rep.MaxCostError) {
			v := *r.CostError
			rep.MaxCostError = &v
		}
	}
	rep.WithinToleranc = rep.Sessions > 0 && (rep.MaxCostError == nil || *rep.MaxCostError <= Tolerance)
	return rep, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func usageOf(r Row) trace.Usage {
	return trace.Usage{
		InputFresh: r.Usage.InputFresh, CacheRead: r.Usage.CacheRead,
		CacheWrite5m: r.Usage.CacheWrite5m, CacheWrite1h: r.Usage.CacheWrite1h,
		Output: r.Usage.Output,
	}
}

// runDir is the archive path of one run. The model segment is the id the
// cell was launched with (an alias such as "sonnet"), while the row's
// Model is the one the harness served ("claude-sonnet-5"), so the path
// is found rather than rebuilt.
func runDir(archive string, r Row) string {
	direct := filepath.Join(archive, r.Task, r.Model, r.Harness, r.Arm, fmt.Sprint(r.I))
	if _, err := os.Stat(direct); err == nil {
		return direct
	}
	matches, _ := filepath.Glob(filepath.Join(archive, r.Task, "*", r.Harness, r.Arm, fmt.Sprint(r.I)))
	if len(matches) > 0 {
		return matches[0]
	}
	return direct
}

// sessionOf reads the harness session id from the run's disclosure, then
// from its trace, so a reconciliation works on an archive alone.
func sessionOf(archive string, r Row) string {
	raw, err := os.ReadFile(filepath.Join(runDir(archive, r), "harness.json"))
	if err == nil {
		var d struct {
			Transcript *string `json:"transcript_path"`
			Pins       struct {
				Harness map[string]any `json:"harness"`
			} `json:"pins"`
		}
		if json.Unmarshal(raw, &d) == nil && d.Transcript != nil {
			base := filepath.Base(*d.Transcript)
			if s := strings.TrimSuffix(base, ".jsonl"); s != base {
				return s
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(runDir(archive, r), "trace.jsonl")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			var ev struct {
				Session string `json:"session"`
			}
			if json.Unmarshal([]byte(line), &ev) == nil && ev.Session != "" {
				return ev.Session
			}
		}
	}
	return ""
}

// harnessCost reads the harness's own figure for a run: the row's when
// it holds it, else the disclosure's, so an archive written before the
// cost change of 2026-09-06 still reconciles.
func harnessCost(archive string, r Row) *float64 {
	raw, err := os.ReadFile(filepath.Join(runDir(archive, r), "harness.json"))
	if err == nil {
		var d struct {
			Cost *float64 `json:"harness_cost_usd"`
		}
		if json.Unmarshal(raw, &d) == nil && d.Cost != nil {
			return d.Cost
		}
	}
	return nil
}

// Text renders the report as the table the notes carry.
func (r *ReconcileReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "| task | arm | i | session | ledger in/read/w5m/w1h/out | harness in/read/w5m/w1h/out | max delta | ledger usd | harness usd | ratio |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|\n")
	rows := append([]ReconcileRow{}, r.Rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Task != rows[j].Task {
			return rows[i].Task < rows[j].Task
		}
		if rows[i].Arm != rows[j].Arm {
			return rows[i].Arm < rows[j].Arm
		}
		return rows[i].I < rows[j].I
	})
	for _, w := range rows {
		if w.Note != "" {
			fmt.Fprintf(&b, "| %s | %s | %d | | | | | | | %s |\n", w.Task, w.Arm, w.I, w.Note)
			continue
		}
		maxd := 0
		for _, d := range w.Delta {
			if a := abs(d); a > maxd {
				maxd = a
			}
		}
		sess := w.Session
		if len(sess) > 8 {
			sess = sess[:8]
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %d/%d/%d/%d/%d | %d/%d/%d/%d/%d | %d | %s | %s | %s |\n",
			w.Task, w.Arm, w.I, sess,
			w.Ledger.InputFresh, w.Ledger.CacheRead, w.Ledger.CacheWrite5m, w.Ledger.CacheWrite1h, w.Ledger.Output,
			w.Harness.InputFresh, w.Harness.CacheRead, w.Harness.CacheWrite5m, w.Harness.CacheWrite1h, w.Harness.Output,
			maxd, f4(w.LedgerUSD), f4(w.HarnessUSD), f6(w.Ratio))
	}
	fmt.Fprintf(&b, "\n%d session(s) with a ledger; max absolute token delta %d; max cost error %s; within %.0f%%: %s\n",
		r.Sessions, r.MaxTokenDelta, f6(r.MaxCostError), r.Tolerance*100, yesNo(r.WithinToleranc))
	return b.String()
}

func f4(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.4f", *p)
}

func f6(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.6f", *p)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
