// Package report computes bench-spec section 5 metrics from run rows and
// renders report.json (saga.bench.report/1) and report.md in the fixed
// section order of section 7.2. Every number is rounded to six decimals
// and the JSON is canonical so two runs over the same rows are
// byte-identical (section 8.4).
package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/metrics"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/schema"
)

// Schema is the report schema id.
const Schema = "saga.bench.report/1"

// PerTask is one task's row in an arm.
type PerTask struct {
	Task          string   `json:"task"`
	N             int      `json:"n"`
	C             int      `json:"c"`
	PassRate      float64  `json:"pass_rate"`
	MedianTokens  float64  `json:"median_tokens"`
	MedianCostUSD *float64 `json:"median_cost_usd"`
	MedianWallS   float64  `json:"median_wall_s"`
	MedianTurns   float64  `json:"median_turns"`
}

// PerSolved is section 5.6; nil means undefined (no solved run).
type PerSolved struct {
	Tokens *float64 `json:"tokens"`
	USD    *float64 `json:"usd"`
}

// ArmStats is one arm's summary.
type ArmStats struct {
	Runs                 int                 `json:"runs"`
	Tasks                int                 `json:"tasks"`
	K                    int                 `json:"k"`
	Model                string              `json:"model"`
	Harness              string              `json:"harness"`
	PassAt1              float64             `json:"pass_at_1"`
	PassAt1CI95          []float64           `json:"pass_at_1_ci95"`
	CleanPassAt1         float64             `json:"clean_pass_at_1"`
	PassK                map[string]float64  `json:"pass_k"`
	Instability          float64             `json:"instability"`
	Medians              map[string]*float64 `json:"medians"`
	Means                map[string]*float64 `json:"means"`
	PerSolved            PerSolved           `json:"per_solved"`
	FalseDone            *float64            `json:"false_done"`
	FalseDoneReason      *string             `json:"false_done_reason"`
	FalseDoneStructural  *float64            `json:"false_done_structural"`
	NoClaimRate          float64             `json:"no_claim_rate"`
	ClaimContradiction   ClaimContradiction  `json:"claim_contradiction_rate"`
	RegressionRate       float64             `json:"regression_rate"`
	RegressionRatePass   *float64            `json:"regression_rate_pass"`
	RegressionRateFail   *float64            `json:"regression_rate_fail"`
	ScopeViolationRate   float64             `json:"scope_violation_rate"`
	ScopeFilesMedian     float64             `json:"scope_files_median"`
	CheatRate            *float64            `json:"cheat_rate"`
	Outcomes             map[string]int      `json:"outcomes"`
	BlockedReachAttempts int                 `json:"blocked_reach_attempts"`
	PerTask              []PerTask           `json:"per_task"`

	cells []metrics.Cell
}

// ClaimContradiction is the trace-spec 5.9 metric: runs whose final-turn
// claim verdict is contradicted over runs with at least one claim, split
// by hidden-oracle outcome (oracle-fail: the share of false-done the
// deterministic checks catch; oracle-pass: the false-positive rate,
// bound 0.02).
type ClaimContradiction struct {
	RunsWithClaims int      `json:"runs_with_claims"`
	OracleFail     *float64 `json:"oracle_fail"`
	OraclePass     *float64 `json:"oracle_pass"`
}

// Comparison is one paired delta (section 5.5).
type Comparison struct {
	Metric      string            `json:"metric"`
	K           int               `json:"k,omitempty"`
	Arms        []string          `json:"arms"`
	A           *float64          `json:"A"`
	B           *float64          `json:"B"`
	Delta       *float64          `json:"delta"`
	CI95        []*float64        `json:"ci95"`
	Wilcoxon    *metrics.Wilcoxon `json:"wilcoxon"`
	Equivalence map[string]any    `json:"equivalence,omitempty"`
	Supported   *bool             `json:"supported,omitempty"`
}

