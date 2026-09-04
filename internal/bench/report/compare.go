package report

import (
	"fmt"
	"math"
	"sort"

	"github.com/ddh4r4m/saga/internal/bench/metrics"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/cli"
)

// Archive is one loaded run directory.
type Archive struct {
	Dir      string
	Manifest *run.Manifest
	Hash     string
	Rows     []run.Row
}

// Load reads a run directory.
func Load(dir string) (*Archive, error) {
	m, h, rows, err := run.ReadArchive(dir)
	if err != nil {
		return nil, err
	}
	return &Archive{Dir: dir, Manifest: m, Hash: h, Rows: rows}, nil
}

type pairedMetric struct {
	name  string
	k     int
	value func(c metrics.Cell) float64 // per-task value; NaN when undefined
	agg   func(vals []float64) float64 // arm-level summary over task values
}

func cellMedian(f func(metrics.Run) (float64, bool)) func(metrics.Cell) float64 {
	return func(c metrics.Cell) float64 {
		v := metrics.TaskMedians([]metrics.Cell{c}, f)
		return v[0]
	}
}

func meanFinite(vals []float64) float64 {
	fin := metrics.Finite(vals)
	if len(fin) == 0 {
		return math.NaN()
	}
	return metrics.Mean(fin)
}

func medianFinite(vals []float64) float64 {
	fin := metrics.Finite(vals)
	if len(fin) == 0 {
		return math.NaN()
	}
	return metrics.Median(fin)
}

func cellRate(pred func(metrics.Run) bool) func(metrics.Cell) float64 {
	return func(c metrics.Cell) float64 {
		if len(c.Runs) == 0 {
			return math.NaN()
		}
		n := 0
		for _, r := range c.Runs {
			if pred(r) {
				n++
			}
		}
		return float64(n) / float64(len(c.Runs))
	}
}

func cellFalseDone(c metrics.Cell) float64 {
	claimed, fd := 0, 0
	for _, r := range c.Runs {
		if r.ClaimedDone != nil && *r.ClaimedDone {
			claimed++
			if !r.Pass {
				fd++
			}
		}
	}
	if claimed == 0 {
		return math.NaN()
	}
	return float64(fd) / float64(claimed)
}

