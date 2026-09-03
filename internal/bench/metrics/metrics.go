// Package metrics computes the bench-spec section 5 statistics from run
// rows with fixed algorithms so recomputation from an archive is
// bit-identical (section 8.4): pass@1, the unbiased pass^k, per-task
// medians, per-solved ratios, Wilcoxon signed-rank and the seeded task
// bootstrap.
package metrics

import (
	"math"
	"math/rand"
	"sort"
)

// Run is the view of one run row the metrics need.
type Run struct {
	Task        string
	Pass        bool
	Clean       bool // pass and no cheating detector fired
	Tokens      float64
	Cost        *float64
	WallS       float64
	Turns       float64
	ClaimedDone *bool
	Regressed   bool
	ScopeViol   int
	Flagged     bool
	Excluded    bool // outcome infra: not a result (section 3.3)
}

// Cell is the K runs of one task.
type Cell struct {
	Task string
	Runs []Run
}

// Group orders runs by task, dropping excluded runs.
func Group(runs []Run) []Cell {
	by := map[string][]Run{}
	var order []string
	for _, r := range runs {
		if r.Excluded {
			continue
		}
		if _, ok := by[r.Task]; !ok {
			order = append(order, r.Task)
		}
		by[r.Task] = append(by[r.Task], r)
	}
	sort.Strings(order)
	cells := make([]Cell, 0, len(order))
	for _, t := range order {
		cells = append(cells, Cell{Task: t, Runs: by[t]})
	}
	return cells
}

// Passes counts passing runs in the cell.
func (c Cell) Passes() int {
	n := 0
	for _, r := range c.Runs {
		if r.Pass {
			n++
		}
	}
	return n
}

// Rate is the per-task pass rate (1/K) sum pass(t, i).
func (c Cell) Rate() float64 {
	if len(c.Runs) == 0 {
		return 0
	}
	return float64(c.Passes()) / float64(len(c.Runs))
}

// Mean of xs; 0 when empty.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// Median of xs (average of the two middle values); 0 when empty.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// PassAt1 is the mean over tasks of the per-task pass rate (section 5.1).
func PassAt1(cells []Cell) float64 {
	rates := make([]float64, 0, len(cells))
	for _, c := range cells {
		rates = append(rates, c.Rate())
	}
	return Mean(rates)
}

// CleanPassAt1 is pass@1 with flagged passes counted as failures.
func CleanPassAt1(cells []Cell) float64 {
	rates := make([]float64, 0, len(cells))
	for _, c := range cells {
		if len(c.Runs) == 0 {
			rates = append(rates, 0)
			continue
		}
		n := 0
		for _, r := range c.Runs {
			if r.Clean {
				n++
			}
		}
		rates = append(rates, float64(n)/float64(len(c.Runs)))
	}
	return Mean(rates)
}

// PassPowK is the unbiased estimator C(c, k) / C(n, k) of the probability
// that k runs all pass, given c passes in n runs (section 5.2); 0 when
// c < k, and 0 when k > n.
func PassPowK(c, n, k int) float64 {
	if k <= 0 {
		return 1
	}
	if c < k || n < k {
		return 0
	}
	p := 1.0
	for j := 0; j < k; j++ {
		p *= float64(c-j) / float64(n-j)
	}
	return p
}

// PassK is mean over tasks of PassPowK (section 5.2).
func PassK(cells []Cell, k int) float64 {
	vals := make([]float64, 0, len(cells))
	for _, c := range cells {
		vals = append(vals, PassPowK(c.Passes(), len(c.Runs), k))
	}
	return Mean(vals)
}

// TaskMedians is m(t) = median over the cell's runs of f (section 5.3),
// in cell order. Runs where f returns ok=false are skipped; a task with
// no value yields NaN.
func TaskMedians(cells []Cell, f func(Run) (float64, bool)) []float64 {
	out := make([]float64, 0, len(cells))
	for _, c := range cells {
		var xs []float64
		for _, r := range c.Runs {
			if v, ok := f(r); ok {
				xs = append(xs, v)
			}
		}
		if len(xs) == 0 {
			out = append(out, math.NaN())
			continue
		}
		out = append(out, Median(xs))
	}
	return out
}

// Finite drops NaN values.
func Finite(xs []float64) []float64 {
	var out []float64
	for _, x := range xs {
		if !math.IsNaN(x) {
			out = append(out, x)
		}
	}
	return out
}

// PerSolved is sum of f over every run divided by the number of passing
// runs (section 5.6); nil when nothing passed (printed as infinity).
func PerSolved(cells []Cell, f func(Run) (float64, bool)) *float64 {
	num, den := 0.0, 0
	for _, c := range cells {
		for _, r := range c.Runs {
			if v, ok := f(r); ok {
				num += v
			}
			if r.Pass {
				den++
			}
		}
	}
	if den == 0 {
		return nil
	}
	v := num / float64(den)
	return &v
}

