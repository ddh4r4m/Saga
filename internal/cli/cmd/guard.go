package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	claudecode "github.com/ddh4r4m/saga/adapters/claude-code"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/guard"
	"github.com/ddh4r4m/saga/internal/harness/claude"
	"github.com/ddh4r4m/saga/internal/hookio"
)

const guardUsage = `saga guard: command validation (guard-spec section 2)

usage:
  saga guard check-cmd [--shell bash|zsh|sh] [--cwd DIR] [--permission-mode M] [--env-file F] [--json] -- <command>
  saga guard hook <harness> <event>         deny-only safety hook; reads the payload on stdin
  saga guard policy show [--json]           effective policy, sources and trust state
  saga guard policy validate [FILE]         parse a policy file; repo policies must only tighten

exit codes: 0 allow, 1 ask, 2 usage, parse or policy failure (fail closed), 3 deny
not implemented yet: pwsh and cmd shells, exec --mask, snapshot, mask, deps, mcp, audit
`

func (a *App) cmdGuard(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.Stdout, guardUsage)
		return cli.Errorf(cli.ExitUsage, "guard: subcommand required")
	}
	switch args[0] {
	case "check-cmd":
		return a.guardCheckCmd(args[1:])
	case "hook":
		return a.guardHook(args[1:])
	case "policy":
		return a.guardPolicy(args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(a.Stdout, guardUsage)
		return nil
	}
	fmt.Fprint(a.Stderr, guardUsage)
	return cli.Errorf(cli.ExitUsage, "guard: unknown subcommand %q", args[0])
}

// splitDashDash separates flags from the command after `--`.
func splitDashDash(args []string) (flags, cmd []string) {
	for i, x := range args {
		if x == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// guardHook is `saga guard hook claude-code PreToolUse`: the deny-only
// safety net of docs/12 row 6, registered identically in both bench arms
// so it is never a treatment. It is deliberately not the composed
// `saga hook` chain: it takes no snapshot, reads no policy file, and
// touches nothing under .saga. A hard deny (D1 to D11) renders the
// documented deny JSON; everything else, including an ask verdict and
// any internal error, renders {} and exits 0, because a false deny in
// one arm would be exactly the asymmetry this hook exists to avoid.
func (a *App) guardHook(args []string) error {
	fs := a.flags("guard hook")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	rest := positionals[fs]
	if len(rest) != 2 {
		return cli.Errorf(cli.ExitUsage, "usage: saga guard hook <harness> <event>")
	}
	harness, event := rest[0], rest[1]
	if harness != claudecode.Harness {
		return cli.Errorf(cli.ExitUsage, "guard hook: only %s is implemented", claudecode.Harness)
	}
	raw, err := io.ReadAll(a.Stdin)
	if err != nil {
		fmt.Fprintln(a.Stdout, "{}")
		return nil
	}
	in, perr := claude.Parse(event, raw)
	if perr != nil || in == nil {
		// Fail open, and say so in the log: the payload is the harness's,
		// and a parse failure is our problem, not the agent's.
		guard.LogSafetyAt(os.Getenv(guard.LogEnv), "", "", "", guard.SafetyDecision{Err: fmt.Errorf("payload: %v", perr)}, &a.Started)
		fmt.Fprintln(a.Stdout, "{}")
		return nil
	}
	if in.Event != hookio.EventPreToolUse {
		fmt.Fprintln(a.Stdout, "{}")
		return nil
	}
	cwd := in.Cwd
	if cwd == "" {
		cwd = a.Cwd
	}
	d := guard.SafetyCheck(in.ToolName, in.ToolInput, cwd, os.Environ())
	guard.LogSafetyAt(os.Getenv(guard.LogEnv), in.SessionID, in.ToolUseID, commandOf(in.ToolInput), d, &a.Started)
	if !d.Deny {
		fmt.Fprintln(a.Stdout, "{}")
		return nil
	}
	out := map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       "deny",
		"permissionDecisionReason": d.Reason(),
	}}
	return writeJSON(a.Stdout, out)
}

// commandOf pulls the command text out of a tool input for hashing; it
// is never logged in the clear.
func commandOf(toolInput json.RawMessage) string {
	var in struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(toolInput, &in)
	return in.Command
}

func (a *App) guardCheckCmd(args []string) error {
	flagArgs, cmdArgs := splitDashDash(args)
	fs := a.flags("guard check-cmd")
	shell := fs.String("shell", "", "shell the harness will use: bash, zsh or sh (empty: assumed bash, allow becomes ask)")
	cwd := fs.String("cwd", "", "working directory the command runs in")
	mode := fs.String("permission-mode", "default", "harness permission mode (acceptEdits grants in-scope edits)")
	envFile := fs.String("env-file", "", "KEY=VALUE lines to use as the shell environment instead of the process environment")
	asJSON := fs.Bool("json", false, "emit the saga.guard.decision/1 record")
	if err := parseFlags(fs, flagArgs); err != nil {
		return err
	}
	if len(cmdArgs) == 0 {
		if pos := positionals[fs]; len(pos) > 0 {
			cmdArgs = pos
		} else {
			return cli.Errorf(cli.ExitUsage, "guard check-cmd: no command after --")
		}
	}
	req := guard.Request{Command: strings.Join(cmdArgs, " "), Shell: *shell, Cwd: *cwd, PermissionMode: *mode}
	if req.Cwd == "" {
		req.Cwd = a.Cwd
	}
	if *envFile != "" {
		raw, err := os.ReadFile(*envFile)
		if err != nil {
			return cli.Wrap(cli.ExitEnvironment, "--env-file", err)
		}
		req.Env = []string{}
		for _, line := range strings.Split(string(raw), "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				req.Env = append(req.Env, line)
			}
		}
	}
	d, err := guard.Check(req)
	var perr *guard.ParseError
	if err != nil && !errors.As(err, &perr) {
		return err
	}
	if d != nil {
		if *asJSON {
			if verr := d.Validate(); verr != nil {
				return cli.Wrap(cli.ExitUsage, "decision record", verr)
			}
			if werr := writeJSON(a.Stdout, d); werr != nil {
				return cli.Wrap(cli.ExitEnvironment, "json", werr)
			}
		} else {
			fmt.Fprintf(a.Stdout, "%s: %s\n", d.Decision, d.Reason)
			for _, s := range d.Segments {
				rule := s.Rule
				if rule == "" {
					rule = "-"
				}
				fmt.Fprintf(a.Stdout, "  %-5s %-20s %-4s %s\n", s.Verdict, s.Class, rule, strings.Join(s.Argv, " "))
			}
		}
	}
	if perr != nil {
		return cli.Wrap(cli.ExitUsage, "guard", perr)
	}
	if code := d.ExitCode(); code != cli.ExitOK {
		return &cli.Error{Code: code, Msg: "guard: " + string(d.Decision)}
	}
	return nil
}

