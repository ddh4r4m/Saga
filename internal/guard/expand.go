package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"

	"github.com/ddh4r4m/saga/internal/canon"
)

const (
	// maxDepth is the wrapper recursion cap of guard-spec 2.3.
	maxDepth = 4
	// globCap bounds the directory entries one command may enumerate.
	globCap = 20000
	// loopCap bounds how many literal loop values are unrolled.
	loopCap = 8
	// placeholders in resolved argv
	phSubshell   = "<subshell>"
	phProcSubst  = "<procsubst>"
	phXargsInput = "<xargs-input>"
	phFindMatch  = "<find-match>"
)

var errGlobCap = errors.New("glob cap exceeded")

// expander walks a parsed command, expanding words as the shell would on
// data only, and produces segments.
type expander struct {
	shell      string
	lang       syntax.LangVariant
	cwd        string
	cwdUnknown bool
	dirStack   []string
	home       string
	root       string
	env        map[string]string
	unsetVars  map[string]bool // assigned from an unresolvable value
	pol        *Policy
	scope      *scope
	readdirN   int
	segs       []*Segment
	src        string
}

func newExpander(shell, cwd, home, root string, env map[string]string, pol *Policy) *expander {
	e := &expander{shell: shell, cwd: cwd, home: home, root: root, pol: pol, unsetVars: map[string]bool{}}
	e.env = make(map[string]string, len(env))
	for k, v := range env {
		e.env[k] = v
	}
	e.env["HOME"] = home
	switch shell {
	case "sh":
		e.lang = syntax.LangPOSIX
	case "zsh":
		e.lang = syntax.LangZsh
	default:
		e.lang = syntax.LangBash
	}
	e.scope = newScope(cwd, home, root, pol)
	return e
}

func (e *expander) parse(src string) (*syntax.File, error) {
	p := syntax.NewParser(syntax.Variant(e.lang))
	return p.Parse(strings.NewReader(src), "")
}

// run parses and walks the command.
func (e *expander) run(cmd string) ([]*Segment, error) {
	f, err := e.parse(cmd)
	if err != nil {
		return nil, err
	}
	e.src = cmd
	e.stmts(f.Stmts, 0)
	return e.segs, nil
}

// withSrc re-parses body as its own script (sh -c payloads, heredocs,
// npm scripts) and classifies its statements at depth.
func (e *expander) withSrc(body string, lang syntax.LangVariant, depth int, seg *Segment, remote bool) {
	if depth > maxDepth {
		seg.Unresolved = append(seg.Unresolved, "wrapper depth > 4")
		return
	}
	saveSrc, saveLang := e.src, e.lang
	e.src, e.lang = body, lang
	f, err := e.parse(body)
	if err != nil {
		seg.Unresolved = append(seg.Unresolved, "nested script parse: "+err.Error())
	} else {
		start := len(e.segs)
		e.stmts(f.Stmts, depth)
		if remote {
			for _, s := range e.segs[start:] {
				s.remote = true
			}
		}
		if seg.sudo {
			for _, s := range e.segs[start:] {
				s.sudo = true
			}
		}
	}
	e.src, e.lang = saveSrc, saveLang
}

func (e *expander) stmts(list []*syntax.Stmt, depth int) {
	for _, s := range list {
		e.stmt(s, depth, nil)
	}
}

func (e *expander) newSeg(n syntax.Node, depth int) *Segment {
	raw := ""
	if n != nil {
		a, b := n.Pos().Offset(), n.End().Offset()
		if a <= b && int(b) <= len(e.src) {
			raw = strings.TrimSpace(e.src[a:b])
		}
	}
	s := &Segment{Raw: canon.CleanText(raw), depth: depth}
	if e.cwdUnknown {
		s.Unresolved = append(s.Unresolved, "cwd (after an unresolvable cd)")
	}
	return s
}

func (e *expander) add(s *Segment) { e.segs = append(e.segs, s) }

// stmt walks one statement and returns the segment a following pipe
// stage reads from.
func (e *expander) stmt(s *syntax.Stmt, depth int, stdinFrom *Segment) *Segment {
	var last *Segment
	switch c := s.Cmd.(type) {
	case *syntax.CallExpr:
		return e.call(s, c, depth, stdinFrom)
	case *syntax.BinaryCmd:
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			left := e.stmt(c.X, depth, stdinFrom)
			return e.stmt(c.Y, depth, left)
		}
		e.stmt(c.X, depth, nil)
		last = e.stmt(c.Y, depth, nil)
	case *syntax.Block:
		e.stmts(c.Stmts, depth)
	case *syntax.Subshell:
		e.stmts(c.Stmts, depth)
	case *syntax.IfClause:
		for ic := c; ic != nil; ic = ic.Else {
			e.stmts(ic.Cond, depth)
			e.stmts(ic.Then, depth)
		}
	case *syntax.WhileClause:
		e.stmts(c.Cond, depth)
		e.stmts(c.Do, depth)
	case *syntax.ForClause:
		e.forClause(s, c, depth)
	case *syntax.CaseClause:
		probe := e.newSeg(s, depth)
		e.expandWord(probe, c.Word)
		for _, it := range c.Items {
			e.stmts(it.Stmts, depth)
		}
	case *syntax.FuncDecl:
		if forkBomb(c) {
			seg := e.newSeg(s, depth)
			seg.Argv = []string{c.Name.Value}
			seg.Class, seg.Rule = ClassDestructive, "D9"
			seg.Reason = "D9: fork bomb (function " + c.Name.Value + " pipes into itself in the background)"
			e.add(seg)
			return seg
		}
		e.stmt(c.Body, depth, nil)
	case *syntax.TimeClause:
		if c.Stmt != nil {
			return e.stmt(c.Stmt, depth, stdinFrom)
		}
	case *syntax.CoprocClause:
		if c.Stmt != nil {
			return e.stmt(c.Stmt, depth, stdinFrom)
		}
	case *syntax.DeclClause:
		seg := e.newSeg(s, depth)
		seg.Argv = []string{c.Variant.Value}
		for _, a := range c.Args {
			e.assign(seg, a)
		}
		if c.Variant.Value == "export" && len(c.Args) == 0 {
			seg.Argv = []string{"export", "-p"}
		}
		e.redirs(seg, s.Redirs)
		e.add(seg)
		return seg
	case *syntax.LetClause, *syntax.ArithmCmd, *syntax.TestClause:
		seg := e.newSeg(s, depth)
		seg.Argv = []string{"test"}
		e.redirs(seg, s.Redirs)
		e.add(seg)
		return seg
	}
	if len(s.Redirs) > 0 {
		seg := e.newSeg(s, depth)
		seg.Argv = []string{}
		e.redirs(seg, s.Redirs)
		e.add(seg)
		return seg
	}
	return last
}

