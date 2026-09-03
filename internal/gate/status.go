package gate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/snapshot"
	"github.com/ddh4r4m/saga/internal/store"
)

// LedgerExclude are the paths gate writes; they are left out of the tree
// hash so the checker's own writes never move it.
var LedgerExclude = []string{".saga/evidence", ".saga/red", ".saga/contract.md", ".saga/observed", ".saga/snap", ".saga/trace"}

// Gate states of saga.gate.status/1.
const (
	StateMet       = "met"
	StateUnmet     = "unmet"
	StateUnproven  = "unproven"
	StateAttested  = "attested"
	StateAbandoned = "abandoned"
	StateManual    = "manual"
)

// Loaded is a contract with everything the subcommands share: the
// resolved base, the config read from it, the verified request, and the
// tree hash.
type Loaded struct {
	Root        string
	Store       *store.Store
	Contract    *Contract
	Base        string
	BaseDisplay string
	Head        string
	Config      Config
	Request     []byte
	RequestHash string
	Sentences   []Sentence
	Mode        string
	TreeHash    string
	RiskRemoved bool
	// Missing is set when the contract does not exist in the working tree.
	Missing bool
	// TrackedAtHead reports a contract tracked at HEAD (the hook's
	// "contract deleted" case).
	TrackedAtHead bool
}

// Load reads and verifies the contract and computes the tree hash.
// Errors carry the uniform exit code: 6 for environment refusals, 2 for
// parse failures.
func Load(root string, s *store.Store) (*Loaded, error) { return LoadWith(root, s, true) }

// LoadLite is Load without the tree hash, for the tool-event hook steps
// where a write-tree over a large repository would cost too much.
func LoadLite(root string, s *store.Store) (*Loaded, error) { return LoadWith(root, s, false) }

// LoadWith is Load with the tree hash optional.
func LoadWith(root string, s *store.Store, withTree bool) (*Loaded, error) {
	l := &Loaded{Root: root, Store: s}
	if !snapshot.IsRepo(root) {
		return l, cli.Errorf(cli.ExitEnvironment, "%s is not a git repository", root)
	}
	l.Head = snapshot.Head(root)
	l.TrackedAtHead = l.Head != "" && TrackedAt(root, "HEAD", ContractPath)
	p := join(root, ContractPath)
	if err := store.CheckShape(p); err != nil {
		return l, cli.Wrap(cli.ExitEnvironment, "", err)
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		l.Missing = true
		return l, cli.Errorf(cli.ExitUsage, "no %s; run saga gate init", ContractPath)
	}
	if err != nil {
		return l, cli.Wrap(cli.ExitEnvironment, "", err)
	}
	c, err := Parse(raw)
	if err != nil {
		return l, cli.Wrap(cli.ExitUsage, "", err)
	}
	l.Contract = c
	base, err := ResolveBase(root, c.Base)
	if err != nil {
		return l, cli.Wrap(cli.ExitUsage, "", err)
	}
	l.Base = base
	l.BaseDisplay = c.Base
	if l.BaseDisplay == "" {
		l.BaseDisplay = "HEAD"
	}
	cfg, err := LoadConfig(root, base)
	if err != nil {
		return l, cli.Wrap(cli.ExitUsage, "", err)
	}
	l.Config = cfg
	l.Mode = "minimal"
	if cfg.Present {
		l.Mode = "full"
	}
	rp := join(root, RequestPath)
	if err := store.CheckShape(rp); err != nil {
		return l, cli.Wrap(cli.ExitEnvironment, "", err)
	}
	if req, err := os.ReadFile(rp); err == nil {
		l.Request = req
		l.RequestHash = canon.SHA256(req)
	}
	sents, err := VerifyRequest(c, l.Request, l.RequestHash)
	if err != nil {
		return l, cli.Wrap(cli.ExitUsage, "", err)
	}
	l.Sentences = sents
	if bc, ok := ShowAt(root, base, ContractPath); ok && c.Risk == "" {
		if bcc, err := Parse(bc); err == nil && bcc.Risk != "" {
			l.RiskRemoved = true
			c.Risk = bcc.Risk
		}
	}
	if withTree {
		if th, err := snapshot.Tree(root, LedgerExclude...); err == nil {
			l.TreeHash = th
		}
	}
	return l, nil
}

