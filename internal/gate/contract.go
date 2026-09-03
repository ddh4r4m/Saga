// Package gate implements `saga gate` (docs/specs/gate-spec.md v0.3):
// the contract parser, request traceability, red proof, evidence,
// diff guards, the approval store and the hook step. Every decision is
// a parse, a hash, an exit status, a glob or a regex match; nothing here
// consults a model.
package gate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
)

// ContractPath is the repo-relative path of the active contract.
const ContractPath = ".saga/contract.md"

// RequestPath is the repo-relative path of the verbatim request.
const RequestPath = ".saga/request.md"

// MaxContractBytes is the size cap of gate-spec section 2.6 row 17.
const MaxContractBytes = 1 << 20

// Known guard ids, for WAIVE: validation (section 5.1).
var GuardIDs = map[string]bool{"G-SCOPE": true, "G-TESTDEL": true, "G-SKIP": true, "G-LEDGER": true, "G-ASSERT": true, "G-DEP": true, "G-HARDCODE": true}

// Red proof modes (section 3.2).
const (
	RedBaseline = "baseline"
	RedMutation = "mutation"
	RedControl  = "control"
	RedNone     = "none"
)

// Contract is a parsed .saga/contract.md.
type Contract struct {
	Title string
	// Slug is CONTRACT: or the slug of the title; it names the evidence
	// namespace and prefixes every qualified gate id.
	Slug string
	// Headers.
	ContractHeader string
	Request        string
	In             []string
	Out            []string
	Base           string
	Risk           string

	Gates   []*Gate
	Abandon []Abandon
	Waive   []Waive

	// Raw form, kept for the checker's writes.
	Lines           []string
	EOL             string
	TrailingNewline bool
	raw             []byte
}

// Gate is one ledger row.
type Gate struct {
	ID      string
	Outcome string
	Checked bool
	// Line is the 0-based index of the gate line; AttrLines the indices
	// of its attribute lines in order; Indent the indent of the first
	// attribute (four spaces when there is none).
	Line      int
	AttrLines []int
	Indent    string

	Check     string
	Expect    string
	CWD       string
	Red       string
	RedCheck  string
	RedExpect string
	Evidence  string
	From      []From
	Witness   []string

	has map[string]bool
}

// Has reports whether the attribute key appeared on the gate.
func (g *Gate) Has(key string) bool { return g.has[key] }

// Runnable reports whether the gate has CHECK: and EXPECT:.
func (g *Gate) Runnable() bool { return g.Has("CHECK") && g.Has("EXPECT") }

// RedMode is the declared or default red-proof mode.
func (g *Gate) RedMode() string {
	if g.Red == "" {
		return RedBaseline
	}
	return g.Red
}

// From is one `FROM: R<n> "<quote>"` line.
type From struct {
	Sentence int
	Quote    string
	Line     int
}

// Abandon is one `ABANDON: <id> <reason>` statement.
type Abandon struct {
	ID     string
	Reason string
	Line   int
}

// Waive is one `WAIVE: <guard> <path> <hunk> <reason>` statement.
type Waive struct {
	Guard  string
	Path   string
	Hunk   string
	Reason string
	Line   int
}

// ParseError is a fail-closed parse failure: exit 2, no evidence, never
// ALL MET. Rule is the row of gate-spec section 2.6 (0 for a grammar
// failure the table does not enumerate).
type ParseError struct {
	Rule int
	Line int
	Msg  string
}

// ID names the rule as "2.6-<row>" ("grammar" for row 0).
func (e *ParseError) ID() string {
	if e.Rule == 0 {
		return "grammar"
	}
	return "2.6-" + strconv.Itoa(e.Rule)
}

// Error implements error.
func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("contract.md:%d: %s: %s", e.Line, e.ID(), e.Msg)
	}
	return fmt.Sprintf("contract.md: %s: %s", e.ID(), e.Msg)
}

func perr(rule, line int, format string, args ...any) *ParseError {
	return &ParseError{Rule: rule, Line: line, Msg: fmt.Sprintf(format, args...)}
}

