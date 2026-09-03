// Package guard implements the command validation of guard-spec section 2:
// a shell command is parsed with the target shell's grammar, expanded
// without execution (tilde, parameters, globs, braces, quotes), split
// into segments, unwrapped through sudo, env, sh -c, xargs, find -exec,
// npm run and the other wrappers of section 2.3, and every segment is
// classified on its resolved argv against the hard-deny list D1 to D11
// and the allow-list policy. The decision is the maximum severity over
// segments (CVE-2026-25723: one bad pipe stage fails the pipe).
//
// Guard is deterministic and never runs anything: command substitutions
// are classified as their own segments and mark the outer segment
// unresolvable, globs are expanded with a read-only, capped readdir, and
// the only process guard spawns is `git config --get alias.<x>` for git
// aliases. PowerShell and cmd are not implemented yet (see
// docs/specs/IMPLEMENTATION-STATUS.md).
package guard

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/schema"
)

// DecisionSchema is the schema id of the decision record.
const DecisionSchema = "saga.guard.decision/1"

// Class is a segment class (guard-spec 2.4).
type Class string

// The seven classes.
const (
	ClassRead         Class = "read"
	ClassMutateIn     Class = "mutate_in_scope"
	ClassMutateOut    Class = "mutate_out_of_scope"
	ClassDestructive  Class = "destructive"
	ClassInterpreter  Class = "interpreter_exec"
	ClassNetworkFetch Class = "network_fetch"
	ClassUnresolvable Class = "unresolvable"
)

// Verdict is allow, ask or deny.
type Verdict string

// The verdicts, in severity order.
const (
	Allow Verdict = "allow"
	Ask   Verdict = "ask"
	Deny  Verdict = "deny"
)

func severity(v Verdict) int {
	switch v {
	case Ask:
		return 1
	case Deny:
		return 2
	}
	return 0
}

func verdictOf(sev int) Verdict {
	switch {
	case sev >= 2:
		return Deny
	case sev == 1:
		return Ask
	}
	return Allow
}

// Request is one command to classify.
type Request struct {
	// Command is the raw command string as the harness will pass it to
	// the shell.
	Command string
	// Shell is bash, zsh or sh. Empty means the adapter could not tell:
	// bash is assumed and allow downgrades to ask (guard-spec 2.1).
	Shell string
	// Cwd is the working directory the shell would start in.
	Cwd string
	// Env is the environment the shell would see (KEY=VALUE). Nil means
	// the process environment.
	Env []string
	// PermissionMode is the harness permission mode; acceptEdits,
	// bypassPermissions, dontAsk and auto grant in-scope edits without a
	// prompt (guard-spec 2.6).
	PermissionMode string
	// Policy overrides the loaded policy (tests). Nil loads it.
	Policy *Policy
	// PolicyHash overrides the recorded hash when Policy is set.
	PolicyHash string
	// RepoRoot overrides repository discovery from Cwd.
	RepoRoot string
	// Notes are carried into the decision record.
	Notes []string
}

// Segment is one classified simple command.
type Segment struct {
	Raw        string   `json:"raw"`
	Argv       []string `json:"argv"`
	Class      Class    `json:"class"`
	Rule       string   `json:"rule,omitempty"`
	Paths      []string `json:"paths"`
	Unresolved []string `json:"unresolved"`
	Reason     string   `json:"reason,omitempty"`
	// Verdict is the per-segment severity after policy.
	Verdict Verdict `json:"verdict"`

	// internal
	sudo            bool
	remote          bool // runs on a remote host or in a container
	remoteExec      bool // the segment itself is the remote executor (ssh, docker exec)
	globRoots       []string
	writes          []string
	truncates       []string
	reads           []string
	stdinFrom       *Segment
	depth           int
	shellBody       bool // sh -c or heredoc body already unwrapped into child segments
	assignOnly      bool
	argvClean       []bool
	argvSubst       []string
	lastSubst       string
	heredoc         string
	fetchSubst      bool
	procFetch       bool
	scriptFile      string
	interpreterBody bool
	xargs           bool
	xargsFrom       *Segment
	find            *findSpec
	fromFind        bool
}

// SnapshotInfo is the snapshot field of the decision record. Snapshots
// are not wired into check-cmd yet, so taken is always false.
type SnapshotInfo struct {
	Taken bool `json:"taken"`
	Turn  *int `json:"turn"`
}

