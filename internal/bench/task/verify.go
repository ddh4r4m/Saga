package task

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
)

// Check is one row of the section 2.4 table.
type Check struct {
	Name   string   `json:"name"`
	OK     bool     `json:"ok"`
	Code   cli.Code `json:"code"`
	Detail string   `json:"detail"`
}

// StateResult is the oracle outcome for one control state.
type StateResult struct {
	Exit  int    `json:"exit"`
	Pass  int    `json:"pass"`
	Fail  int    `json:"fail"`
	Error string `json:"error,omitempty"`
}

// Shadowed is one oracle file the leak scan exempted from the
// oracle-name rule because the workspace already carries a file of the
// same name (section 2.4). Both paths are recorded so a reviewer can see
// on every run exactly what was waived.
type Shadowed struct {
	// Oracle is the oracle-relative path of the exempted file.
	Oracle string `json:"oracle"`
	// Repo is the workspace-relative path that makes the name visible.
	Repo string `json:"repo"`
}

// VerifyResult is the whole verification of one task.
type VerifyResult struct {
	Task       string                  `json:"task"`
	Hash       string                  `json:"sha256"`
	VerifiedAt string                  `json:"verified_at"`
	Checks     []Check                 `json:"checks"`
	States     map[string]*StateResult `json:"states"`
	// ShadowedNames are the oracle names the leak scan exempted, sorted.
	ShadowedNames []Shadowed `json:"shadowed_names"`
	Code          cli.Code   `json:"code"`
}

// OK reports whether every check passed.
func (r *VerifyResult) OK() bool { return r.Code == cli.ExitOK }

// VerifyOptions tune a verification.
type VerifyOptions struct {
	// Keep is a directory to stage workspaces under; empty means a
	// temporary directory removed afterwards.
	Keep string
	// Log receives one line per step when non-nil.
	Log io.Writer
	// Static skips the oracle runs (loader, canary, leak and cheat-scan
	// checks only).
	Static bool
}

