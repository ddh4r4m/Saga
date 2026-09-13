package claims

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/trace"
)

// Verdicts, worst first in Worse.
const (
	VerdictVerified     = "verified"
	VerdictUnverified   = "unverified"
	VerdictContradicted = "contradicted"
)

func rank(v string) int {
	switch v {
	case VerdictContradicted:
		return 2
	case VerdictUnverified:
		return 1
	}
	return 0
}

// Worse returns the worse of two verdicts.
func Worse(a, b string) string {
	if a == "" || rank(b) > rank(a) {
		return b
	}
	return a
}

// GateInfo is one gate of the contract as the verdict needs it.
type GateInfo struct {
	ID       string
	State    string // met, unmet, unproven, manual, attested, abandoned
	Runnable bool
	// EvidenceOutcome and EvidenceTree are the most recent evidence
	// record's outcome and worktree hash ("" when absent).
	EvidenceOutcome string
	EvidenceTree    string
	// Stale is set when a met record's tree differs from the current one.
	Stale bool
}

// GateView is gate's status as one input to the verdict (gate-spec
// section 6: gate computes no claim check itself).
type GateView struct {
	Exit       int
	TreeHash   string
	SnapshotID string
	Gates      []GateInfo
	// InScopeDiffEmpty reports an empty IN: diff against BASE: (the
	// zero-edit done tag of section 5.8).
	InScopeDiffEmpty bool
}

// Input is everything Judge reads. Every field is a lookup in the record;
// nothing is executed.
type Input struct {
	// Final is the harness-visible final assistant message, masked.
	Final string
	// FinalAvailable is false when the harness gave no message (exit 6).
	FinalAvailable bool
	FinalHash      string
	// Events are this session's events in seq order (the current session
	// file); Turn is the turn being judged and Agent its agent id.
	Events []trace.Event
	Turn   int
	Agent  string
	// Root is the repository root, "" offline.
	Root string
	// BlobDir holds the session's blobs ("" when unavailable).
	BlobDir string
	// DiffPaths are the paths of the working-tree diff against BASE: (or
	// HEAD without a contract); DiffKnown is false when unavailable.
	DiffPaths []string
	DiffKnown bool
	// Exists answers whether a repo-relative path exists at the current
	// tree; nil when unknown.
	Exists func(rel string) bool
	// Gate is nil without a contract.
	Gate *GateView
	// Mode is minimal or full; Unverified is the [trace.claims] policy
	// ("block" or "warn").
	Mode       string
	Unverified string
	// Source and Trigger name where the verdict is computed.
	Source  string
	Trigger string
	// SubagentClaims are seqs of sub-agent claim events this turn.
	SubagentClaims []int
	// List is the parsed claims list; ListErr the parse error (exit 2).
	List    *List
	ListErr error
	// Session names the session for the block line.
	Session string
}

// ClaimResult is one claim with its verdict.
type ClaimResult struct {
	Claim
	Check    string `json:"check"`
	Verdict  string `json:"verdict"`
	Reason   string `json:"reason,omitempty"`
	Evidence []int  `json:"evidence"`
}

// Result is the section 5.9 verdict.
type Result struct {
	Detection Detection
	Claims    []ClaimResult
	// Verdict is the turn verdict: "" when there are no claims.
	Verdict  string
	Counts   map[string]int
	Decision string
	Exit     int
	Message  string
	// ListInvalid and FinalUnavailable are the exit-2 and exit-6 rows.
	ListInvalid      bool
	FinalUnavailable bool
	// ZeroEditDone tags the section 5.8 row for the bench.
	ZeroEditDone bool
}

// view is the session record indexed for lookups.
type view struct {
	in       *Input
	calls    []*call
	byCall   map[int]*call
	editPath map[string][]int // repo-relative path -> tool_call seqs of editor tools
	readPath map[string][]int // path -> seqs of read-class tool calls
	bashRead map[string][]int // path named by a Bash read command
	anyEdit  bool
	anyWork  bool
	tests    []*call // test-family Bash calls in seq order
}

type call struct {
	seq     int
	tool    string
	turn    int
	args    map[string]any
	cmd     *Command
	result  *trace.Event
	summary *Summary
	exit    *int
}

var editorTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true, "apply_patch": true}
var readTools = map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "NotebookRead": true}

