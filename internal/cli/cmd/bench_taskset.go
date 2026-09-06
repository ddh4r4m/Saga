package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/report"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
)

// benchTaskset is `saga bench taskset <glob> [--write <file>]`: the
// task-set freeze artefact of docs/12 row 15. It prints one
// `<id> <content-hash>` line per task and a final `set <sha256>` line
// computed the way the manifest's task_set.sha256 is, so a frozen file
// and a manifest from the same corpus agree by construction. `saga bench
// run` refuses when a task no longer matches, which is what makes a
// corpus edit during a pilot a deliberate act rather than a silent one.
func (a *App) benchTaskset(args []string) error {
	fs := a.flags("bench taskset")
	write := fs.String("write", "", "write the listing to this file (bench/tasks/"+run.FrozenName+")")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	globs := positionals[fs]
	if len(globs) == 0 {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench taskset <tasks-glob>... [--write <file>]")
	}
	var tasks []*task.Task
	for _, g := range globs {
		for _, sub := range strings.Split(g, ",") {
			found, err := task.Find(a.abs(strings.TrimSpace(sub)))
			if err != nil || len(found) == 0 {
				return cli.Errorf(cli.ExitUsage, "taskset: no tasks match %s", sub)
			}
			for _, d := range found {
				t, err := task.Load(d)
				if err != nil {
					return err
				}
				tasks = append(tasks, t)
			}
		}
	}
	text, set, err := run.TasksetText(tasks)
	if err != nil {
		return err
	}
	if *write != "" {
		p := a.abs(*write)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "taskset", err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "taskset", err)
		}
		fmt.Fprintf(a.Stdout, "%d tasks, %s, written to %s\n", len(tasks), set, p)
		return nil
	}
	fmt.Fprint(a.Stdout, text)
	return nil
}

// benchBadge is `saga bench badge <run-dir> --metric <m>`: the badge line
// of bench-spec 7.3. It refuses at any tier below publish, which is
// docs/12 commitment 4: a number from a smoke, a user run or a dev run
// may appear in its own report and nowhere else, because those tiers do
// not carry the K and the pre-registration a claim needs.
func (a *App) benchBadge(args []string) error {
	fs := a.flags("bench badge")
	metric := fs.String("metric", "", "metric to cite (a key of the report's arms block)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	pos := positionals[fs]
	if len(pos) != 1 || *metric == "" {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench badge <run-dir> --metric <m>")
	}
	arch, err := report.Load(a.abs(pos[0]))
	if err != nil {
		return err
	}
	if err := report.BadgeTierOK(arch.Manifest.Tier); err != nil {
		return err
	}
	return cli.Errorf(cli.ExitUsage, "badge: rendering is not implemented; the tier rule of docs/12 commitment 4 is, and this archive passes it")
}
