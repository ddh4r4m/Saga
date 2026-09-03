package report

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
)

func manifest(k int, arm string) *run.Manifest {
	return &run.Manifest{Schema: run.ManifestSchema, Created: "2026-09-03T00:00:00Z", Tier: "smoke", TaskSet: run.TaskSet{SHA256: "sha256:" + strings.Repeat("0", 64)},
		Arms: []run.Arm{{ID: arm, Components: []string{}}}, Models: []run.Model{{ID: "m"}}, Harnesses: []run.HarnessRef{{Name: "replay"}}, K: k, RunSeed: strings.Repeat("0", 64),
		BootstrapSeed: run.BootstrapSeed, AbstainListSHA256: run.AbstainHash, Isolation: "worktree", Images: map[string]string{}, Host: run.Host{OS: "linux", Arch: "arm64"}}
}

// fixture builds rows from a pass pattern per task; tokens = 100*i,
// cost = 0.1*i, wall = 10, turns = i.
func fixture(arm string, pattern map[string][]bool) []run.Row {
	var rows []run.Row
	for tk, passes := range pattern {
		for i, p := range passes {
			cost := 0.1 * float64(i+1)
			claimed := true
			rows = append(rows, run.Row{Schema: run.RowSchema, Task: tk, Model: "m", Harness: "replay", Arm: arm, I: i + 1, Outcome: "completed",
				ClaimedDone: &claimed, Oracle: run.OracleRow{Pass: p, Tests: map[string]string{}, Regressed: []string{}},
				Scan: task.ScanResult{ScopeViolations: []string{}, Detectors: []string{}}, Usage: run.Usage{InputFresh: 100 * (i + 1), Source: "test"},
				CostUSD: &cost, WallS: 10, Turns: i + 1, Compliance: []any{}, Artifacts: map[string]string{}})
		}
	}
	return rows
}

func TestBuildKnownStats(t *testing.T) {
	// a: 5/5, b: 2/5, c: 0/5, d: 4/5 -> pass@1 0.55, pass^5 0.25, pass^3 0.35.
	rows := fixture("A", map[string][]bool{"a": {true, true, true, true, true}, "b": {true, false, true, false, false}, "c": {false, false, false, false, false}, "d": {true, true, true, true, false}})
	m := manifest(5, "A")
	r := Build(m, "sha256:"+strings.Repeat("1", 64), rows, "runs/x")
	a := r.Arms["A"]
	if a.PassAt1 != 0.55 || a.PassK["5"] != 0.25 || a.PassK["3"] != 0.35 || a.PassK["1"] != 0.55 || a.Instability != 0.3 {
		t.Errorf("pass metrics: %+v %+v", a.PassAt1, a.PassK)
	}
	if a.Runs != 20 || a.Tasks != 4 || a.CleanPassAt1 != 0.55 {
		t.Errorf("counts %+v", a)
	}
	// per-task median tokens 300, median cost 0.3, wall 10, turns 3.
	if *a.Medians["tokens"] != 300 || *a.Medians["cost_usd"] != 0.3 || *a.Medians["wall_s"] != 10 || *a.Medians["turns"] != 3 {
		t.Errorf("medians %v", a.Medians)
	}
	// tokens/solved: 4 tasks x 1500 = 6000 over 11 passes.
	if a.PerSolved.Tokens == nil || math.Abs(*a.PerSolved.Tokens-6000.0/11) > 1e-6 || a.PerSolved.USD == nil || math.Abs(*a.PerSolved.USD-6.0/11) > 1e-6 {
		t.Errorf("per solved %+v", a.PerSolved)
	}
	// false-done: 20 claimed, 9 failed.
	if a.FalseDone == nil || math.Abs(*a.FalseDone-0.45) > 1e-9 {
		t.Errorf("false done %v", a.FalseDone)
	}
	if ut := r.Instability["unstable_tasks"].([]string); len(ut) != 2 || ut[0] != "b" || ut[1] != "d" {
		t.Errorf("unstable %v", ut)
	}
	broken := false
	for _, n := range r.Negative {
		if n["kind"] == "possibly_broken_task" && n["task"] == "c" {
			broken = true
		}
	}
	if !broken || len(r.Negative) == 0 {
		t.Errorf("negative section %v", r.Negative)
	}
	if err := r.Validate(); err != nil {
		t.Errorf("report schema: %v", err)
	}
	j1, _ := r.JSON()
	r2 := Build(m, "sha256:"+strings.Repeat("1", 64), rows, "runs/x")
	j2, _ := r2.JSON()
	if !bytes.Equal(j1, j2) || r.Markdown() != r2.Markdown() {
		t.Error("report not deterministic")
	}
	md := r.Markdown()
	for _, h := range []string{"## 1. Header", "## 2. Setup", "## 3. Primary outcome", "## 4. Secondary outcomes", "## 5. Variance", "## 6. Negative results", "## 7. Exploratory", "## 8. Cheating and scope scan", "## 9. Threats", "## 10. Reproduce"} {
		if !strings.Contains(md, h) {
			t.Errorf("markdown lacks %s", h)
		}
	}
	if strings.Contains(md, "\u2014") {
		t.Error("em dash in report")
	}
}