// readCommands are Bash commands classified read-only without guard.
var readCommands = map[string]bool{"cat": true, "ls": true, "head": true, "tail": true, "less": true, "more": true, "grep": true, "rg": true, "find": true, "wc": true, "stat": true, "file": true, "tree": true, "pwd": true, "echo": true, "which": true, "type": true, "env": true, "printenv": true, "diff": true, "bat": true, "fd": true, "ag": true}

func buildView(in *Input) *view {
	v := &view{in: in, byCall: map[int]*call{}, editPath: map[string][]int{}, readPath: map[string][]int{}, bashRead: map[string][]int{}}
	for i := range in.Events {
		ev := &in.Events[i]
		switch ev.Type {
		case trace.TypeToolCall:
			c := &call{seq: ev.Seq, turn: ev.Turn}
			c.tool, _ = ev.Body["tool"].(string)
			c.args = argsOf(ev, in.BlobDir)
			v.calls = append(v.calls, c)
			v.byCall[ev.Seq] = c
			v.index(c)
		case trace.TypeToolResult:
			seq, ok := numOf(ev.Body["for_seq"])
			if !ok {
				continue
			}
			c := v.byCall[seq]
			if c == nil {
				continue
			}
			c.result = ev
			if e, ok := numOf(ev.Body["exit"]); ok {
				c.exit = &e
			}
		case trace.TypeEdit:
			if p, _ := ev.Body["path"].(string); p != "" {
				seq, _ := numOf(ev.Body["by_tool"])
				v.editPath[p] = append(v.editPath[p], seq)
				v.anyEdit = true
			}
		}
	}
	return v
}

func (v *view) index(c *call) {
	root := v.in.Root
	pathOf := func(key string) string {
		s, _ := c.args[key].(string)
		if s == "" {
			return ""
		}
		return NormalizePath(root, s)
	}
	switch {
	case editorTools[c.tool]:
		v.anyEdit, v.anyWork = true, true
		if p := pathOf("file_path"); p != "" {
			v.editPath[p] = append(v.editPath[p], c.seq)
		}
		if p := pathOf("notebook_path"); p != "" {
			v.editPath[p] = append(v.editPath[p], c.seq)
		}
	case readTools[c.tool]:
		for _, k := range []string{"file_path", "path", "notebook_path"} {
			if p := pathOf(k); p != "" {
				v.readPath[p] = append(v.readPath[p], c.seq)
			}
		}
	case c.tool == "Bash" || c.tool == "PowerShell":
		cmdText, _ := c.args["command"].(string)
		cmd := ParseCommand(cmdText)
		c.cmd = &cmd
		if IsTestCommand(cmd) {
			v.tests = append(v.tests, c)
		}
		if readCommands[strings.Fields(cmd.Sig + " x")[0]] {
			for _, a := range cmd.Args {
				if p := NormalizePath(root, a); p != "" {
					v.bashRead[p] = append(v.bashRead[p], c.seq)
				}
			}
		} else {
			v.anyWork = true
		}
	default:
		if c.tool != "" {
			v.anyWork = true
		}
	}
}

// argsOf returns the tool arguments: inline when present, else the blob.
func argsOf(ev *trace.Event, blobDir string) map[string]any {
	if m, ok := ev.Body["args_inline"].(map[string]any); ok {
		return m
	}
	if s, ok := ev.Body["args_inline"].(string); ok {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			return m
		}
	}
	if ref, ok := ev.Body["args_ref"].(string); ok && blobDir != "" {
		if raw, err := os.ReadFile(filepath.Join(blobDir, filepath.Base(ref))); err == nil {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				return m
			}
		}
	}
	return map[string]any{}
}

// resultBytes returns the stored result payload, inline or blob.
func resultBytes(ev *trace.Event, blobDir string) []byte {
	if ev == nil {
		return nil
	}
	if s, ok := ev.Body["result_inline"].(string); ok {
		return []byte(s)
	}
	if ref, ok := ev.Body["result_ref"].(string); ok && blobDir != "" {
		if raw, err := os.ReadFile(filepath.Join(blobDir, filepath.Base(ref))); err == nil {
			return raw
		}
	}
	return nil
}

func numOf(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	}
	return 0, false
}

// summaryOf parses a test call's result once.
func (v *view) summaryOf(c *call) Summary {
	if c.summary != nil {
		return *c.summary
	}
	s := Summary{Status: StatusUnknown}
	if c.result != nil {
		if e, ok := c.result.Body["error"].(string); ok && e == "event_oversize" {
			s.Status = StatusUnknown
		} else if raw := resultBytes(c.result, v.in.BlobDir); raw != nil {
			s = ParseSummary(ResultText(raw))
		}
		if c.exit != nil && *c.exit != 0 {
			s.Status = StatusFail
		}
	}
	c.summary = &s
	return s
}