var (
	reTitle   = regexp.MustCompile(`^# (\S.*)$`)
	reHeader  = regexp.MustCompile(`^(CONTRACT|REQUEST|IN|OUT|BASE|RISK):( .*)?$`)
	reGate    = regexp.MustCompile(`^- \[( |x)\] (.*)$`)
	reGateID  = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*):( .*)?$`)
	reAttr    = regexp.MustCompile(`^(CHECK|EXPECT|CWD|FROM|RED|RED-CHECK|RED-EXPECT|WITNESS|EVIDENCE):( .*)?$`)
	reStmt    = regexp.MustCompile(`^(ABANDON|WAIVE):( .*)?$`)
	reFence   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	reFrom    = regexp.MustCompile(`^R([1-9][0-9]*) "(.+)"$`)
	reGuardID = regexp.MustCompile(`^G-[A-Z][A-Z-]*$`)
	reHunk    = regexp.MustCompile(`^[0-9a-f]{12}$`)
	reSHA     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	reSlug    = regexp.MustCompile(`[^a-z0-9]+`)
)

// Parse parses contract bytes under the section 2.2 grammar and the
// section 2.6 rules that need no other file (rows 1 to 10, 13 to 15, 18,
// 19 and the size cap of 17). Rows 11, 12 and 16 need the request file
// and the evidence store: see VerifyRequest and CheckLedger.
func Parse(raw []byte) (*Contract, error) {
	if len(raw) > MaxContractBytes {
		return nil, perr(17, 0, "contract exceeds %d bytes", MaxContractBytes)
	}
	c := &Contract{raw: raw, EOL: "\n"}
	text := string(raw)
	if i := strings.IndexByte(text, '\n'); i > 0 && text[i-1] == '\r' {
		c.EOL = "\r\n"
	}
	c.TrailingNewline = strings.HasSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	if c.TrailingNewline {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	c.Lines = lines
	if len(lines) == 0 {
		return nil, perr(0, 1, "empty contract; the first line must be a `# <title>` line")
	}
	m := reTitle.FindStringSubmatch(lines[0])
	if m == nil {
		return nil, perr(0, 1, "the first line must be `# <title>`")
	}
	c.Title = strings.TrimSpace(m[1])

	ids := map[string]int{}
	var cur *Gate
	inFence := false
	var fenceChar byte
	fenceLen := 0
	sawGate := false
	for i := 1; i < len(lines); i++ {
		l := lines[i]
		n := i + 1
		if inFence {
			if fm := reFence.FindStringSubmatch(l); fm != nil && fm[1][0] == fenceChar && len(fm[1]) >= fenceLen && strings.TrimSpace(fm[2]) == "" {
				inFence = false
			}
			continue
		}
		if fm := reFence.FindStringSubmatch(l); fm != nil {
			inFence, fenceChar, fenceLen = true, fm[1][0], len(fm[1])
			cur = nil
			continue
		}
		if strings.TrimSpace(l) == "" {
			cur = nil
			continue
		}
		lead := len(l) - len(strings.TrimLeft(l, " \t"))
		if strings.Contains(l[:lead], "\t") {
			return nil, perr(6, n, "tab indentation")
		}
		if lead == 0 {
			if gm := reGate.FindStringSubmatch(l); gm != nil {
				cur = nil
				im := reGateID.FindStringSubmatch(gm[2])
				if im == nil {
					return nil, perr(3, n, "gate line needs `<id>: <outcome>`; id is ALPHA {ALNUM|-|_}")
				}
				outcome := strings.TrimSpace(im[2])
				if outcome == "" {
					return nil, perr(0, n, "gate %s has an empty outcome", im[1])
				}
				if prev, dup := ids[im[1]]; dup {
					return nil, perr(2, n, "duplicate gate id %s (first at line %d)", im[1], prev)
				}
				ids[im[1]] = n
				g := &Gate{ID: im[1], Outcome: outcome, Checked: gm[1] == "x", Line: i, Indent: "    ", has: map[string]bool{}}
				c.Gates = append(c.Gates, g)
				cur = g
				sawGate = true
				continue
			}
			if hm := reHeader.FindStringSubmatch(l); hm != nil {
				cur = nil
				if sawGate {
					return nil, perr(14, n, "header %s: after a gate", hm[1])
				}
				v := strings.TrimSpace(hm[2])
				if v == "" {
					return nil, perr(0, n, "header %s: has no value", hm[1])
				}
				if err := c.setHeader(hm[1], v, n); err != nil {
					return nil, err
				}
				continue
			}
			if sm := reStmt.FindStringSubmatch(l); sm != nil {
				cur = nil
				v := strings.TrimSpace(sm[2])
				if sm[1] == "ABANDON" {
					id, reason, _ := strings.Cut(v, " ")
					reason = strings.TrimSpace(reason)
					if id == "" || reason == "" {
						return nil, perr(13, n, "ABANDON: needs `<id> <reason>`")
					}
					c.Abandon = append(c.Abandon, Abandon{ID: id, Reason: reason, Line: i})
				} else {
					f := strings.Fields(v)
					if len(f) < 4 {
						return nil, perr(18, n, "WAIVE: needs `<guard-id> <path> <hunk-hash> <reason>`")
					}
					c.Waive = append(c.Waive, Waive{Guard: f[0], Path: f[1], Hunk: f[2], Reason: strings.Join(f[3:], " "), Line: i})
				}
				continue
			}
			if am := reAttr.FindStringSubmatch(l); am != nil {
				return nil, perr(5, n, "%s: at column 1; gate attributes are indented 2 to 4 spaces", am[1])
			}
			// prose
			cur = nil
			continue
		}
		body := l[lead:]
		if sm := reStmt.FindStringSubmatch(body); sm != nil {
			return nil, perr(14, n, "%s: must start at column 1", sm[1])
		}
		am := reAttr.FindStringSubmatch(body)
		if am == nil {
			// Indented prose (a continuation line); it ends the gate block.
			cur = nil
			continue
		}
		if lead > 4 {
			return nil, perr(5, n, "%s: indented %d spaces; attributes are indented 2 to 4", am[1], lead)
		}
		if cur == nil {
			return nil, perr(5, n, "%s: is not attached to a gate", am[1])
		}
		v := strings.TrimSpace(am[2])
		if v == "" {
			return nil, perr(0, n, "%s: has no value", am[1])
		}
		if err := cur.setAttr(am[1], v, i, n); err != nil {
			return nil, err
		}
		if len(cur.AttrLines) == 1 {
			cur.Indent = l[:lead]
		}
	}
	if err := c.validate(ids); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Contract) setHeader(key, v string, n int) error {
	switch key {
	case "CONTRACT":
		if c.ContractHeader != "" {
			return perr(0, n, "CONTRACT: repeated")
		}
		c.ContractHeader = v
	case "REQUEST":
		if c.Request != "" {
			return perr(0, n, "REQUEST: repeated")
		}
		if !reSHA.MatchString(v) {
			return perr(0, n, "REQUEST: must be sha256:<64 hex>")
		}
		c.Request = v
	case "IN":
		c.In = append(c.In, SplitGlobs(v)...)
	case "OUT":
		c.Out = append(c.Out, SplitGlobs(v)...)
	case "BASE":
		if c.Base != "" {
			return perr(0, n, "BASE: repeated")
		}
		if strings.ContainsAny(v, " \t") {
			return perr(0, n, "BASE: must be one git rev")
		}
		c.Base = v
	case "RISK":
		if c.Risk != "" {
			return perr(19, n, "RISK: repeated")
		}
		if v != "impossible" {
			return perr(19, n, "RISK: %q is outside the closed set (impossible)", v)
		}
		c.Risk = v
	}
	return nil
}

