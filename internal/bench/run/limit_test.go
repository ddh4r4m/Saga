package run

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/trace"
)

// The line the harness returned 132 times in the pilot of 2026-09-13,
// verbatim from fixtures/harness/session-limit.txt.
const limitLine = "You've hit your session limit · resets 4:40am (Asia/Calcutta)"

// limitOnce answers the first collect for each (task, k) with the
// harness limit reply and every later one normally, which is the shape
// a wait is meant to survive: the window reopens and the same row runs.
type limitOnce struct {
	*adapter.Replay
	mu    sync.Mutex
	fired map[string]bool
	// always keeps answering with the limit, for the second-refusal path.
	always bool
	// line overrides the reply, for the unreadable-reset path.
	line string
}

func (l *limitOnce) Collect(ctx context.Context, in *adapter.CollectInput) (*adapter.CollectOutput, error) {
	out, err := l.Replay.Collect(ctx, in)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	key := in.Task.ID
	if !l.always && l.fired[key] {
		return out, nil
	}
	l.fired[key] = true
	line := limitLine
	if l.line != "" {
		line = l.line
	}
	out.Outcome = "infra"
	out.OutcomeReason = "harness-limit: " + line
	out.Limit = line
	out.FinalMessage = line
	out.Turns, out.ToolCalls, out.ToolSequence = 1, 0, nil
	return out, nil
}

// stubClock freezes the wait so a reset four hours away costs the test
// nothing, and records what the runner asked to sleep until.
func stubClock(t *testing.T, now time.Time) *[]time.Time {
	t.Helper()
	var slept []time.Time
	oldNow, oldSleep := nowFn, sleepUntilFn
	nowFn = func() time.Time { return now }
	sleepUntilFn = func(ctx context.Context, at time.Time) error {
		slept = append(slept, at)
		return nil
	}
	t.Cleanup(func() { nowFn, sleepUntilFn = oldNow, oldSleep })
	return &slept
}

func istOrSkip(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Calcutta")
	if err != nil {
		t.Skip("no tzdata")
	}
	return loc
}

// TestOnLimitWaitResumesTheRow: the account's session window ending is
// not an outcome of the run. The runner keeps the abandoned attempt,
// sleeps until the announced reset plus the grace, runs the same row
// again, and records the interruption in the manifest. Without this the
// pilot of 2026-09-13 graded 132 such replies as completed failures.
func TestOnLimitWaitResumesTheRow(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	loc := istOrSkip(t)
	// The pilot's own clock: hit at 03:11:53 IST, reset announced 4:40am.
	now := time.Date(2026, 9, 13, 3, 11, 53, 0, loc)
	slept := stubClock(t, now)

	tasks := loadTasks(t, "ts-0001-slug-collapse")
	out := filepath.Join(t.TempDir(), "runs")
	res, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: &limitOnce{Replay: &adapter.Replay{Patch: "gold"}, fired: map[string]bool{}},
		K: 1, Out: out, Seed: strings.Repeat("ab", 32),
	})
	if err != nil {
		t.Fatalf("the run did not survive a harness limit: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("%d rows, want 1", len(res.Rows))
	}
	// The row is the retry, not the refusal.
	if res.Rows[0].Outcome == "infra" {
		t.Errorf("the row is still the limit reply: %v", deref(res.Rows[0].OutcomeReason))
	}
	// It slept until the announced reset plus the grace, not for a
	// guessed interval.
	if len(*slept) != 1 {
		t.Fatalf("slept %d times, want 1", len(*slept))
	}
	want := time.Date(2026, 9, 13, 4, 40, 0, 0, loc).Add(adapter.LimitResetGrace)
	if !(*slept)[0].Equal(want) {
		t.Errorf("slept until %s, want %s", (*slept)[0], want)
	}
	// The manifest says the run was interrupted and resumed.
	if len(res.Manifest.LimitWaits) != 1 {
		t.Fatalf("manifest records %d limit waits, want 1", len(res.Manifest.LimitWaits))
	}
	w := res.Manifest.LimitWaits[0]
	if w.Task != tasks[0].ID || w.K != 1 || w.Message != limitLine {
		t.Errorf("limit wait %+v", w)
	}
	// The abandoned attempt is kept whole beside the row that replaced
	// it: on the pilot's transition row it held 12 turns of real work.
	att := filepath.Join(out, tasks[0].ID, "default", "replay", "A", "1", "attempt-1")
	if _, err := os.Stat(filepath.Join(att, "run.json")); err != nil {
		t.Errorf("attempt-1 was not kept: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(att, "run.json")); err == nil {
		var r Row
		json.Unmarshal(raw, &r)
		if r.Outcome != "infra" {
			t.Errorf("attempt-1 is not the refusal: %s", r.Outcome)
		}
	}
}

// TestOnLimitStopEndsTheRun: `--on-limit stop` does not wait, and the
// refusal names the row and says how many were not run.
func TestOnLimitStopEndsTheRun(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	loc := istOrSkip(t)
	slept := stubClock(t, time.Date(2026, 9, 13, 3, 11, 53, 0, loc))

	tasks := loadTasks(t, "ts-0001-slug-collapse", "ts-0005-retry-backoff")
	_, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: &limitOnce{Replay: &adapter.Replay{Patch: "gold"}, fired: map[string]bool{}},
		K: 1, Out: filepath.Join(t.TempDir(), "runs"), Seed: strings.Repeat("ab", 32),
		OnLimit: adapter.OnLimitStop,
	})
	if err == nil {
		t.Fatal("--on-limit stop did not stop the run")
	}
	if code := cli.CodeOf(err); code != cli.ExitRefusal {
		t.Errorf("exit %v, want %v: %v", code, cli.ExitRefusal, err)
	}
	for _, want := range []string{tasks[0].ID, "harness limit", "not run"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	if len(*slept) != 0 {
		t.Errorf("stop slept %d times", len(*slept))
	}
}