// Report is saga.bench.report/1.
type Report struct {
	Schema             string               `json:"schema"`
	Manifest           string               `json:"manifest"`
	Manifests          map[string]string    `json:"manifests,omitempty"`
	Tier               string               `json:"tier"`
	Created            *string              `json:"created"`
	TaskSetSHA256      *string              `json:"task_set_sha256"`
	K                  int                  `json:"k"`
	BootstrapSeed      int64                `json:"bootstrap_seed"`
	BootstrapResamples int                  `json:"bootstrap_resamples"`
	Isolation          *string              `json:"isolation"`
	TotalCostUSD       *float64             `json:"total_cost_usd"`
	Arms               map[string]*ArmStats `json:"arms"`
	Primary            *Comparison          `json:"primary"`
	Secondary          []Comparison         `json:"secondary"`
	PerSolved          map[string]PerSolved `json:"per_solved"`
	Instability        map[string]any       `json:"instability"`
	Negative           []map[string]any     `json:"negative"`
	Exploratory        []map[string]any     `json:"exploratory"`
	Exclusions         map[string]int       `json:"exclusions"`
	Contamination      []map[string]any     `json:"contamination"`
	DetectorPrecision  map[string]*float64  `json:"detector_precision"`
	Reproduce          []string             `json:"reproduce"`

	armOrder []string
	// armMeta is the manifest's arm entry (components, control blocks)
	// per arm id, for the setup section; not serialised (the schema is
	// closed and the manifests carry it).
	armMeta map[string]run.Arm
}

func f6(x float64) *float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	v := metrics.Round6(x)
	return &v
}

func f6p(p *float64) *float64 {
	if p == nil {
		return nil
	}
	return f6(*p)
}

func strp(s string) *string { return &s }

// toMetrics converts rows to the metrics view.
func toMetrics(rows []run.Row) []metrics.Run {
	out := make([]metrics.Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, metrics.Run{
			Task: r.Task, Pass: r.Oracle.Pass, Clean: r.Oracle.Pass && !r.Scan.Flagged,
			Tokens: float64(r.Usage.Total()), Cost: r.CostUSD, WallS: r.WallS, Turns: float64(r.Turns),
			ClaimedDone: r.ClaimedDone, ClaimedDoneStructural: r.ClaimedDoneStructural, ClaimVerdict: deref(r.ClaimVerdict),
			Regressed: len(r.Oracle.Regressed) > 0, ScopeViol: len(r.Scan.ScopeViolations),
			Flagged: r.Scan.Flagged, Excluded: r.Outcome == "infra",
		})
	}
	return out
}

func tokensOf(r metrics.Run) (float64, bool) { return r.Tokens, true }
func wallOf(r metrics.Run) (float64, bool)   { return r.WallS, true }
func turnsOf(r metrics.Run) (float64, bool)  { return r.Turns, true }
func costOf(r metrics.Run) (float64, bool) {
	if r.Cost == nil {
		return 0, false
	}
	return *r.Cost, true
}

