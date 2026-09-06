package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
)

// approveCapableSaga is a saga stand-in that behaves the way the real
// one does for the two calls this path makes: `gate check --json`
// reports a gate as approved only once a record file exists in
// SAGA_APPROVAL_DIR, and `gate check --approve --json` writes it. That
// is enough to drive "a run consumes, never creates" without a real
// gate; the real command is covered by internal/gate's own suite.
func approveCapableSaga(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "saga")
	script := `#!/bin/sh
case "$1 $2" in
  "--version ") echo "saga 0.0.0-test"; exit 0;;
esac
if [ "$1" = "gate" ] && [ "$2" = "check" ]; then
  rec="$SAGA_APPROVAL_DIR/g1.json"
  approve=no
  for a in "$@"; do [ "$a" = "--approve" ] && approve=yes; done
  if [ "$approve" = "yes" ]; then
    mkdir -p "$SAGA_APPROVAL_DIR" && printf '{"gate":"G1"}' > "$rec"
  fi
  if [ -f "$rec" ]; then
    printf '{"gates":[{"id":"G1","approval":"present"}]}'
    exit 1
  fi
  printf '{"gates":[{"id":"G1","approval":"missing"}]}'
  exit 4
fi
echo '{}'
exit 1
`
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPrepareConsumesCorpusApprovalNeverCreatesIt (ADR 0010 decision 3):
// against an empty store a gate arm is infra with the reason said out
// loud, and it does not approve itself out of the problem; against a
// filled store it proceeds. Before this, Prepare ran `check --approve`
// in every run, which is why every run needed the owner at a terminal.
func TestPrepareConsumesCorpusApprovalNeverCreatesIt(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	home := t.TempDir()
	t.Setenv("SAGA_HOME", home)
	tk := loadTasks(t, "ts-0001-slug-collapse")[0]
	set := "sha256:" + strings.Repeat("cd", 32)
	store, err := adapter.CorpusStoreDir(set)
	if err != nil {
		t.Fatal(err)
	}
	sagaBin := approveCapableSaga(t)

	prepare := func(t *testing.T) error {
		t.Helper()
		root := t.TempDir()
		ws, cfg := filepath.Join(root, "ws"), filepath.Join(root, "cfg")
		if err := os.MkdirAll(cfg, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := task.Stage(context.Background(), tk, ws); err != nil {
			t.Fatal(err)
		}
		c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: sagaBin}
		_, err := c.Prepare(context.Background(), &adapter.PrepareInput{
			Task: tk, Workspace: ws, ConfigDir: cfg, Components: []string{"gate"},
			Prompt: adapter.StagedPrompt(tk.Prompt(), []string{"gate"}),
			Blocks: []string{}, Limits: adapter.Limits{WallS: 60, MaxTurns: 5, USD: 1},
			TaskSetSHA256: set,
		})
		return err
	}

	// Empty store: refused, and the reason names the gate and the way out.
	err = prepare(t)
	if err == nil {
		t.Fatal("a gate arm prepared against an empty corpus store")
	}
	var na *adapter.NotPreApproved
	if !errors.As(err, &na) {
		t.Fatalf("error %v is not NotPreApproved", err)
	}
	if !strings.Contains(na.Error(), "not pre-approved") || !strings.Contains(na.Error(), "G1") {
		t.Errorf("reason %q", na.Error())
	}
	if !strings.Contains(na.Error(), "approve-corpus") {
		t.Errorf("the reason does not name the way out: %q", na.Error())
	}
	// And it did not approve itself: the store is still empty.
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Errorf("Prepare wrote %d records into the corpus store; a run must never approve", len(entries))
	}

	// The owner approves once, and then the same Prepare proceeds.
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "g1.json"), []byte(`{"gate":"G1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepare(t); err != nil {
		t.Errorf("a gate arm refused against a filled corpus store: %v", err)
	}
}

// TestGateArmWithoutApprovalIsInfra: the runner turns the refusal into
// an excluded run rather than a graded one, because the arm did not run
// the treatment the manifest names.
func TestGateArmWithoutApprovalIsInfra(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	t.Setenv("SAGA_HOME", t.TempDir())
	tasks := loadTasks(t, "ts-0001-slug-collapse")
	c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: approveCapableSaga(t)}
	res, err := Run(context.Background(), Options{
		Tasks: tasks, Adapter: c, K: 1, Out: filepath.Join(t.TempDir(), "runs"),
		Seed: strings.Repeat("cd", 32), Components: []string{"gate"}, Arm: "B", Verify: false,
	})
	if err != nil && cli.CodeOf(err) != cli.ExitOK {
		t.Logf("run returned %v", err)
	}
	if res == nil || len(res.Rows) != 1 {
		t.Fatalf("rows: %+v", res)
	}
	row := res.Rows[0]
	if row.Outcome != "infra" {
		t.Errorf("outcome %q, want infra", row.Outcome)
	}
	if row.OutcomeReason == nil || !strings.Contains(*row.OutcomeReason, "not pre-approved") {
		t.Errorf("reason %q", deref(row.OutcomeReason))
	}
}

// TestApproveCorpusFillsTheStoreAndIsIdempotent (ADR 0010 decision 2):
// the owner's one act writes one record per gate, and running it again
// changes nothing. It is driven with a saga stand-in, because the real
// command is a human act and this test runs under a harness; what the
// real `gate check --approve` records is internal/gate's own suite.
func TestApproveCorpusFillsTheStoreAndIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	t.Setenv("SAGA_HOME", t.TempDir())
	tk := loadTasks(t, "ts-0001-slug-collapse")[0]
	set := "sha256:" + strings.Repeat("ef", 32)
	store, err := adapter.CorpusStoreDir(set)
	if err != nil {
		t.Fatal(err)
	}
	sagaBin := approveCapableSaga(t)

	approve := func(t *testing.T, check bool) *adapter.ApproveCorpusResult {
		t.Helper()
		root := t.TempDir()
		c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "test", SagaBinary: sagaBin}
		res, err := c.ApproveCorpus(context.Background(), &adapter.ApproveCorpusInput{
			Task: tk, Workspace: filepath.Join(root, "ws"), ConfigDir: root,
			CorpusStore: store, Check: check,
		})
		if err != nil {
			t.Fatalf("approve-corpus: %v", err)
		}
		return res
	}

	// Before: the check reports the gap and writes nothing.
	res := approve(t, true)
	if len(res.Missing) == 0 {
		t.Error("--check reported no gap on an empty store")
	}
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Errorf("--check wrote %d records; it must write none", len(entries))
	}

	// The owner's act fills it.
	res = approve(t, false)
	if len(res.Missing) != 0 {
		t.Errorf("still missing after approving: %v", res.Missing)
	}
	if res.Approved == 0 {
		t.Error("approved nothing")
	}
	first, err := os.ReadDir(store)
	if err != nil || len(first) == 0 {
		t.Fatalf("store after approving: %v %v", first, err)
	}

	// Again: same records, nothing added, and the check is now clean.
	approve(t, false)
	second, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Errorf("re-running wrote %d records, was %d; approval must be idempotent", len(second), len(first))
	}
	if res := approve(t, true); len(res.Missing) != 0 {
		t.Errorf("--check still reports %v after approving", res.Missing)
	}
}