func TestPerSolvedUndefined(t *testing.T) {
	rows := fixture("A", map[string][]bool{"a": {false, false}})
	r := Build(manifest(2, "A"), "sha256:"+strings.Repeat("2", 64), rows, "runs/y")
	if r.Arms["A"].PerSolved.Tokens != nil || !strings.Contains(r.Markdown(), "infinity") {
		t.Error("undefined per-solved must be null and print as infinity")
	}
}

func TestCompareKnownDeltas(t *testing.T) {
	a := &Archive{Dir: "runs/a", Manifest: manifest(5, "A"), Hash: "sha256:" + strings.Repeat("a", 64),
		Rows: fixture("A", map[string][]bool{"t1": {true, false, false, false, false}, "t2": {false, false, false, false, false}, "t3": {true, true, false, false, false}, "t4": {true, false, false, false, false}, "t5": {false, false, false, false, false}, "t6": {true, true, true, false, false}})}
	b := &Archive{Dir: "runs/b", Manifest: manifest(5, "B"), Hash: "sha256:" + strings.Repeat("b", 64),
		Rows: fixture("B", map[string][]bool{"t1": {true, true, true, true, true}, "t2": {true, true, true, false, false}, "t3": {true, true, true, true, false}, "t4": {true, true, true, false, false}, "t5": {false, false, false, false, false}, "t6": {true, true, true, true, true}})}
	r, err := Compare(a, b, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	// A rates: .2 0 .4 .2 0 .6 -> mean 0.2333; B: 1 .6 .8 .6 0 1 -> mean 0.6667; delta 0.4333.
	p := r.Primary
	if p == nil || p.Metric != "pass_at_1" || math.Abs(*p.Delta-(0.666667-0.233333)) > 1e-5 {
		t.Fatalf("primary %+v", p)
	}
	// Differences: .8 .6 .4 .4 0 .4 -> n=5 (zero dropped), all positive: W=15, p=2/32.
	if p.Wilcoxon.N != 5 || p.Wilcoxon.W != 15 || math.Abs(p.Wilcoxon.P-0.0625) > 1e-12 {
		t.Errorf("wilcoxon %+v", p.Wilcoxon)
	}
	if p.CI95[0] == nil || *p.CI95[0] <= 0 || p.Supported == nil || !*p.Supported {
		t.Errorf("ci %v supported %v", p.CI95, p.Supported)
	}
	// Cost medians equal in both arms: delta 0, CI includes 0, listed as null.
	var costNull, brokenT5 bool
	for _, n := range r.Negative {
		if n["kind"] == "null" && n["metric"] == "cost_median" {
			costNull = true
		}
		if n["kind"] == "possibly_broken_task" && n["task"] == "t5" {
			brokenT5 = true
		}
	}
	if !costNull || !brokenT5 {
		t.Errorf("negative %v", r.Negative)
	}
	for _, c := range r.Secondary {
		if c.Metric == "cost_median" && (c.Equivalence == nil || c.Equivalence["ci_inside"] != true) {
			t.Errorf("equivalence on cost: %v", c.Equivalence)
		}
	}
	if err := r.Validate(); err != nil {
		t.Errorf("schema: %v", err)
	}
	j1, _ := r.JSON()
	r2, _ := Compare(a, b, 0.05)
	j2, _ := r2.JSON()
	if !bytes.Equal(j1, j2) {
		t.Error("compare not deterministic")
	}
	// Unpaired task sets are a usage error.
	c := &Archive{Dir: "runs/c", Manifest: manifest(5, "C"), Hash: b.Hash, Rows: fixture("C", map[string][]bool{"t1": {true, true, true, true, true}})}
	if _, err := Compare(a, c, 0); cli.CodeOf(err) != cli.ExitUsage {
		t.Errorf("unpaired: %v", err)
	}
}