func (a *App) guardPolicy(args []string) error {
	if len(args) == 0 {
		return cli.Errorf(cli.ExitUsage, "guard policy: show or validate")
	}
	switch args[0] {
	case "show":
		fs := a.flags("guard policy show")
		asJSON := fs.Bool("json", false, "emit the effective policy as JSON")
		if err := parseFlags(fs, args[1:]); err != nil {
			return err
		}
		root := guard.RepoRoot(a.Cwd)
		lp, err := guard.LoadPolicy(nil, root)
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "policy", err)
		}
		if *asJSON {
			return writeJSON(a.Stdout, map[string]any{
				"schema": guard.PolicySchema, "hash": lp.Policy.Hash(), "user_file": lp.UserFile, "repo_file": lp.RepoFile,
				"repo_hash": lp.RepoHash, "repo_trusted": lp.RepoTrusted, "repo_applied": lp.RepoApplied, "notes": lp.Notes,
				"policy": lp.Policy,
			})
		}
		fmt.Fprintf(a.Stdout, "policy hash  %s\n", lp.Policy.Hash())
		fmt.Fprintf(a.Stdout, "user policy  %s\n", orNone(lp.UserFile))
		fmt.Fprintf(a.Stdout, "repo policy  %s\n", orNone(lp.RepoFile))
		if lp.RepoFile != "" {
			fmt.Fprintf(a.Stdout, "repo hash    %s trusted=%v applied=%v\n", lp.RepoHash, lp.RepoTrusted, lp.RepoApplied)
		}
		for _, n := range lp.Notes {
			fmt.Fprintf(a.Stdout, "note         %s\n", n)
		}
		p := lp.Policy
		fmt.Fprintf(a.Stdout, "net.fetch    %v\n", p.NetFetch())
		fmt.Fprintf(a.Stdout, "scope.extra  %s\n", strings.Join(p.Scope.Extra, ", "))
		fmt.Fprintf(a.Stdout, "git.protected %s\n", strings.Join(p.Git.Protected, ", "))
		fmt.Fprintf(a.Stdout, "allow.read   %s\n", strings.Join(p.Allow.Read, " | "))
		fmt.Fprintf(a.Stdout, "allow.mutate_in_scope %s\n", strings.Join(p.Allow.MutateInScope, " | "))
		fmt.Fprintf(a.Stdout, "allow.mutate_out_of_scope %s\n", strings.Join(p.Allow.MutateOutOfScope, " | "))
		fmt.Fprintf(a.Stdout, "deny.extra   %s\n", strings.Join(p.Deny.Extra, " | "))
		fmt.Fprintf(a.Stdout, "ask          interpreter_exec_untracked=%v unresolvable=%v sudo=%v\n", p.AskInterpreterUntracked(), p.AskUnresolvable(), p.AskSudo())
		fmt.Fprintf(a.Stdout, "credentials  %d paths (%d extra), allow_read %d\n", len(p.CredentialPaths()), len(p.Credentials.PathsExtra), len(p.Credentials.AllowRead))
		if lp.RepoFile != "" && !lp.RepoTrusted {
			return cli.Errorf(cli.ExitApproval, "repo policy %s is not trusted", lp.RepoFile)
		}
		return nil
	case "validate":
		file := ""
		if len(args) > 1 {
			file = args[1]
		}
		if file == "" {
			root := guard.RepoRoot(a.Cwd)
			for _, rel := range guard.PolicyFiles {
				if _, err := os.Stat(root + "/" + rel); err == nil {
					file = root + "/" + rel
					break
				}
			}
			if file == "" {
				return cli.Errorf(cli.ExitUsage, "guard policy validate: no policy file (.saga/policy.toml)")
			}
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return cli.Wrap(cli.ExitEnvironment, "read", err)
		}
		p, err := guard.ParsePolicy(raw)
		if err != nil {
			return cli.Wrap(cli.ExitUsage, file, err)
		}
		loos := guard.Loosenings(p)
		fmt.Fprintf(a.Stdout, "%s: valid %s\n", file, guard.PolicySchema)
		if len(loos) > 0 {
			for _, l := range loos {
				fmt.Fprintf(a.Stdout, "  loosens %s\n", l)
			}
			if strings.Contains(file, ".saga/") {
				return cli.Errorf(cli.ExitUsage, "repo policy touches a fixed table (%d field(s)); only a user policy may loosen", len(loos))
			}
		}
		return nil
	}
	return cli.Errorf(cli.ExitUsage, "guard policy: unknown subcommand %q", args[0])
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