// Decision is the saga.guard.decision/1 record (guard-spec 2.6).
type Decision struct {
	Schema       string       `json:"schema"`
	Decision     Verdict      `json:"decision"`
	Reason       string       `json:"reason"`
	Shell        string       `json:"shell"`
	ShellAssumed bool         `json:"shell_assumed"`
	Segments     []Segment    `json:"segments"`
	Rules        []string     `json:"rules"`
	Snapshot     SnapshotInfo `json:"snapshot"`
	PolicyHash   string       `json:"policy_hash"`
	ElapsedMS    int64        `json:"elapsed_ms"`
	Notes        []string     `json:"notes,omitempty"`
}

// ExitCode maps the decision onto the uniform table: allow 0, ask 1,
// deny 3.
func (d *Decision) ExitCode() cli.Code {
	switch d.Decision {
	case Deny:
		return cli.ExitRefusal
	case Ask:
		return cli.ExitFinding
	}
	return cli.ExitOK
}

// Validate checks the record against the embedded schema.
func (d *Decision) Validate() error {
	v, err := schema.Normalize(d)
	if err != nil {
		return err
	}
	return schema.ValidateID(DecisionSchema, v)
}

// ParseError is a command the shell grammar rejects. The caller fails
// closed (exit 2).
type ParseError struct {
	Shell string
	Err   error
}

func (e *ParseError) Error() string { return fmt.Sprintf("%s parse: %v", e.Shell, e.Err) }
func (e *ParseError) Unwrap() error { return e.Err }

// reasonCap is the harness envelope cap of 150 tokens (guard-spec 2.6).
const reasonCap = 150 * 4

// Check classifies one command. A grammar error returns a deny decision
// and a *ParseError; a policy error returns the error alone.
func Check(req Request) (*Decision, error) {
	start := time.Now()
	shell := req.Shell
	assumed := false
	switch shell {
	case "bash", "zsh", "sh":
	case "":
		shell, assumed = "bash", true
	default:
		return nil, cli.Errorf(cli.ExitUsage, "unknown shell %q (bash, zsh or sh)", req.Shell)
	}
	env := envMap(req.Env)
	cwd := req.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	root := req.RepoRoot
	if root == "" {
		root = repoRootFor(cwd)
	}
	pol := req.Policy
	hash := req.PolicyHash
	notes := append([]string(nil), req.Notes...)
	if pol == nil {
		lp, err := LoadPolicy(env, root)
		if err != nil {
			return nil, cli.Wrap(cli.ExitUsage, "policy", err)
		}
		pol = lp.Policy
		notes = append(notes, lp.Notes...)
	}
	if hash == "" {
		hash = pol.Hash()
	}
	home := env["HOME"]
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	d := &Decision{Schema: DecisionSchema, Shell: shell, ShellAssumed: assumed, PolicyHash: hash, Segments: []Segment{}, Rules: []string{}}
	ex := newExpander(shell, cwd, home, root, env, pol)
	segs, perr := ex.run(req.Command)
	if perr != nil {
		d.Decision = Deny
		d.Reason = "parse failure (fail closed): " + perr.Error()
		d.Notes = notes
		d.ElapsedMS = time.Since(start).Milliseconds()
		return d, &ParseError{Shell: shell, Err: perr}
	}
	grant := grantsEdits(req.PermissionMode)
	cl := &classifier{scope: ex.scope, pol: pol, shell: shell, cwd: cwd, root: root, env: env}
	sev := 0
	var reasons []string
	rules := map[string]bool{}
	for _, s := range segs {
		cl.classify(s)
		v := decide(s, pol, grant)
		if assumed && v == Allow {
			v = Ask
		}
		s.Verdict = v
		if severity(v) > sev {
			sev = severity(v)
		}
		if s.Rule != "" {
			rules[s.Rule] = true
		}
		if v != Allow && s.Reason != "" {
			reasons = append(reasons, s.Reason)
		}
		if s.Paths == nil {
			s.Paths = []string{}
		}
		if s.Unresolved == nil {
			s.Unresolved = []string{}
		}
		if s.Argv == nil {
			s.Argv = []string{}
		}
		d.Segments = append(d.Segments, *s)
	}
	d.Decision = verdictOf(sev)
	for r := range rules {
		d.Rules = append(d.Rules, r)
	}
	sort.Strings(d.Rules)
	// Deny reasons first, then the rest.
	sort.SliceStable(reasons, func(i, j int) bool {
		return strings.HasPrefix(reasons[i], "D") && !strings.HasPrefix(reasons[j], "D")
	})
	switch {
	case d.Decision == Allow:
		d.Reason = "allow"
	case len(reasons) == 0 && assumed:
		d.Reason = "shell not known to the adapter: bash assumed, allow downgraded to ask"
	default:
		d.Reason = strings.Join(reasons, " | ")
	}
	if assumed && d.Decision == Ask && len(reasons) == 0 {
		notes = append(notes, "shell_assumed")
	}
	if len(d.Reason) > reasonCap {
		d.Reason = d.Reason[:reasonCap-3] + "..."
	}
	d.Reason = canon.CleanText(d.Reason)
	d.Notes = notes
	d.ElapsedMS = time.Since(start).Milliseconds()
	return d, nil
}

