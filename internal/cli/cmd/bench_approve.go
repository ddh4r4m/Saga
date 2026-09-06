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
	// remembering to clear them.
	frozen, err := run.FindFrozen(tasks)
	if err != nil {
		return err
	}
	if frozen == nil {
		return cli.Errorf(cli.ExitUsage, "approve-corpus: no %s beside the tasks; freeze the set first with `saga bench taskset --write`", run.FrozenName)
	}
	if err := frozen.Check(tasks); err != nil {
		return err
	}
	if frozen.Set == "" {
		return cli.Errorf(cli.ExitUsage, "approve-corpus: %s carries no `set` line", frozen.Path)
	}
	store, err := adapter.CorpusStoreDir(frozen.Set)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(store, 0o700); err != nil {
		return cli.Wrap(cli.ExitEnvironment, "corpus store", err)
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
	fmt.Fprintf(a.Stdout, "%s %d of %d tasks for binary %s, task set %s\n", verb, covered, len(tasks), short16(binHash), short16(frozen.Set))
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