// ArmFrom computes an arm's statistics from its rows.
func ArmFrom(m *run.Manifest, rows []run.Row) *ArmStats {
	cells := metrics.Group(toMetrics(rows))
	a := &ArmStats{K: m.K, PassK: map[string]float64{}, Medians: map[string]*float64{}, Means: map[string]*float64{}, Outcomes: map[string]int{}, PerTask: []PerTask{}, cells: cells}
	if len(m.Models) > 0 {
		a.Model = m.Models[0].ID
	}
	if len(m.Harnesses) > 0 {
		a.Harness = m.Harnesses[0].Name
	}
	a.Runs = len(rows)
	a.Tasks = len(cells)
	for _, r := range rows {
		a.Outcomes[r.Outcome]++
		a.BlockedReachAttempts += r.BlockedReachAttempts
		if r.Model != "" && r.Outcome != "infra" {
			a.Model = r.Model
		}
	}
	a.PassAt1 = metrics.Round6(metrics.PassAt1(cells))
	rates := make([]float64, len(cells))
	for i, c := range cells {
		rates[i] = c.Rate()
	}
	ci := metrics.Bootstrap(m.BootstrapSeed, len(cells), func(s []int) float64 {
		sum := 0.0
		for _, i := range s {
			sum += rates[i]
		}
		return sum / float64(len(s))
	})
	a.PassAt1CI95 = []float64{metrics.Round6(ci[0]), metrics.Round6(ci[1])}
	if len(cells) == 0 {
		a.PassAt1CI95 = []float64{0, 0}
	}
	a.CleanPassAt1 = metrics.Round6(metrics.CleanPassAt1(cells))
	for _, k := range []int{1, 3, 5, m.K} {
		if k <= m.K {
			a.PassK[fmt.Sprint(k)] = metrics.Round6(metrics.PassK(cells, k))
		}
	}
	a.Instability = metrics.Round6(a.PassAt1 - a.PassK[fmt.Sprint(m.K)])
	for name, f := range map[string]func(metrics.Run) (float64, bool){"tokens": tokensOf, "cost_usd": costOf, "wall_s": wallOf, "turns": turnsOf} {
		med := metrics.Finite(metrics.TaskMedians(cells, f))
		if len(med) == 0 {
			a.Medians[name], a.Means[name] = nil, nil
			continue
		}
		a.Medians[name] = f6(metrics.Median(med))
		a.Means[name] = f6(metrics.Mean(med))
	}
	a.PerSolved = PerSolved{Tokens: f6p(metrics.PerSolved(cells, tokensOf)), USD: f6p(metrics.PerSolved(cells, costOf))}
	claimed, falseDone, unknown := 0, 0, 0
	claimedS, falseDoneS := 0, 0
	withClaims, contraPass, contraFail, claimsPass, claimsFail := 0, 0, 0, 0, 0
	regressed, regPass, regFail, passN, failN, scopeRuns, flaggedPass := 0, 0, 0, 0, 0, 0, 0
	var scopeCounts []float64
	total := 0
	for _, c := range cells {
		for _, r := range c.Runs {
			total++
			switch {
			case r.ClaimedDone == nil:
				unknown++
			case *r.ClaimedDone:
				claimed++
				if !r.Pass {
					falseDone++
				}
			}
			if r.ClaimedDoneStructural != nil && *r.ClaimedDoneStructural {
				claimedS++
				if !r.Pass {
					falseDoneS++
				}
			}
			if r.ClaimVerdict != "" {
				withClaims++
				if r.Pass {
					claimsPass++
					if r.ClaimVerdict == "contradicted" {
						contraPass++
					}
				} else {
					claimsFail++
					if r.ClaimVerdict == "contradicted" {
						contraFail++
					}
				}
			}
			if r.Regressed {
				regressed++
			}
			if r.Pass {
				passN++
				if r.Regressed {
					regPass++
				}
				if r.Flagged {
					flaggedPass++
				}
			} else {
				failN++
				if r.Regressed {
					regFail++
				}
			}
			if r.ScopeViol > 0 {
				scopeRuns++
			}
			scopeCounts = append(scopeCounts, float64(r.ScopeViol))
		}
	}
	switch {
	case unknown > 0:
		a.FalseDone = nil
		a.FalseDoneReason = strp(fmt.Sprintf("%d runs carry no claimed_done verdict", unknown))
	case claimed == 0:
		a.FalseDone = nil
		a.FalseDoneReason = strp("no run claimed done")
	default:
		a.FalseDone = f6(float64(falseDone) / float64(claimed))
		a.FalseDoneReason = strp("claimed_done copied from the trace claim event (claims.txt " + run.ClaimsHash[:23] + ", abstain.txt " + run.AbstainHash[:23] + ")")
	}
	if claimedS > 0 {
		a.FalseDoneStructural = f6(float64(falseDoneS) / float64(claimedS))
	}
	if total > 0 {
		a.NoClaimRate = metrics.Round6(float64(unknown) / float64(total))
	}
	a.ClaimContradiction = ClaimContradiction{RunsWithClaims: withClaims}
	if claimsPass > 0 {
		a.ClaimContradiction.OraclePass = f6(float64(contraPass) / float64(claimsPass))
	}
	if claimsFail > 0 {
		a.ClaimContradiction.OracleFail = f6(float64(contraFail) / float64(claimsFail))
	}
	if total > 0 {
		a.RegressionRate = metrics.Round6(float64(regressed) / float64(total))
		a.ScopeViolationRate = metrics.Round6(float64(scopeRuns) / float64(total))
	}
	if passN > 0 {
		a.RegressionRatePass = f6(float64(regPass) / float64(passN))
		a.CheatRate = f6(float64(flaggedPass) / float64(passN))
	}
	if failN > 0 {
		a.RegressionRateFail = f6(float64(regFail) / float64(failN))
	}
	a.ScopeFilesMedian = metrics.Round6(metrics.Median(scopeCounts))
	for _, c := range cells {
		pt := PerTask{Task: c.Task, N: len(c.Runs), C: c.Passes(), PassRate: metrics.Round6(c.Rate())}
		pt.MedianTokens = metrics.Round6(metrics.TaskMedians([]metrics.Cell{c}, tokensOf)[0])
		pt.MedianWallS = metrics.Round6(metrics.TaskMedians([]metrics.Cell{c}, wallOf)[0])
		pt.MedianTurns = metrics.Round6(metrics.TaskMedians([]metrics.Cell{c}, turnsOf)[0])
		pt.MedianCostUSD = f6(metrics.TaskMedians([]metrics.Cell{c}, costOf)[0])
		a.PerTask = append(a.PerTask, pt)
	}
	return a
}

