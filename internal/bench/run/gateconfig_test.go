package run

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// TestStagedGateArmReadsItsConfig (2026-09-06 dev run finding 1): the
// gate reads config through `git show <base>:.saga/config.toml`, and
// .saga is gitignored, so until this fix arm B ran with the gate's own
// defaults in every run: require_red on, mode "minimal", and three runs
// held at a block they could not clear. The proof is the Stop step's own
// mode, taken from a really staged workspace.
func TestStagedGateArmReadsItsConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tk := loadTasks(t, "ts-0001-slug-collapse")[0]
	c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: fakeSaga(t)}
	ws, cfg, prep := prepArm(t, tk, c, []string{"gate"})

	// The disclosure says the config is at the base commit and names it.
	present, _ := prep.Disclosure["gate_config_present"].(bool)
	if !present {
		t.Fatalf("gate_config_present false: %v", prep.Disclosure["gate_config_sha256_reason"])
	}
	sha, _ := prep.Disclosure["gate_config_sha256"].(string)
	if !strings.HasPrefix(sha, "sha256:") {
		t.Errorf("gate_config_sha256 %q", sha)
	}
	_ = cfg

	// And the gate agrees: it reads the config, so the mode is not
	// "minimal" and require_red is off.
	st := store.Open(ws)
	l, err := gate.Load(ws, st)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !l.Config.Present {
		t.Fatal("the gate read no config at the base commit")
	}
	if l.Config.RequireRedOn() {
		t.Error("arm B ran with require_red on; docs/12 section 4 stages it off")
	}
	if l.Mode == "minimal" {
		t.Errorf("mode %q: minimal is the no-config fallback", l.Mode)
	}

	// The Stop step's own trace event carries the same mode, which is the
	// field the dev run's archive showed as "minimal" on all three
	// timed-out runs.
	obs, err := st.ReadObserved("gate-cfg-1")
	if err != nil {
		t.Fatal(err)
	}
	obs.MaskSalt = "salt"
	if err := st.WriteObserved(obs); err != nil {
		t.Fatal(err)
	}
	layer := &gate.Layer{Store: st}
	if _, err := layer.Run(context.Background(), &hookio.Input{
		Harness: "claude-code", Event: hookio.EventStop, SessionID: "gate-cfg-1",
	}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	mode := stopEventMode(t, st, "gate-cfg-1")
	if mode == "minimal" || mode == "" {
		t.Errorf("the Stop trace event reads mode %q, the fallback the dev run recorded", mode)
	}
}

// stopEventMode reads the mode of the last gate stop event.
func stopEventMode(t *testing.T, st *store.Store, session string) string {
	t.Helper()
	segs, err := trace.Segments(trace.SessionDir(st, session))
	if err != nil {
		t.Fatal(err)
	}
	mode := ""
	for _, seg := range segs {
		raw, err := os.ReadFile(seg)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var ev struct {
				Type string         `json:"type"`
				Body map[string]any `json:"body"`
			}
			if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "gate" {
				continue
			}
			if ev.Body["kind"] == "stop" {
				if m, ok := ev.Body["mode"].(string); ok {
					mode = m
				}
			}
		}
	}
	return mode
}

// TestStagedStoreStaysOutOfTheGradedDiff: the config and contract now
// live in the base commit, so the diff the oracle grades must still
// carry no .saga path. If it ever did, arm B would be graded on files
// the bench put there.
func TestStagedStoreStaysOutOfTheGradedDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tk := loadTasks(t, "ts-0001-slug-collapse")[0]
	c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: fakeSaga(t)}
	ws, _, _ := prepArm(t, tk, c, []string{"gate"})

	// An edit of the kind an agent makes, plus a write under .saga of the
	// kind the gate makes as it runs.
	target := filepath.Join(ws, "saga-probe-edit.txt")
	if err := os.WriteFile(target, []byte("an agent's edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".saga", "config.toml"), []byte("[gate]\nrequire_red = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := task.Diff(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diff), "saga-probe-edit.txt") {
		t.Errorf("the diff lost the agent's edit:\n%s", diff)
	}
	for _, line := range strings.Split(string(diff), "\n") {
		if strings.Contains(line, ".saga/") {
			t.Errorf("the graded diff carries a store path: %q", line)
		}
	}
}

// unstagedGate is a replay adapter that reports what a gate arm looks
// like when its config never reached the base commit.
type unstagedGate struct{ *adapter.Replay }

func (u unstagedGate) Prepare(ctx context.Context, in *adapter.PrepareInput) (*adapter.PrepareOutput, error) {
	out, err := u.Replay.Prepare(ctx, in)
	if err != nil {
		return out, err
	}
	out.Disclosure["gate_config_present"] = false
	out.Disclosure["gate_config_sha256"] = nil
	out.Disclosure["gate_config_sha256_reason"] = "no .saga/config.toml at the base commit; the gate would run on its defaults"
	return out, nil
}

// TestGateArmWithoutItsConfigIsInfra: a gate arm that would run on the
// gate's defaults is not the treatment the manifest names, so grading it
// would report a comparison nobody ran. It is excluded as infra with the
// reason said out loud, which is what would have caught the dev run's
// finding 1 the first time rather than three smokes later.
func TestGateArmWithoutItsConfigIsInfra(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tasks := loadTasks(t, "ts-0001-slug-collapse")
	res, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: unstagedGate{&adapter.Replay{Patch: "gold"}}, K: 1,
		Out: filepath.Join(t.TempDir(), "runs"), Seed: strings.Repeat("cd", 32),
		Components: []string{"gate"}, Arm: "B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("%d rows", len(res.Rows))
	}
	row := res.Rows[0]
	if row.Outcome != "infra" {
		t.Errorf("outcome %q, want infra", row.Outcome)
	}
	if row.OutcomeReason == nil || !strings.Contains(*row.OutcomeReason, "gate config not at base") {
		t.Errorf("reason %q", deref(row.OutcomeReason))
	}
	// A bare arm carries no such key and is unaffected.
	res, err = Run(context.Background(), Options{
		Tasks: tasks, Adapter: &adapter.Replay{Patch: "gold"}, K: 1,
		Out: filepath.Join(t.TempDir(), "runs-a"), Seed: strings.Repeat("cd", 32), Arm: "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0].Outcome == "infra" {
		t.Errorf("a bare arm was excluded: %v", res.Rows[0].OutcomeReason)
	}
}