func (g *Gate) setAttr(key, v string, idx, n int) error {
	if key != "FROM" && key != "WITNESS" && g.has[key] {
		return perr(7, n, "%s: repeated on gate %s", key, g.ID)
	}
	g.has[key] = true
	g.AttrLines = append(g.AttrLines, idx)
	switch key {
	case "CHECK":
		g.Check = v
	case "EXPECT":
		g.Expect = v
	case "CWD":
		g.CWD = v
	case "RED":
		switch v {
		case RedBaseline, RedMutation, RedControl, RedNone:
			g.Red = v
		default:
			return perr(0, n, "RED: %q is not baseline, mutation, control or none", v)
		}
	case "RED-CHECK":
		g.RedCheck = v
	case "RED-EXPECT":
		g.RedExpect = v
	case "WITNESS":
		g.Witness = append(g.Witness, SplitGlobs(v)...)
	case "EVIDENCE":
		if !reSHA.MatchString(v) {
			return perr(0, n, "EVIDENCE: must be sha256:<64 hex>")
		}
		g.Evidence = v
	case "FROM":
		fm := reFrom.FindStringSubmatch(v)
		if fm == nil {
			return perr(0, n, "FROM: needs `R<n> \"<verbatim span>\"`")
		}
		sn, _ := strconv.Atoi(fm[1])
		g.From = append(g.From, From{Sentence: sn, Quote: fm[2], Line: idx})
	}
	return nil
}

