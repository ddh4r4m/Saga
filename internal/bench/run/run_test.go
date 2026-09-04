package run

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/trace"
)

func needRunners(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"git", "bash", "node", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

func loadTasks(t *testing.T, ids ...string) []*task.Task {
	t.Helper()
	var out []*task.Task
	for _, id := range ids {
		tk, err := task.Load(filepath.Join("..", "..", "..", "bench", "tasks", id))
		if err != nil {
			t.Skipf("corpus task %s: %v", id, err)
		}
		out = append(out, tk)
	}
	return out
}

func TestSeeds(t *testing.T) {
	seed := strings.Repeat("0a", 32)
	a, b := Seeds(seed, "ts-0001", 1), Seeds(seed, "ts-0001", 2)
	if len(a) != 64 || a == b || a != Seeds(seed, "ts-0001", 1) || a == Seeds(seed, "ts-0002", 1) {
		t.Errorf("seeds %s %s", a, b)
	}
}

func TestClaimedDone(t *testing.T) {
	if ok, _ := ClaimedDone("completed", "Applied gold. Done."); !ok {
		t.Error("plain completion must claim done")
	}
	for _, msg := range []string{"I cannot complete this: the tests contradict each other.", "ABANDON: contradictory tests", "This needs a human decision.", "blocked on missing credentials"} {
		if ok, _ := ClaimedDone("completed", msg); ok {
			t.Errorf("%q must abstain", msg)
		}
	}
	if ok, _ := ClaimedDone("abandon", "anything"); ok {
		t.Error("abandon terminal must not claim done")
	}
}

func TestRepeats(t *testing.T) {
	c := func(tool, h string) adapter.ToolCall { return adapter.ToolCall{Tool: tool, ArgsHash: h} }
	if repeats([]adapter.ToolCall{c("a", "1"), c("a", "1"), c("a", "1"), c("a", "1"), c("b", "1"), c("b", "1")}) != 1 {
		t.Error("one repeat streak expected")
	}
}

// TestRunReplayEndToEnd runs the replay adapter over two tasks with K=3
// alternating gold and broken-1, so the cell has c=2 of n=3 passes on
// each task, and checks the archive, the manifest and every row.
func TestRunReplayEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tasks := loadTasks(t, "ts-0001-slug-collapse", "py-0006-contact-dedupe")
	out := filepath.Join(t.TempDir(), "runs")
	res, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold,broken-1"}, K: 3, Out: out, Arm: "A", Tier: "smoke",
		Seed: strings.Repeat("ab", 32), Model: "replay", Prices: trace.DefaultPrices(), Version: "test",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Rows) != 6 {
		t.Fatalf("%d rows", len(res.Rows))
	}
	passes := map[string]int{}
	for _, r := range res.Rows {
		if r.Schema != RowSchema || r.Manifest != res.ManifestHash || r.Harness != "replay" || r.Arm != "A" {
			t.Errorf("row header %+v", r)
		}
		if err := r.Validate(); err != nil {
			t.Errorf("row %s/%d invalid: %v", r.Task, r.I, err)
		}
		if r.Outcome != "completed" {
			t.Errorf("row %s/%d outcome %s (%v)", r.Task, r.I, r.Outcome, deref(r.OutcomeReason))
		}
		wantPass := r.I%2 == 1
		if r.Oracle.Pass != wantPass {
			t.Errorf("row %s/%d pass %v, want %v (exit %d, apply %v)", r.Task, r.I, r.Oracle.Pass, wantPass, r.Oracle.Exit, deref(r.Oracle.ApplyError))
		}
		if r.Oracle.Pass {
			passes[r.Task]++
		}
		if r.ClaimedDone == nil || !*r.ClaimedDone {
			t.Errorf("row %s/%d claimed_done %v", r.Task, r.I, r.ClaimedDone)
		}
		if r.CostUSD != nil || r.CostUSDReason == nil {
			t.Errorf("replay must be unpriced: %v %v", r.CostUSD, r.CostUSDReason)
		}
		if r.Scan.Flagged {
			t.Errorf("gold/broken flagged: %+v", r.Scan)
		}
		if len(r.Scan.ScopeViolations) != 0 {
			t.Errorf("scope violations on a control: %v", r.Scan.ScopeViolations)
		}
		dir := filepath.Join(out, r.Task, "replay", "replay", "A", strings.TrimLeft(strings.Repeat(" ", 0)+string(rune('0'+r.I)), " "))
		for _, name := range []string{"run.json", "trace.jsonl", "harness.json", "workspace.diff", "oracle.txt", "scan.json", "final_message.txt", "SHA256SUMS"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("archive missing %s/%s", dir, name)
			}
		}
		if tr, _ := os.ReadFile(filepath.Join(dir, "trace.jsonl")); !strings.Contains(string(tr), `"type":"tool_call"`) {
			t.Errorf("replay trace has no tool_call event: %q", tr)
		}
		if h := r.Artifacts["diff"]; !strings.HasPrefix(h, "sha256:") {
			t.Errorf("artifact hash %v", r.Artifacts)
		}
	}
	for _, tk := range tasks {
		if passes[tk.ID] != 2 {
			t.Errorf("%s: %d passes, want 2", tk.ID, passes[tk.ID])
		}
	}
	m, hash, rows, err := ReadArchive(out)
	if err != nil {
		t.Fatal(err)
	}
	if hash != res.ManifestHash || m.K != 3 || len(rows) != 6 || m.TaskSet.Tasks[0].ID != "ts-0001-slug-collapse" || m.AbstainListSHA256 != AbstainHash {
		t.Errorf("archive: hash %s want %s, k %d, rows %d", hash, res.ManifestHash, m.K, len(rows))
	}
	for _, r := range rows {
		if r.Seed != Seeds(m.RunSeed, r.Task, r.I) {
			t.Errorf("seed mismatch on %s/%d", r.Task, r.I)
		}
	}
}