// forkBomb reports a function that pipes into itself in the background.
func forkBomb(f *syntax.FuncDecl) bool {
	name := f.Name.Value
	found := false
	syntax.Walk(f.Body, func(n syntax.Node) bool {
		if b, ok := n.(*syntax.BinaryCmd); ok && (b.Op == syntax.Pipe || b.Op == syntax.PipeAll) {
			if callsName(b.X, name) && callsName(b.Y, name) {
				found = true
			}
		}
		return !found
	})
	return found || name == ":"
}

func callsName(s *syntax.Stmt, name string) bool {
	c, ok := s.Cmd.(*syntax.CallExpr)
	return ok && len(c.Args) > 0 && c.Args[0].Lit() == name
}

func (e *expander) forClause(s *syntax.Stmt, c *syntax.ForClause, depth int) {
	wi, ok := c.Loop.(*syntax.WordIter)
	if !ok {
		e.stmts(c.Do, depth)
		return
	}
	name := wi.Name.Value
	probe := e.newSeg(s, depth)
	var values []string
	if wi.InPos.IsValid() {
		for _, w := range wi.Items {
			values = append(values, e.expandWord(probe, w)...)
		}
	}
	prev, had := e.env[name]
	prevUnset := e.unsetVars[name]
	defer func() {
		if had {
			e.env[name] = prev
		} else {
			delete(e.env, name)
		}
		e.unsetVars[name] = prevUnset
	}()
	if len(values) == 0 || len(probe.Unresolved) > 0 || !wi.InPos.IsValid() {
		delete(e.env, name)
		e.unsetVars[name] = true
		e.stmts(c.Do, depth)
		return
	}
	if len(values) > loopCap {
		values = values[:loopCap]
	}
	e.unsetVars[name] = false
	for _, v := range values {
		e.env[name] = v
		e.stmts(c.Do, depth)
	}
}

// assign binds a variable for the statements that follow.
func (e *expander) assign(seg *Segment, a *syntax.Assign) {
	if a.Name == nil || a.Naked {
		return
	}
	name := a.Name.Value
	if a.Array != nil || a.Index != nil {
		delete(e.env, name)
		e.unsetVars[name] = true
		return
	}
	val := ""
	if a.Value != nil {
		before := len(seg.Unresolved)
		e.unsetParams(seg, a.Value)
		v, err := expand.Literal(e.cfg(seg, 0), a.Value)
		if err != nil || len(seg.Unresolved) > before {
			delete(e.env, name)
			e.unsetVars[name] = true
			return
		}
		val = v
	}
	if a.Append {
		val = e.env[name] + val
	}
	e.env[name] = val
	e.unsetVars[name] = false
}

// call handles a simple command.
func (e *expander) call(s *syntax.Stmt, c *syntax.CallExpr, depth int, stdinFrom *Segment) *Segment {
	seg := e.newSeg(s, depth)
	seg.stdinFrom = stdinFrom
	if len(c.Args) == 0 {
		for _, a := range c.Assigns {
			e.assign(seg, a)
		}
		seg.Argv = []string{}
		seg.assignOnly = true
		e.redirs(seg, s.Redirs)
		e.add(seg)
		return seg
	}
	for _, w := range c.Args {
		before := len(seg.Unresolved)
		fields := e.expandWord(seg, w)
		clean := len(seg.Unresolved) == before
		for _, f := range fields {
			seg.Argv = append(seg.Argv, f)
			seg.argvClean = append(seg.argvClean, clean)
			seg.argvSubst = append(seg.argvSubst, seg.lastSubst)
		}
		seg.lastSubst = ""
	}
	e.stripWrappers(seg)
	e.redirs(seg, s.Redirs)
	e.add(seg)
	e.unwrap(seg, depth)
	e.track(seg)
	return seg
}

// stripWrappers removes sudo, env, nice, nohup, time, timeout, command,
// exec, builtin, stdbuf, caffeinate and doas prefixes in place.
func (e *expander) stripWrappers(seg *Segment) {
	for len(seg.Argv) > 0 {
		verb := baseName(seg.Argv[0])
		switch verb {
		case "sudo", "doas":
			seg.sudo = true
			n := 1
			for n < len(seg.Argv) && strings.HasPrefix(seg.Argv[n], "-") {
				switch seg.Argv[n] {
				case "-u", "-g", "-h", "-p", "-C", "-D", "-r", "-t", "-T", "-U", "--user", "--group", "--host", "--prompt", "--chdir", "--role", "--type", "--other-user":
					n += 2
				case "--":
					n++
					goto stripped
				default:
					n++
				}
			}
		stripped:
			seg.drop(n)
		case "env":
			n := 1
			for n < len(seg.Argv) {
				a := seg.Argv[n]
				switch {
				case a == "-u" || a == "--unset" || a == "-C" || a == "--chdir" || a == "-S" || a == "--split-string":
					n += 2
				case a == "-i" || a == "-" || a == "--ignore-environment" || a == "-0" || a == "--null" || a == "-v":
					n++
				case strings.HasPrefix(a, "-"):
					n++
				case strings.Contains(a, "=") && !strings.HasPrefix(a, "/") && !strings.HasPrefix(a, "."):
					kv := strings.SplitN(a, "=", 2)
					if syntax.ValidName(kv[0]) {
						n++
						continue
					}
					goto envDone
				default:
					goto envDone
				}
			}
		envDone:
			if n >= len(seg.Argv) {
				// bare env: prints the environment (D7 in classify)
				return
			}
			seg.drop(n)
		case "nice", "nohup", "time", "command", "exec", "builtin", "stdbuf", "caffeinate", "ionice", "chrt", "unbuffer", "script":
			if verb == "script" {
				return
			}
			n := 1
			for n < len(seg.Argv) && strings.HasPrefix(seg.Argv[n], "-") {
				a := seg.Argv[n]
				if (verb == "nice" && (a == "-n" || a == "--adjustment")) || (verb == "stdbuf" && (a == "-i" || a == "-o" || a == "-e")) || (verb == "caffeinate" && a == "-t") || (verb == "ionice" && (a == "-c" || a == "-n" || a == "-p")) {
					n += 2
					continue
				}
				n++
			}
			if n >= len(seg.Argv) {
				return
			}
			seg.drop(n)
		case "timeout":
			n := 1
			for n < len(seg.Argv) && strings.HasPrefix(seg.Argv[n], "-") {
				a := seg.Argv[n]
				if a == "-s" || a == "--signal" || a == "-k" || a == "--kill-after" {
					n += 2
					continue
				}
				n++
			}
			n++ // the duration
			if n >= len(seg.Argv) {
				return
			}
			seg.drop(n)
		default:
			return
		}
	}
}