// CheckLedger applies row 16 of section 2.6: every EVIDENCE: line must
// name a record that exists.
func (l *Loaded) CheckLedger() error {
	for _, g := range l.Contract.Gates {
		if g.Evidence == "" {
			continue
		}
		rec, _, err := LoadEvidence(l.Store, l.Contract.Slug, g.ID)
		if err != nil {
			return cli.Wrap(cli.ExitUsage, "", &ParseError{Rule: 16, Line: g.Line + 1, Msg: err.Error()})
		}
		if rec == nil {
			return cli.Wrap(cli.ExitUsage, "", &ParseError{Rule: 16, Line: g.Line + 1, Msg: fmt.Sprintf("gate %s EVIDENCE: names no record in the store", g.ID)})
		}
	}
	return nil
}

// OracleFor resolves a gate's oracle under the loaded config.
func (l *Loaded) OracleFor(g *Gate, timeoutS int) Oracle {
	if timeoutS <= 0 {
		timeoutS = l.Config.TimeoutS
	}
	return Oracle{Check: g.Check, Expect: g.Expect, CWD: g.CWD, Shell: l.Config.Shell, TimeoutS: timeoutS, OutputCap: OutputCap}
}

// FromRef is one FROM: reference in status output.
type FromRef struct {
	Sentence string `json:"sentence"`
	Quote    string `json:"quote"`
}

// RedStatus is the red-proof view of one gate.
type RedStatus struct {
	Mode     string  `json:"mode"`
	Valid    bool    `json:"valid"`
	ProvedAt *string `json:"proved_at"`
	Reason   *string `json:"reason"`
}

// GateStatus is one row of saga.gate.status/1.
type GateStatus struct {
	ID         string    `json:"id"`
	Outcome    string    `json:"outcome"`
	State      string    `json:"state"`
	Runnable   bool      `json:"runnable"`
	From       []FromRef `json:"from"`
	Evidence   *string   `json:"evidence"`
	Red        RedStatus `json:"red"`
	Approval   string    `json:"approval"`
	Failure    *string   `json:"failure"`
	Diagnostic *string   `json:"diagnostic"`

	gate *Gate
}

// Summary counts states.
type Summary struct {
	Gates     int `json:"gates"`
	Met       int `json:"met"`
	Unmet     int `json:"unmet"`
	Abandoned int `json:"abandoned"`
	Attested  int `json:"attested"`
	Unproven  int `json:"unproven"`
	Manual    int `json:"manual"`
	Waivers   int `json:"waivers"`
}

// Uncovered is one sentence no gate claims to discharge.
type UncoveredSentence struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Confidence string `json:"confidence"`
}

// Coverage is the section 2.4 report.
type Coverage struct {
	Sentences int                 `json:"sentences"`
	Covered   int                 `json:"covered"`
	Ignored   int                 `json:"ignored"`
	Uncovered []UncoveredSentence `json:"uncovered"`
}

// Handoff names an abandoned gate.
type Handoff struct {
	Gate  string `json:"gate"`
	State string `json:"state"`
}

// Budget is the hook emission accounting.
type Budget struct {
	BytesEmitted int `json:"bytes_emitted"`
	TokensEst    int `json:"tokens_est"`
	Ceiling      int `json:"ceiling"`
}