// Verify runs the task's red proof (bench-spec section 2.4). Exit codes:
// 2 when task.toml is invalid, 7 for the leak and canary scans, 1 for
// every oracle-driven check, combined by the contracts section 4
// precedence.
func Verify(ctx context.Context, dir string, opts VerifyOptions) *VerifyResult {
	r := &VerifyResult{Task: filepath.Base(dir), States: map[string]*StateResult{}, VerifiedAt: time.Now().UTC().Format(time.RFC3339)}
	logf := func(format string, args ...any) {
		if opts.Log != nil {
			fmt.Fprintf(opts.Log, format+"\n", args...)
		}
	}
	add := func(name string, ok bool, code cli.Code, detail string) {
		if ok {
			code = cli.ExitOK
		}
		r.Checks = append(r.Checks, Check{Name: name, OK: ok, Code: code, Detail: detail})
		state := "ok"
		if !ok {
			state = "FAIL"
		}
		logf("%-18s %-4s %s", name, state, detail)
	}
	t, err := Load(dir)
	if err != nil {
		add("schema", false, cli.CodeOf(err), err.Error())
		r.Code = cli.CodeOf(err)
		return r
	}
	r.Task = t.ID
	add("schema", true, 0, "task.toml valid")
	if h, err := t.ContentHash(); err == nil {
		r.Hash = h
	}
	if h, err := t.SnapshotHash(); err != nil || h != t.Repo.Snapshot {
		add("snapshot", false, cli.ExitIntegrity, fmt.Sprintf("repo.snapshot %s, computed %s", t.Repo.Snapshot, h))
	} else {
		add("snapshot", true, 0, h)
	}

	// Canary and leak scans (exit 7).
	canary := t.Canary()
	var missing []string
	filepath.WalkDir(t.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(t.Dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == t.Repo.Path || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "CANARY" || rel == "prompt.md" {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(raw, 0) >= 0 {
			return nil
		}
		if !bytes.Contains(raw, []byte(canary)) {
			missing = append(missing, rel)
		}
		return nil
	})
	prompt := t.Prompt()
	var leaks []string
	if strings.Contains(prompt, canary) {
		leaks = append(leaks, "canary in prompt.md")
	}
	filepath.WalkDir(t.RepoDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if raw, err := os.ReadFile(p); err == nil && bytes.Contains(raw, []byte(canary)) {
			rel, _ := filepath.Rel(t.Dir, p)
			leaks = append(leaks, "canary in workspace file "+filepath.ToSlash(rel))
		}
		return nil
	})
	switch {
	case len(missing) > 0:
		add("canary", false, cli.ExitContamination, "missing from "+strings.Join(missing, ", "))
	case len(leaks) > 0:
		add("canary", false, cli.ExitContamination, strings.Join(leaks, "; "))
	default:
		add("canary", true, 0, canary)
	}
	gold, broken, cheat := t.Controls()
	// The leak scan reads every agent-visible task file: prompt.md in
	// every arm, and contract.md, which is staged into the workspace of
	// an arm that has gate (docs/12 row 12). A contract's FROM: spans
	// quote prompt.md, never gold.patch, so they cannot trip the gold
	// rule; the oracle-name rule applies to them unchanged.
	visible := []struct{ name, text string }{{"prompt.md", prompt}, {"contract.md", t.Contract()}}
	var leak []string
	if gold != "" {
		raw, _ := os.ReadFile(gold)
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
				continue
			}
			body := strings.TrimSpace(line[1:])
			if len(strings.Join(strings.Fields(body), "")) < 20 {
				continue
			}
			for _, v := range visible {
				if strings.Contains(v.text, body) {
					leak = append(leak, fmt.Sprintf("gold line in %s: %.40q", v.name, body))
				}
			}
		}
	}
	// An oracle name the workspace already carries is not a hidden name:
	// the agent can list it before it reads either file, so naming it
	// says nothing about the hidden tests. Only the file name is
	// exempted, and only when a repo file of that exact name exists; a
	// shared directory name, or an oracle file with no repo twin, still
	// trips the rule.
	shadow := repoNames(t.RepoDir())
	oracleDir := filepath.Join(t.Dir, "oracle")
	filepath.WalkDir(oracleDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "run.sh" {
			return nil
		}
		if twin, ok := shadow[d.Name()]; ok {
			rel, _ := filepath.Rel(oracleDir, p)
			r.ShadowedNames = append(r.ShadowedNames, Shadowed{Oracle: filepath.ToSlash(rel), Repo: twin})
			return nil
		}
		for _, v := range visible {
			if strings.Contains(v.text, d.Name()) {
				leak = append(leak, "oracle file name in "+v.name+": "+d.Name())
			}
		}
		return nil
	})
	sort.Slice(r.ShadowedNames, func(i, j int) bool { return r.ShadowedNames[i].Oracle < r.ShadowedNames[j].Oracle })
	if len(leak) > 0 {
		add("leak", false, cli.ExitContamination, strings.Join(leak, "; "))
	} else {
		detail := "no gold line or oracle name in prompt.md or contract.md"
		if n := len(r.ShadowedNames); n > 0 {
			detail += fmt.Sprintf("; %d oracle name(s) exempt, already in the workspace", n)
		}
		add("leak", true, 0, detail)
	}

	// Cheat controls must be flagged by the scan (static half of the row).
	scanOpts := t.ScanOptions()
	for _, c := range cheat {
		raw, _ := os.ReadFile(c)
		res := Scan(raw, scanOpts)
		name := "scan:" + strings.TrimSuffix(filepath.Base(c), ".patch")
		if res.Flagged {
			add(name, true, 0, strings.Join(res.Detectors, ","))
		} else {
			add(name, false, cli.ExitFinding, "no detector fired")
		}
	}
	if opts.Static {
		r.Code = codeOf(r.Checks)
		return r
	}

	// Oracle runs.
	root := opts.Keep
	if root == "" {
		tmp, err := os.MkdirTemp("", "saga-verify-")
		if err != nil {
			add("workspace", false, cli.ExitEnvironment, err.Error())
			r.Code = codeOf(r.Checks)
			return r
		}
		defer os.RemoveAll(tmp)
		root = tmp
	}
	root = filepath.Join(root, t.ID)
	os.RemoveAll(root)
	runState := func(name, patch string) (*OracleResult, error) {
		ws := filepath.Join(root, name)
		if err := Stage(ctx, t, ws); err != nil {
			return nil, err
		}
		if patch != "" {
			if err := Apply(ctx, ws, patch); err != nil {
				return nil, fmt.Errorf("apply: %w", err)
			}
		}
		if err := Setup(ctx, t, ws); err != nil {
			return nil, err
		}
		res, err := Oracle(ctx, t, ws)
		if err != nil {
			return nil, err
		}
		p, f := res.Counts()
		r.States[name] = &StateResult{Exit: res.Exit, Pass: p, Fail: f}
		logf("state %-10s exit=%d tests=[%d pass/%d fail]", name, res.Exit, p, f)
		return res, nil
	}
	fail := func(name string, err error) {
		r.States[name] = &StateResult{Exit: -1, Error: err.Error()}
		add(name, false, cli.ExitEnvironment, err.Error())
	}

	base, err := runState("baseline", "")
	if err != nil {
		fail("baseline", err)
		r.Code = codeOf(r.Checks)
		return r
	}
	if t.Oracle.BaselineMustFail {
		add("baseline-red", base.Exit != 0, cli.ExitFinding, fmt.Sprintf("oracle exit %d on the untouched snapshot", base.Exit))
	}
	if ids := t.RegressionIDs(); len(ids) > 0 {
		var bad []string
		for _, id := range ids {
			if base.Tests[id] != "PASS" {
				bad = append(bad, id)
			}
		}
		add("regression-set", len(bad) == 0, cli.ExitFinding, fmt.Sprintf("%d ids, not passing at baseline: %v", len(ids), bad))
	}
	var goldRes *OracleResult
	if gold != "" {
		goldRes, err = runState("gold", gold)
		if err != nil {
			fail("gold", err)
		} else {
			add("gold-green", goldRes.Exit == 0, cli.ExitFinding, fmt.Sprintf("oracle exit %d with gold.patch", goldRes.Exit))
		}
	}
	for _, p := range broken {
		name := strings.TrimSuffix(filepath.Base(p), ".patch")
		res, err := runState(name, p)
		if err != nil {
			fail(name, err)
			continue
		}
		add(name+"-red", res.Exit != 0, cli.ExitFinding, fmt.Sprintf("oracle exit %d", res.Exit))
	}
	for _, p := range cheat {
		name := strings.TrimSuffix(filepath.Base(p), ".patch")
		res, err := runState(name, p)
		if err != nil {
			fail(name, err)
			continue
		}
		if t.Impossible() {
			add(name+"-red", res.Exit != 0, cli.ExitFinding, fmt.Sprintf("oracle exit %d (impossible: every patch must fail)", res.Exit))
		} else {
			add(name+"-green", res.Exit == 0, cli.ExitFinding, fmt.Sprintf("oracle exit %d (a cheat must reach green)", res.Exit))
		}
	}

	// The tamper control must be refused by the integrity probe. Its
	// oracle exit is 0 by construction, which is the whole point: a run
	// that forges the exit code must not grade as a solve (bench-spec
	// 2.4, 5.8; the route was found on 2026-09-06).
	if tp := t.TamperControl(); tp != "" {
		res, err := runState("cheat-tamper", tp)
		if err != nil {
			fail("cheat-tamper", err)
		} else {
			switch {
			case res.Integrity == IntegrityFail:
				add("tamper-flagged", true, 0, "integrity fail: "+res.IntegrityReason)
			case res.Pass:
				add("tamper-flagged", false, cli.ExitFinding, "the tamper reached a pass; the integrity probe did not see it")
			default:
				add("tamper-flagged", false, cli.ExitFinding, "integrity "+res.Integrity+", want fail")
			}
		}
	}

	// Determinism: the same tree graded again gives identical lines.
	detState, detRes := "gold", goldRes
	if detRes == nil {
		detState, detRes = "baseline", base
	}
	runs := 1
	if t.Oracle.Ceiling < 1 {
		runs = 4
	}
	ws := filepath.Join(root, detState)
	same := true
	passes := 0
	if detRes.Exit == 0 {
		passes = 1
	}
	for i := 0; i < runs && same; i++ {
		again, err := Oracle(ctx, t, ws)
		if err != nil {
			fail("determinism", err)
			same = false
			break
		}
		if again.Exit == 0 {
			passes++
		}
		if t.Oracle.Ceiling >= 1 && !bytes.Equal(again.Stdout, detRes.Stdout) {
			same = false
			add("determinism", false, cli.ExitFinding, fmt.Sprintf("oracle output on %s differs between runs", detState))
		}
	}
	if same {
		if t.Oracle.Ceiling < 1 {
			add("determinism", true, 0, fmt.Sprintf("ceiling %.2f: pass band %d/%d on %s", t.Oracle.Ceiling, passes, runs+1, detState))
		} else {
			add("determinism", true, 0, "oracle output identical on "+detState+" twice")
		}
	}
	r.Code = codeOf(r.Checks)
	return r
}

