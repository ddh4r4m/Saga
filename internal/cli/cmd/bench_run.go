package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/report"
	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

func (a *App) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(a.Cwd, p)
}

func (a *App) benchRun(args []string) error {
	fs := a.flags("bench run")
	tasksGlob := fs.String("tasks", "", "task directory glob, or a comma-separated list of globs (bench/tasks/*)")
	adapterName := fs.String("adapter", "", "bare | replay | claude-code")
	k := fs.Int("k", 1, "runs per task")
	out := fs.String("out", "", "archive directory")
	var arms []string
	fs.Func("arm", "arm spec <id>[:bare|:<component>,...]; repeat for interleaved arms, each archived under <out>/<id> (default A:bare)", func(v string) error {
		arms = append(arms, v)
		return nil
	})
	wallCap := fs.Float64("wall-cap", 0, "cap every run's wall limit at this many seconds (0: the task's limit)")
	model := fs.String("model", "", "model id (claude-code --model; labels the cell)")
	seed := fs.String("seed", "", "64-hex run seed (drawn and recorded when empty)")
	replayPatch := fs.String("replay", "gold", "replay adapter: control name, comma list cycling by run, path, none or abandon")
	budget := fs.Float64("budget", 0, "refuse when the estimate exceeds this many usd")
	tier := fs.String("tier", "user", "smoke | user | dev | publish")
	claudeBin := fs.String("claude-bin", "claude", "claude executable")
	sagaBin := fs.String("saga-bin", "", "saga executable the hooks call (default: this binary)")
	noVerify := fs.Bool("no-verify", false, "skip verify-task before the run (tests only)")
	keep := fs.Bool("keep", false, "keep the temporary workspaces")
	noReport := fs.Bool("no-report", false, "do not write report.json and report.md")
	prereg := fs.String("prereg", "", "pre-registration file to freeze into the archive as preregistration.md (docs/12 row 15)")
	unfrozen := fs.Bool("unfrozen", false, "run against tasks that do not match TASKSET.sha256, recording task_set.frozen = false")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *tasksGlob == "" || *adapterName == "" || *out == "" {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench run --tasks <glob> --adapter <name> --k <n> --out <dir>")
	}
	var dirs []string
	for _, g := range strings.Split(*tasksGlob, ",") {
		// Trim the whitespace a shell-built list carries, carriage
		// returns included: a stray \r makes a path that does not exist
		// and prints identically to one that does.
		pattern := a.abs(strings.Trim(g, " \t\r\n"))
		found, err := task.Find(pattern)
		if err != nil || len(found) == 0 {
			// Say what was looked at and what was there: a pattern that
			// resolves to a directory holding no task.toml is a different
			// mistake from one that resolves to nothing, and the message
			// that only said "no tasks match" cost a live run.
			return cli.Errorf(cli.ExitUsage, "run: no tasks match %q (resolved to %q; %s)", g, pattern, task.WhyNoMatch(pattern))
		}
		dirs = append(dirs, found...)
	}
	var tasks []*task.Task
	for _, d := range dirs {
		t, err := task.Load(d)
		if err != nil {
			return err
		}
		tasks = append(tasks, t)
	}
	if *sagaBin == "" {
		if exe, err := os.Executable(); err == nil {
			*sagaBin = exe
		}
	}
	var ad adapter.Adapter
	switch *adapterName {
	case "replay":
		ad = &adapter.Replay{Patch: *replayPatch}
	case "claude-code":
		ad = &adapter.ClaudeCode{Binary: *claudeBin, SagaBinary: *sagaBin, Model: *model}
	case "bare":
		return cli.Errorf(cli.ExitEnvironment, "run: the bare adapter is not implemented yet (bench-spec 6.1); use replay or claude-code")
	default:
		return cli.Errorf(cli.ExitUsage, "run: unknown adapter %q", *adapterName)
	}
	prices := trace.DefaultPrices()
	if s, err := store.Find(a.Cwd); err == nil && s.Exists() {
		if p, err := trace.LoadPrices(s); err == nil {
			prices = p
		}
	}
	if len(arms) == 0 {
		arms = []string{"A"}
	}
	var specs []run.ArmSpec
	for _, v := range arms {
		sp, err := run.ParseArm(v)
		if err != nil {
			return err
		}
		specs = append(specs, sp)
	}
	ctx, cancel := signalContext()
	defer cancel()
	outDir := a.abs(*out)
	opts := run.Options{
		Tasks: tasks, Adapter: ad, K: *k, Out: outDir, Tier: *tier, Seed: *seed, Model: *model,
		Prices: prices, SagaBinary: *sagaBin, Version: a.Version, Budget: *budget, Verify: !*noVerify, Keep: *keep, WallCapS: *wallCap, Log: a.Stderr,
	}
	if *prereg != "" {
		opts.Prereg = a.abs(*prereg)
	}
	opts.Unfrozen = *unfrozen
	report := func(dir string, res *run.Result, err error) error {
		if res != nil && len(res.Rows) > 0 && !*noReport {
			if rerr := a.writeReport(dir, false); rerr != nil && err == nil {
				err = rerr
			}
		}
		// The archive-root SHA256SUMS is written last, so it covers the
		// report as well as the manifest, the rows and the frozen
		// pre-registration (bench-spec 3.4).
		if res != nil {
			if serr := run.WriteArchiveSums(dir); serr != nil && err == nil {
				err = serr
			}
		}
		if res != nil {
			fmt.Fprintf(a.Stdout, "manifest %s: %d runs, %d not run, spent %.4f usd, archive %s\n", res.ManifestHash, len(res.Rows), res.NotRun, res.SpentUSD, dir)
		}
		return err
	}
	if len(specs) == 1 {
		opts.Arm, opts.Components = specs[0].ID, specs[0].Components
		res, err := run.Run(ctx, opts)
		return report(outDir, res, err)
	}
	results, err := run.RunArms(ctx, opts, specs)
	for i, res := range results {
		if rerr := report(filepath.Join(outDir, specs[i].ID), res, nil); rerr != nil && err == nil {
			err = rerr
		}
	}
	return err
}

