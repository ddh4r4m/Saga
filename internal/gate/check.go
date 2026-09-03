package gate

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/snapshot"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// FailureTailBytes caps the failure diagnostic shown on the terminal
// (section 4.4; never persisted).
const FailureTailBytes = 4096

// CheckOptions tune `saga gate check` and `reverify`.
type CheckOptions struct {
	Only         []string
	Approve      bool
	NoRequireRed bool
	Advisory     bool
	TimeoutS     int
	// CI executes without an approval store (section 6.4).
	CI bool
	// Reverify ignores committed evidence and re-executes every runnable
	// gate; red records are re-validated against the current binding and
	// control proofs re-run.
	Reverify bool
	Session  string
	Turn     int
	Log      func(string)
}

func (o CheckOptions) log(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

// AgentShellError is the refusal when approve or attest runs from an
// agent's shell.
func AgentShellError(what string) error {
	if m := AgentShell(); m != "" {
		return cli.Errorf(cli.ExitRefusal, "saga gate %s is a human act; refused inside an agent shell (%s is set)", what, m)
	}
	return nil
}

type runCtx struct {
	l        *Loaded
	opts     CheckOptions
	tc       Toolchain
	approval string
	masker   *trace.Masker
	updates  map[string]EvidenceUpdate
	diag     map[string]string
	missing  []string
	snap     *snapshot.Snapshot
	dirty    bool
	guards   GuardsSummary
	findings []Finding
}

// Check executes the approved runnable gates, records evidence and red
// proofs, rewrites the ledger lines and returns the resulting status.
func Check(l *Loaded, opts CheckOptions) (*Report, error) {
	if opts.Approve {
		if err := AgentShellError("check --approve"); err != nil {
			return nil, err
		}
	}
	if err := l.CheckLedger(); err != nil {
		return nil, err
	}
	if opts.NoRequireRed {
		f := false
		l.Config.RequireRed = &f
	}
	rc := &runCtx{l: l, opts: opts, tc: CurrentToolchain(l.Config.Shell), updates: map[string]EvidenceUpdate{}, diag: map[string]string{}}
	rc.masker = trace.NewMasker(canon.ULID())
	if !opts.CI {
		dir, err := ApprovalDir(l.Root)
		if err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "", err)
		}
		rc.approval = dir
	}
	findings, err := GuardDiff(GuardInput{Root: l.Root, Store: l.Store, Base: l.Base, Contract: l.Contract, Config: l.Config, Advisory: opts.Advisory})
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "guards", err)
	}
	rc.findings = findings
	rc.guards = GuardsSummary{Clean: true, Waivers: []string{}}
	for _, f := range findings {
		if f.Blocks() {
			rc.guards.Clean = false
		}
		if f.Waived {
			rc.guards.Waivers = append(rc.guards.Waivers, f.ID+" "+f.Path+" "+f.Hunk)
		}
	}
	if entries, err := DiffPaths(l.Root, "HEAD"); err == nil {
		rc.dirty = len(entries) > 0
	}
	if snap, err := snapshot.Take(l.Root, "gate", opts.Session, opts.Turn, LedgerExclude...); err == nil {
		rc.snap = snap
	}
	for _, g := range l.Contract.Gates {
		if !selected(opts.Only, g.ID) || !g.Runnable() || l.Contract.Abandoned(g.ID) != nil {
			continue
		}
		if err := rc.gate(g); err != nil {
			return nil, err
		}
	}
	if len(rc.updates) > 0 {
		if err := store.WriteFileAtomic(join(l.Root, ContractPath), l.Contract.Rewrite(rc.updates), 0o644); err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "contract write", err)
		}
		c, err := Parse(l.Contract.Rewrite(rc.updates))
		if err != nil {
			return nil, cli.Wrap(cli.ExitUsage, "rewritten contract", err)
		}
		l.Contract = c
	}
	r, err := Status(l, StatusOptions{Advisory: opts.Advisory, SkipGuards: true})
	if err != nil {
		return nil, err
	}
	r.Guards = findings
	if r.Guards == nil {
		r.Guards = []Finding{}
	}
	r.ApprovalMissing = rc.missing
	for _, gs := range r.Gates {
		if d, ok := rc.diag[gs.gate.ID]; ok && d != "" {
			gs.Diagnostic = strp(d)
		}
	}
	r.finish(l, StatusOptions{Advisory: opts.Advisory})
	return r, nil
}

