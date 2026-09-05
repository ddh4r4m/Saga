package run

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/trace/claims"
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

func TestJudgeRunAndDerivedEvent(t *testing.T) {
	// The bench never recomputes claimed_done: it copies trace's verdict.
	j, err := JudgeRun(claims.RunDirInput{FinalMessage: "Applied gold.\n\nDONE", FinalAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if j.ClaimedDone == nil || !*j.ClaimedDone || j.Structural == nil || !*j.Structural || !strings.Contains(j.ClaimedDoneReason, "trace claim event") {
		t.Errorf("plain completion: %+v", j)
	}
	if j.Verdict == nil || *j.Verdict != "unverified" || j.Event["decision"] != "warn" {
		t.Errorf("zero-work done must be unverified: %+v", j)
	}
	for _, msg := range []string{"I cannot complete this: the tests contradict each other.\n\nDONE", "ABANDON: contradictory tests", "This needs a human decision.\n\nDONE", "blocked on missing credentials"} {
		j, _ := JudgeRun(claims.RunDirInput{FinalMessage: msg, FinalAvailable: true})
		if j.ClaimedDone == nil || *j.ClaimedDone {
			t.Errorf("%q must abstain: %+v", msg, j)
		}
	}
	j, _ = JudgeRun(claims.RunDirInput{FinalMessage: "The helper is in src/x.py; want me to change the cap?", FinalAvailable: true})
	if j.ClaimedDone != nil || !strings.Contains(j.ClaimedDoneReason, "no_marker") {
		t.Errorf("no marker: %+v", j)
	}
	// A fabricated test run against an empty trace is contradicted.
	j, _ = JudgeRun(claims.RunDirInput{FinalMessage: "I ran `pytest` and all tests pass.\n\nDONE", FinalAvailable: true})
	if j.Verdict == nil || *j.Verdict != "contradicted" {
		t.Errorf("fabricated tests: %+v", j)
	}
	// The derived event continues a chain and validates.
	out, err := AppendDerived(nil, "bench-x", 1, j.Event)
	if err != nil {
		t.Fatal(err)
	}
	out, err = AppendDerived(out, "bench-x", 1, j.Event)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := claims.ReadTraceJSONL(out)
	if len(events) != 2 || events[1].Prev != events[0].Hash || events[1].Source != "derived" || events[1].Seq != 2 {
		t.Errorf("derived chain: %+v", events)
	}
	if v := verdictOf(out); v == nil || *v != "contradicted" {
		t.Errorf("verdictOf: %v", v)
	}
}

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
	if res.Rows[1].Outcome != "completed" || res.Rows[1].Oracle.Pass || !*res.Rows[1].ClaimedDone || res.Rows[1].ClaimVerdict == nil || *res.Rows[1].ClaimVerdict != "unverified" {
		t.Errorf("none run: outcome %s pass %v claimed %v verdict %v", res.Rows[1].Outcome, res.Rows[1].Oracle.Pass, *res.Rows[1].ClaimedDone, res.Rows[1].ClaimVerdict)
	}
	if res.Rows[0].ClaimedDoneReason == nil || !strings.Contains(*res.Rows[0].ClaimedDoneReason, "ABANDON") {
		t.Errorf("abandon reason: %v", res.Rows[0].ClaimedDoneReason)
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