func unstable(a *ArmStats) []string {
	var out []string
	for _, c := range a.cells {
		if p := c.Passes(); p > 0 && p < len(c.Runs) {
			out = append(out, c.Task)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// Build renders a single-arm report.
func Build(m *run.Manifest, hash string, rows []run.Row, dir string) *Report {
	arm := "A"
	if len(m.Arms) > 0 {
		arm = m.Arms[0].ID
	}
	r := newReport(m, hash)
	a := ArmFrom(m, rows)
	r.Arms[arm] = a
	r.armOrder = []string{arm}
	if len(m.Arms) > 0 {
		r.armMeta = map[string]run.Arm{arm: m.Arms[0]}
	}
	r.PerSolved[arm] = a.PerSolved
	r.Instability[arm] = a.Instability
	r.Instability["unstable_tasks"] = unstable(a)
	r.Exclusions["infra"] = a.Outcomes["infra"]
	r.TotalCostUSD = totalCost(rows)
	r.Negative = append(r.Negative, map[string]any{"kind": "no_comparison", "note": "single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report"})
	for _, pt := range a.PerTask {
		if pt.C == 0 {
			r.Negative = append(r.Negative, map[string]any{"kind": "possibly_broken_task", "task": pt.Task, "pass_all_arms": 0.0})
		}
	}
	if a.Outcomes["infra"] > 0 {
		r.Negative = append(r.Negative, map[string]any{"kind": "exclusions", "infra": a.Outcomes["infra"]})
	}
	if m.K < 5 {
		r.Negative = append(r.Negative, map[string]any{"kind": "k_below_5", "k": m.K, "note": "K < 5: no claim is licensed (ADR 0001)"})
	}
	r.Reproduce = []string{fmt.Sprintf("saga bench run --manifest %s/manifest.json", dir), fmt.Sprintf("saga bench report %s", dir)}
	return r
}

func totalCost(rows []run.Row) *float64 {
	sum, n := 0.0, 0
	for _, r := range rows {
		if r.CostUSD != nil {
			sum += *r.CostUSD
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return f6(sum)
}

func newReport(m *run.Manifest, hash string) *Report {
	created := m.Created
	iso := m.Isolation
	ts := m.TaskSet.SHA256
	return &Report{
		Schema: Schema, Manifest: hash, Tier: m.Tier, Created: &created, TaskSetSHA256: &ts, K: m.K,
		BootstrapSeed: m.BootstrapSeed, BootstrapResamples: metrics.Resamples, Isolation: &iso,
		Arms: map[string]*ArmStats{}, Secondary: []Comparison{}, PerSolved: map[string]PerSolved{}, Instability: map[string]any{},
		Negative: []map[string]any{}, Exploratory: []map[string]any{}, Exclusions: map[string]int{}, Contamination: []map[string]any{},
		DetectorPrecision: map[string]*float64{"hard_coded": nil, "assertion_edit": nil, "skip_marker": nil, "test_delete": nil, "env_tamper": nil},
		Reproduce:         []string{},
	}
}

// Validate checks the report against the embedded schema.
func (r *Report) Validate() error {
	v, err := schema.Normalize(r)
	if err != nil {
		return err
	}
	return schema.ValidateID(Schema, v)
}

// JSON is the canonical rendering (sorted keys, no whitespace) plus a
// trailing newline.
func (r *Report) JSON() ([]byte, error) {
	b, err := canon.JSON(r)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func fmtF(p *float64) string {
	if p == nil {
		return "null"
	}
	return fmt.Sprintf("%.3f", *p)
}

func fmtInf(p *float64) string {
	if p == nil {
		return "infinity (no solved run)"
	}
	return fmt.Sprintf("%.3f", *p)
}

func fmtCI(ci []*float64) string {
	if len(ci) != 2 || ci[0] == nil || ci[1] == nil {
		return "(CI undefined)"
	}
	return fmt.Sprintf("(95%% CI %.3f, %.3f)", *ci[0], *ci[1])
}

func fmtW(w *metrics.Wilcoxon) string {
	if w == nil {
		return "Wilcoxon: not computed"
	}
	mode := "normal approximation"
	if w.Exact {
		mode = "exact"
	}
	return fmt.Sprintf("Wilcoxon n=%d W=%.1f p=%.4g r=%.3f (%s)", w.N, w.W, w.P, w.R, mode)
}

// Markdown renders report.md in the section 7.2 order.
func (r *Report) Markdown() string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	order := r.armOrder
	if len(order) == 0 {
		for k := range r.Arms {
			order = append(order, k)
		}
		sort.Strings(order)
	}
	w("# saga bench report")
	w("")
	w("## 1. Header")
	w("")
	w("- manifest: `%s`", r.Manifest)
	for id, h := range r.Manifests {
		w("- manifest %s: `%s`", id, h)
	}
	w("- tier: %s", r.Tier)
	w("- created: %s", deref(r.Created))
	w("- total cost: %s usd", fmtF(r.TotalCostUSD))
	w("- pre-registration: none (no preregistration.md in this archive)")
	w("")
	w("## 2. Setup")
	w("")
	for _, id := range order {
		a := r.Arms[id]
		w("- arm %s: model `%s`, harness `%s`, %d runs over %d tasks, K=%d, outcomes %s", id, a.Model, a.Harness, a.Runs, a.Tasks, a.K, fmtOutcomes(a.Outcomes))
	}
	for _, id := range order {
		if meta, ok := r.armMeta[id]; ok {
			w("- arm %s: components %s; control blocks %s", id, fmtList(meta.Components), fmtList(meta.BlocksInControl))
		}
	}
	w("- task set: `%s`", deref(r.TaskSetSHA256))
	w("- isolation: %s", deref(r.Isolation))
	w("- exclusions: %d infra", r.Exclusions["infra"])
	w("- bootstrap: %d resamples of tasks, seed %d", r.BootstrapResamples, r.BootstrapSeed)
	w("")
	w("## 3. Primary outcome")
	w("")
	if r.Primary == nil {
		w("No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).")
	} else {
		p := r.Primary
		w("Metric `%s` (default primary; no pre-registration file): arms %s. A=%s B=%s delta=%s %s. %s.", p.Metric, strings.Join(p.Arms, " vs "), fmtF(p.A), fmtF(p.B), fmtF(p.Delta), fmtCI(p.CI95), fmtW(p.Wilcoxon))
		if p.Supported != nil {
			w("")
			w("Supported (CI excludes zero): %v.", *p.Supported)
		}
	}
	w("")
	w("## 4. Secondary outcomes")
	w("")
	w("| arm | pass@1 | 95%% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |")
	w("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, id := range order {
		a := r.Arms[id]
		w("| %s | %.3f | %.3f, %.3f | %.3f | %.3f | %.3f | %s | %.3f | %.3f | %s | %s | %s | %s | %s | %s | %s |", id, a.PassAt1, a.PassAt1CI95[0], a.PassAt1CI95[1], a.CleanPassAt1, a.PassK[fmt.Sprint(a.K)], a.Instability, fmtF(a.FalseDone), a.RegressionRate, a.ScopeViolationRate, fmtF(a.CheatRate), fmtF(a.Medians["tokens"]), fmtF(a.Medians["cost_usd"]), fmtF(a.Medians["wall_s"]), fmtF(a.Medians["turns"]), fmtInf(a.PerSolved.Tokens), fmtInf(a.PerSolved.USD))
	}
	for _, id := range order {
		if a := r.Arms[id]; a.FalseDoneReason != nil {
			w("")
			w("false-done for arm %s: %s.", id, *a.FalseDoneReason)
		}
	}
	w("")
	w("| arm | false-done (structural DONE only) | no-claim rate | runs with claims | claim contradiction, oracle-fail | claim contradiction, oracle-pass (bound 0.020) |")
	w("|---|---|---|---|---|---|")
	for _, id := range order {
		a := r.Arms[id]
		w("| %s | %s | %.3f | %d | %s | %s |", id, fmtF(a.FalseDoneStructural), a.NoClaimRate, a.ClaimContradiction.RunsWithClaims, fmtF(a.ClaimContradiction.OracleFail), fmtF(a.ClaimContradiction.OraclePass))
	}
	if len(r.Secondary) > 0 {
		w("")
		w("| metric | A | B | delta | 95%% CI | Wilcoxon |")
		w("|---|---|---|---|---|---|")
		for _, c := range r.Secondary {
			name := c.Metric
			if c.K > 0 {
				name = fmt.Sprintf("%s (k=%d)", c.Metric, c.K)
			}
			w("| %s | %s | %s | %s | %s | %s |", name, fmtF(c.A), fmtF(c.B), fmtF(c.Delta), fmtCI(c.CI95), fmtW(c.Wilcoxon))
		}
	}
	w("")
	w("## 5. Variance")
	w("")
	for _, id := range order {
		a := r.Arms[id]
		var ks []string
		for _, k := range []string{"1", "3", "5", fmt.Sprint(a.K)} {
			if v, ok := a.PassK[k]; ok {
				ks = append(ks, fmt.Sprintf("pass^%s=%.3f", k, v))
			}
		}
		w("- arm %s: pass@1=%.3f, %s, instability (pass@1 - pass^K)=%.3f", id, a.PassAt1, strings.Join(ks, ", "), a.Instability)
	}
	if ut, ok := r.Instability["unstable_tasks"].([]string); ok {
		if len(ut) == 0 {
			w("- unstable tasks (0 < c < K): none")
		} else {
			w("- unstable tasks (0 < c < K): %s", strings.Join(ut, ", "))
		}
	}
	w("")
	w("| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |")
	w("|---|---|---|---|---|---|---|---|---|")
	for _, id := range order {
		for _, pt := range r.Arms[id].PerTask {
			w("| %s | %s | %d | %d | %.3f | %.0f | %s | %.1f | %.0f |", id, pt.Task, pt.N, pt.C, pt.PassRate, pt.MedianTokens, fmtF(pt.MedianCostUSD), pt.MedianWallS, pt.MedianTurns)
		}
	}
	w("")
	w("## 6. Negative results")
	w("")
	for _, n := range r.Negative {
		w("- %s", describe(n))
	}
	w("")
	w("## 7. Exploratory")
	w("")
	if len(r.Exploratory) == 0 {
		w("None.")
	}
	for _, e := range r.Exploratory {
		w("- %s", describe(e))
	}
	w("")
	w("## 8. Cheating and scope scan")
	w("")
	for _, id := range order {
		a := r.Arms[id]
		w("- arm %s: cheat rate %s (flagged passes over passes), scope-violation rate %.3f, median out-of-scope files %.1f, blocked reach attempts %d", id, fmtF(a.CheatRate), a.ScopeViolationRate, a.ScopeFilesMedian, a.BlockedReachAttempts)
	}
	w("")
	w("Detector precision: not yet characterised on the gate-spec 10.1 labelled corpus; every value is null.")
	w("")
	w("## 9. Threats")
	w("")
	w("| threat | status in this run |")
	w("|---|---|")
	w("| model drift behind a stable id | snapshot and fingerprint recorded as null with reasons in harness.json; single cell, no merge across snapshots |")
	w("| contamination | created dates are post-cutoff only if the price table declares cutoffs; not checked in this runner, contamination list is %d long |", len(r.Contamination))
	w("| oracle wrong or gameable | every task passed verify-task at its content hash before the run; cheating scan ran on every row |")
	w("| control arm reaches the component | not applicable: no component blocks are configured in this runner |")
	w("| run-to-run variance | K=%d; pass^k, per-task medians and bootstrap CIs are printed above |", r.K)
	w("| isolation | %s: package caches and the operator's harness config are replaced, the host is shared (badge refused) |", deref(r.Isolation))
	w("")
	w("## 10. Reproduce")
	w("")
	for _, c := range r.Reproduce {
		w("    %s", c)
	}
	return b.String()
}

func deref(p *string) string {
	if p == nil {
		return "null"
	}
	return *p
}

func fmtOutcomes(o map[string]int) string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, o[k]))
	}
	return strings.Join(parts, " ")
}

func describe(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "kind" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := []string{fmt.Sprintf("**%v**", m["kind"])}
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			parts = append(parts, fmt.Sprintf("%s=%.3f", k, v))
		case []*float64:
			parts = append(parts, fmt.Sprintf("%s=%s", k, fmtCI(v)))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return strings.Join(parts, " ")
}

func fmtList(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return "`" + strings.Join(xs, "`, `") + "`"
}
