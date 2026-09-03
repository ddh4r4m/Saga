// Package cmd is the saga subcommand tree. It uses the standard flag
// package (ADR 0008 allows cobra; nothing here needs it) and maps every
// failure onto the uniform exit codes of internal/cli/exit.go.
package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	claudecode "github.com/ddh4r4m/saga/adapters/claude-code"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/harness/claude"
	"github.com/ddh4r4m/saga/internal/hook"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

const usage = `saga: a local, model-agnostic layer for coding agents

usage: saga <command> [flags]

  init                                  write .saga/ (gitignore, config skeleton)
  install --harness claude-code         bind saga hook <harness> <event> in the harness settings
          [--dry-run] [--shared] [--project <dir>]
  hook <harness> <event>                the composed hook entry (reads stdin JSON)
  trace tail   [session] [-n N] [--type t,..] [--json]
  trace ledger [session] [--json]
  trace budget [--show] [--session-usd x] [--json]
  trace verify <session>
  trace prices show | use <file>
  trace doctor [--json]                 trace's subset of saga doctor
  doctor [--json]                       environment, hooks, usage source, pins
  bench verify-task|run|report|compare  the measurement harness (saga bench -h)
  gate init|status|check|reverify|attest|approve|lint|guard-diff
                                        contracts, evidence, red proof, diff guards (saga gate -h)
  version

exit codes (contracts section 4): 0 ok, 1 finding, 2 usage, 3 refusal, 4 approval, 5 integrity, 6 environment, 7 contamination
`

// App carries the process context through the subcommands.
type App struct {
	Version string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Cwd     string
}

// Main runs the CLI and returns the exit code.
func Main(version string, args []string, stdin io.Reader, stdout, stderr io.Writer) cli.Code {
	cwd, _ := os.Getwd()
	app := &App{Version: version, Stdin: stdin, Stdout: stdout, Stderr: stderr, Cwd: cwd}
	err := app.run(args)
	if err != nil {
		code := cli.CodeOf(err)
		if code != cli.ExitOK {
			fmt.Fprintf(stderr, "saga: %v\n", err)
		}
		return code
	}
	return cli.ExitOK
}

func (a *App) run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.Stdout, usage)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(a.Stdout, usage)
		return nil
	case "version", "--version", "-v":
		fmt.Fprintf(a.Stdout, "saga %s\n", a.Version)
		return nil
	case "init":
		return a.cmdInit(args[1:])
	case "install":
		return a.cmdInstall(args[1:])
	case "hook":
		return a.cmdHook(args[1:])
	case "trace":
		return a.cmdTrace(args[1:])
	case "doctor":
		return a.cmdDoctor(args[1:])
	case "bench":
		return a.cmdBench(args[1:])
	case "gate":
		return a.cmdGate(args[1:])
	}
	fmt.Fprint(a.Stderr, usage)
	return cli.Errorf(cli.ExitUsage, "unknown command %q", args[0])
}

func (a *App) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	return fs
}

// parseFlags parses flags interspersed with positionals (the standard
// flag package stops at the first positional); positionals are collected
// in order and readable through positional.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return cli.Wrap(cli.ExitUsage, fs.Name(), err)
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	positionals[fs] = pos
	return nil
}

var positionals = map[*flag.FlagSet][]string{}

func (a *App) store() (*store.Store, error) {
	s, err := store.Find(a.Cwd)
	if err != nil {
		return s, cli.Errorf(cli.ExitEnvironment, ".saga not found under %s: run saga init", s.Root)
	}
	return s, nil
}

func mapShape(err error) error {
	var se *store.ShapeError
	if errors.As(err, &se) {
		return cli.Wrap(cli.ExitEnvironment, "", err)
	}
	return err
}

func (a *App) cmdInit(args []string) error {
	fs := a.flags("init")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	root := store.RepoRoot(a.Cwd)
	created, err := store.Init(root)
	if err != nil {
		return mapShape(cli.Wrap(cli.ExitEnvironment, "init", err))
	}
	if created {
		fmt.Fprintf(a.Stdout, "initialised %s\n", filepath.Join(root, store.Dir))
	} else {
		fmt.Fprintf(a.Stdout, "%s already initialised\n", filepath.Join(root, store.Dir))
	}
	return nil
}