func (rc *runCtx) gate(g *Gate) error {
	l, opts := rc.l, rc.opts
	o := l.OracleFor(g, opts.TimeoutS)
	oh := o.Hash()
	_, wh := WitnessesIn(l.Root, l.Contract, g)
	qid := l.Contract.Qualified(g.ID)
	var approvalHash *string
	if !opts.CI {
		identity := ApprovalIdentity(ContractPath, g.ID, oh, rc.tc.Platform, os.Getenv("PATH"), wh)
		a, err := LoadApproval(rc.approval, identity)
		if err != nil {
			return cli.Wrap(cli.ExitEnvironment, "", err)
		}
		if a == nil && opts.Approve {
			if a, err = WriteApproval(rc.approval, identity, qid); err != nil {
				return cli.Wrap(cli.ExitEnvironment, "approve", err)
			}
			opts.log("approved %s: %s", qid, describeOracle(o))
		}
		if a == nil {
			rc.missing = append(rc.missing, qid)
			opts.log("%s: approval required; resolved oracle: %s", qid, describeOracle(o))
			return nil
		}
		approvalHash = strp(identity)
	}
	dir := join(l.Root, g.CWD)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return rc.record(g, o, oh, nil, false, "start-failure", "CWD: "+g.CWD+" is not a directory", approvalHash, nil)
	}
	ctx := context.Background()
	timeout := time.Duration(o.TimeoutS) * time.Second
	expect, _ := CompileExpect(g.Expect)
	binding := RedBinding{OracleHash: oh, WitnessHash: wh, RequestHash: l.RequestHash, Toolchain: rc.tc}

	// Red proof first: baseline needs the oracle run on the untouched tree.
	var main *Result
	var mainMatched bool
	redRec, _, _ := LoadRed(l.Store, l.Contract.Slug, g.ID)
	valid := false
	if redRec != nil {
		valid, _ = RedValid(l.Root, redRec, binding, l.Config)
		if !valid || (opts.Reverify && redRec.Mode == RedControl) {
			RemoveRed(l.Store, l.Contract.Slug, g.ID)
			redRec, valid = nil, false
		}
	}
	if !valid {
		switch g.RedMode() {
		case RedBaseline:
			empty, err := DiffEmpty(l.Root, l.Base, l.Contract.In, FoldCase())
			if err == nil && empty {
				main = Run(ctx, o.Shell, g.Check, dir, timeout, o.OutputCap)
				mainMatched, _ = expect.Match(main.Output)
				if ok, why := RealRed(main, mainMatched, l.Config); ok {
					rec := &RedRecord{Gate: qid, Mode: RedBaseline, OracleHash: oh, WitnessHash: wh, RequestHash: strp(l.RequestHash), ProvedAt: time.Now().UTC().Format(time.RFC3339), Base: ShortRev(l.Base), Red: summarize(main, mainMatched), Toolchain: rc.tc}
					if _, err := WriteRed(l.Store, l.Contract.Slug, g.ID, rec); err != nil {
						return cli.Wrap(cli.ExitEnvironment, "red record", err)
					}
					opts.log("%s: baseline red recorded (exit %d)", qid, main.Exit)
				} else {
					opts.log("%s: baseline red rejected: %s", qid, why)
				}
			}
		case RedControl:
			ctl := Run(ctx, o.Shell, g.RedCheck, dir, timeout, o.OutputCap)
			rexp, _ := CompileExpect(g.RedExpect)
			cm, _ := rexp.Match(ctl.Output)
			em, _ := expect.Match(ctl.Output)
			if ctl.Exit == 0 && cm && !em && !ctl.TimedOut && !ctl.Truncated {
				rec := &RedRecord{Gate: qid, Mode: RedControl, OracleHash: oh, WitnessHash: wh, RequestHash: strp(l.RequestHash), ProvedAt: time.Now().UTC().Format(time.RFC3339), Base: ShortRev(l.Base), Red: summarize(ctl, em), Toolchain: rc.tc}
				if _, err := WriteRed(l.Store, l.Contract.Slug, g.ID, rec); err != nil {
					return cli.Wrap(cli.ExitEnvironment, "red record", err)
				}
				opts.log("%s: control red recorded", qid)
			} else {
				opts.log("%s: control proof failed (exit %d, red-expect %t, expect %t)", qid, ctl.Exit, cm, em)
			}
		}
	}
	if main == nil {
		main = Run(ctx, o.Shell, g.Check, dir, timeout, o.OutputCap)
		mainMatched, _ = expect.Match(main.Output)
	}
	pass := main.Exit == 0 && mainMatched && !main.TimedOut && !main.Truncated && !main.StartFail
	failure := ""
	switch {
	case pass:
	case main.StartFail:
		failure = "start-failure"
	case main.TimedOut:
		failure = "timeout"
	case main.Truncated:
		failure = "output-cap"
	case main.Exit != 0:
		failure = "exit"
	default:
		failure = "no-match"
	}
	var redHash *string
	if rr, h, _ := LoadRed(l.Store, l.Contract.Slug, g.ID); rr != nil {
		redHash = strp(h)
	}
	return rc.record(g, o, oh, main, mainMatched, failure, "", approvalHash, redHash)
}

