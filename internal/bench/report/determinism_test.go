package report

import (
	ctx "context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/bench/task"
)

// regenerate builds the report twice from the same rows and returns both
// renderings of each file.
func regenerate(t *testing.T, dir string) (jsonA, jsonB, mdA, mdB string) {
	t.Helper()
	arch, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	build := func() (string, string) {
		rep := Build(arch.Manifest, arch.Hash, arch.Rows, dir)
		if err := rep.Validate(); err != nil {
			t.Fatalf("report schema: %v", err)
		}
		js, err := rep.JSON()
		if err != nil {
			t.Fatal(err)
		}
		return string(js), rep.Markdown()
	}
	jsonA, mdA = build()
	jsonB, mdB = build()
	return
}

// firstDiff names the first differing line, which is far more useful
// than a diff of two multi-kilobyte documents.
func firstDiff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la) && i < len(lb); i++ {
		if la[i] != lb[i] {
			return "line " + itoa(i+1) + ":\n  first:  " + la[i] + "\n  second: " + lb[i]
		}
	}
	if len(la) != len(lb) {
		return "different line counts: " + itoa(len(la)) + " and " + itoa(len(lb))
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestReportRegenerationIsByteIdentical (docs/12 section 8): report.json
// and report.md regenerated from rows.jsonl must be byte-identical. A
// report that differs run to run cannot be checked by a reader, and a
// map iteration or a wall-clock stamp is exactly the kind of thing that
// slips in unnoticed. `created` comes from the manifest and is
// deliberately part of the output.
func TestReportRegenerationIsByteIdentical(t *testing.T) {
	root := filepath.Join("..", "..", "..", "bench", "results", "smoke-2026-09-06-2")
	for _, arm := range []string{"A", "B"} {
		dir := filepath.Join(root, arm)
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
			t.Skipf("archive %s not present: %v", dir, err)
		}
		ja, jb, ma, mb := regenerate(t, dir)
		if ja != jb {
			t.Errorf("arm %s: report.json is not deterministic: %s", arm, firstDiff(ja, jb))
		}
		if ma != mb {
			t.Errorf("arm %s: report.md is not deterministic: %s", arm, firstDiff(ma, mb))
		}
	}
}

// TestReportDeterminismOnTheFrozenSet runs the whole pipeline over the
// frozen 40-task corpus with the replay adapter, both arms, K=1, and
// regenerates each arm's report twice. It exercises what the smoke
// archives cannot: the runner, the grader and the scan on every task of
// the set a pre-registration would name.
func TestReportDeterminismOnTheFrozenSet(t *testing.T) {
	if testing.Short() {
		t.Skip("short: this stages and grades 80 runs")
	}
	tasks := corpusTasks(t)
	if len(tasks) == 0 {
		t.Skip("no corpus")
	}
	out := t.TempDir()
	start := time.Now()
	results, err := run.RunArms(ctx.Background(), run.Options{
		Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold"}, K: 1, Out: out, Tier: "smoke",
		Seed: strings.Repeat("ab", 32), Verify: false, WallCapS: 120,
	}, []run.ArmSpec{{ID: "A"}, {ID: "B", Components: []string{"gate"}}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Logf("%d tasks, 2 arms, K=1: %d + %d runs in %s", len(tasks), len(results[0].Rows), len(results[1].Rows), time.Since(start).Round(time.Millisecond))
	for _, arm := range []string{"A", "B"} {
		dir := filepath.Join(out, arm)
		ja, jb, ma, mb := regenerate(t, dir)
		if ja != jb {
			t.Errorf("arm %s: report.json is not deterministic: %s", arm, firstDiff(ja, jb))
		}
		if ma != mb {
			t.Errorf("arm %s: report.md is not deterministic: %s", arm, firstDiff(ma, mb))
		}
	}
}

// corpusTasks loads the frozen corpus, skipping when it is not present.
func corpusTasks(t *testing.T) []*task.Task {
	t.Helper()
	dirs, err := task.Find(filepath.Join("..", "..", "..", "bench", "tasks", "*"))
	if err != nil || len(dirs) == 0 {
		return nil
	}
	var out []*task.Task
	for _, d := range dirs {
		tk, err := task.Load(d)
		if err != nil {
			t.Fatalf("load %s: %v", d, err)
		}
		out = append(out, tk)
	}
	return out
}