func (c *Contract) validate(ids map[string]int) error {
	if len(c.Gates) == 0 {
		return perr(1, 0, "zero gates")
	}
	if len(c.In) == 0 {
		return perr(0, 0, "IN: is required")
	}
	c.Slug = c.ContractHeader
	if c.Slug == "" {
		c.Slug = Slug(c.Title)
	}
	for _, list := range [][]string{c.In, c.Out} {
		for _, g := range list {
			if bad := badPath(g); bad != "" {
				return perr(9, 0, "%s: %s", g, bad)
			}
		}
	}
	for _, g := range c.Gates {
		n := g.Line + 1
		if g.Has("CHECK") != g.Has("EXPECT") {
			return perr(4, n, "gate %s has CHECK: without EXPECT: or vice versa", g.ID)
		}
		if g.Has("EXPECT") {
			if _, err := CompileExpect(g.Expect); err != nil {
				return perr(8, n, "gate %s EXPECT: %v", g.ID, err)
			}
		}
		if g.Has("RED-EXPECT") {
			if _, err := CompileExpect(g.RedExpect); err != nil {
				return perr(8, n, "gate %s RED-EXPECT: %v", g.ID, err)
			}
		}
		if g.Has("CWD") {
			if bad := badPath(g.CWD); bad != "" {
				return perr(9, n, "gate %s CWD: %s", g.ID, bad)
			}
		}
		for _, w := range g.Witness {
			if bad := badPath(w); bad != "" {
				return perr(9, n, "gate %s WITNESS: %s: %s", g.ID, w, bad)
			}
		}
		if c.Request != "" && len(g.From) == 0 {
			return perr(10, n, "REQUEST: present and gate %s has no FROM:", g.ID)
		}
		if c.Request == "" && len(g.From) > 0 {
			return perr(10, n, "gate %s has FROM: without REQUEST:", g.ID)
		}
		ctrl := g.Red == RedControl
		if ctrl && (!g.Has("RED-CHECK") || !g.Has("RED-EXPECT")) {
			return perr(15, n, "gate %s: RED: control needs both RED-CHECK: and RED-EXPECT:", g.ID)
		}
		if !ctrl && (g.Has("RED-CHECK") || g.Has("RED-EXPECT")) {
			return perr(15, n, "gate %s: RED-CHECK:/RED-EXPECT: without RED: control", g.ID)
		}
		if g.Has("RED-CHECK") != g.Has("RED-EXPECT") {
			return perr(15, n, "gate %s: RED-CHECK: and RED-EXPECT: go together", g.ID)
		}
	}
	for _, a := range c.Abandon {
		if _, ok := ids[a.ID]; !ok {
			return perr(13, a.Line+1, "ABANDON: names unknown gate %s", a.ID)
		}
	}
	seenReason := map[string]bool{}
	for _, w := range c.Waive {
		n := w.Line + 1
		if !reGuardID.MatchString(w.Guard) || !GuardIDs[w.Guard] {
			return perr(18, n, "WAIVE: unknown guard id %s", w.Guard)
		}
		if w.Guard == "G-LEDGER" {
			return perr(18, n, "WAIVE: G-LEDGER cannot be waived")
		}
		if !reHunk.MatchString(w.Hunk) {
			return perr(18, n, "WAIVE: hunk hash must be 12 hex digits")
		}
		if len(strings.Fields(w.Reason)) < 8 {
			return perr(18, n, "WAIVE: reason must be at least 8 tokens")
		}
		if seenReason[w.Reason] {
			return perr(18, n, "WAIVE: reason repeats another waiver byte for byte")
		}
		seenReason[w.Reason] = true
	}
	return nil
}

// badPath reports why a repo-relative path or glob escapes the repo.
func badPath(p string) string {
	if p == "" {
		return "empty path"
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || (len(p) > 1 && p[1] == ':') {
		return "absolute path"
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return "`..` segment"
		}
	}
	return ""
}