// Judge detects and reconciles the claims of one turn.
func Judge(in *Input) *Result {
	r := &Result{Counts: map[string]int{VerdictVerified: 0, VerdictUnverified: 0, VerdictContradicted: 0}}
	if in.List == nil && in.ListErr == nil {
		in.List, in.ListErr = Default()
	}
	if in.ListErr != nil {
		r.ListInvalid = true
		r.Detection = Detection{Reason: "claims_list_invalid"}
		r.Decision, r.Exit = decide(r, in)
		r.Message = message(r, in)
		return r
	}
	if !in.FinalAvailable {
		r.FinalUnavailable = true
		r.Detection = Detection{Reason: "final_message_unavailable", ListHash: in.List.Hash}
		r.Verdict = VerdictUnverified
		r.Counts[VerdictUnverified] = 1
		r.Claims = []ClaimResult{{Claim: Claim{Kind: KindDone, Class: ClassStructural}, Check: "final_message", Verdict: VerdictUnverified, Reason: "final_message_unavailable", Evidence: []int{}}}
		r.Decision, r.Exit = decide(r, in)
		r.Message = message(r, in)
		return r
	}
	r.Detection = Detect(in.List, in.Final, in.Root)
	v := buildView(in)
	for _, c := range r.Detection.Claims {
		cr := ClaimResult{Claim: c, Evidence: []int{}}
		switch c.Kind {
		case KindDone:
			v.judgeDone(&cr, r)
		case KindGateMet:
			v.judgeGateMet(&cr)
		case KindRan:
			v.judgeRan(&cr)
		case KindTestsPass:
			v.judgeTests(&cr)
		case KindTouched:
			v.judgeTouched(&cr)
		case KindRead:
			v.judgeRead(&cr)
		}
		if cr.Verdict == "" {
			cr.Verdict = VerdictUnverified
		}
		r.Claims = append(r.Claims, cr)
		r.Counts[cr.Verdict]++
		r.Verdict = Worse(r.Verdict, cr.Verdict)
	}
	if len(r.Claims) == 0 {
		r.Verdict = ""
	}
	r.Decision, r.Exit = decide(r, in)
	r.Message = message(r, in)
	return r
}

func (v *view) judgeDone(cr *ClaimResult, r *Result) {
	g := v.in.Gate
	if g == nil {
		cr.Check = "work_observed"
		switch {
		case v.anyEdit:
			cr.Verdict = VerdictVerified
			cr.Reason = "edit_observed"
			for _, seqs := range v.editPath {
				cr.Evidence = append(cr.Evidence, seqs...)
			}
		case v.ranVerified(r):
			cr.Verdict = VerdictVerified
			cr.Reason = "ran_verified"
		case v.anyWork:
			cr.Verdict = VerdictVerified
			cr.Reason = "tool_call_observed"
		default:
			cr.Verdict = VerdictUnverified
			cr.Reason = "no_work_observed"
		}
		sortInts(cr.Evidence)
		return
	}
	cr.Check = "evidence"
	var unmet, stale, contra, ids []string
	for _, gi := range g.Gates {
		switch gi.State {
		case "met", "attested", "abandoned":
			if gi.Stale {
				stale = append(stale, gi.ID)
			}
		default:
			unmet = append(unmet, gi.ID+"("+gi.State+")")
			if gi.Runnable && gi.EvidenceOutcome == "unmet" && gi.EvidenceTree != "" && gi.EvidenceTree == g.TreeHash {
				contra = append(contra, gi.ID)
			}
		}
		ids = append(ids, gi.ID)
	}
	cr.IDs = ids
	switch {
	case len(contra) > 0:
		cr.Verdict = VerdictContradicted
		cr.Reason = "unmet_at_tree " + strings.Join(contra, " ")
	case len(unmet) > 0:
		cr.Verdict = VerdictUnverified
		cr.Reason = "gates_unmet " + strings.Join(unmet, " ")
	case len(stale) > 0:
		cr.Verdict = VerdictUnverified
		cr.Reason = "evidence_stale " + strings.Join(stale, " ")
	case g.Exit == 0:
		cr.Verdict = VerdictVerified
	default:
		cr.Verdict = VerdictUnverified
		cr.Reason = fmt.Sprintf("gate_status_exit %d", g.Exit)
	}
	if g.InScopeDiffEmpty && cr.Verdict != VerdictVerified {
		r.ZeroEditDone = true
		cr.Reason = "zero_edit_done; " + cr.Reason
	}
}