// relativize rewrites absolute paths under the repo root to repo-relative
// form in a diagnostic (section 4.4).
func relativize(root, s string) string {
	for _, r := range []string{root, "/private" + root} {
		s = strings.ReplaceAll(s, r+"/", "")
	}
	if canonRoot, err := filepath.EvalSymlinks(root); err == nil && canonRoot != root {
		s = strings.ReplaceAll(s, canonRoot+"/", "")
	}
	return s
}

func summarize(r *Result, matched bool) RunSummary {
	return RunSummary{Exit: r.Exit, Matched: matched, OutputSHA256: r.OutputSHA256(), OutputBytes: r.Bytes}
}

func describeOracle(o Oracle) string {
	cwd := o.CWD
	if cwd == "" {
		cwd = "."
	}
	return fmt.Sprintf("%s -c %q in %s, expect %q, timeout %ds", o.Shell, o.Check, cwd, o.Expect, o.TimeoutS)
}

// record writes the evidence record for one run and queues the ledger
// line update. Successful output is fingerprinted only; failing output
// is shown as a masked tail and never stored (section 4.4).
func (rc *runCtx) record(g *Gate, o Oracle, oh string, res *Result, matched bool, failure, note string, approval, red *string) error {
	l := rc.l
	pass := failure == ""
	rec := &EvidenceRecord{
		Gate: l.Contract.Qualified(g.ID), ContractHash: l.Contract.Hash(), OracleHash: oh, Outcome: OutcomeUnmet,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Resolved:  &Resolved{Shell: o.Shell, CWD: g.CWD, Platform: rc.tc.Platform, PathFingerprint: rc.tc.PathFingerprint, PathEntries: rc.tc.PathEntries, TimeoutS: o.TimeoutS, OutputCapBytes: o.OutputCap},
		Tree:      rc.tree(), RedProof: red, Guards: rc.guards, Approval: approval,
	}
	if pass {
		rec.Outcome = OutcomeMet
	} else {
		rec.Failure = strp(failure)
	}
	if res != nil {
		rec.StartedAt = res.StartedAt.UTC().Format(time.RFC3339)
		exit, bytes, dur := res.Exit, res.Bytes, res.DurationMS
		rec.ExitStatus, rec.OutputBytes, rec.DurationMS = &exit, &bytes, &dur
		rec.OutputSHA256 = strp(res.OutputSHA256())
		if matched {
			e, _ := CompileExpect(g.Expect)
			_, span := e.Match(res.Output)
			rec.Matched = &Matched{Kind: e.Kind, ExpectHash: canon.SHA256([]byte(g.Expect)), SpanBytes: span}
		}
		if !pass {
			tail, _ := rc.masker.MaskString(FailureTail(res.Output, FailureTailBytes))
			tail = relativize(l.Root, tail)
			rc.diag[g.ID] = fmt.Sprintf("exit %d, %s, %d bytes\n%s", res.Exit, failure, res.Bytes, tail)
		}
	} else if note != "" {
		rc.diag[g.ID] = note
	}
	hash, err := WriteEvidence(l.Store, l.Contract.Slug, g.ID, rec)
	if err != nil {
		return cli.Wrap(cli.ExitEnvironment, "evidence record", err)
	}
	u := EvidenceUpdate{Checked: pass}
	if pass {
		u.Evidence = hash
	}
	rc.updates[g.ID] = u
	return nil
}