func (a *App) cmdInstall(args []string) error {
	fs := a.flags("install")
	harness := fs.String("harness", "", "harness to install for (claude-code)")
	dryRun := fs.Bool("dry-run", false, "print the settings that would be written")
	shared := fs.Bool("shared", false, "write .claude/settings.json instead of settings.local.json")
	project := fs.String("project", "", "project directory (default: repository root)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *harness != claudecode.Harness {
		return cli.Errorf(cli.ExitUsage, "install: --harness must be %s", claudecode.Harness)
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	if *project == "" {
		*project = s.Root
	}
	binary, err := os.Executable()
	if err != nil {
		return cli.Wrap(cli.ExitEnvironment, "install", err)
	}
	file, result, entries, err := claudecode.Install(*project, binary, *shared, *dryRun)
	if err != nil {
		return mapShape(cli.Wrap(cli.ExitEnvironment, "install", err))
	}
	if *dryRun {
		fmt.Fprintf(a.Stdout, "# would write %s\n%s", file, result)
		return nil
	}
	m, _, err := s.ReadManifest()
	if err != nil {
		return cli.Wrap(cli.ExitUsage, "install", err)
	}
	m.SagaVersion = a.Version
	if !contains(m.Layers, "trace") {
		m.Layers = append(m.Layers, "trace")
	}
	kept := m.Entries[:0]
	for _, e := range m.Entries {
		if e.Harness != claudecode.Harness {
			kept = append(kept, e)
		}
	}
	m.Entries = append(kept, entries...)
	if err := s.WriteManifest(m); err != nil {
		return cli.Wrap(cli.ExitEnvironment, "install", err)
	}
	fmt.Fprintf(a.Stdout, "wrote %d hook bindings to %s and %s\n", len(entries), file, s.Path("manifest.json"))
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// cmdHook is the composed entry. It never returns an error to Main so
// that stdout carries exactly one JSON object; the exit code comes from
// the entry.
func (a *App) cmdHook(args []string) error {
	if len(args) != 2 {
		fmt.Fprintln(a.Stdout, "{}")
		return cli.Errorf(cli.ExitUsage, "usage: saga hook <harness> <event>")
	}
	harness, event := args[0], args[1]
	if harness != claude.Name {
		fmt.Fprintln(a.Stdout, "{}")
		return cli.Errorf(cli.ExitUsage, "hook: unknown harness %q", harness)
	}
	s, _ := store.Find(a.Cwd)
	cfg := store.DefaultConfig()
	if s.Exists() {
		c, err := s.Config()
		if err != nil {
			fmt.Fprintf(a.Stderr, "saga hook: %v (using defaults)\n", err)
		} else {
			cfg = c
		}
	}
	stderr := func(m string) { fmt.Fprintln(a.Stderr, m) }
	layer := &trace.Layer{Store: s, Version: a.Version, Config: cfg, Components: []string{"trace", "gate", "doctor"}, Stderr: stderr}
	// Order of contracts section 1: trace records first, gate decides,
	// trace's Finalize records the merged decision. Gate is inactive
	// without a contract, so it is always in the chain.
	entry := &hook.Entry{
		Harness: harness, Parse: claude.Parse, Render: claude.Render,
		Layers: []hookio.Layer{layer, &gate.Layer{Store: s, Stderr: stderr}}, Deadline: time.Duration(cfg.Hook.DeadlineMS) * time.Millisecond,
		Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr,
	}
	code := entry.Run(event)
	if code != cli.ExitOK {
		return &cli.Error{Code: code, Msg: "hook " + event + ": " + code.String()}
	}
	return nil
}

func (a *App) cmdTrace(args []string) error {
	if len(args) == 0 {
		return cli.Errorf(cli.ExitUsage, "usage: saga trace <tail|ledger|budget|verify|prices|doctor>")
	}
	switch args[0] {
	case "tail":
		return a.traceTail(args[1:])
	case "ledger":
		return a.traceLedger(args[1:])
	case "budget":
		return a.traceBudget(args[1:])
	case "verify":
		return a.traceVerify(args[1:])
	case "prices":
		return a.tracePrices(args[1:])
	case "doctor":
		return a.cmdDoctor(args[1:])
	}
	return cli.Errorf(cli.ExitUsage, "trace: unknown subcommand %q", args[0])
}

func (a *App) pickSession(s *store.Store, arg string) (string, error) {
	if arg != "" {
		return trace.SafeID(arg), nil
	}
	sessions, err := trace.ListSessions(s)
	if err != nil {
		return "", cli.Wrap(cli.ExitEnvironment, "sessions", err)
	}
	if len(sessions) == 0 {
		return "", cli.Errorf(cli.ExitFinding, "no sessions recorded under %s", s.Path("trace", "sessions"))
	}
	return sessions[0], nil
}

func positional(fs *flag.FlagSet) string {
	if p := positionals[fs]; len(p) > 0 {
		return p[0]
	}
	return ""
}

func (a *App) traceTail(args []string) error {
	fs := a.flags("trace tail")
	n := fs.Int("n", 20, "number of events")
	types := fs.String("type", "", "comma-separated event types")
	asJSON := fs.Bool("json", false, "emit events as JSON lines")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	session, err := a.pickSession(s, positional(fs))
	if err != nil {
		return err
	}
	events, err := trace.ReadAll(trace.SessionDir(s, session))
	if err != nil {
		return cli.Wrap(cli.ExitIntegrity, "tail", err)
	}
	want := map[string]bool{}
	for _, t := range strings.Split(*types, ",") {
		if t != "" {
			want[t] = true
		}
	}
	var sel []trace.Event
	for _, ev := range events {
		if len(want) == 0 || want[ev.Type] {
			sel = append(sel, ev)
		}
	}
	if *n > 0 && len(sel) > *n {
		sel = sel[len(sel)-*n:]
	}
	if len(sel) == 0 {
		return cli.Errorf(cli.ExitFinding, "no events")
	}
	for _, ev := range sel {
		if *asJSON {
			b, _ := json.Marshal(ev)
			fmt.Fprintln(a.Stdout, string(b))
			continue
		}
		fmt.Fprintf(a.Stdout, "#%-5d t%-3d %-24s %-14s %s\n", ev.Seq, ev.Turn, ev.TS, ev.Type, summary(ev))
	}
	return nil
}

func summary(ev trace.Event) string {
	switch ev.Type {
	case trace.TypeToolCall:
		return fmt.Sprintf("%v decision=%v", ev.Body["tool"], ev.Body["decision"])
	case trace.TypeToolResult:
		return fmt.Sprintf("for=%v bytes=%v err=%v", ev.Body["for_seq"], ev.Body["result_bytes"], ev.Body["error"])
	case trace.TypeModelCall:
		u, _ := ev.Body["usage"].(map[string]any)
		return fmt.Sprintf("%v in=%v read=%v w5m=%v w1h=%v out=%v", ev.Body["model_served"], u["input_fresh"], u["cache_read"], u["cache_write_5m"], u["cache_write_1h"], u["output"])
	case trace.TypeTurn, trace.TypeSession, trace.TypeSubagent, trace.TypeCompaction:
		return fmt.Sprintf("%v", ev.Body["phase"])
	case trace.TypeBudget:
		return fmt.Sprintf("%v %v %v/%v", ev.Body["action"], ev.Body["metric"], ev.Body["value"], ev.Body["limit"])
	}
	return ""
}

func (a *App) traceLedger(args []string) error {
	fs := a.flags("trace ledger")
	asJSON := fs.Bool("json", false, "emit saga.trace.report/1")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	session, err := a.pickSession(s, positional(fs))
	if err != nil {
		return err
	}
	rows, err := trace.ReadLedger(trace.SessionDir(s, session))
	if err != nil {
		return cli.Wrap(cli.ExitIntegrity, "ledger", err)
	}
	cfg, _ := s.Config()
	rep := trace.Summarize(session, rows, cfg.Trace.Budget.SessionUSD)
	if *asJSON {
		return writeJSON(a.Stdout, rep)
	}
	fmt.Fprint(a.Stdout, rep.Text())
	if len(rows) == 0 {
		return cli.Errorf(cli.ExitFinding, "no ledger rows for session %s (no usage source read yet)", session)
	}
	return nil
}

func (a *App) traceBudget(args []string) error {
	fs := a.flags("trace budget")
	show := fs.Bool("show", true, "print the budget and current spend")
	sessionUSD := fs.Float64("session-usd", -1, "set [trace.budget] session_usd")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	if *sessionUSD >= 0 {
		if err := setSessionUSD(s, *sessionUSD); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "budget", err)
		}
	}
	cfg, err := s.Config()
	if err != nil {
		return cli.Wrap(cli.ExitUsage, "budget", err)
	}
	out := map[string]any{"session_usd_limit": cfg.Trace.Budget.SessionUSD, "soft_pct": cfg.Trace.Budget.SoftPct, "compact_pct": cfg.Trace.Budget.CompactPct, "hard_pct": cfg.Trace.Budget.HardPct, "hard_action": cfg.Trace.Budget.HardAction}
	if sessions, _ := trace.ListSessions(s); len(sessions) > 0 {
		rows, _ := trace.ReadLedger(trace.SessionDir(s, sessions[0]))
		out["session"] = sessions[0]
		if len(rows) > 0 {
			out["session_usd"] = rows[len(rows)-1].CumUSD
		}
	}
	if *asJSON {
		return writeJSON(a.Stdout, out)
	}
	if *show {
		limit := "unlimited"
		if cfg.Trace.Budget.SessionUSD != nil {
			limit = fmt.Sprintf("%.2f", *cfg.Trace.Budget.SessionUSD)
		}
		fmt.Fprintf(a.Stdout, "session budget %s usd  soft %d%%  compact %d%%  hard %d%% (%s)\n", limit, cfg.Trace.Budget.SoftPct, cfg.Trace.Budget.CompactPct, cfg.Trace.Budget.HardPct, cfg.Trace.Budget.HardAction)
		if v, ok := out["session_usd"]; ok {
			fmt.Fprintf(a.Stdout, "latest session %v spent %.4f usd\n", out["session"], v)
		}
	}
	return nil
}