// TestOnLimitSecondRefusalStops: one wait per row. A window that is
// still closed after the announced reset ends the run rather than
// looping, and the archive keeps what did run.
func TestOnLimitSecondRefusalStops(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	loc := istOrSkip(t)
	slept := stubClock(t, time.Date(2026, 9, 13, 3, 11, 53, 0, loc))

	tasks := loadTasks(t, "ts-0001-slug-collapse")
	out := filepath.Join(t.TempDir(), "runs")
	res, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: &limitOnce{Replay: &adapter.Replay{Patch: "gold"}, fired: map[string]bool{}, always: true},
		K: 2, Out: out, Seed: strings.Repeat("ab", 32),
	})
	if err == nil {
		t.Fatal("a window still closed after the reset did not stop the run")
	}
	if !strings.Contains(err.Error(), "again") {
		t.Errorf("the refusal does not say the limit held: %v", err)
	}
	if len(*slept) != 1 {
		t.Errorf("slept %d times, want exactly one wait per row", len(*slept))
	}
	if len(res.Rows) == 0 {
		t.Error("the archive kept no rows")
	}
}

// TestOnLimitWithoutAReadableResetTakesTheShortWait: a reply that names
// no time this machine can read is still a limit, and the runner waits a
// fixed short interval rather than guessing at a schedule.
func TestOnLimitWithoutAReadableResetTakesTheShortWait(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	now := time.Date(2026, 9, 13, 3, 11, 53, 0, time.UTC)
	slept := stubClock(t, now)

	tasks := loadTasks(t, "ts-0001-slug-collapse")
	a := &limitOnce{Replay: &adapter.Replay{Patch: "gold"}, fired: map[string]bool{}, line: limitNoReset}
	if _, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: a, K: 1, Out: filepath.Join(t.TempDir(), "runs"),
		Seed: strings.Repeat("ab", 32),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(*slept) != 1 {
		t.Fatalf("slept %d times, want 1", len(*slept))
	}
	if want := now.Add(adapter.LimitRetryDelay); !(*slept)[0].Equal(want) {
		t.Errorf("slept until %s, want the short delay %s", (*slept)[0], want)
	}
}

const limitNoReset = "You've hit your session limit"

// TestEstimateScalesByTheModelPriceRatio: the cost hints in the corpus
// were measured on Opus (docs/12 §6), so an estimate for another model
// has to be scaled or it prices every cell as an Opus one. That is not
// cosmetic: the estimate is the only guard between an operator and a
// spend, and a Haiku cell over the pilot's twenty tasks estimated 48.50
// usd, which the user tier's 20 usd cap refused although the run costs
// about a tenth of that.
//
// The pilot's own figure is pinned: 48.50 for claude-opus-5 over the
// frozen tasks 1-20 at K=5 across two arms, which is what
// bench/results/pilot-2026-09-13/provenance.txt records.
func TestEstimateScalesByTheModelPriceRatio(t *testing.T) {
	tasks := loadTasks(t, pilotBatch(t)...)
	if len(tasks) != 20 {
		t.Fatalf("the pilot batch is %d tasks, want 20", len(tasks))
	}
	prices := trace.DefaultPrices()
	if prices.CalibrationModel != "claude-opus-5" {
		t.Fatalf("the calibration model moved to %q; the pinned figures below were measured against claude-opus-5", prices.CalibrationModel)
	}

	arms := 2.0
	for _, tc := range []struct {
		model string
		total float64
		ratio float64
		known bool
	}{
		{"claude-opus-5", 48.50, 1.0, true},
		{"claude-haiku-4-5-20251001", 9.70, 0.2, true},
		// A model the pinned table does not carry keeps the calibration
		// model's price rather than guessing, and says so to the caller.
		{"sonnet", 48.50, 1.0, false},
		{"", 48.50, 1.0, false},
	} {
		per, ratio := EstimateFor(tasks, 5, tc.model, prices)
		if got := per * arms; math.Abs(got-tc.total) > 0.005 {
			t.Errorf("%q: %.2f usd over %d tasks, want %.2f", tc.model, got, len(tasks), tc.total)
		}
		if math.Abs(ratio-tc.ratio) > 1e-9 {
			t.Errorf("%q: ratio %v, want %v", tc.model, ratio, tc.ratio)
		}
		if _, ok := prices.CostRatio(tc.model); ok != tc.known {
			t.Errorf("%q: known %v, want %v", tc.model, ok, tc.known)
		}
	}
	// The Haiku cell has to fit the user tier's cap, which is the whole
	// point of the change.
	haiku, _ := EstimateFor(tasks, 5, "claude-haiku-4-5-20251001", prices)
	if haiku*arms > 20 {
		t.Errorf("the Haiku cell estimates %.2f usd and the user tier caps at 20", haiku*arms)
	}
	// And Estimate without a model is the old figure exactly, so nothing
	// that did not ask for a model moved.
	if got := Estimate(tasks, 5) * arms; math.Abs(got-48.50) > 0.005 {
		t.Errorf("the model-less estimate moved to %.2f", got)
	}
}

// pilotBatch is the twenty tasks of the pilot, read from the corpus so
// the test follows a re-freeze rather than pinning names twice.
func pilotBatch(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, id := range corpusIDs(t) {
		if m := regexp.MustCompile(`-(\d{4})-`).FindStringSubmatch(id); m != nil {
			if n, _ := strconv.Atoi(m[1]); n >= 1 && n <= 20 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
