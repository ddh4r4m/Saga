package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/store"
)

const gateUsage = `usage: saga gate <subcommand>

  init        [--request <file|->] [--base <rev>]
  status      [--strict] [--json] [--out <file>]
  check       [--gate <id>,...] [--approve] [--no-require-red] [--advisory] [--timeout <s>] [--jobs <n>] [--json]
  reverify    [--gate <id>,...] [--base <rev>] [--ci] [--json]
  attest      <id> --note <text>
  approve     [--gate <id>,...] [--revoke]
  lint        [--strict] [--json]
  guard-diff  [--base <rev>] [--incremental] [--predict --path <p>] [--guard <id>,...] [--advisory] [--json]
  install | uninstall
`

func (a *App) cmdGate(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.Stdout, gateUsage)
		return cli.Errorf(cli.ExitUsage, "gate: subcommand required")
	}
	switch args[0] {
	case "init":
		return a.gateInit(args[1:])
	case "status":
		return a.gateStatus(args[1:])
	case "check":
		return a.gateCheck(args[1:], false)
	case "reverify":
		return a.gateCheck(args[1:], true)
	case "attest":
		return a.gateAttest(args[1:])
	case "approve":
		return a.gateApprove(args[1:])
	case "lint":
		return a.gateLint(args[1:])
	case "guard-diff":
		return a.gateGuardDiff(args[1:])
	case "install", "uninstall":
		return a.gateInstall(args[0] == "install")
	}
	fmt.Fprint(a.Stderr, gateUsage)
	return cli.Errorf(cli.ExitUsage, "gate: unknown subcommand %q", args[0])
}

func splitIDs(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) gateStore() (*store.Store, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// emit writes the report as text or JSON (to stdout or --out) and
// returns the report's exit code as an error.
func (a *App) emit(rep *gate.Report, asJSON bool, outFile string) error {
	if err := rep.Validate(); err != nil {
		return cli.Wrap(cli.ExitUsage, "status report", err)
	}
	if asJSON {
		var w io.Writer = a.Stdout
		if outFile != "" {
			f, err := os.Create(outFile)
			if err != nil {
				return cli.Wrap(cli.ExitEnvironment, "--out", err)
			}
			defer f.Close()
			w = f
		}
		if err := writeJSON(w, rep); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "json", err)
		}
	} else {
		for _, l := range rep.Lines {
			fmt.Fprintln(a.Stdout, l)
		}
	}
	if rep.Exit != 0 {
		return &cli.Error{Code: cli.Code(rep.Exit), Msg: "gate: " + cli.Code(rep.Exit).String()}
	}
	return nil
}

func (a *App) gateInit(args []string) error {
	fs := a.flags("gate init")
	request := fs.String("request", "", "request file to copy verbatim to .saga/request.md (- for stdin)")
	base := fs.String("base", "", "git rev to write as BASE:")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	root := store.RepoRoot(a.Cwd)
	if _, err := store.Init(root); err != nil {
		return mapShape(cli.Wrap(cli.ExitEnvironment, "init", err))
	}
	s := store.Open(root)
	m, _, _ := s.ReadManifest()
	if !contains(m.Layers, "gate") {
		m.Layers = append(m.Layers, "gate")
		if err := s.WriteManifest(m); err != nil {
			return cli.Wrap(cli.ExitEnvironment, "manifest", err)
		}
	}
	var reqHash string
	if *request != "" {
		var raw []byte
		var err error
		if *request == "-" {
			raw, err = io.ReadAll(a.Stdin)
		} else {
			raw, err = os.ReadFile(*request)
		}
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "--request", err)
		}
		p := s.Path("request.md")
		if existing, err := os.ReadFile(p); err == nil && string(existing) != string(raw) {
			return cli.Errorf(cli.ExitUsage, "%s exists with different content; remove it first", p)
		}
		if err := store.WriteFileAtomic(p, raw, 0o644); err != nil {
			return mapShape(cli.Wrap(cli.ExitEnvironment, "request", err))
		}
		reqHash = canon.SHA256(raw)
		fmt.Fprint(a.Stdout, gate.Numbered(string(raw)))
	}
	cp := s.Path("contract.md")
	if _, err := os.Lstat(cp); err == nil {
		fmt.Fprintf(a.Stdout, "%s already exists\n", cp)
		return nil
	}
	var b strings.Builder
	b.WriteString("# Contract: " + filepath.Base(root) + "\n\n")
	if reqHash != "" {
		b.WriteString("REQUEST: " + reqHash + "\n")
	}
	b.WriteString("IN: src/**, tests/**\n")
	if *base != "" {
		b.WriteString("BASE: " + *base + "\n")
	}
	b.WriteString("\n- [ ] G1: state the outcome that can fail\n    CHECK: false\n    EXPECT: replace this oracle\n")
	if reqHash != "" {
		b.WriteString("    FROM: R1 \"quote a span of R1 verbatim\"\n")
	}
	if err := store.WriteFileAtomic(cp, []byte(b.String()), 0o644); err != nil {
		return mapShape(cli.Wrap(cli.ExitEnvironment, "contract", err))
	}
	fmt.Fprintf(a.Stdout, "wrote %s skeleton; edit it, then run saga gate lint\n", cp)
	return nil
}