// Wilcoxon is the paired signed-rank result (section 5.5).
type Wilcoxon struct {
	N     int     `json:"n"`
	W     float64 `json:"W"`
	P     float64 `json:"p"`
	R     float64 `json:"r"`
	Z     float64 `json:"z"`
	Exact bool    `json:"exact"`
}

// SignedRank tests paired samples a, b (differences b - a). Zero
// differences are dropped, ties take mid-ranks, the exact distribution
// is used for n <= 25 and the normal approximation with continuity
// correction otherwise. W is the sum of ranks of positive differences.
func SignedRank(a, b []float64) Wilcoxon {
	var d []float64
	for i := range a {
		if i < len(b) && !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			if diff := b[i] - a[i]; diff != 0 {
				d = append(d, diff)
			}
		}
	}
	n := len(d)
	if n == 0 {
		return Wilcoxon{N: 0, P: 1, Exact: true}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return math.Abs(d[idx[i]]) < math.Abs(d[idx[j]]) })
	ranks := make([]float64, n)
	tieCorr := 0.0
	for i := 0; i < n; {
		j := i
		for j+1 < n && math.Abs(d[idx[j+1]]) == math.Abs(d[idx[i]]) {
			j++
		}
		mid := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			ranks[idx[k]] = mid
		}
		t := float64(j - i + 1)
		tieCorr += t*t*t - t
		i = j + 1
	}
	wPlus := 0.0
	for i := range d {
		if d[i] > 0 {
			wPlus += ranks[i]
		}
	}
	fn := float64(n)
	mean := fn * (fn + 1) / 4
	variance := fn*(fn+1)*(2*fn+1)/24 - tieCorr/48
	z := 0.0
	if variance > 0 {
		cc := 0.0
		switch {
		case wPlus > mean:
			cc = -0.5
		case wPlus < mean:
			cc = 0.5
		}
		z = (wPlus - mean + cc) / math.Sqrt(variance)
	}
	res := Wilcoxon{N: n, W: wPlus, Z: z, R: z / math.Sqrt(fn)}
	if n <= 25 {
		res.Exact = true
		res.P = exactP(ranks, wPlus)
	} else {
		res.P = 2 * (1 - phi(math.Abs(z)))
	}
	if res.P > 1 {
		res.P = 1
	}
	return res
}

// exactP is the two-sided p-value from the permutation distribution of
// W+ over the observed ranks (doubled to integers so mid-ranks fit).
func exactP(ranks []float64, w float64) float64 {
	total := 0
	r2 := make([]int, len(ranks))
	for i, r := range ranks {
		r2[i] = int(math.Round(2 * r))
		total += r2[i]
	}
	counts := make([]float64, total+1)
	counts[0] = 1
	for _, r := range r2 {
		for s := total; s >= r; s-- {
			counts[s] += counts[s-r]
		}
	}
	all := math.Ldexp(1, len(ranks))
	w2 := int(math.Round(2 * w))
	le, ge := 0.0, 0.0
	for s, c := range counts {
		if s <= w2 {
			le += c
		}
		if s >= w2 {
			ge += c
		}
	}
	p := 2 * math.Min(le, ge) / all
	if p > 1 {
		p = 1
	}
	return p
}

func phi(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// Resamples is the fixed bootstrap size (section 5.5).
const Resamples = 10000

// Bootstrap draws Resamples task samples with replacement from n tasks
// using the seeded generator and returns the percentile 95% interval of
// stat over the samples. stat receives the sampled task indices. NaN
// statistics are dropped; an empty result is [NaN, NaN].
func Bootstrap(seed int64, n int, stat func(sample []int) float64) [2]float64 {
	if n == 0 {
		return [2]float64{math.NaN(), math.NaN()}
	}
	rng := rand.New(rand.NewSource(seed))
	vals := make([]float64, 0, Resamples)
	sample := make([]int, n)
	for b := 0; b < Resamples; b++ {
		for i := range sample {
			sample[i] = rng.Intn(n)
		}
		if v := stat(sample); !math.IsNaN(v) {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return [2]float64{math.NaN(), math.NaN()}
	}
	sort.Float64s(vals)
	return [2]float64{percentile(vals, 0.025), percentile(vals, 0.975)}
}

func percentile(sorted []float64, p float64) float64 {
	i := int(math.Round(p * float64(len(sorted)-1)))
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// Round6 rounds to six decimals, the report's fixed precision.
func Round6(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	return math.Round(x*1e6) / 1e6
}