func (rc *runCtx) tree() *TreeInfo {
	t := &TreeInfo{Base: ShortRev(rc.l.Base), Head: rc.l.Head, Dirty: rc.dirty}
	if rc.l.TreeHash != "" {
		t.WorktreeHash = strp(rc.l.TreeHash)
	}
	if rc.snap != nil {
		t.SnapshotID = strp(rc.snap.ID)
	}
	return t
}

// Attest meets a manual gate by a human act (section 4.2).
func Attest(l *Loaded, id, note string) (*Report, error) {
	if err := AgentShellError("attest"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(note) == "" {
		return nil, cli.Errorf(cli.ExitUsage, "attest: --note is required")
	}
	g := l.Contract.Gate(id)
	if g == nil {
		return nil, cli.Errorf(cli.ExitUsage, "attest: unknown gate %s", id)
	}
	if g.Runnable() {
		return nil, cli.Errorf(cli.ExitUsage, "attest: %s is runnable; run saga gate check", id)
	}
	if l.Contract.Abandoned(id) != nil {
		return nil, cli.Errorf(cli.ExitFinding, "attest: %s is abandoned", id)
	}
	by := "unknown"
	if u, err := user.Current(); err == nil {
		by = u.Username
	}
	tc := CurrentToolchain(l.Config.Shell)
	rc := &runCtx{l: l, tc: tc, guards: GuardsSummary{Clean: true, Waivers: []string{}}}
	if entries, err := DiffPaths(l.Root, "HEAD"); err == nil {
		rc.dirty = len(entries) > 0
	}
	rec := &EvidenceRecord{
		Gate: l.Contract.Qualified(id), ContractHash: l.Contract.Hash(), OracleHash: canon.SHA256([]byte("manual")), Outcome: OutcomeAttested,
		StartedAt: time.Now().UTC().Format(time.RFC3339), Tree: rc.tree(), Guards: rc.guards, By: by, Note: canon.CleanText(note),
	}
	hash, err := WriteEvidence(l.Store, l.Contract.Slug, id, rec)
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "evidence record", err)
	}
	raw := l.Contract.Rewrite(map[string]EvidenceUpdate{id: {Checked: true, Evidence: hash}})
	if err := store.WriteFileAtomic(join(l.Root, ContractPath), raw, 0o644); err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "contract write", err)
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "rewritten contract", err)
	}
	l.Contract = c
	return Status(l, StatusOptions{})
}

// Approve records (or revokes) consent for the selected runnable gates
// and returns one line per gate naming the resolved oracle.
func Approve(l *Loaded, only []string, revoke bool) ([]string, error) {
	if err := AgentShellError("approve"); err != nil {
		return nil, err
	}
	dir, err := ApprovalDir(l.Root)
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "", err)
	}
	tc := CurrentToolchain(l.Config.Shell)
	var lines []string
	for _, g := range l.Contract.Gates {
		if !selected(only, g.ID) || !g.Runnable() {
			continue
		}
		o := l.OracleFor(g, 0)
		_, wh := WitnessesIn(l.Root, l.Contract, g)
		identity := ApprovalIdentity(ContractPath, g.ID, o.Hash(), tc.Platform, os.Getenv("PATH"), wh)
		if revoke {
			if err := RevokeApproval(dir, identity); err != nil {
				return lines, cli.Wrap(cli.ExitEnvironment, "revoke", err)
			}
			lines = append(lines, fmt.Sprintf("revoked %s", l.Contract.Qualified(g.ID)))
			continue
		}
		if _, err := WriteApproval(dir, identity, l.Contract.Qualified(g.ID)); err != nil {
			return lines, cli.Wrap(cli.ExitEnvironment, "approve", err)
		}
		lines = append(lines, fmt.Sprintf("approved %s: %s", l.Contract.Qualified(g.ID), describeOracle(o)))
	}
	if len(lines) == 0 {
		return lines, cli.Errorf(cli.ExitUsage, "approve: no runnable gate selected")
	}
	return lines, nil
}
