package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
)

// benchApproveCorpus is `saga bench approve-corpus <tasks-glob>`
// (ADR 0010): the owner approves the frozen task set once, for one saga
// binary, and every later run consumes those records without a human
// present. It is the same human act `saga gate check --approve` is, and
// it refuses under an agent shell for the same reason and with the same
// message; --check reports what is missing and approves nothing, which
// is what the launcher calls.
func (a *App) benchApproveCorpus(args []string) error {
	fs := a.flags("bench approve-corpus")
	sagaBin := fs.String("saga-bin", "", "saga executable the approvals are bound to (default: this binary)")
	check := fs.Bool("check", false, "report which tasks lack a record and approve nothing")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	globs := positionals[fs]
	if len(globs) == 0 {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench approve-corpus <tasks-glob>... [--check] [--saga-bin <path>]")
	}
	tasks, err := a.loadTasks(globs)
	if err != nil {
		return err
	}
	// Approving is a human act; checking is not, because it writes
	// nothing. The refusal is gate's own, so its wording and its
	// detection stay in one place.
	if !*check {
		if err := gate.HumanActError("check --approve", nil); err != nil {
			return err
		}
	}
	if *sagaBin == "" {
		exe, err := os.Executable()
		if err != nil {
			return cli.Wrap(cli.ExitEnvironment, "saga binary", err)
		}
		*sagaBin = exe
	}
	// The store is keyed by the frozen task set, so a corpus edit
	// invalidates the approvals by construction rather than by anyone
	// remembering to clear them. `run.CorpusKey` is the only place that
	// name is decided, and the runner calls the same function: keying
	// the two sides separately is what sent the 2026-09-13 dev run's
	// approvals to one store and its lookups to another.
	setHash, err := run.CorpusKey(tasks)
	if err != nil {
		return err
	}
	store, err := adapter.CorpusStoreDir(setHash)
	if err != nil {
		return err
	}
	// Only approving creates the store. A --check that created it would
	// leave an empty directory behind that a later run mistakes for a
	// store with nothing approved in it yet, which is the shape the
	// 2026-09-13 failure took.
	if !*check {
		if err := os.MkdirAll(store, 0o700); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "corpus store", err)
		}
	}
	binHash := adapter.FileSHA256(*sagaBin)
	if binHash == "" {
		return cli.Errorf(cli.ExitEnvironment, "approve-corpus: %s is unreadable", *sagaBin)
	}

	// The composed PATH the identity is taken over. It is printed so a
	// mismatch between approving and running is visible rather than
	// showing up as "covered 0 of 40" with no reason (brief 2026-09-06
	// canonical bench path).
	probe := &adapter.ClaudeCode{SagaBinary: *sagaBin}
	fmt.Fprintf(a.Stdout, "path: %s\n", strings.Join(probe.BenchPath(""), string(os.PathListSeparator)))

	// With no store at all, no task can be covered and the reason is the
	// same for every one of them, so the forty workspace stagings that
	// would learn it forty times are skipped. The report is not: the
	// caller is a launcher's preflight, and the list of task ids is how
	// an operator sees which batch is about to run.
	noStore := false
	if fi, err := os.Stat(store); *check && (err != nil || !fi.IsDir()) {
		noStore = true
	}

	ctx, cancel := signalContext()
	defer cancel()

	scratch, err := os.MkdirTemp("", "saga-approve-")
	if err != nil {
		return cli.Wrap(cli.ExitEnvironment, "scratch", err)
	}
	defer os.RemoveAll(scratch)

	covered := 0
	var problems []string
	for _, t := range tasks {
		if noStore {
			problems = append(problems, t.ID)
			fmt.Fprintf(a.Stdout, "%s not approved: no corpus approval store for task set %s\n", t.ID, short16(setHash))
			continue
		}
		c := &adapter.ClaudeCode{Binary: "/nonexistent/claude", Version: "approve-corpus", SagaBinary: *sagaBin}
		ws := filepath.Join(scratch, t.ID, "ws")
		cfg := filepath.Join(scratch, t.ID, "cfg")
		if err := os.MkdirAll(cfg, 0o755); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "scratch", err)
		}
		res, err := c.ApproveCorpus(ctx, &adapter.ApproveCorpusInput{
			Task: t, Workspace: ws, ConfigDir: cfg, CorpusStore: store, Check: *check,
		})
		// The workspace is deleted whatever happened: it holds a copy of
		// the task's repo and nothing worth keeping.
		os.RemoveAll(filepath.Join(scratch, t.ID))
		if err != nil {
			problems = append(problems, t.ID)
			fmt.Fprintf(a.Stdout, "%s failed: %v\n", t.ID, err)
			continue
		}
		switch {
		case len(res.Missing) > 0:
			problems = append(problems, t.ID)
			fmt.Fprintf(a.Stdout, "%s not approved: %s\n", t.ID, strings.Join(res.Missing, " "))
		case *check:
			covered++
			fmt.Fprintf(a.Stdout, "%s covered %d gates\n", t.ID, res.Gates)
		default:
			covered++
			fmt.Fprintf(a.Stdout, "%s approved %d gates\n", t.ID, res.Approved)
		}
	}
	verb := "approved"
	if *check {
		verb = "covered"
	}
	fmt.Fprintf(a.Stdout, "%s %d of %d tasks for binary %s, task set %s\n", verb, covered, len(tasks), short16(binHash), short16(setHash))
	if len(problems) > 0 {
		return cli.Errorf(cli.ExitApproval, "approve-corpus: %d of %d tasks lack approval: %s", len(problems), len(tasks), strings.Join(problems, " "))
	}
	return nil
}

