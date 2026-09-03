package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
)

const benchUsage = `usage: saga bench <verify-task|run|report|compare>

  verify-task <task-dir>... [--keep <dir>] [--static] [--json]
  run     --tasks <glob> --adapter <bare|replay|claude-code> [--k <n>] --out <dir>
          [--arm <id>] [--model <id>] [--seed <hex>] [--replay <patch>] [--budget <usd>]
          [--tier smoke|user|dev|publish] [--claude-bin <path>] [--saga-bin <path>]
  report  <run-dir> [--json]
  compare <run-dir-a> <run-dir-b> [--epsilon <x>] [--json]
`

func (a *App) cmdBench(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.Stdout, benchUsage)
		return cli.Errorf(cli.ExitUsage, "bench: subcommand required")
	}
	switch args[0] {
	case "verify-task":
		return a.benchVerifyTask(args[1:])
	case "run":
		return a.benchRun(args[1:])
	case "report":
		return a.benchReport(args[1:])
	case "compare":
		return a.benchCompare(args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(a.Stdout, benchUsage)
		return nil
	}
	return cli.Errorf(cli.ExitUsage, "bench: unknown subcommand %q", args[0])
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func (a *App) benchVerifyTask(args []string) error {
	fs := a.flags("bench verify-task")
	keep := fs.String("keep", "", "stage workspaces under this directory and keep them")
	static := fs.Bool("static", false, "loader, canary, leak and scan checks only; no oracle runs")
	asJSON := fs.Bool("json", false, "emit one JSON document per task")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	dirs := positionals[fs]
	if len(dirs) == 0 {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench verify-task <task-dir>...")
	}
	var expanded []string
	for _, d := range dirs {
		found, err := task.Find(filepath.Join(a.Cwd, d))
		if !filepath.IsAbs(d) && (err != nil || len(found) == 0) {
			found, err = task.Find(d)
		}
		if filepath.IsAbs(d) {
			found, err = task.Find(d)
		}
		if err != nil || len(found) == 0 {
			return cli.Errorf(cli.ExitUsage, "verify-task: no task at %s", d)
		}
		expanded = append(expanded, found...)
	}
	ctx, cancel := signalContext()
	defer cancel()
	var codes []cli.Code
	for _, dir := range expanded {
		var log = a.Stderr
		if *asJSON {
			log = nil
		}
		res := task.Verify(ctx, dir, task.VerifyOptions{Keep: *keep, Static: *static, Log: log})
		if *asJSON {
			a.Stdout.Write(res.JSON())
		} else {
			fmt.Fprint(a.Stdout, res.Text())
		}
		codes = append(codes, res.Code)
	}
	if code := cli.Precedence(codes...); code != cli.ExitOK {
		return &cli.Error{Code: code, Msg: "verify-task: " + code.String()}
	}
	return nil
}