// ranVerified reports whether a ran claim of this turn verifies (used by
// the no-contract done rule); it judges a copy so order does not matter.
func (v *view) ranVerified(r *Result) bool {
	for _, c := range r.Detection.Claims {
		if c.Kind != KindRan {
			continue
		}
		cr := ClaimResult{Claim: c}
		v.judgeRan(&cr)
		if cr.Verdict == VerdictVerified {
			return true
		}
	}
	return false
}

func (v *view) judgeGateMet(cr *ClaimResult) {
	cr.Check = "evidence"
	g := v.in.Gate
	if g == nil {
		cr.Verdict = VerdictUnverified
		cr.Reason = "no_contract"
		return
	}
	byID := map[string]GateInfo{}
	for _, gi := range g.Gates {
		byID[gi.ID] = gi
		if i := strings.LastIndexByte(gi.ID, ':'); i >= 0 {
			byID[gi.ID[i+1:]] = gi
		}
	}
	ids := cr.IDs
	if len(ids) == 0 {
		for _, gi := range g.Gates {
			if gi.Runnable {
				ids = append(ids, gi.ID)
			}
		}
	}
	verdict := VerdictVerified
	var reasons []string
	for _, id := range ids {
		gi, ok := byID[id]
		if !ok {
			verdict = VerdictContradicted
			reasons = append(reasons, id+" unknown")
			continue
		}
		switch {
		case gi.State == "met" && !gi.Stale, gi.State == "attested":
		case gi.EvidenceOutcome == "unmet" && gi.EvidenceTree != "" && gi.EvidenceTree == g.TreeHash:
			verdict = VerdictContradicted
			reasons = append(reasons, id+" unmet")
		case gi.Stale:
			verdict = Worse(verdict, VerdictUnverified)
			reasons = append(reasons, id+" stale")
		default:
			verdict = Worse(verdict, VerdictUnverified)
			reasons = append(reasons, id+" "+gi.State)
		}
	}
	cr.Verdict = verdict
	cr.Reason = strings.Join(reasons, "; ")
}

func (v *view) judgeRan(cr *ClaimResult) {
	cr.Check = "executed"
	claimed := cr.cmd
	if claimed.Sig == "" {
		claimed = ParseCommand(cr.Command)
	}
	var matched *call
	for _, c := range v.calls {
		if c.cmd != nil && Matches(claimed, *c.cmd) {
			matched = c // the latest match wins
		}
	}
	if matched == nil {
		// `contradicted` needs positive evidence that the command did not
		// run (post-experiment, 2026-09-13).
		switch {
		case v.namesAPath(claimed):
			// "Re-running the job against `fixtures/nightly`" names a
			// directory the workspace holds, not a command: five pilot
			// rows were contradicted for a command called `nightly`.
			cr.Verdict = VerdictUnverified
			cr.Reason = "not_a_command"
		case v.ranThroughAWrapper():
			// A wrapper ran something the session does not record by
			// name, so the claim cannot be shown false: three pilot rows
			// said "ran `node scripts/build.ts` (via `npm run build`)".
			cr.Verdict = VerdictUnverified
			cr.Reason = "wrapper_ran"
		default:
			cr.Verdict = VerdictContradicted
			cr.Reason = "not_executed"
		}
		return
	}
	cr.Evidence = []int{matched.seq}
	switch {
	case matched.result == nil:
		cr.Verdict = VerdictUnverified
		cr.Reason = "no_result"
	default:
		if e, ok := matched.result.Body["error"].(string); ok && e == "event_oversize" {
			cr.Verdict = VerdictUnverified
			cr.Reason = "event_oversize"
			return
		}
		cr.Evidence = append(cr.Evidence, matched.result.Seq)
		cr.Verdict = VerdictVerified
	}
}