// unwrap handles wrappers whose payload needs re-parsing or binding.
func (e *expander) unwrap(seg *Segment, depth int) {
	if len(seg.Argv) == 0 {
		return
	}
	if depth > maxDepth {
		seg.Unresolved = append(seg.Unresolved, "wrapper depth > 4")
		return
	}
	verb := baseName(seg.Argv[0])
	switch verb {
	case "sh", "bash", "zsh", "dash", "ksh", "mksh":
		e.unwrapShell(seg, verb, depth)
	case "xargs":
		e.unwrapXargs(seg, depth)
	case "find", "fd":
		if verb == "find" {
			e.unwrapFind(seg, depth)
		}
	case "npx", "bunx":
		e.unwrapNpx(seg, depth, 1)
	case "npm", "yarn", "pnpm", "bun":
		e.unwrapPackageScript(seg, verb, depth)
	case "make", "just":
		e.unwrapMake(seg, verb, depth)
	case "git":
		e.unwrapGitAlias(seg, depth)
	case "ssh":
		e.unwrapSSH(seg, depth)
	case "docker", "podman", "nerdctl":
		e.unwrapDockerExec(seg, depth)
	case "kubectl":
		e.unwrapKubectlExec(seg, depth)
	case "eval":
		seg.Unresolved = append(seg.Unresolved, "eval")
	default:
		if seg.heredoc != "" {
			e.heredocConsumer(seg, verb, depth)
		}
	}
}

// unwrapShell handles `sh -c 'script'`, `bash script.sh` and `sh <<EOF`.
func (e *expander) unwrapShell(seg *Segment, verb string, depth int) {
	lang := syntax.LangBash
	switch verb {
	case "sh", "dash":
		lang = syntax.LangPOSIX
	case "zsh":
		lang = syntax.LangZsh
	}
	for i := 1; i < len(seg.Argv); i++ {
		a := seg.Argv[i]
		if a == "-c" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c")) {
			if i+1 >= len(seg.Argv) {
				return
			}
			seg.shellBody = true
			if !seg.argvClean[i+1] {
				seg.Unresolved = append(seg.Unresolved, "dynamic "+verb+" -c payload")
				if seg.argvSubst[i+1] == "fetch" {
					seg.fetchSubst = true
				}
				return
			}
			e.withSrc(seg.Argv[i+1], lang, depth+1, seg, false)
			return
		}
		if a == "--" {
			continue
		}
		if a == "-s" || a == "-" {
			break
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		// script file
		if a == phProcSubst && seg.procFetch {
			seg.fetchSubst = true
			seg.shellBody = true
		}
		seg.scriptFile = a
		return
	}
	if seg.heredoc != "" {
		seg.shellBody = true
		e.withSrc(seg.heredoc, lang, depth+1, seg, false)
	}
}

// heredocConsumer marks interpreters fed a heredoc body.
func (e *expander) heredocConsumer(seg *Segment, verb string, _ int) {
	if isInterpreter(verb) {
		seg.interpreterBody = true
	}
}

// unwrapXargs replaces `xargs [opts] cmd` by `cmd <xargs-input>`.
func (e *expander) unwrapXargs(seg *Segment, depth int) {
	n := 1
	repl := ""
	for n < len(seg.Argv) && strings.HasPrefix(seg.Argv[n], "-") {
		a := seg.Argv[n]
		switch {
		case a == "-I" || a == "--replace" || a == "-i":
			if n+1 < len(seg.Argv) {
				repl = seg.Argv[n+1]
			}
			n += 2
		case strings.HasPrefix(a, "-I") && len(a) > 2:
			repl = a[2:]
			n++
		case a == "-n" || a == "-P" || a == "-L" || a == "-s" || a == "-d" || a == "-a" || a == "-E" || a == "--max-args" || a == "--max-procs" || a == "--max-lines" || a == "--delimiter" || a == "--arg-file":
			n += 2
		default:
			n++
		}
	}
	if n >= len(seg.Argv) {
		// bare xargs runs echo
		seg.Argv = []string{"echo", phXargsInput}
		seg.argvClean = []bool{true, true}
		seg.argvSubst = []string{"", ""}
		return
	}
	seg.xargsFrom = seg.stdinFrom
	seg.xargs = true
	seg.drop(n)
	if repl != "" {
		for i, a := range seg.Argv {
			if strings.Contains(a, repl) {
				seg.Argv[i] = strings.ReplaceAll(a, repl, phXargsInput)
			}
		}
	} else {
		seg.Argv = append(seg.Argv, phXargsInput)
		seg.argvClean = append(seg.argvClean, true)
		seg.argvSubst = append(seg.argvSubst, "")
	}
	e.stripWrappers(seg)
	e.unwrap(seg, depth+1)
}

// findSpec is the parsed shape of a find command.
type findSpec struct {
	starts   []string
	names    []string
	filtered bool
	delete   bool
	execs    [][]string
}