// decide applies guard-spec 2.6 to one classified segment.
func decide(s *Segment, pol *Policy, grant bool) Verdict {
	var v Verdict
	switch s.Class {
	case ClassRead:
		v = Allow
	case ClassMutateIn:
		if matchAny(pol.Allow.MutateInScope, s.Argv) || grant {
			v = Allow
		} else {
			v = Ask
			if s.Reason == "" {
				s.Reason = "mutate_in_scope: no allow rule and the permission mode does not grant edits"
			}
		}
	case ClassNetworkFetch:
		if pol.NetFetch() {
			v = Allow
		} else {
			v = Ask
			if s.Reason == "" {
				s.Reason = "network_fetch: net.fetch is false"
			}
		}
	case ClassMutateOut:
		if matchAny(pol.Allow.MutateOutOfScope, s.Argv) {
			v = Allow
		} else {
			v = Ask
			if s.Reason == "" {
				s.Reason = "mutate_out_of_scope: " + strings.Join(s.Paths, ", ")
			}
		}
	case ClassInterpreter:
		if pol.AskInterpreterUntracked() {
			v = Ask
			if s.Reason == "" {
				s.Reason = "interpreter_exec: effects opaque to the parser"
			}
		} else {
			v = Allow
		}
	case ClassUnresolvable:
		if pol.AskUnresolvable() {
			v = Ask
			if s.Reason == "" {
				s.Reason = "unresolvable: " + strings.Join(s.Unresolved, ", ")
			}
		} else {
			v = Allow
		}
	case ClassDestructive:
		v = Deny
	default:
		v = Ask
	}
	if s.sudo && v == Allow && pol.AskSudo() {
		// sudo raises the segment one severity class (guard-spec 2.3):
		// allow becomes ask. An asking segment stays ask (a deny needs a
		// rule id); a destructive one is already denied.
		v = Ask
		if s.Reason == "" {
			s.Reason = "sudo: " + strings.Join(s.Argv, " ")
		}
	}
	return v
}

// grantsEdits maps the harness permission mode onto the 2.6 edit grant.
func grantsEdits(mode string) bool {
	switch strings.ToLower(mode) {
	case "acceptedits", "bypasspermissions", "dontask", "auto", "edits", "yolo":
		return true
	}
	return false
}

// matchAny matches resolved argv against policy patterns: the first token
// is the literal command name, the remaining tokens are globs matched
// positionally, and a trailing `*` matches any number of further args.
func matchAny(patterns []string, argv []string) bool {
	for _, p := range patterns {
		if matchPattern(p, argv) {
			return true
		}
	}
	return false
}

func matchPattern(pattern string, argv []string) bool {
	toks := strings.Fields(pattern)
	if len(toks) == 0 || len(argv) == 0 {
		return false
	}
	if toks[0] != argv[0] && toks[0] != baseName(argv[0]) {
		return false
	}
	toks, rest := toks[1:], argv[1:]
	for i, t := range toks {
		if t == "*" && i == len(toks)-1 {
			return true
		}
		if i >= len(rest) {
			return false
		}
		if ok, err := globMatch(t, rest[i]); err != nil || !ok {
			return false
		}
	}
	return len(rest) == len(toks)
}

func envMap(env []string) map[string]string {
	if env == nil {
		env = os.Environ()
	}
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}