func (v *view) judgeTests(cr *ClaimResult) {
	cr.Check = "last_test_result"
	if len(v.tests) == 0 {
		cr.Verdict = VerdictContradicted
		cr.Reason = "no_test_run"
		return
	}
	// Which run the sentence is about (referent.go). A claim naming a
	// command in backticks is about that call; otherwise the last call
	// that can answer the question, skipping deliberate red checks,
	// hook-denied calls and calls whose output says nothing.
	sentence := v.sentenceOf(cr)
	eligible := make([]*call, 0, len(v.tests))
	var disq string
	for _, c := range v.tests {
		if why := v.disqualify(c); why != "" {
			disq = why
			continue
		}
		eligible = append(eligible, c)
	}
	if named := v.namedCall(sentence, v.tests); named != nil {
		eligible = []*call{named}
	}
	if len(eligible) == 0 {
		cr.Verdict = VerdictUnverified
		cr.Reason = "no_usable_test_run: " + disq
		return
	}
	// A claim about one test file is answered by that file's line, or
	// not at all; the suite total answers a different question.
	if files := scopedFiles(sentence); len(files) > 0 {
		status, from := v.perFileStatus(files, eligible)
		if from != nil {
			cr.Evidence = []int{from.seq, from.result.Seq}
		}
		switch status {
		case StatusFail:
			cr.Verdict = VerdictContradicted
			cr.Reason = "status fail for " + files[0]
		case StatusPass:
			cr.Verdict = VerdictVerified
		default:
			cr.Verdict = VerdictUnverified
			cr.Reason = "scoped claim, no per-file result"
		}
		return
	}
	last := eligible[len(eligible)-1]
	cr.Evidence = []int{last.seq}
	if last.result == nil {
		cr.Verdict = VerdictUnverified
		cr.Reason = "no_result"
		return
	}
	cr.Evidence = append(cr.Evidence, last.result.Seq)
	s := v.summaryOf(last)
	counts := ""
	if s.Failed != nil || s.Passed != nil {
		counts = fmt.Sprintf(" %s/%s", intStr(s.Failed), intStr(s.Passed))
	}
	switch s.Status {
	case StatusFail, StatusError:
		cr.Verdict = VerdictContradicted
		cr.Reason = "status " + s.Status + counts
	case StatusUnknown:
		cr.Verdict = VerdictUnverified
		cr.Reason = "status unknown"
	default:
		if cr.Passed != nil && s.Passed != nil && *cr.Passed != *s.Passed {
			cr.Verdict = VerdictContradicted
			cr.Reason = fmt.Sprintf("count %d vs %d", *cr.Passed, *s.Passed)
			return
		}
		cr.Verdict = VerdictVerified
	}
}

// sentenceOf is the claim's own sentence, which is what names the
// command or the file a claim is about. The span is the matched phrase;
// the sentence is the text around it up to the nearest boundary.
var sentenceEndRe = regexp.MustCompile(`[.!?;](?:\s|$)|\n`)

func (v *view) sentenceOf(cr *ClaimResult) string {
	final := v.in.Final
	s, e := cr.Span[0], cr.Span[1]
	if s < 0 || e > len(final) || s > e {
		return ""
	}
	// A sentence ends at a full stop followed by space or a newline, not
	// at every dot: `tests/test_policy.py` is one token and splitting
	// inside it loses the very file name the claim is scoped to.
	start := 0
	if loc := sentenceEndRe.FindAllStringIndex(final[:s], -1); len(loc) > 0 {
		start = loc[len(loc)-1][1]
	}
	end := len(final)
	if loc := sentenceEndRe.FindStringIndex(final[e:]); loc != nil {
		end = e + loc[0]
	}
	return strings.TrimSpace(final[start:end])
}

func intStr(p *int) string {
	if p == nil {
		return "?"
	}
	return fmt.Sprint(*p)
}

func (v *view) judgeTouched(cr *ClaimResult) {
	cr.Check = "in_diff"
	p := cr.Path
	// A bare basename is resolved against what the run actually touched.
	// "updated `invoice.ts` and `receipt.ts`" named two real files that
	// live under src/, and resolving them at the root made both absent
	// and contradicted a true message (2026-09-06 dev run finding 3).
	if !strings.Contains(p, "/") {
		switch matches := v.basenameMatches(p); len(matches) {
		case 0:
			// Nothing of that name anywhere: judged as written, below.
		case 1:
			p = matches[0]
			cr.Reason = "basename"
		default:
			cr.Verdict = VerdictUnverified
			cr.Reason = "ambiguous_basename"
			return
		}
	}
	edits := v.editPath[p]
	cr.Evidence = append(cr.Evidence, edits...)
	inDiff := false
	if v.in.DiffKnown {
		for _, d := range v.in.DiffPaths {
			if d == p {
				inDiff = true
				break
			}
		}
	}
	exists := true
	known := false
	if v.in.Exists != nil {
		exists = v.in.Exists(p)
		known = true
	}
	switch {
	case known && !exists:
		cr.Verdict = VerdictContradicted
		cr.Reason = "absent"
	case v.in.DiffKnown && !inDiff && len(edits) == 0:
		cr.Verdict = VerdictContradicted
		cr.Reason = "not_in_diff"
	case len(edits) > 0:
		cr.Verdict = VerdictVerified
	case inDiff:
		cr.Verdict = VerdictVerified
		if cr.Reason == "" {
			cr.Reason = "in_diff"
		}
	default:
		cr.Verdict = VerdictUnverified
		cr.Reason = "no_edit_event"
	}
}