// Slug lowercases s, maps runs of non-alphanumerics to one hyphen, trims
// the edges and drops a leading "contract" label.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "contract:")
	s = strings.Trim(reSlug.ReplaceAllString(s, "-"), "-")
	if s == "" {
		return "contract"
	}
	return s
}

// Qualified returns the "<slug>:<id>" form of a gate id.
func (c *Contract) Qualified(id string) string { return c.Slug + ":" + id }

// Gate returns the gate with the given id, or nil.
func (c *Contract) Gate(id string) *Gate {
	for _, g := range c.Gates {
		if g.ID == id {
			return g
		}
	}
	return nil
}

// Abandoned returns the ABANDON: statement for a gate, or nil.
func (c *Contract) Abandoned(id string) *Abandon {
	for i := range c.Abandon {
		if c.Abandon[i].ID == id {
			return &c.Abandon[i]
		}
	}
	return nil
}

// Hash is the contract_hash of section 4.3: sha256 of the contract with
// every EVIDENCE: line removed and every [x] normalised to [ ].
func (c *Contract) Hash() string {
	drop := map[int]bool{}
	gateLine := map[int]bool{}
	for _, g := range c.Gates {
		gateLine[g.Line] = true
		for _, a := range g.AttrLines {
			if reAttr.FindStringSubmatch(strings.TrimLeft(c.Lines[a], " "))[1] == "EVIDENCE" {
				drop[a] = true
			}
		}
	}
	var b strings.Builder
	for i, l := range c.Lines {
		if drop[i] {
			continue
		}
		if gateLine[i] {
			l = "- [ ]" + l[5:]
		}
		b.WriteString(l)
		b.WriteString(c.EOL)
	}
	return canon.SHA256([]byte(b.String()))
}

// Raw returns the bytes the contract was parsed from.
func (c *Contract) Raw() []byte { return c.raw }

// EvidenceUpdate is one checker write into the ledger: mark the gate
// [x] or [ ] and set or clear its EVIDENCE: line.
type EvidenceUpdate struct {
	Checked bool
	// Evidence is the record hash to write, or "" to remove the line.
	Evidence string
}

// Rewrite returns the contract bytes with the updates applied, preserving
// every other byte, the line-ending style and the trailing newline.
func (c *Contract) Rewrite(updates map[string]EvidenceUpdate) []byte {
	byLine := map[int]*Gate{}
	evLine := map[int]*Gate{}
	for _, g := range c.Gates {
		byLine[g.Line] = g
		for _, a := range g.AttrLines {
			if reAttr.FindStringSubmatch(strings.TrimLeft(c.Lines[a], " "))[1] == "EVIDENCE" {
				evLine[a] = g
			}
		}
	}
	var b strings.Builder
	for i, l := range c.Lines {
		if g, ok := evLine[i]; ok {
			if _, upd := updates[g.ID]; upd {
				continue
			}
		}
		if g, ok := byLine[i]; ok {
			if u, upd := updates[g.ID]; upd {
				box := "- [ ]"
				if u.Checked {
					box = "- [x]"
				}
				l = box + l[5:]
			}
		}
		b.WriteString(l)
		b.WriteString(c.EOL)
		// After the last attribute line of a gate (or the gate line when it
		// has none), insert the new EVIDENCE: line.
		if g := c.lastLineOwner(i); g != nil {
			if u, upd := updates[g.ID]; upd && u.Evidence != "" {
				b.WriteString(g.Indent + "EVIDENCE: " + u.Evidence)
				b.WriteString(c.EOL)
			}
		}
	}
	out := b.String()
	if !c.TrailingNewline {
		out = strings.TrimSuffix(out, c.EOL)
	}
	return []byte(out)
}

// lastLineOwner returns the gate whose block ends at line i (counting the
// gate line and its non-EVIDENCE attribute lines), or nil.
func (c *Contract) lastLineOwner(i int) *Gate {
	for _, g := range c.Gates {
		last := g.Line
		for _, a := range g.AttrLines {
			if reAttr.FindStringSubmatch(strings.TrimLeft(c.Lines[a], " "))[1] != "EVIDENCE" && a > last {
				last = a
			}
		}
		if last == i {
			return g
		}
	}
	return nil
}