// Report is saga.gate.status/1 (section 7.2).
type Report struct {
	Schema       string        `json:"schema"`
	Contract     string        `json:"contract"`
	ContractHash string        `json:"contract_hash"`
	Base         string        `json:"base"`
	Head         string        `json:"head"`
	TreeHash     *string       `json:"tree_hash"`
	Mode         string        `json:"mode"`
	Risk         *string       `json:"risk"`
	RiskRemoved  bool          `json:"risk_removed"`
	Exit         int           `json:"exit"`
	ProgressHash string        `json:"progress_hash"`
	Summary      Summary       `json:"summary"`
	Gates        []*GateStatus `json:"gates"`
	Coverage     *Coverage     `json:"coverage"`
	Guards       []Finding     `json:"guards"`
	Handoff      []Handoff     `json:"handoff"`
	Budget       Budget        `json:"budget"`
	Lint         []LintWarning `json:"lint,omitempty"`

	// Lines is the human rendering, one per gate plus the summary.
	Lines []string `json:"-"`
	// ApprovalMissing lists gates that need approval (exit 4).
	ApprovalMissing []string `json:"-"`
}

// Validate checks the report against its schema.
func (r *Report) Validate() error {
	v, err := schema.Normalize(r)
	if err != nil {
		return err
	}
	return schema.ValidateID(StatusSchema, v)
}