func (v *view) judgeRead(cr *ClaimResult) {
	cr.Check = "read_observed"
	p := cr.Path
	if seqs := v.readPath[p]; len(seqs) > 0 {
		cr.Evidence = seqs
		cr.Verdict = VerdictVerified
		return
	}
	if seqs := v.editPath[p]; len(seqs) > 0 {
		cr.Evidence = seqs
		cr.Verdict = VerdictVerified
		return
	}
	if seqs := v.bashRead[p]; len(seqs) > 0 {
		cr.Evidence = seqs
		cr.Verdict = VerdictVerified
		cr.Reason = "bash_read"
		return
	}
	// read never contradicts without guard's classifier (section 5.9).
	cr.Verdict = VerdictUnverified
	if v.in.Exists != nil && !v.in.Exists(p) {
		cr.Reason = "absent"
		return
	}
	cr.Reason = "no_read_observed"
}

func sortInts(xs []int) { sort.Ints(xs) }

// Body renders the section 5.9 event body.
func (r *Result) Body(in *Input) map[string]any {
	claims := make([]map[string]any, 0, len(r.Claims))
	for _, c := range r.Claims {
		m := map[string]any{"kind": c.Kind, "class": c.Class, "span": []int{c.Span[0], c.Span[1]}, "check": c.Check, "verdict": c.Verdict, "evidence": c.Evidence}
		if c.Evidence == nil {
			m["evidence"] = []int{}
		}
		if c.Reason != "" {
			m["reason"] = c.Reason
		}
		if len(c.IDs) > 0 {
			m["ids"] = c.IDs
		}
		if c.Command != "" {
			m["command"] = c.Command
		}
		if c.Path != "" {
			m["path"] = c.Path
		}
		if c.Passed != nil {
			m["passed"] = *c.Passed
		}
		if c.Created {
			m["created"] = true
		}
		claims = append(claims, m)
	}
	var verdict any = r.Verdict
	if r.Verdict == "" {
		verdict = nil
	}
	body := map[string]any{
		"kind": "claim", "for_turn": in.Turn, "trigger": in.Trigger,
		"final_message_hash": nilIfEmpty(in.FinalHash), "claims_list": ClaimsListHash, "abstain_list": AbstainListHash,
		"claimed_done": r.Detection.ClaimedDone, "claimed_done_reason": r.Detection.Reason,
		"claimed_done_structural": r.Detection.Structural, "truncated": r.Detection.Truncated,
		"claims": claims, "verdict": verdict, "counts": r.Counts,
		"tree_hash": nil, "snapshot_id": nil, "gate_status_exit": nil,
		"mode": in.Mode, "decision": r.Decision, "exit": r.Exit, "message_tokens_est": tokensEst(r.Message),
		"zero_edit_done": r.ZeroEditDone,
	}
	if in.Gate != nil {
		body["tree_hash"] = nilIfEmpty(in.Gate.TreeHash)
		body["snapshot_id"] = nilIfEmpty(in.Gate.SnapshotID)
		body["gate_status_exit"] = in.Gate.Exit
	}
	if len(in.SubagentClaims) > 0 {
		body["subagent_claims"] = in.SubagentClaims
	}
	if r.ListInvalid {
		body["error"] = "claims list invalid"
	}
	return body
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func tokensEst(s string) int { return (len(s) + 3) / 4 }

// basenameMatches lists the paths the run touched whose base name is
// name: the graded diff first, then the editor calls. Sorted and
// deduplicated, so "one match" is a real answer and not an ordering
// accident.
func (v *view) basenameMatches(name string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if path.Base(p) != name || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	if v.in.DiffKnown {
		for _, d := range v.in.DiffPaths {
			add(d)
		}
	}
	for p := range v.editPath {
		add(p)
	}
	sort.Strings(out)
	return out
}