// Compare builds the paired two-arm report (section 4.1, 5.5). The two
// archives must share the task set; otherwise exit 2.
func Compare(a, b *Archive, epsilon float64) (*Report, error) {
	sa := ArmFrom(a.Manifest, a.Rows)
	sb := ArmFrom(b.Manifest, b.Rows)
	armA, armB := "A", "B"
	if len(a.Manifest.Arms) > 0 {
		armA = a.Manifest.Arms[0].ID
	}
	if len(b.Manifest.Arms) > 0 {
		armB = b.Manifest.Arms[0].ID
	}
	if armA == armB {
		armA, armB = "A", "B"
	}
	tasksA := map[string]metrics.Cell{}
	for _, c := range sa.cells {
		tasksA[c.Task] = c
	}
	var pairs []string
	for _, c := range sb.cells {
		if _, ok := tasksA[c.Task]; ok {
			pairs = append(pairs, c.Task)
		}
	}
	sort.Strings(pairs)
	if len(pairs) != len(sa.cells) || len(pairs) != len(sb.cells) || len(pairs) == 0 {
		return nil, cli.Errorf(cli.ExitUsage, "compare: arms are not paired: %d tasks in A, %d in B, %d shared", len(sa.cells), len(sb.cells), len(pairs))
	}
	if a.Manifest.K != b.Manifest.K {
		return nil, cli.Errorf(cli.ExitUsage, "compare: K differs (%d vs %d)", a.Manifest.K, b.Manifest.K)
	}
	if sa.Harness != sb.Harness {
		return nil, cli.Errorf(cli.ExitUsage, "compare: harness differs (%s vs %s)", sa.Harness, sb.Harness)
	}
	cellsB := map[string]metrics.Cell{}
	for _, c := range sb.cells {
		cellsB[c.Task] = c
	}
	k := a.Manifest.K
	defs := []pairedMetric{
		{"pass_at_1", 0, cellRate(func(r metrics.Run) bool { return r.Pass }), meanFinite},
		{"pass_k", k, func(c metrics.Cell) float64 { return metrics.PassPowK(c.Passes(), len(c.Runs), k) }, meanFinite},
		{"clean_pass_at_1", 0, cellRate(func(r metrics.Run) bool { return r.Clean }), meanFinite},
		{"false_done", 0, cellFalseDone, meanFinite},
		{"regression_rate", 0, cellRate(func(r metrics.Run) bool { return r.Regressed }), meanFinite},
		{"scope_violation_rate", 0, cellRate(func(r metrics.Run) bool { return r.ScopeViol > 0 }), meanFinite},
		{"tokens_median", 0, cellMedian(tokensOf), medianFinite},
		{"cost_median", 0, cellMedian(costOf), medianFinite},
		{"wall_s_median", 0, cellMedian(wallOf), medianFinite},
		{"turns_median", 0, cellMedian(turnsOf), medianFinite},
	}
	r := newReport(a.Manifest, a.Hash)
	r.Manifests = map[string]string{armA: a.Hash, armB: b.Hash}
	r.Arms[armA], r.Arms[armB] = sa, sb
	r.armOrder = []string{armA, armB}
	r.armMeta = map[string]run.Arm{}
	if len(a.Manifest.Arms) > 0 {
		r.armMeta[armA] = a.Manifest.Arms[0]
	}
	if len(b.Manifest.Arms) > 0 {
		r.armMeta[armB] = b.Manifest.Arms[0]
	}
	r.PerSolved[armA], r.PerSolved[armB] = sa.PerSolved, sb.PerSolved
	r.Instability[armA], r.Instability[armB] = sa.Instability, sb.Instability
	ut := append(unstable(sa), unstable(sb)...)
	sort.Strings(ut)
	r.Instability["unstable_tasks"] = dedupe(ut)
	r.Exclusions["infra"] = sa.Outcomes["infra"] + sb.Outcomes["infra"]
	r.TotalCostUSD = addCost(totalCost(a.Rows), totalCost(b.Rows))
	seed := a.Manifest.BootstrapSeed
	for _, d := range defs {
		va := make([]float64, len(pairs))
		vb := make([]float64, len(pairs))
		for i, t := range pairs {
			va[i] = d.value(tasksA[t])
			vb[i] = d.value(cellsB[t])
		}
		c := Comparison{Metric: d.name, K: d.k, Arms: []string{armA, armB}}
		aggA, aggB := d.agg(va), d.agg(vb)
		c.A, c.B = f6(aggA), f6(aggB)
		if c.A != nil && c.B != nil {
			c.Delta = f6(aggB - aggA)
		}
		ci := metrics.Bootstrap(seed, len(pairs), func(s []int) float64 {
			xa := make([]float64, len(s))
			xb := make([]float64, len(s))
			for i, j := range s {
				xa[i], xb[i] = va[j], vb[j]
			}
			return d.agg(xb) - d.agg(xa)
		})
		c.CI95 = []*float64{f6(ci[0]), f6(ci[1])}
		wc := metrics.SignedRank(va, vb)
		c.Wilcoxon = &wc
		if epsilon > 0 && c.CI95[0] != nil && c.CI95[1] != nil {
			inside := *c.CI95[0] >= -epsilon && *c.CI95[1] <= epsilon
			c.Equivalence = map[string]any{"epsilon": epsilon, "ci_inside": inside}
		}
		if d.name == "pass_at_1" {
			supported := c.CI95[0] != nil && c.CI95[1] != nil && (*c.CI95[0] > 0 || *c.CI95[1] < 0)
			c.Supported = &supported
			pc := c
			r.Primary = &pc
		}
		r.Secondary = append(r.Secondary, c)
		null := c.Delta == nil || c.CI95[0] == nil || c.CI95[1] == nil || (*c.CI95[0] <= 0 && *c.CI95[1] >= 0)
		if null {
			entry := map[string]any{"kind": "null", "metric": d.name}
			if c.Delta != nil {
				entry["delta"] = *c.Delta
				entry["ci95"] = c.CI95
				entry["note"] = fmt.Sprintf("no detectable difference at n=%d", len(pairs))
			} else {
				entry["note"] = "undefined in at least one arm"
			}
			r.Negative = append(r.Negative, entry)
		}
	}
	for _, t := range pairs {
		if tasksA[t].Passes() == 0 && cellsB[t].Passes() == 0 {
			r.Negative = append(r.Negative, map[string]any{"kind": "possibly_broken_task", "task": t, "pass_all_arms": 0.0})
		}
	}
	if r.Exclusions["infra"] > 0 {
		r.Negative = append(r.Negative, map[string]any{"kind": "exclusions", "infra": r.Exclusions["infra"]})
	}
	if k < 5 {
		r.Negative = append(r.Negative, map[string]any{"kind": "k_below_5", "k": k, "note": "K < 5: no claim is licensed (ADR 0001)"})
	}
	r.Negative = append(r.Negative, map[string]any{"kind": "component_unused", "note": "not evaluated: this runner records no component usage; treat every treatment arm as no-exposure until the trace carries component events"})
	r.Reproduce = []string{fmt.Sprintf("saga bench compare %s %s", a.Dir, b.Dir)}
	return r, nil
}

func dedupe(xs []string) []string {
	out := []string{}
	for i, x := range xs {
		if i == 0 || xs[i-1] != x {
			out = append(out, x)
		}
	}
	return out
}

func addCost(a, b *float64) *float64 {
	if a == nil && b == nil {
		return nil
	}
	s := 0.0
	if a != nil {
		s += *a
	}
	if b != nil {
		s += *b
	}
	return f6(s)
}