func (a *App) writeReport(dir string, print bool) error {
	arch, err := report.Load(dir)
	if err != nil {
		return err
	}
	rep := report.Build(arch.Manifest, arch.Hash, arch.Rows, dir)
	if err := rep.Validate(); err != nil {
		return cli.Wrap(cli.ExitUsage, "report schema", err)
	}
	js, err := rep.JSON()
	if err != nil {
		return err
	}
	md := rep.Markdown()
	if err := os.WriteFile(filepath.Join(dir, "report.json"), js, 0o644); err != nil {
		return cli.Wrap(cli.ExitEnvironment, "report", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(md), 0o644); err != nil {
		return cli.Wrap(cli.ExitEnvironment, "report", err)
	}
	if print {
		fmt.Fprint(a.Stdout, md)
	}
	return nil
}

func (a *App) benchReport(args []string) error {
	fs := a.flags("bench report")
	asJSON := fs.Bool("json", false, "print report.json instead of report.md")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	dir := positional(fs)
	if dir == "" {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench report <run-dir> [--json]")
	}
	dir = a.abs(dir)
	if err := a.writeReport(dir, !*asJSON); err != nil {
		return err
	}
	if *asJSON {
		raw, _ := os.ReadFile(filepath.Join(dir, "report.json"))
		a.Stdout.Write(raw)
	}
	return nil
}

func (a *App) benchCompare(args []string) error {
	fs := a.flags("bench compare")
	epsilon := fs.Float64("epsilon", 0, "equivalence bound for the TOST-style check")
	asJSON := fs.Bool("json", false, "print JSON instead of markdown")
	outFlag := fs.String("out", "", "write compare.json and compare.md here (default: the second run dir)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	pos := positionals[fs]
	if len(pos) != 2 {
		return cli.Errorf(cli.ExitUsage, "usage: saga bench compare <run-dir-a> <run-dir-b>")
	}
	ra, err := report.Load(a.abs(pos[0]))
	if err != nil {
		return err
	}
	rb, err := report.Load(a.abs(pos[1]))
	if err != nil {
		return err
	}
	rep, err := report.Compare(ra, rb, *epsilon)
	if err != nil {
		return err
	}
	if err := rep.Validate(); err != nil {
		return cli.Wrap(cli.ExitUsage, "report schema", err)
	}
	js, err := rep.JSON()
	if err != nil {
		return err
	}
	md := rep.Markdown()
	dir := a.abs(pos[1])
	if *outFlag != "" {
		dir = a.abs(*outFlag)
		os.MkdirAll(dir, 0o755)
	}
	os.WriteFile(filepath.Join(dir, "compare.json"), js, 0o644)
	os.WriteFile(filepath.Join(dir, "compare.md"), []byte(md), 0o644)
	if *asJSON {
		a.Stdout.Write(js)
	} else {
		fmt.Fprint(a.Stdout, md)
	}
	return nil
}