func parseFind(argv []string) findSpec {
	var f findSpec
	i := 1
	for i < len(argv) && !strings.HasPrefix(argv[i], "-") && argv[i] != "(" && argv[i] != "!" && argv[i] != "\\(" {
		f.starts = append(f.starts, argv[i])
		i++
	}
	if len(f.starts) == 0 {
		f.starts = []string{"."}
	}
	for i < len(argv) {
		a := argv[i]
		switch a {
		case "-name", "-iname", "-path", "-ipath", "-wholename", "-iwholename", "-regex", "-iregex", "-lname", "-ilname":
			if i+1 < len(argv) {
				f.names = append(f.names, argv[i+1])
			}
			f.filtered = true
			i += 2
		case "-type", "-newer", "-anewer", "-cnewer", "-mtime", "-mmin", "-atime", "-amin", "-ctime", "-cmin", "-size", "-user", "-group", "-perm", "-uid", "-gid", "-inum", "-links", "-samefile", "-newermt", "-fstype", "-used", "-context":
			f.filtered = true
			i += 2
		case "-empty", "-executable", "-readable", "-writable", "-nouser", "-nogroup", "-true", "-false":
			f.filtered = true
			i++
		case "-maxdepth", "-mindepth", "-printf", "-fprint", "-fprintf", "-fls", "-D", "-O":
			i += 2
		case "-delete":
			f.delete = true
			i++
		case "-exec", "-execdir", "-ok", "-okdir":
			var payload []string
			j := i + 1
			for j < len(argv) && argv[j] != ";" && argv[j] != "\\;" && argv[j] != "+" {
				payload = append(payload, argv[j])
				j++
			}
			f.execs = append(f.execs, payload)
			i = j + 1
		default:
			i++
		}
	}
	return f
}

// unwrapFind creates a child segment per -exec payload with {} bound to
// the start paths (or a pseudo path under them when the find is filtered).
func (e *expander) unwrapFind(seg *Segment, depth int) {
	f := parseFind(seg.Argv)
	seg.find = &f
	for _, payload := range f.execs {
		if len(payload) == 0 {
			continue
		}
		child := &Segment{Raw: seg.Raw, depth: depth + 1, sudo: seg.sudo, remote: seg.remote, find: &f, fromFind: true}
		child.Unresolved = append(child.Unresolved, seg.Unresolved...)
		var bound []string
		for _, start := range f.starts {
			bound = append(bound, e.findBinding(f, start)...)
		}
		for _, a := range payload {
			if strings.Contains(a, "{}") {
				for _, b := range bound {
					child.Argv = append(child.Argv, strings.ReplaceAll(a, "{}", b))
					child.argvClean = append(child.argvClean, true)
					child.argvSubst = append(child.argvSubst, "")
				}
				continue
			}
			child.Argv = append(child.Argv, a)
			child.argvClean = append(child.argvClean, true)
			child.argvSubst = append(child.argvSubst, "")
		}
		e.stripWrappers(child)
		e.add(child)
		e.unwrap(child, depth+1)
	}
}

// findBinding returns the paths a find over start would hand to -exec or
// xargs: the start itself, <start>/<name pattern> when filtered by name,
// or <start>/<find-match> for other filters.
func (e *expander) findBinding(f findSpec, start string) []string {
	if len(f.names) > 0 {
		var out []string
		for _, n := range f.names {
			out = append(out, filepath.Join(start, filepath.Base(n)))
		}
		return out
	}
	if f.filtered {
		return []string{filepath.Join(start, phFindMatch)}
	}
	return []string{start}
}

// unwrapNpx strips `npx [opts] pkg` so `pkg args` is classified.
func (e *expander) unwrapNpx(seg *Segment, depth, from int) {
	n := from
	for n < len(seg.Argv) && strings.HasPrefix(seg.Argv[n], "-") {
		a := seg.Argv[n]
		if a == "-p" || a == "--package" || a == "-c" || a == "--call" || a == "--shell" || a == "--node-arg" || a == "-n" {
			if a == "-c" || a == "--call" {
				seg.Unresolved = append(seg.Unresolved, "npx --call")
			}
			n += 2
			continue
		}
		n++
	}
	if n >= len(seg.Argv) {
		return
	}
	// strip a @version suffix on the package name
	pkg := seg.Argv[n]
	if i := strings.LastIndexByte(pkg, '@'); i > 0 {
		pkg = pkg[:i]
	}
	seg.Argv[n] = pkg
	seg.drop(n)
	e.unwrap(seg, depth+1)
}

var yarnBuiltins = map[string]bool{"add": true, "install": true, "remove": true, "upgrade": true, "up": true, "init": true, "link": true, "unlink": true, "publish": true, "npm": true, "dlx": true, "info": true, "why": true, "config": true, "cache": true, "workspaces": true, "workspace": true, "version": true, "audit": true, "list": true, "outdated": true, "pack": true, "set": true, "bin": true, "exec": true, "node": true, "dedupe": true, "patch": true, "create": true, "global": true, "login": true, "logout": true, "owner": true, "tag": true, "import": true, "licenses": true, "check": true, "store": true, "prune": true, "rebuild": true, "update": true, "i": true, "rm": true, "un": true, "uninstall": true, "ls": true, "run": true, "test": true, "start": true, "build": true, "x": true, "pm": true, "repl": true}

// unwrapPackageScript resolves `npm run s`, `npm test`, `yarn s`,
// `pnpm run s` to the script body in package.json and classifies it.
func (e *expander) unwrapPackageScript(seg *Segment, verb string, depth int) {
	if len(seg.Argv) < 2 {
		return
	}
	sub := seg.Argv[1]
	script := ""
	switch {
	case (verb == "pnpm" || verb == "yarn") && sub == "dlx", verb == "bun" && sub == "x":
		e.unwrapNpx(seg, depth, 2)
		return
	case verb == "npm" && (sub == "exec" || sub == "x"):
		e.unwrapNpx(seg, depth, 2)
		return
	case sub == "run" || sub == "run-script":
		n := 2
		for n < len(seg.Argv) && strings.HasPrefix(seg.Argv[n], "-") {
			n++
		}
		if n < len(seg.Argv) {
			script = seg.Argv[n]
		}
	case sub == "test" || sub == "t" || sub == "tst":
		script = "test"
	case sub == "start":
		script = "start"
	case sub == "stop", sub == "restart":
		script = sub
	case (verb == "yarn" || verb == "pnpm" || verb == "bun") && !yarnBuiltins[sub] && !strings.HasPrefix(sub, "-"):
		script = sub
	}
	if script == "" {
		return
	}
	body, found, err := e.packageScript(script)
	if err != nil {
		seg.Unresolved = append(seg.Unresolved, "package.json: "+err.Error())
		return
	}
	if !found {
		if script == "start" && verb == "npm" {
			body = "node server.js"
		} else if script == "test" && verb == "npm" {
			return // npm errors out: no script
		} else {
			seg.Unresolved = append(seg.Unresolved, "package.json has no script "+script)
			return
		}
	}
	e.withSrc(body, syntax.LangPOSIX, depth+1, seg, false)
}