// StatusOptions tune a status computation.
type StatusOptions struct {
	Strict   bool
	Advisory bool
	Only     []string
	// SkipGuards leaves the guard run out (predict-only callers).
	SkipGuards bool
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func selected(only []string, id string) bool {
	if len(only) == 0 {
		return true
	}
	for _, o := range only {
		if o == id {
			return true
		}
	}
	return false
}

// Status computes the ledger state without executing anything (section
// 7: status never runs CHECK:).
func Status(l *Loaded, opts StatusOptions) (*Report, error) {
	c := l.Contract
	r := &Report{Schema: StatusSchema, Contract: c.Slug, ContractHash: c.Hash(), Base: ShortRev(l.Base), Head: l.Head, TreeHash: strp(l.TreeHash), Mode: l.Mode, Risk: strp(c.Risk), RiskRemoved: l.RiskRemoved, Guards: []Finding{}, Handoff: []Handoff{}, Budget: Budget{Ceiling: 400}}
	tc := CurrentToolchain(l.Config.Shell)
	approvalDir, _ := ApprovalDir(l.Root)
	for _, g := range c.Gates {
		gs := &GateStatus{ID: c.Qualified(g.ID), Outcome: canon.CleanText(g.Outcome), Runnable: g.Runnable(), From: []FromRef{}, Approval: "n/a", Red: RedStatus{Mode: g.RedMode()}, gate: g}
		for _, f := range g.From {
			gs.From = append(gs.From, FromRef{Sentence: fmt.Sprintf("R%d", f.Sentence), Quote: f.Quote})
		}
		rec, hash, _ := LoadEvidence(l.Store, c.Slug, g.ID)
		if rec != nil && hash == g.Evidence && g.Evidence != "" {
			gs.Evidence = strp(hash)
		}
		switch {
		case c.Abandoned(g.ID) != nil:
			gs.State = StateAbandoned
			r.Handoff = append(r.Handoff, Handoff{Gate: gs.ID, State: StateAbandoned})
		case !g.Runnable():
			gs.State = StateManual
			if gs.Evidence != nil && rec.Outcome == OutcomeAttested {
				gs.State = StateAttested
			}
		default:
			o := l.OracleFor(g, 0)
			oh := o.Hash()
			_, wh := WitnessesIn(l.Root, l.Contract, g)
			if approvalDir != "" {
				identity := ApprovalIdentity(ContractPath, g.ID, oh, tc.Platform, os.Getenv("PATH"), wh)
				if a, _ := LoadApproval(approvalDir, identity); a != nil {
					gs.Approval = "present"
				} else {
					gs.Approval = "missing"
				}
			} else {
				gs.Approval = "missing"
			}
			red, _, _ := LoadRed(l.Store, c.Slug, g.ID)
			gs.Red = redStatus(l, g, red, RedBinding{OracleHash: oh, WitnessHash: wh, RequestHash: l.RequestHash, Toolchain: tc})
			gs.State = StateUnmet
			if gs.Evidence != nil && rec.Outcome == OutcomeMet && rec.OracleHash == oh {
				if rec.Tree != nil && rec.Tree.WorktreeHash != nil && l.TreeHash != "" && *rec.Tree.WorktreeHash != l.TreeHash {
					gs.Failure = strp("stale: tree changed since the check; run saga gate check")
				} else if l.Config.RequireRedOn() && !gs.Red.Valid {
					gs.State = StateUnproven
				} else {
					gs.State = StateMet
				}
			} else if rec != nil && rec.Failure != nil {
				gs.Failure = rec.Failure
			}
		}
		r.Gates = append(r.Gates, gs)
	}
	if !opts.SkipGuards {
		findings, err := GuardDiff(GuardInput{Root: l.Root, Store: l.Store, Base: l.Base, Contract: c, Config: l.Config, Advisory: opts.Advisory})
		if err != nil {
			return r, cli.Wrap(cli.ExitEnvironment, "guards", err)
		}
		if findings != nil {
			r.Guards = findings
		}
	}
	r.Coverage = coverage(l)
	r.finish(l, opts)
	return r, nil
}

func redStatus(l *Loaded, g *Gate, rec *RedRecord, b RedBinding) RedStatus {
	rs := RedStatus{Mode: g.RedMode()}
	if g.RedMode() == RedNone {
		rs.Reason = strp(ReasonDeclaredNone)
		return rs
	}
	if rec == nil {
		switch g.RedMode() {
		case RedBaseline:
			if empty, err := DiffEmpty(l.Root, l.Base, l.Contract.In, FoldCase()); err == nil && !empty {
				rs.Reason = strp(ReasonBaselineMissed)
			}
		case RedMutation:
			rs.Reason = strp(ReasonNoOperator)
		}
		return rs
	}
	ok, why := RedValid(l.Root, rec, b, l.Config)
	rs.Valid = ok
	rs.ProvedAt = strp(rec.ProvedAt)
	if !ok {
		rs.Reason = strp(why)
	}
	return rs
}

func coverage(l *Loaded) *Coverage {
	if l.Sentences == nil {
		return nil
	}
	covered := map[int]bool{}
	for _, g := range l.Contract.Gates {
		for _, f := range g.From {
			covered[f.Sentence] = true
		}
	}
	cov := &Coverage{Sentences: len(l.Sentences), Uncovered: []UncoveredSentence{}}
	for _, s := range l.Sentences {
		switch Classify(s, covered[s.N]) {
		case Covered:
			cov.Covered++
		case Ignored:
			cov.Ignored++
		default:
			cov.Uncovered = append(cov.Uncovered, UncoveredSentence{ID: s.ID, Text: canon.CleanText(s.Text), Confidence: "heuristic"})
		}
	}
	return cov
}

// finish fills the summary, exit code, progress hash and text lines.
func (r *Report) finish(l *Loaded, opts StatusOptions) {
	var codes []cli.Code
	r.Summary = Summary{Gates: len(r.Gates)}
	var tuples []string
	for _, gs := range r.Gates {
		switch gs.State {
		case StateMet:
			r.Summary.Met++
		case StateUnmet:
			r.Summary.Unmet++
			codes = append(codes, cli.ExitFinding)
		case StateUnproven:
			r.Summary.Unproven++
			codes = append(codes, cli.ExitIntegrity)
		case StateAttested:
			r.Summary.Attested++
		case StateAbandoned:
			r.Summary.Abandoned++
			codes = append(codes, cli.ExitFinding)
		case StateManual:
			r.Summary.Manual++
			codes = append(codes, cli.ExitFinding)
		}
		ev := ""
		if gs.Evidence != nil {
			ev = *gs.Evidence
		}
		tuples = append(tuples, fmt.Sprintf("%s\x00%s\x00%s\x00%t", gs.ID, gs.State, ev, gs.Red.Valid))
	}
	for _, f := range r.Guards {
		if f.Waived {
			r.Summary.Waivers++
		}
		if f.Blocks() {
			codes = append(codes, cli.ExitRefusal)
		}
	}
	if len(r.ApprovalMissing) > 0 {
		codes = append(codes, cli.ExitApproval)
	}
	if opts.Strict && r.Coverage != nil && len(r.Coverage.Uncovered) > 0 {
		codes = append(codes, cli.ExitFinding)
	}
	sort.Strings(tuples)
	tuples = append(tuples, "claim\x00null")
	r.ProgressHash = canon.SHA256([]byte(strings.Join(tuples, "\n")))
	r.Exit = int(cli.Precedence(codes...))
	r.Lines = r.render()
}

func (r *Report) render() []string {
	var lines []string
	for _, gs := range r.Gates {
		label := strings.ToUpper(gs.State)
		switch gs.State {
		case StateUnproven:
			label = "MET (UNPROVEN"
			if gs.Red.Reason != nil {
				label += ": " + *gs.Red.Reason
			}
			label += ")"
		case StateUnmet:
			if gs.Failure != nil {
				label += " (" + *gs.Failure + ")"
			}
		}
		if gs.Runnable && gs.Approval == "missing" {
			label += "  [approval required: saga gate approve --gate " + gs.gate.ID + "]"
		}
		lines = append(lines, fmt.Sprintf("%-28s %s", gs.ID, label))
		if gs.Diagnostic != nil && *gs.Diagnostic != "" {
			for _, dl := range strings.Split(strings.TrimRight(*gs.Diagnostic, "\n"), "\n") {
				lines = append(lines, "    | "+dl)
			}
		}
	}
	for _, f := range r.Guards {
		state := "BLOCKS"
		if f.Waived {
			state = "waived"
		} else if f.Advisory {
			state = "advisory"
		}
		lines = append(lines, fmt.Sprintf("%-28s %s %s %s (%s pre=%d post=%d)", f.ID, state, f.Path, f.Hunk, f.Rule, f.Pre, f.Post))
	}
	if r.Coverage != nil {
		for _, u := range r.Coverage.Uncovered {
			lines = append(lines, fmt.Sprintf("%-28s uncovered (heuristic): %s", u.ID, u.Text))
		}
	}
	switch {
	case len(r.Handoff) > 0:
		lines = append(lines, "HANDOFF REQUIRED")
	case r.Exit == 0:
		lines = append(lines, "ALL MET")
	case r.Exit == int(cli.ExitApproval):
		lines = append(lines, "APPROVAL REQUIRED")
	default:
		lines = append(lines, fmt.Sprintf("%d unmet, %d unproven, %d manual, %d guard findings", r.Summary.Unmet, r.Summary.Unproven, r.Summary.Manual, len(r.Guards)))
	}
	return lines
}

// StopReason is the ids-only Stop block text of section 9, capped at the
// ceiling; over budget it collapses to the fixed 20-token line.
func (r *Report) StopReason(sessionShareLeft int) string {
	var parts []string
	var unmet []string
	for _, gs := range r.Gates {
		if gs.State == StateUnmet || gs.State == StateUnproven || gs.State == StateManual {
			unmet = append(unmet, gs.ID+"("+gs.State+")")
		}
	}
	n := len(unmet)
	if n > 0 {
		parts = append(parts, "unmet "+strings.Join(unmet, " "))
	}
	var guards []string
	for _, f := range r.Guards {
		if f.Blocks() {
			guards = append(guards, f.ID+" "+f.Path)
		}
	}
	if len(guards) > 0 {
		parts = append(parts, "guards "+strings.Join(guards, ", "))
	}
	if r.Coverage != nil && len(r.Coverage.Uncovered) > 0 {
		var ids []string
		for _, u := range r.Coverage.Uncovered {
			ids = append(ids, u.ID)
		}
		parts = append(parts, "uncovered "+strings.Join(ids, " "))
	}
	if len(r.Handoff) > 0 {
		var ids []string
		for _, h := range r.Handoff {
			ids = append(ids, h.Gate)
		}
		parts = append(parts, "handoff "+strings.Join(ids, " "))
	}
	msg := "saga gate: " + strings.Join(parts, "; ") + "; run: saga gate status"
	if canon.TokensEstString(msg) > 400 || canon.TokensEstString(msg) > sessionShareLeft {
		msg = fmt.Sprintf("saga gate: %d unmet; run saga gate status", n+len(guards))
	}
	return msg
}
