package trace

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSonnet5PriceRowFitsTheHarness recomputes the least-squares fit of
// the harness's own total_cost_usd over the twelve archived runs of the
// third smoke. The pinned row was the previous generation's sheet, which
// is the whole of the 1.5 ratio the smoke found; this test makes a
// future harness price change break here rather than quietly move every
// bench number (docs/12 row 13, 2026-09-06).
func TestSonnet5PriceRowFitsTheHarness(t *testing.T) {
	type obs struct {
		u    Usage
		cost float64
	}
	var rows []obs
	paths, _ := filepath.Glob(filepath.Join("..", "..", "bench", "results", "smoke-2026-09-06-2", "*", "*", "*", "*", "*", "*", "run.json"))
	if len(paths) == 0 {
		t.Skip("archived smoke not present")
	}
	for _, p := range paths {
		var row struct {
			Usage Usage `json:"usage"`
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			t.Fatal(err)
		}
		hraw, err := os.ReadFile(strings.Replace(p, "run.json", "harness.json", 1))
		if err != nil {
			t.Fatal(err)
		}
		var h struct {
			Cost *float64 `json:"harness_cost_usd"`
		}
		if err := json.Unmarshal(hraw, &h); err != nil || h.Cost == nil {
			continue
		}
		rows = append(rows, obs{row.Usage, *h.Cost})
	}
	if len(rows) != 12 {
		t.Fatalf("%d runs with a harness cost, want 12", len(rows))
	}

	// Four identified columns: cache_write_5m is zero on every run,
	// because Claude Code writes 1h cache, so its price cannot be fitted.
	for _, r := range rows {
		if r.u.CacheWrite5m != 0 {
			t.Fatalf("cache_write_5m is nonzero (%d): the fit's rank assumption no longer holds", r.u.CacheWrite5m)
		}
	}
	col := func(r obs, i int) float64 {
		switch i {
		case 0:
			return float64(r.u.InputFresh)
		case 1:
			return float64(r.u.CacheRead)
		case 2:
			return float64(r.u.CacheWrite1h)
		default:
			return float64(r.u.Output)
		}
	}
	// Normal equations, 4x4, solved by Gauss-Jordan with partial pivoting.
	var m [4][5]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			for _, r := range rows {
				m[i][j] += col(r, i) * col(r, j)
			}
		}
		for _, r := range rows {
			m[i][4] += col(r, i) * r.cost
		}
	}
	for c := 0; c < 4; c++ {
		piv := c
		for r := c; r < 4; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[piv][c]) {
				piv = r
			}
		}
		m[c], m[piv] = m[piv], m[c]
		for r := 0; r < 4; r++ {
			if r == c {
				continue
			}
			f := m[r][c] / m[c][c]
			for k := c; k < 5; k++ {
				m[r][k] -= f * m[c][k]
			}
		}
	}
	got := [4]float64{}
	for i := 0; i < 4; i++ {
		got[i] = m[i][4] / m[i][i] * 1e6
	}
	want := [4]float64{2.00, 0.20, 4.00, 10.00} // input, cache_read, cache_write_1h, output
	names := [4]string{"in", "cache_read", "cache_write_1h", "out"}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("%s fitted %.9f per million, want %.2f", names[i], got[i], want[i])
		}
	}

	// The pinned row must reproduce the harness's figure on every run.
	tab := DefaultPrices()
	for i, r := range rows {
		usd, reason := tab.Cost("claude-sonnet-5", r.u)
		if usd.Total == nil {
			t.Fatalf("run %d unpriced: %s", i, reason)
		}
		if math.Abs(*usd.Total-r.cost) > 1e-9 {
			t.Errorf("run %d: pinned %.9f, harness %.9f", i, *usd.Total, r.cost)
		}
	}
}