func TestRunRefusals(t *testing.T) {
	tasks := loadTasks(t, "ts-0001-slug-collapse")
	_, err := Run(context.Background(), Options{Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold"}, K: 5, Out: t.TempDir(), Budget: 0.10})
	if cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("over-budget run: %v", err)
	}
	_, err = Run(context.Background(), Options{Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold"}, K: 0, Out: t.TempDir()})
	if cli.CodeOf(err) != cli.ExitUsage {
		t.Errorf("k=0: %v", err)
	}
	_, err = Run(context.Background(), Options{Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold"}, K: 1, Out: t.TempDir(), Seed: "zz"})
	if cli.CodeOf(err) != cli.ExitUsage {
		t.Errorf("bad seed: %v", err)
	}
}

// TestRunImpossibleAbandon: on the impossible task the replay adapter's
// abandon terminal passes and a completion with no patch is a false
// done.
func TestRunImpossibleAbandon(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tasks := loadTasks(t, "py-0007-version-sort-impossible")
	res, err := Run(context.Background(), Options{Tasks: tasks, Adapter: &adapter.Replay{Patch: "abandon,none"}, K: 2, Out: filepath.Join(t.TempDir(), "runs"), Seed: strings.Repeat("cd", 32)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0].Outcome != "abandon" || !res.Rows[0].Oracle.Pass || *res.Rows[0].ClaimedDone {
		t.Errorf("abandon run: %+v", res.Rows[0])
	}
	if res.Rows[1].Outcome != "completed" || res.Rows[1].Oracle.Pass || !*res.Rows[1].ClaimedDone {
		t.Errorf("none run: outcome %s pass %v claimed %v", res.Rows[1].Outcome, res.Rows[1].Oracle.Pass, *res.Rows[1].ClaimedDone)
	}
}

func TestParseArm(t *testing.T) {
	for _, tc := range []struct {
		in   string
		id   string
		comp string
		bad  bool
	}{
		{"A", "A", "", false}, {"A:bare", "A", "", false}, {"B:gate", "B", "gate", false},
		{"B:gate,guard", "", "", true}, {":gate", "", "", true}, {"a/b", "", "", true},
	} {
		a, err := ParseArm(tc.in)
		if tc.bad != (err != nil) || (!tc.bad && (a.ID != tc.id || strings.Join(a.Components, ",") != tc.comp)) {
			t.Errorf("%q: %+v %v", tc.in, a, err)
		}
	}
}

func TestRunArmsInterleaved(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tasks := loadTasks(t, "ts-0001-slug-collapse")
	var log bytes.Buffer
	out := filepath.Join(t.TempDir(), "runs")
	results, err := RunArms(context.Background(), Options{Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold"}, K: 2, Out: out, Seed: strings.Repeat("ab", 32), Log: &log, WallCapS: 300}, []ArmSpec{{ID: "A"}, {ID: "B", Components: []string{"gate"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || len(results[0].Rows) != 2 || len(results[1].Rows) != 2 {
		t.Fatalf("results: %d", len(results))
	}
	// Interleaved per task and index: A1, B1, A2, B2 (bench-spec 3.2).
	var order []string
	for _, l := range strings.Split(log.String(), "\n") {
		if strings.Contains(l, " arm ") {
			f := strings.Fields(l)
			order = append(order, f[2]+f[4])
		}
	}
	if strings.Join(order, " ") != "A1/2: B1/2: A2/2: B2/2:" {
		t.Errorf("order %v", order)
	}
	// Same seed_i across arms; separate archives naming their arm.
	for i := range results[0].Rows {
		if results[0].Rows[i].Seed != results[1].Rows[i].Seed || results[0].Rows[i].Arm != "A" || results[1].Rows[i].Arm != "B" {
			t.Errorf("row %d: %s/%s %s/%s", i, results[0].Rows[i].Arm, results[0].Rows[i].Seed[:8], results[1].Rows[i].Arm, results[1].Rows[i].Seed[:8])
		}
	}
	mA, _, _, err := ReadArchive(filepath.Join(out, "A"))
	if err != nil {
		t.Fatal(err)
	}
	mB, _, _, err := ReadArchive(filepath.Join(out, "B"))
	if err != nil {
		t.Fatal(err)
	}
	if mA.RunSeed != mB.RunSeed || mA.Arms[0].ID != "A" || len(mA.Arms[0].BlocksInControl) != 1 || mB.Arms[0].Components[0] != "gate" || len(mB.Arms[0].BlocksInControl) != 0 {
		t.Errorf("manifests: %+v %+v", mA.Arms, mB.Arms)
	}
	if got := results[0].Rows[0].Artifacts; len(got) == 0 {
		t.Error("no artifacts")
	}
}