// setSessionUSD rewrites the session_usd line of [trace.budget] in place,
// keeping the rest of config.toml untouched.
func setSessionUSD(s *store.Store, v float64) error {
	p := s.Path("config.toml")
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	inBudget, done := false, false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			if inBudget && !done {
				lines = append(lines[:i], append([]string{fmt.Sprintf("session_usd = %.2f", v)}, lines[i:]...)...)
				done = true
				break
			}
			inBudget = t == "[trace.budget]"
			continue
		}
		if inBudget && (strings.HasPrefix(t, "session_usd") || strings.HasPrefix(t, "# session_usd")) {
			lines[i] = fmt.Sprintf("session_usd = %.2f", v)
			done = true
			break
		}
	}
	if !done {
		if inBudget {
			lines = append(lines, fmt.Sprintf("session_usd = %.2f", v))
		} else {
			lines = append(lines, "", "[trace.budget]", fmt.Sprintf("session_usd = %.2f", v))
		}
	}
	return store.WriteFileAtomic(p, []byte(strings.Join(lines, "\n")), 0o600)
}

func (a *App) traceVerify(args []string) error {
	fs := a.flags("trace verify")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	session, err := a.pickSession(s, positional(fs))
	if err != nil {
		return err
	}
	res, err := trace.Verify(trace.SessionDir(s, session))
	if err != nil {
		var ve *trace.VerifyError
		if errors.As(err, &ve) {
			return cli.Wrap(cli.ExitIntegrity, "", err)
		}
		return cli.Wrap(cli.ExitEnvironment, "verify", err)
	}
	fmt.Fprintf(a.Stdout, "session %s: %d events in %d segments, %d blobs, head %s\n", session, res.Events, res.Segments, res.Blobs, res.Head)
	return nil
}