func (a *App) load() (*gate.Loaded, error) {
	s, err := a.gateStore()
	if err != nil {
		return nil, err
	}
	l, err := gate.Load(s.Root, s)
	if err != nil {
		return l, mapShape(err)
	}
	return l, nil
}

func (a *App) gateStatus(args []string) error {
	fs := a.flags("gate status")
	strict := fs.Bool("strict", false, "uncovered sentences exit 1")
	asJSON := fs.Bool("json", false, "emit saga.gate.status/1")
	out := fs.String("out", "", "write JSON to file")
	advisory := fs.Bool("advisory", false, "include the experimental guards")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	l, err := a.load()
	if err != nil {
		return err
	}
	if err := l.CheckLedger(); err != nil {
		return err
	}
	rep, err := gate.Status(l, gate.StatusOptions{Strict: *strict, Advisory: *advisory})
	if err != nil {
		return err
	}
	rep.Lint = gate.Lint(l.Contract)
	return a.emit(rep, *asJSON, *out)
}

func (a *App) gateCheck(args []string, reverify bool) error {
	name := "gate check"
	if reverify {
		name = "gate reverify"
	}
	fs := a.flags(name)
	gates := fs.String("gate", "", "comma-separated gate ids")
	approve := fs.Bool("approve", false, "record approval for every selected oracle first (human only)")
	noRed := fs.Bool("no-require-red", false, "downgrade unproven gates to a warning")
	advisory := fs.Bool("advisory", false, "include the experimental guards")
	timeout := fs.Int("timeout", 0, "per-gate timeout in seconds")
	fs.Int("jobs", 1, "accepted; gates run sequentially at M0")
	base := fs.String("base", "", "reverify: override BASE:")
	ci := fs.Bool("ci", false, "reverify: execute without an approval store")
	asJSON := fs.Bool("json", false, "emit saga.gate.status/1")
	out := fs.String("out", "", "write JSON to file")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if !reverify && (*base != "" || *ci) {
		return cli.Errorf(cli.ExitUsage, "check: --base and --ci belong to reverify")
	}
	l, err := a.load()
	if err != nil {
		return err
	}
	if *base != "" {
		l.Contract.Base = *base
		b, err := gate.ResolveBase(l.Root, *base)
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "", err)
		}
		l.Base, l.BaseDisplay = b, *base
	}
	opts := gate.CheckOptions{Only: splitIDs(*gates), Approve: *approve, NoRequireRed: *noRed, Advisory: *advisory, TimeoutS: *timeout, CI: *ci, Reverify: reverify, Log: func(m string) { fmt.Fprintln(a.Stderr, "saga gate: "+m) }}
	rep, err := gate.Check(l, opts)
	if err != nil {
		return mapShape(err)
	}
	return a.emit(rep, *asJSON, *out)
}

func (a *App) gateAttest(args []string) error {
	fs := a.flags("gate attest")
	note := fs.String("note", "", "attestation note (required)")
	asJSON := fs.Bool("json", false, "emit saga.gate.status/1")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id := positional(fs)
	if id == "" {
		return cli.Errorf(cli.ExitUsage, "usage: saga gate attest <id> --note <text>")
	}
	l, err := a.load()
	if err != nil {
		return err
	}
	rep, err := gate.Attest(l, id, *note)
	if err != nil {
		return mapShape(err)
	}
	return a.emit(rep, *asJSON, "")
}

func (a *App) gateApprove(args []string) error {
	fs := a.flags("gate approve")
	gates := fs.String("gate", "", "comma-separated gate ids")
	revoke := fs.Bool("revoke", false, "remove the approval records instead")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	l, err := a.load()
	if err != nil {
		return err
	}
	lines, err := gate.Approve(l, splitIDs(*gates), *revoke)
	for _, ln := range lines {
		fmt.Fprintln(a.Stdout, ln)
	}
	return mapShape(err)
}