// packageScript reads scripts.<name> from the nearest package.json.
func (e *expander) packageScript(name string) (string, bool, error) {
	dir := e.cwd
	for {
		raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
		if err == nil {
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal(raw, &pkg); err != nil {
				return "", false, err
			}
			body, ok := pkg.Scripts[name]
			return body, ok, nil
		}
		if dir == e.root || filepath.Dir(dir) == dir {
			return "", false, errors.New("not found")
		}
		dir = filepath.Dir(dir)
	}
}

// unwrapMake reads the recipe of a Makefile or justfile target.
func (e *expander) unwrapMake(seg *Segment, verb string, depth int) {
	dir := e.cwd
	file := ""
	var targets []string
	for i := 1; i < len(seg.Argv); i++ {
		a := seg.Argv[i]
		switch {
		case a == "-C" || a == "--directory":
			if i+1 < len(seg.Argv) {
				dir = filepath.Join(e.cwd, seg.Argv[i+1])
				i++
			}
		case a == "-f" || a == "--file" || a == "--makefile" || a == "--justfile":
			if i+1 < len(seg.Argv) {
				file = seg.Argv[i+1]
				i++
			}
		case strings.HasPrefix(a, "-"), strings.Contains(a, "="):
		default:
			targets = append(targets, a)
		}
	}
	var candidates []string
	if file != "" {
		candidates = []string{filepath.Join(dir, file)}
	} else if verb == "just" {
		candidates = []string{filepath.Join(dir, "justfile"), filepath.Join(dir, "Justfile"), filepath.Join(dir, ".justfile")}
	} else {
		candidates = []string{filepath.Join(dir, "GNUmakefile"), filepath.Join(dir, "makefile"), filepath.Join(dir, "Makefile")}
	}
	var raw []byte
	for _, c := range candidates {
		if b, err := os.ReadFile(c); err == nil {
			raw = b
			break
		}
	}
	if raw == nil {
		seg.Unresolved = append(seg.Unresolved, verb+": no "+filepath.Base(candidates[len(candidates)-1])+" in "+dir)
		return
	}
	recipes := parseRecipes(string(raw), verb == "just")
	if len(targets) == 0 {
		if len(recipes.order) == 0 {
			seg.Unresolved = append(seg.Unresolved, verb+": no targets")
			return
		}
		targets = []string{recipes.order[0]}
	}
	for _, t := range targets {
		lines, ok := recipes.byName[t]
		if !ok {
			seg.Unresolved = append(seg.Unresolved, verb+": target "+t+" not found or generated")
			continue
		}
		for _, l := range lines {
			if strings.Contains(l, "$(") || strings.Contains(l, "${") || strings.Contains(l, "{{") {
				seg.Unresolved = append(seg.Unresolved, verb+": recipe line uses a make variable: "+l)
				continue
			}
			e.withSrc(l, syntax.LangPOSIX, depth+1, seg, false)
		}
	}
}

type recipeSet struct {
	order  []string
	byName map[string][]string
}

// parseRecipes reads target: lines and their indented recipe lines.
func parseRecipes(src string, just bool) recipeSet {
	rs := recipeSet{byName: map[string][]string{}}
	cur := []string{}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "\t") || (just && strings.HasPrefix(line, "  ")) {
			l := strings.TrimSpace(line)
			l = strings.TrimLeft(l, "@-+")
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			for _, n := range cur {
				rs.byName[n] = append(rs.byName[n], l)
			}
			continue
		}
		cur = nil
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, ".") || strings.Contains(trim, ":=") || strings.HasPrefix(trim, "export ") || strings.HasPrefix(trim, "include ") || strings.HasPrefix(trim, "set ") {
			continue
		}
		i := strings.IndexByte(trim, ':')
		if i <= 0 || strings.Contains(trim[:i], "=") {
			continue
		}
		for _, n := range strings.Fields(trim[:i]) {
			if strings.ContainsAny(n, "%$") {
				continue
			}
			if just {
				n = strings.Fields(n)[0]
			}
			if _, seen := rs.byName[n]; !seen {
				rs.byName[n] = nil
				rs.order = append(rs.order, n)
			}
			cur = append(cur, n)
		}
	}
	return rs
}