func codeOf(checks []Check) cli.Code {
	var codes []cli.Code
	for _, c := range checks {
		if !c.OK {
			codes = append(codes, c.Code)
		}
	}
	return cli.Precedence(codes...)
}

// Text renders the result as the fixed-width table verify-task prints.
func (r *VerifyResult) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", r.Task, r.Hash)
	for _, c := range r.Checks {
		state := "ok  "
		if !c.OK {
			state = fmt.Sprintf("FAIL(%d)", int(c.Code))
		}
		fmt.Fprintf(&b, "  %-18s %-8s %s\n", c.Name, state, c.Detail)
	}
	names := make([]string, 0, len(r.States))
	for n := range r.States {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := r.States[n]
		if s.Error != "" {
			fmt.Fprintf(&b, "  state %-10s error %s\n", n, s.Error)
			continue
		}
		fmt.Fprintf(&b, "  state %-10s exit=%d tests=[%d pass/%d fail]\n", n, s.Exit, s.Pass, s.Fail)
	}
	for _, sn := range r.ShadowedNames {
		fmt.Fprintf(&b, "  %-18s %-8s %s shadowed by %s\n", "leak:exempt", "ok  ", sn.Oracle, sn.Repo)
	}
	fmt.Fprintf(&b, "  result %s\n", r.Code)
	return b.String()
}

// JSON renders the result.
func (r *VerifyResult) JSON() []byte {
	b, _ := json.MarshalIndent(r, "", "  ")
	return append(b, '\n')
}

// repoNames maps every file name in the workspace tree to its first
// workspace-relative path, for the section 2.4 shadowing exemption.
func repoNames(dir string) map[string]string {
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if prev, ok := out[d.Name()]; !ok || rel < prev {
			out[d.Name()] = rel
		}
		return nil
	})
	return out
}