// loadTasks expands the globs a bench subcommand was given.
func (a *App) loadTasks(globs []string) ([]*task.Task, error) {
	var out []*task.Task
	for _, g := range globs {
		for _, sub := range strings.Split(g, ",") {
			pattern := a.abs(strings.Trim(sub, " \t\r\n"))
			found, err := task.Find(pattern)
			if err != nil || len(found) == 0 {
				return nil, cli.Errorf(cli.ExitUsage, "no tasks match %q (resolved to %q; %s)", sub, pattern, task.WhyNoMatch(pattern))
			}
			for _, d := range found {
				t, err := task.Load(d)
				if err != nil {
					return nil, err
				}
				out = append(out, t)
			}
		}
	}
	return out, nil
}

// short16 is the first 16 hex characters of a sha256 string, which is
// how the bench names a binary and a task set in prose.
func short16(h string) string {
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) > 16 {
		h = h[:16]
	}
	return "sha256:" + h
}

// benchEstimate is `saga bench estimate <glob> --k <n> [--arms <n>]`:
// the runner's own cost estimate for a cell, printed before anything is
// spent. It reads task.toml and runs nothing, so a launcher can put the
// number in front of the operator and refuse below it.
func (a *App) benchEstimate(args []string) error {
	fs := a.flags("bench estimate")
	k := fs.Int("k", 1, "runs per task")
	arms := fs.Int("arms", 2, "arms in the invocation; the estimate is per arm times this")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	globs := positionals[fs]
	if len(globs) == 0 {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench estimate <tasks-glob>... [--k <n>] [--arms <n>]")
	}
	tasks, err := a.loadTasks(globs)
	if err != nil {
		return err
	}
	if *k < 1 || *arms < 1 {
		return cli.Errorf(cli.ExitUsage, "estimate: k and arms must be at least 1")
	}
	per := run.Estimate(tasks, *k)
	fmt.Fprintf(a.Stdout, "%.2f\n", per*float64(*arms))
	fmt.Fprintf(a.Stderr, "estimate: %d tasks, k=%d, %d arms: %.2f usd per arm, %.2f usd total (bench-spec 4.5; the runner caps at 1.5x)\n",
		len(tasks), *k, *arms, per, per*float64(*arms))
	return nil
}