var gitVerbs = map[string]bool{"add": true, "am": true, "apply": true, "archive": true, "bisect": true, "blame": true, "branch": true, "bundle": true, "cat-file": true, "check-ignore": true, "checkout": true, "cherry-pick": true, "clean": true, "clone": true, "commit": true, "config": true, "count-objects": true, "describe": true, "diff": true, "difftool": true, "fetch": true, "filter-branch": true, "for-each-ref": true, "format-patch": true, "fsck": true, "gc": true, "grep": true, "help": true, "init": true, "log": true, "ls-files": true, "ls-remote": true, "ls-tree": true, "merge": true, "merge-base": true, "mergetool": true, "mv": true, "name-rev": true, "notes": true, "pull": true, "push": true, "range-diff": true, "rebase": true, "reflog": true, "remote": true, "repack": true, "replace": true, "request-pull": true, "reset": true, "restore": true, "rev-list": true, "rev-parse": true, "revert": true, "rm": true, "shortlog": true, "show": true, "show-branch": true, "show-ref": true, "sparse-checkout": true, "stash": true, "status": true, "submodule": true, "switch": true, "symbolic-ref": true, "tag": true, "update-index": true, "update-ref": true, "var": true, "verify-commit": true, "verify-tag": true, "whatchanged": true, "worktree": true, "write-tree": true, "read-tree": true, "commit-tree": true, "hash-object": true, "lfs": true, "version": true, "maintenance": true, "prune": true, "prune-packed": true, "stripspace": true, "cherry": true, "annotate": true, "instaweb": true, "daemon": true, "send-email": true, "svn": true, "flow": true, "credential": true, "credential-store": true, "credential-cache": true, "web--browse": true, "citool": true, "gui": true, "gitk": true, "unpack-objects": true, "update-server-info": true, "column": true, "interpret-trailers": true, "mailinfo": true, "mktree": true, "pack-refs": true, "rerere": true, "checkout-index": true, "diff-tree": true, "diff-index": true, "diff-files": true, "ls-tree-r": true, "mktag": true, "pack-objects": true, "index-pack": true, "fast-import": true, "fast-export": true, "filter-repo": true, "bugreport": true, "diagnose": true, "hook": true, "scalar": true, "sizer": true}