func (a *App) tracePrices(args []string) error {
	if len(args) == 0 || args[0] == "show" {
		s, err := a.store()
		if err != nil {
			return err
		}
		t, err := trace.LoadPrices(s)
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "prices", err)
		}
		fmt.Fprintf(a.Stdout, "%s observed %s  %s\n", trace.PricesSchema, t.Observed, t.Hash)
		fmt.Fprintf(a.Stdout, "%-28s %-10s %8s %8s %10s %10s %10s\n", "model", "vendor", "in", "out", "cache_rd", "write_5m", "write_1h")
		for _, m := range t.Models {
			fmt.Fprintf(a.Stdout, "%-28s %-10s %8s %8s %10s %10s %10s\n", m.ID, m.Vendor, fnum(m.In), fnum(m.Out), fnum(m.CacheRead), fnum(m.CacheWrite5m), fnum(m.CacheWrite1h))
		}
		return nil
	}
	if args[0] == "use" && len(args) == 2 {
		s, err := a.store()
		if err != nil {
			return err
		}
		t, err := trace.UsePrices(s, args[1])
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "prices use", err)
		}
		fmt.Fprintf(a.Stdout, "pinned price table %s observed %s\n", t.Hash, t.Observed)
		return nil
	}
	return cli.Errorf(cli.ExitUsage, "usage: saga trace prices show | use <file>")
}

func fnum(p *float64) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprintf("%.4g", *p)
}

func (a *App) cmdDoctor(args []string) error {
	fs := a.flags("doctor")
	asJSON := fs.Bool("json", false, "emit saga.doctor/1")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	s, _ := store.Find(a.Cwd)
	in := trace.DoctorInput{Version: a.Version, Store: s, HarnessName: "claude", InstallHarness: claudecode.Harness}
	if p, err := exec.LookPath("claude"); err == nil {
		in.HarnessFound = true
		in.HarnessVersion = p
		if out, err := exec.Command(p, "--version").Output(); err == nil {
			in.HarnessVersion = strings.TrimSpace(string(out))
		}
	}
	files := []string{claudecode.SettingsFile(s.Root, false), claudecode.SettingsFile(s.Root, true), claudecode.UserSettingsFile()}
	in.SettingsFiles = files
	for _, r := range claudecode.Registration(files) {
		in.Hooks = append(in.Hooks, trace.HookRegistration{Event: r.Event, Registered: r.Registered, File: r.File, Command: r.Command})
	}
	rep, code := trace.Doctor(in)
	if err := rep.Validate(); err != nil {
		return cli.Wrap(cli.ExitUsage, "doctor report", err)
	}
	if *asJSON {
		if err := writeJSON(a.Stdout, rep); err != nil {
			return err
		}
	} else {
		fmt.Fprint(a.Stdout, rep.Text())
	}
	if code != cli.ExitOK {
		return &cli.Error{Code: code, Msg: "doctor: " + code.String()}
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
