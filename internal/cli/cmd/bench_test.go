package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/cli"
)

func TestBenchCLIEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	for _, bin := range []string{"git", "bash", "node", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	tasks := filepath.Join(repo, "bench", "tasks", "ts-000[15]-*")
	root := t.TempDir()
	seed := strings.Repeat("ef", 32)
	outA := filepath.Join(root, "a")
	out, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--replay", "gold", "--k", "2", "--out", outA, "--arm", "A", "--seed", seed, "--tier", "smoke", "--no-verify")
	if code != cli.ExitOK {
		t.Fatalf("run A: %v\n%s\n%s", code, out, errs)
	}
	outB := filepath.Join(root, "b")
	if _, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--replay", "gold,broken-1", "--k", "2", "--out", outB, "--arm", "B", "--seed", seed, "--tier", "smoke", "--no-verify"); code != cli.ExitOK {
		t.Fatalf("run B: %v %s", code, errs)
	}
	for _, f := range []string{"manifest.json", "rows.jsonl", "report.json", "report.md", "exclusions.jsonl", "abstain.txt"} {
		if _, err := os.Stat(filepath.Join(outA, f)); err != nil {
			t.Errorf("archive lacks %s", f)
		}
	}
	md, _, code := runIn(t, repo, "", "bench", "report", outA)
	if code != cli.ExitOK || !strings.Contains(md, "## 6. Negative results") || !strings.Contains(md, "| A | 1.000 |") {
		t.Errorf("report: %v\n%s", code, md)
	}
	before, _ := os.ReadFile(filepath.Join(outA, "report.json"))
	runIn(t, repo, "", "bench", "report", outA)
	after, _ := os.ReadFile(filepath.Join(outA, "report.json"))
	if string(before) != string(after) {
		t.Error("report.json not byte-identical across runs")
	}
	cmp, errs, code := runIn(t, repo, "", "bench", "compare", outA, outB)
	if code != cli.ExitOK {
		t.Fatalf("compare: %v %s", code, errs)
	}
	// A: gold on every run (rate 1); B: gold then broken-1 (rate 0.5): delta -0.5.
	if !strings.Contains(cmp, "delta=-0.500") || !strings.Contains(cmp, "Wilcoxon n=2") {
		t.Errorf("compare output:\n%s", cmp)
	}
	if _, err := os.Stat(filepath.Join(outB, "compare.json")); err != nil {
		t.Error("compare.json not written")
	}
	// Refusals: over budget is exit 3, unknown adapter is exit 2.
	if _, _, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--k", "5", "--out", filepath.Join(root, "c"), "--budget", "0.01", "--no-verify"); code != cli.ExitRefusal {
		t.Errorf("budget refusal: %v", code)
	}
	if _, _, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "nope", "--out", filepath.Join(root, "d")); code != cli.ExitUsage {
		t.Errorf("unknown adapter: %v", code)
	}
}