// unwrapGitAlias resolves `git <alias>` through `git config --get`.
func (e *expander) unwrapGitAlias(seg *Segment, depth int) {
	i := 1
	for i < len(seg.Argv) && strings.HasPrefix(seg.Argv[i], "-") {
		a := seg.Argv[i]
		if a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "--namespace" {
			i += 2
			continue
		}
		i++
	}
	if i >= len(seg.Argv) {
		return
	}
	sub := seg.Argv[i]
	if gitVerbs[sub] {
		return
	}
	dir := e.cwd
	if e.cwdUnknown {
		dir = e.root
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get", "alias."+sub).Output()
	if err != nil || len(out) == 0 {
		return
	}
	alias := strings.TrimSpace(string(out))
	if strings.HasPrefix(alias, "!") {
		rest := seg.Argv[i+1:]
		body := alias[1:]
		if len(rest) > 0 {
			var quoted []string
			for _, r := range rest {
				q, err := syntax.Quote(r, syntax.LangBash)
				if err != nil {
					q = r
				}
				quoted = append(quoted, q)
			}
			body += " " + strings.Join(quoted, " ")
		}
		seg.shellBody = true
		e.withSrc(body, syntax.LangBash, depth+1, seg, false)
		return
	}
	fields := strings.Fields(alias)
	argv := append([]string{}, seg.Argv[:i]...)
	argv = append(argv, fields...)
	argv = append(argv, seg.Argv[i+1:]...)
	seg.Argv = argv
	seg.argvClean = boolsOf(len(argv), true)
	seg.argvSubst = make([]string, len(argv))
	e.unwrap(seg, depth+1)
}

func boolsOf(n int, v bool) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// unwrapSSH parses the remote payload of `ssh host cmd...`.
func (e *expander) unwrapSSH(seg *Segment, depth int) {
	seg.remoteExec = true
	i := 1
	for i < len(seg.Argv) && strings.HasPrefix(seg.Argv[i], "-") {
		a := seg.Argv[i]
		if len(a) == 2 && strings.ContainsRune("bcDeEFIiJLlmOopQRSWw", rune(a[1])) {
			i += 2
			continue
		}
		i++
	}
	i++ // host
	if i >= len(seg.Argv) {
		return
	}
	body := strings.Join(seg.Argv[i:], " ")
	if !allClean(seg.argvClean[i:]) {
		seg.Unresolved = append(seg.Unresolved, "dynamic remote command")
		return
	}
	e.withSrc(body, syntax.LangPOSIX, depth+1, seg, true)
}

func allClean(b []bool) bool {
	for _, v := range b {
		if !v {
			return false
		}
	}
	return true
}

// unwrapDockerExec handles `docker exec [opts] container cmd...`.
func (e *expander) unwrapDockerExec(seg *Segment, depth int) {
	if len(seg.Argv) < 3 || seg.Argv[1] != "exec" {
		return
	}
	seg.remoteExec = true
	i := 2
	for i < len(seg.Argv) && strings.HasPrefix(seg.Argv[i], "-") {
		a := seg.Argv[i]
		if a == "-u" || a == "--user" || a == "-w" || a == "--workdir" || a == "-e" || a == "--env" || a == "--env-file" {
			i += 2
			continue
		}
		i++
	}
	i++ // container
	if i >= len(seg.Argv) {
		return
	}
	e.remoteChild(seg, seg.Argv[i:], depth)
}

// unwrapKubectlExec handles `kubectl exec pod -- cmd...`.
func (e *expander) unwrapKubectlExec(seg *Segment, depth int) {
	if len(seg.Argv) < 3 || seg.Argv[1] != "exec" {
		return
	}
	seg.remoteExec = true
	for i, a := range seg.Argv {
		if a == "--" && i+1 < len(seg.Argv) {
			e.remoteChild(seg, seg.Argv[i+1:], depth)
			return
		}
	}
}

// remoteChild classifies a remote argv as its own segment.
func (e *expander) remoteChild(seg *Segment, argv []string, depth int) {
	child := &Segment{Raw: seg.Raw, depth: depth + 1, sudo: seg.sudo, remote: true}
	child.Argv = append([]string{}, argv...)
	child.argvClean = boolsOf(len(argv), true)
	child.argvSubst = make([]string, len(argv))
	child.Unresolved = append(child.Unresolved, seg.Unresolved...)
	e.stripWrappers(child)
	e.add(child)
	e.unwrap(child, depth+1)
}

// track follows cd, pushd and popd so later relative paths resolve.
func (e *expander) track(seg *Segment) {
	if len(seg.Argv) == 0 {
		return
	}
	switch baseName(seg.Argv[0]) {
	case "cd", "pushd":
		target := e.home
		if len(seg.Argv) > 1 {
			target = seg.Argv[len(seg.Argv)-1]
			if strings.HasPrefix(target, "-") && len(seg.Argv) > 2 {
				target = seg.Argv[len(seg.Argv)-1]
			}
		}
		if len(seg.Unresolved) > 0 || target == "-" || strings.Contains(target, "<") {
			e.cwdUnknown = true
			return
		}
		if baseName(seg.Argv[0]) == "pushd" {
			e.dirStack = append(e.dirStack, e.cwd)
		}
		e.cwd = canonPath(e.cwd, target)
	case "popd":
		if n := len(e.dirStack); n > 0 {
			e.cwd = e.dirStack[n-1]
			e.dirStack = e.dirStack[:n-1]
		} else {
			e.cwdUnknown = true
		}
	}
}

// redirs records write effects and heredoc bodies.
func (e *expander) redirs(seg *Segment, rs []*syntax.Redirect) {
	for _, r := range rs {
		switch r.Op {
		case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.AppClob, syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.RdrInOut:
			if r.Word == nil {
				continue
			}
			for _, f := range e.expandWord(seg, r.Word) {
				seg.writes = append(seg.writes, f)
				if r.Op == syntax.RdrOut || r.Op == syntax.RdrClob || r.Op == syntax.RdrAll || r.Op == syntax.RdrAllClob {
					seg.truncates = append(seg.truncates, f)
				}
			}
		case syntax.Hdoc, syntax.DashHdoc:
			if r.Hdoc == nil {
				continue
			}
			e.unsetParams(seg, r.Hdoc)
			body, err := expand.Document(e.cfg(seg, 0), r.Hdoc)
			if err != nil {
				seg.Unresolved = append(seg.Unresolved, "heredoc: "+err.Error())
				continue
			}
			seg.heredoc = body
		case syntax.WordHdoc:
			if r.Word != nil {
				seg.heredoc = strings.Join(e.expandWord(seg, r.Word), " ")
			}
		case syntax.RdrIn:
			if r.Word != nil {
				for _, f := range e.expandWord(seg, r.Word) {
					seg.reads = append(seg.reads, f)
				}
			}
		}
	}
}

// environ is the read-only harness environment seen by the expander.
type environ struct{ e *expander }

func strVar(s string) expand.Variable {
	return expand.Variable{Set: true, Kind: expand.String, Str: s}
}

func (en environ) Get(name string) expand.Variable {
	switch name {
	case "PWD":
		return strVar(en.e.cwd)
	case "HOME":
		return strVar(en.e.home)
	case "?", "#", "OPTIND":
		return strVar("0")
	case "$", "PPID":
		return strVar("1")
	case "IFS":
		return strVar(" \t\n")
	case "0":
		return strVar(en.e.shell)
	}
	if v, ok := en.e.env[name]; ok {
		return strVar(v)
	}
	return expand.Variable{}
}

func (en environ) Each(fn func(name string, vr expand.Variable) bool) {
	for k, v := range en.e.env {
		if !fn(k, strVar(v)) {
			return
		}
	}
}

// cfg builds the expansion config for one segment. mode 0 disables
// globbing (literal patterns), mode 1 enables it.
func (e *expander) cfg(seg *Segment, mode int) *expand.Config {
	c := &expand.Config{Env: environ{e}, GlobStar: e.shell == "zsh"}
	if mode == 1 {
		c.ReadDir2 = e.readDir
	}
	c.CmdSubst = func(w io.Writer, cs *syntax.CmdSubst) error {
		if val, ok := e.pureSubst(cs); ok {
			e.stmts(cs.Stmts, seg.depth+1)
			_, _ = io.WriteString(w, val)
			return nil
		}
		start := len(e.segs)
		e.stmts(cs.Stmts, seg.depth+1)
		kind := "subshell"
		for _, inner := range e.segs[start:] {
			if isFetchArgv(inner.Argv) {
				kind = "fetch"
			}
		}
		seg.lastSubst = kind
		raw := ""
		if a, b := cs.Pos().Offset(), cs.End().Offset(); a <= b && int(b) <= len(e.src) {
			raw = e.src[a:b]
		}
		seg.Unresolved = append(seg.Unresolved, canon.CleanText(raw))
		_, _ = io.WriteString(w, phSubshell)
		return nil
	}
	c.ProcSubst = func(ps *syntax.ProcSubst) (string, error) {
		start := len(e.segs)
		e.stmts(ps.Stmts, seg.depth+1)
		for _, inner := range e.segs[start:] {
			if isFetchArgv(inner.Argv) {
				seg.procFetch = true
			}
		}
		raw := ""
		if a, b := ps.Pos().Offset(), ps.End().Offset(); a <= b && int(b) <= len(e.src) {
			raw = e.src[a:b]
		}
		seg.Unresolved = append(seg.Unresolved, canon.CleanText(raw))
		return phProcSubst, nil
	}
	return c
}

func (e *expander) readDir(name string) ([]os.DirEntry, error) {
	if e.readdirN > globCap {
		return nil, errGlobCap
	}
	ents, err := os.ReadDir(name)
	e.readdirN += len(ents)
	if e.readdirN > globCap {
		return nil, errGlobCap
	}
	return ents, err
}

// pureSubst computes the value of a command substitution guard can
// evaluate itself without a shell (guard-spec 2.2 pure table).
func (e *expander) pureSubst(cs *syntax.CmdSubst) (string, bool) {
	if len(cs.Stmts) != 1 {
		return "", false
	}
	st := cs.Stmts[0]
	call, ok := st.Cmd.(*syntax.CallExpr)
	if !ok || len(st.Redirs) > 0 || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return "", false
	}
	var argv []string
	for _, w := range call.Args {
		lit := w.Lit()
		if lit == "" {
			// allow quoted literals
			if s, ok := quotedLit(w); ok {
				lit = s
			} else {
				return "", false
			}
		}
		argv = append(argv, lit)
	}
	if e.cwdUnknown && (argv[0] == "pwd" || argv[0] == "git") {
		return "", false
	}
	switch argv[0] {
	case "pwd":
		return e.cwd, true
	case "git":
		if len(argv) == 3 && argv[1] == "rev-parse" && argv[2] == "--show-toplevel" {
			return gitTop(e.cwd), true
		}
	case "basename":
		if len(argv) == 2 {
			return filepath.Base(argv[1]), true
		}
	case "dirname":
		if len(argv) == 2 {
			return filepath.Dir(argv[1]), true
		}
	case "date":
		if len(argv) == 1 {
			return time.Now().Format(time.UnixDate), true
		}
		if len(argv) == 2 && strings.HasPrefix(argv[1], "+") {
			return time.Now().Format("2006-01-02"), true
		}
	case "whoami":
		if u, err := user.Current(); err == nil {
			return u.Username, true
		}
	case "uname":
		if runtime.GOOS == "darwin" {
			return "Darwin", true
		}
		return strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:], true
	case "echo":
		return strings.Join(argv[1:], " "), true
	}
	return "", false
}

