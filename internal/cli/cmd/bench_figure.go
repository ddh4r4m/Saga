package cmd

import (
	"fmt"

	"github.com/ddh4r4m/saga/internal/bench/figure"
	"github.com/ddh4r4m/saga/internal/cli"
)

// benchFigure is `saga bench figure <archive-dir>`: the archive's own
// one-glance figure, drawn from the runner-written compare.json and
// nothing else. Every number on it is the report's; the command
// computes none of them, so the figure cannot disagree with the report
// beside it. It replaced scripts/pilot-figure.py, which needed
// matplotlib and printed the interval as prose.
func (a *App) benchFigure(args []string) error {
	fs := a.flags("bench figure")
	out := fs.String("out", "", "output path (default: <archive-dir>/figure.svg)")
	perTask := fs.Bool("per-task", false, "also write figure-per-task.svg, one dumbbell of median tokens per task")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	pos := positionals[fs]
	if len(pos) != 1 {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench figure <archive-dir> [--out <file>] [--per-task]")
	}
	archive := a.abs(pos[0])
	path, err := figure.Render(archive, *out)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, path)
	if *perTask {
		p, err := figure.RenderPerTask(archive, "")
		if err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, p)
	}
	return nil
}