func (a *App) gateLint(args []string) error {
	fs := a.flags("gate lint")
	strict := fs.Bool("strict", false, "warnings exit 1")
	asJSON := fs.Bool("json", false, "emit warnings as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	l, err := a.load()
	if err != nil {
		return err
	}
	if err := l.CheckLedger(); err != nil {
		return err
	}
	warnings := gate.Lint(l.Contract)
	if *asJSON {
		if warnings == nil {
			warnings = []gate.LintWarning{}
		}
		if err := writeJSON(a.Stdout, map[string]any{"schema": "saga.gate.lint/1", "warnings": warnings, "count": len(warnings)}); err != nil {
			return err
		}
	} else {
		for _, w := range warnings {
			if w.Gate != "" {
				fmt.Fprintf(a.Stdout, "%s %s: %s\n", w.Rule, w.Gate, w.Msg)
			} else {
				fmt.Fprintf(a.Stdout, "%s: %s\n", w.Rule, w.Msg)
			}
		}
		if len(warnings) == 0 {
			fmt.Fprintln(a.Stdout, "LINT OK")
		} else {
			fmt.Fprintf(a.Stdout, "%d warnings\n", len(warnings))
		}
	}
	if *strict && len(warnings) > 0 {
		return cli.Errorf(cli.ExitFinding, "lint: %d warnings under --strict", len(warnings))
	}
	return nil
}

func (a *App) gateGuardDiff(args []string) error {
	fs := a.flags("gate guard-diff")
	base := fs.String("base", "", "override BASE:")
	fs.Bool("incremental", false, "accepted; the diff is always cheap enough to run in full")
	predict := fs.Bool("predict", false, "G-SCOPE prediction for --path")
	path := fs.String("path", "", "path for --predict")
	guards := fs.String("guard", "", "comma-separated guard ids")
	advisory := fs.Bool("advisory", false, "include the experimental guards")
	asJSON := fs.Bool("json", false, "emit findings as JSON")
	out := fs.String("out", "", "write JSON to file")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	l, err := a.load()
	if err != nil {
		return err
	}
	if *base != "" {
		b, err := gate.ResolveBase(l.Root, *base)
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "", err)
		}
		l.Base = b
	}
	in := gate.GuardInput{Root: l.Root, Store: l.Store, Base: l.Base, Contract: l.Contract, Config: l.Config, Only: splitIDs(*guards), Advisory: *advisory}
	var findings []gate.Finding
	if *predict {
		if *path == "" {
			return cli.Errorf(cli.ExitUsage, "--predict needs --path")
		}
		if f := gate.PredictScope(in, *path); f != nil {
			findings = append(findings, *f)
		}
	} else {
		findings, err = gate.GuardDiff(in)
		if err != nil {
			return cli.Wrap(cli.ExitEnvironment, "guard-diff", err)
		}
	}
	if findings == nil {
		findings = []gate.Finding{}
	}
	blocking := 0
	for _, f := range findings {
		if f.Blocks() {
			blocking++
		}
	}
	if *asJSON {
		var w io.Writer = a.Stdout
		if *out != "" {
			f, err := os.Create(*out)
			if err != nil {
				return cli.Wrap(cli.ExitEnvironment, "--out", err)
			}
			defer f.Close()
			w = f
		}
		if err := writeJSON(w, map[string]any{"schema": "saga.gate.guards/1", "base": gate.ShortRev(l.Base), "findings": findings, "blocking": blocking}); err != nil {
			return err
		}
	} else {
		for _, f := range findings {
			state := "BLOCKS"
			if f.Waived {
				state = "waived"
			} else if f.Advisory {
				state = "advisory"
			}
			fmt.Fprintf(a.Stdout, "%-11s %-8s %s %s %s pre=%d post=%d\n", f.ID, state, f.Path, f.Hunk, f.Rule, f.Pre, f.Post)
		}
		if blocking == 0 {
			fmt.Fprintln(a.Stdout, "GUARDS CLEAN")
		}
	}
	if blocking > 0 {
		return cli.Errorf(cli.ExitRefusal, "guard-diff: %d finding(s) without a valid waiver", blocking)
	}
	return nil
}

func (a *App) gateInstall(install bool) error {
	s, err := a.gateStore()
	if err != nil {
		return err
	}
	m, _, err := s.ReadManifest()
	if err != nil {
		return cli.Wrap(cli.ExitUsage, "manifest", err)
	}
	var layers []string
	for _, l := range m.Layers {
		if l != "gate" {
			layers = append(layers, l)
		}
	}
	if install {
		layers = append(layers, "gate")
	}
	m.Layers = layers
	if err := s.WriteManifest(m); err != nil {
		return cli.Wrap(cli.ExitEnvironment, "manifest", err)
	}
	fmt.Fprintf(a.Stdout, "manifest layers: %s\n", strings.Join(m.Layers, ", "))
	return nil
}