func quotedLit(w *syntax.Word) (string, bool) {
	if len(w.Parts) != 1 {
		return "", false
	}
	switch p := w.Parts[0].(type) {
	case *syntax.SglQuoted:
		return p.Value, true
	case *syntax.DblQuoted:
		if len(p.Parts) == 1 {
			if l, ok := p.Parts[0].(*syntax.Lit); ok {
				return l.Value, true
			}
		}
		if len(p.Parts) == 0 {
			return "", true
		}
	}
	return "", false
}

func gitTop(cwd string) string {
	for d := cwd; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return cwd
		}
	}
}

// unsetParams records parameter expansions the harness environment does
// not resolve (no default, not a special parameter).
func (e *expander) unsetParams(seg *Segment, w *syntax.Word) {
	syntax.Walk(w, func(n syntax.Node) bool {
		switch pe := n.(type) {
		case *syntax.CmdSubst, *syntax.ProcSubst:
			return false
		case *syntax.ParamExp:
			if pe.Param == nil {
				return true
			}
			name := pe.Param.Value
			if pe.Length || pe.IsSet {
				return true
			}
			if pe.Excl {
				seg.Unresolved = append(seg.Unresolved, "${!"+name+"}")
				return true
			}
			if pe.Exp != nil {
				switch pe.Exp.Op {
				case syntax.DefaultUnset, syntax.DefaultUnsetOrNull, syntax.AssignUnset, syntax.AssignUnsetOrNull, syntax.AlternateUnset, syntax.AlternateUnsetOrNull:
					return true
				}
			}
			switch name {
			case "?", "#", "$", "!", "-", "0", "PWD", "HOME", "IFS", "OPTIND", "PPID":
				return true
			}
			if _, ok := e.env[name]; ok && !e.unsetVars[name] {
				return true
			}
			seg.Unresolved = append(seg.Unresolved, name)
		}
		return true
	})
}

// hasUnquotedGlob reports a word with glob metacharacters outside quotes.
func hasUnquotedGlob(w *syntax.Word) bool {
	for _, p := range w.Parts {
		if l, ok := p.(*syntax.Lit); ok && hasMeta(l.Value) {
			return true
		}
		if _, ok := p.(*syntax.ExtGlob); ok {
			return true
		}
	}
	return false
}

// expandWord expands one word into fields as the shell would, recording
// unresolved parameters, command substitutions and glob roots on seg.
func (e *expander) expandWord(seg *Segment, w *syntax.Word) []string {
	e.unsetParams(seg, w)
	fields, err := expand.Fields(e.cfg(seg, 1), w)
	if err != nil {
		if errors.Is(err, errGlobCap) {
			seg.Unresolved = append(seg.Unresolved, "glob cap: "+w.Lit())
		} else {
			seg.Unresolved = append(seg.Unresolved, "expand: "+err.Error())
		}
		return []string{wordText(e.src, w)}
	}
	if hasUnquotedGlob(w) {
		lit, lerr := expand.Fields(e.cfg(seg, 0), w)
		if lerr == nil {
			if !equalStrings(lit, fields) {
				for _, pat := range lit {
					if hasMeta(pat) {
						seg.globRoots = append(seg.globRoots, canonPath(e.cwd, globDir(pat)))
					}
				}
			} else if e.shell == "zsh" {
				for _, pat := range lit {
					if hasMeta(pat) {
						seg.Unresolved = append(seg.Unresolved, "zsh nomatch: "+pat)
					}
				}
			}
		}
	}
	return fields
}

func wordText(src string, w *syntax.Word) string {
	a, b := w.Pos().Offset(), w.End().Offset()
	if a <= b && int(b) <= len(src) {
		return src[a:b]
	}
	return w.Lit()
}

// globDir is the directory part of a pattern up to its first
// metacharacter segment.
func globDir(pat string) string {
	segs := strings.Split(filepath.ToSlash(pat), "/")
	var dir []string
	for _, s := range segs {
		if hasMeta(s) {
			break
		}
		dir = append(dir, s)
	}
	if len(dir) == 0 {
		return "."
	}
	d := strings.Join(dir, "/")
	if d == "" {
		return "/"
	}
	return d
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func baseName(p string) string {
	p = strings.TrimSuffix(p, ".exe")
	return filepath.Base(p)
}

// drop removes the first n argv entries, keeping the parallel slices.
func (s *Segment) drop(n int) {
	if n > len(s.Argv) {
		n = len(s.Argv)
	}
	s.Argv = s.Argv[n:]
	if len(s.argvClean) >= n {
		s.argvClean = s.argvClean[n:]
	} else {
		s.argvClean = boolsOf(len(s.Argv), true)
	}
	if len(s.argvSubst) >= n {
		s.argvSubst = s.argvSubst[n:]
	} else {
		s.argvSubst = make([]string, len(s.Argv))
	}
}

func isFetchArgv(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	switch baseName(argv[0]) {
	case "curl", "wget", "fetch", "http", "https", "xh", "aria2c":
		return true
	}
	return false
}

func isInterpreter(verb string) bool {
	switch verb {
	case "python", "python2", "python3", "node", "nodejs", "ruby", "perl", "php", "pwsh", "powershell", "osascript", "lua", "deno", "bun", "tclsh", "irb", "jshell", "swift", "julia", "Rscript", "R", "elixir", "erl", "racket", "guile", "scheme", "awk", "gawk":
		return true
	}
	if strings.HasPrefix(verb, "python3.") {
		return true
	}
	return false
}

// Ensure fmt is used (reason formatting lives in classify.go).
var _ = fmt.Sprintf
