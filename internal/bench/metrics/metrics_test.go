package metrics

import (
	"math"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestPassPowK(t *testing.T) {
	cases := []struct {
		c, n, k int
		want    float64
	}{
		{3, 5, 2, 0.3}, {5, 5, 5, 1}, {2, 5, 3, 0}, {0, 5, 1, 0}, {5, 5, 1, 1}, {4, 5, 1, 0.8}, {4, 5, 3, 0.4}, {3, 5, 5, 0}, {3, 5, 6, 0}, {2, 2, 2, 1},
	}
	for _, c := range cases {
		if got := PassPowK(c.c, c.n, c.k); !near(got, c.want, 1e-12) {
			t.Errorf("PassPowK(%d,%d,%d) = %v, want %v", c.c, c.n, c.k, got, c.want)
		}
	}
}

func cells() []Cell {
	mk := func(task string, passes ...bool) Cell {
		c := Cell{Task: task}
		for i, p := range passes {
			cost := float64(i + 1)
			c.Runs = append(c.Runs, Run{Task: task, Pass: p, Clean: p, Tokens: 100 * float64(i+1), Cost: &cost, WallS: 10, Turns: 3})
		}
		return c
	}
	return []Cell{mk("a", true, true, true, true, true), mk("b", true, false, true, false, false), mk("c", false, false, false, false, false), mk("d", true, true, true, true, false)}
}

func TestPassAt1AndPassK(t *testing.T) {
	cs := cells()
	// rates 1, 0.4, 0, 0.8 -> mean 0.55
	if got := PassAt1(cs); !near(got, 0.55, 1e-12) {
		t.Errorf("pass@1 %v", got)
	}
	// pass^5: 1, 0, 0, 0 -> 0.25; pass^3: 1, 0, 0, C(4,3)/C(5,3)=0.4 -> 0.35
	if got := PassK(cs, 5); !near(got, 0.25, 1e-12) {
		t.Errorf("pass^5 %v", got)
	}
	if got := PassK(cs, 3); !near(got, 0.35, 1e-12) {
		t.Errorf("pass^3 %v", got)
	}
	if got := PassK(cs, 1); !near(got, 0.55, 1e-12) {
		t.Errorf("pass^1 %v (must equal pass@1)", got)
	}
}

func TestMediansAndPerSolved(t *testing.T) {
	cs := cells()
	med := TaskMedians(cs, func(r Run) (float64, bool) { return r.Tokens, true })
	for i, m := range med {
		if m != 300 {
			t.Errorf("task %d median tokens %v", i, m)
		}
	}
	if Median([]float64{4, 1, 3, 2}) != 2.5 || Median([]float64{5}) != 5 || Median(nil) != 0 {
		t.Error("Median")
	}
	// tokens: 1500 per task, 6000 total; passes: 5+2+0+4 = 11
	ps := PerSolved(cs, func(r Run) (float64, bool) { return r.Tokens, true })
	if ps == nil || !near(*ps, 6000.0/11, 1e-9) {
		t.Errorf("per solved %v", ps)
	}
	if PerSolved([]Cell{cs[2]}, func(r Run) (float64, bool) { return r.Tokens, true }) != nil {
		t.Error("per solved with zero denominator must be nil")
	}
}

func TestWilcoxonExact(t *testing.T) {
	// d = [1,2,0,3,2,3]: zero dropped, n = 5, all positive: W+ = 15, p = 2/32.
	w := SignedRank([]float64{1, 2, 3, 4, 5, 6}, []float64{2, 4, 3, 7, 7, 9})
	if w.N != 5 || w.W != 15 || !near(w.P, 0.0625, 1e-12) || !w.Exact {
		t.Errorf("all-positive: %+v", w)
	}
	// d = [3,-1,2,4,-2,5]: ranks 4,1,2.5,5,2.5,6; W+ = 17.5; p = 12/64.
	w = SignedRank([]float64{0, 0, 0, 0, 0, 0}, []float64{3, -1, 2, 4, -2, 5})
	if w.N != 6 || w.W != 17.5 || !near(w.P, 0.1875, 1e-12) {
		t.Errorf("mixed: %+v", w)
	}
	if w.R > 0 == false || !near(w.R, w.Z/math.Sqrt(6), 1e-12) {
		t.Errorf("effect size %+v", w)
	}
	// No differences: n = 0, p = 1.
	if w := SignedRank([]float64{1, 2}, []float64{1, 2}); w.N != 0 || w.P != 1 {
		t.Errorf("no diffs: %+v", w)
	}
	// Symmetric: W+ = W- gives p = 1.
	if w := SignedRank([]float64{0, 0}, []float64{1, -1}); !near(w.P, 1, 1e-12) {
		t.Errorf("symmetric: %+v", w)
	}
}

func TestWilcoxonApprox(t *testing.T) {
	// n = 30, all positive: z = (465 - 232.5 - 0.5) / sqrt(30*31*61/24) = 232/48.6184... = 4.7718
	a := make([]float64, 30)
	b := make([]float64, 30)
	for i := range b {
		b[i] = float64(i + 1)
	}
	w := SignedRank(a, b)
	if w.Exact || w.N != 30 || w.W != 465 {
		t.Errorf("approx: %+v", w)
	}
	if !near(w.Z, 232/math.Sqrt(30*31*61/24.0), 1e-9) {
		t.Errorf("z %v", w.Z)
	}
	if !near(w.P, 2*(1-phi(w.Z)), 1e-15) || w.P > 1e-5 {
		t.Errorf("p %v", w.P)
	}
}

func TestBootstrapDeterministic(t *testing.T) {
	rates := []float64{1, 0.4, 0, 0.8}
	stat := func(s []int) float64 {
		sum := 0.0
		for _, i := range s {
			sum += rates[i]
		}
		return sum / float64(len(s))
	}
	ci1 := Bootstrap(20260902, len(rates), stat)
	ci2 := Bootstrap(20260902, len(rates), stat)
	if ci1 != ci2 {
		t.Errorf("bootstrap not deterministic: %v %v", ci1, ci2)
	}
	if !(ci1[0] <= 0.55 && 0.55 <= ci1[1]) || ci1[0] < 0 || ci1[1] > 1 {
		t.Errorf("ci %v", ci1)
	}
	if ci := Bootstrap(1, 0, stat); !math.IsNaN(ci[0]) {
		t.Errorf("empty ci %v", ci)
	}
}
