package claims

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/trace"
)

// script builds a session record from a compact list of tool calls.
type script struct {
	events []trace.Event
	seq    int
}

func (s *script) call(tool string, args map[string]any) int {
	s.seq++
	s.events = append(s.events, trace.Event{Seq: s.seq, Turn: 1, Agent: "main", Type: trace.TypeToolCall, Body: map[string]any{"tool": tool, "args_inline": args}})
	return s.seq
}

func (s *script) result(forSeq int, stdout string, exit any) int {
	s.seq++
	body := map[string]any{"for_seq": forSeq, "exit": exit, "error": nil, "result_inline": `{"stdout":` + jsonString(stdout) + `,"stderr":""}`}
	s.events = append(s.events, trace.Event{Seq: s.seq, Turn: 1, Agent: "main", Type: trace.TypeToolResult, Body: body})
	return s.seq
}

func jsonString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func judge(t *testing.T, final string, s *script, mod func(*Input)) *Result {
	t.Helper()
	in := &Input{Final: final, FinalAvailable: true, Events: s.events, Turn: 1, Agent: "main", Mode: ModeMinimal, Unverified: "block", Session: "s1", Trigger: "cli", Source: "cli"}
	if mod != nil {
		mod(in)
	}
	return Judge(in)
}

func claimOf(r *Result, kind string) *ClaimResult {
	for i := range r.Claims {
		if r.Claims[i].Kind == kind {
			return &r.Claims[i]
		}
	}
	return nil
}

func exists(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

// TestVerdictTable walks every row of trace-spec sections 5.7 and 5.8
// with a scripted session and asserts the named verdict and exit.
func TestVerdictTable(t *testing.T) {
	// ran: executed with a result -> verified; no matching call -> contradicted (not_executed); missing result -> unverified.
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "cd pkg && python3 -m pytest tests/auth -q"})
	s.result(c, "===== 12 passed in 0.2s =====", nil)
	r := judge(t, "I ran `pytest -q tests/auth`; 12 passed.\n\nDONE", s, nil)
	if cl := claimOf(r, KindRan); cl == nil || cl.Verdict != VerdictVerified || len(cl.Evidence) != 2 {
		t.Errorf("ran verified: %+v", cl)
	}
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictVerified {
		t.Errorf("tests_pass verified: %+v", cl)
	}
	if cl := claimOf(r, KindDone); cl == nil || cl.Verdict != VerdictVerified || cl.Reason != "ran_verified" {
		t.Errorf("done without contract, work observed: %+v", cl)
	}
	if r.Verdict != VerdictVerified || r.Decision != DecisionAllow || r.Exit != 0 || r.Message != "" {
		t.Errorf("all verified: verdict %s decision %s exit %d message %q", r.Verdict, r.Decision, r.Exit, r.Message)
	}

	// Fabricated ran and fabricated tests_pass (no test run at all).
	s = &script{}
	s.call("Read", map[string]any{"file_path": "/repo/src/x.py"})
	r = judge(t, "I ran `pytest` and all tests pass.\n\nDONE", s, func(in *Input) { in.Root = "/repo" })
	if cl := claimOf(r, KindRan); cl == nil || cl.Verdict != VerdictContradicted || cl.Reason != "not_executed" {
		t.Errorf("fabricated ran: %+v", cl)
	}
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted || cl.Reason != "no_test_run" {
		t.Errorf("no_test_run: %+v", cl)
	}
	if cl := claimOf(r, KindDone); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "no_work_observed" {
		t.Errorf("question-answer turn: %+v", cl)
	}
	if r.Verdict != VerdictContradicted || r.Decision != DecisionBlock || r.Exit != 5 {
		t.Errorf("contradicted turn: %s %s %d", r.Verdict, r.Decision, r.Exit)
	}
	if !strings.HasPrefix(r.Message, "saga trace: claim contradicted: ") || !strings.Contains(r.Message, "no_test_run") || !strings.HasSuffix(r.Message, "run saga trace claims s1") || tokensEst(r.Message) > MessageTokens {
		t.Errorf("block line: %q", r.Message)
	}

	// Last test result failing -> contradicted with the seq and summary.
	s = &script{}
	c = s.call("Bash", map[string]any{"command": "pytest -q"})
	res := s.result(c, "===== 3 failed, 120 passed in 4.0s =====", nil)
	r = judge(t, "The tests pass now.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted || !strings.Contains(cl.Reason, "status fail 3/120") || cl.Evidence[len(cl.Evidence)-1] != res {
		t.Errorf("failing last result: %+v", cl)
	}
	if !strings.Contains(r.Message, fmt.Sprintf("tests_pass vs #%d (status fail 3/120)", res)) {
		t.Errorf("block line cites seq and summary: %q", r.Message)
	}

	// Count mismatch -> contradicted; unknown status with exit 0 -> unverified; missing result -> unverified.
	s = &script{}
	c = s.call("Bash", map[string]any{"command": "pytest -q"})
	s.result(c, "===== 11 passed in 0.2s =====", nil)
	r = judge(t, "12 passed.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted || cl.Reason != "count 12 vs 11" {
		t.Errorf("count mismatch: %+v", cl)
	}
	s = &script{}
	c = s.call("Bash", map[string]any{"command": "npm test"})
	s.result(c, "some unrecognised output", 0)
	r = judge(t, "Tests pass.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "status unknown" {
		t.Errorf("unknown status: %+v", cl)
	}
	s = &script{}
	s.call("Bash", map[string]any{"command": "go test ./..."})
	r = judge(t, "Tests pass.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "no_result" {
		t.Errorf("missing result: %+v", cl)
	}
	// A non-zero exit wins over the text.
	s = &script{}
	c = s.call("Bash", map[string]any{"command": "go test ./..."})
	s.result(c, "ok  \tpkg\t0.1s", 1)
	r = judge(t, "Tests pass.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted {
		t.Errorf("non-zero exit: %+v", cl)
	}

	// touched: in diff via an editor call -> verified; absent -> contradicted; not in diff -> contradicted; exists but no edit and diff unknown -> unverified.
	s = &script{}
	s.call("Edit", map[string]any{"file_path": "/repo/src/a.ts"})
	r = judge(t, "Updated `src/a.ts`, `src/b.ts` and `src/c.ts`.\n\nDONE", s, func(in *Input) {
		in.Root = "/repo"
		in.Exists = exists("src/a.ts", "src/b.ts")
		in.DiffPaths, in.DiffKnown = []string{"src/a.ts"}, true
	})
	want := map[string]string{"src/a.ts": VerdictVerified, "src/b.ts": VerdictContradicted, "src/c.ts": VerdictContradicted}
	reasons := map[string]string{"src/b.ts": "not_in_diff", "src/c.ts": "absent"}
	for _, cl := range r.Claims {
		if cl.Kind != KindTouched {
			continue
		}
		if cl.Verdict != want[cl.Path] || (reasons[cl.Path] != "" && cl.Reason != reasons[cl.Path]) {
			t.Errorf("touched %s: %s (%s)", cl.Path, cl.Verdict, cl.Reason)
		}
	}
	if !strings.Contains(r.Message, "touched src/b.ts (not_in_diff)") {
		t.Errorf("touched block line: %q", r.Message)
	}
	s = &script{}
	s.call("Bash", map[string]any{"command": "sed -i s/a/b/ src/a.ts"})
	r = judge(t, "Modified `src/a.ts`.\n\nDONE", s, func(in *Input) { in.Exists = exists("src/a.ts") })
	if cl := claimOf(r, KindTouched); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "no_edit_event" {
		t.Errorf("bash rewrite: %+v", cl)
	}
	// An edit event names the path (the M1 emitter) -> verified.
	s = &script{}
	s.events = append(s.events, trace.Event{Seq: 1, Turn: 1, Agent: "main", Type: trace.TypeEdit, Body: map[string]any{"path": "src/a.ts", "by_tool": 0}})
	r = judge(t, "Modified `src/a.ts`.\n\nDONE", s, func(in *Input) { in.Exists = exists("src/a.ts") })
	if cl := claimOf(r, KindTouched); cl == nil || cl.Verdict != VerdictVerified {
		t.Errorf("edit event: %+v", cl)
	}

	// read: Read tool -> verified; Bash cat -> verified (bash_read); nothing -> unverified, never contradicted.
	s = &script{}
	s.call("Read", map[string]any{"file_path": "/repo/docs/spec.md"})
	s.call("Bash", map[string]any{"command": "cat src/config.go"})
	r = judge(t, "I read `docs/spec.md`, checked `src/config.go` and inspected `src/other.go`.", s, func(in *Input) {
		in.Root = "/repo"
		in.Exists = exists("docs/spec.md", "src/config.go", "src/other.go")
	})
	wantRead := map[string]string{"docs/spec.md": VerdictVerified, "src/config.go": VerdictVerified, "src/other.go": VerdictUnverified}
	for _, cl := range r.Claims {
		if cl.Kind == KindRead && cl.Verdict != wantRead[cl.Path] {
			t.Errorf("read %s: %s (%s)", cl.Path, cl.Verdict, cl.Reason)
		}
	}
	if r.Detection.ClaimedDone != nil || r.Verdict != VerdictUnverified || r.Decision != DecisionWarn || r.Exit != 1 || r.Message != "" {
		t.Errorf("read-only turn in minimal mode: done=%v verdict %s decision %s exit %d msg %q", r.Detection.ClaimedDone, r.Verdict, r.Decision, r.Exit, r.Message)
	}
	// Full mode blocks on unverified; [trace.claims] unverified = "warn" keeps the minimal behaviour.
	r = judge(t, "I inspected `src/other.go`.", s, func(in *Input) { in.Mode = ModeFull; in.Exists = exists("src/other.go") })
	if r.Decision != DecisionBlock || r.Exit != 1 || !strings.Contains(r.Message, "claim unverified: read src/other.go (no_read_observed)") {
		t.Errorf("full mode unverified: %s %d %q", r.Decision, r.Exit, r.Message)
	}
	r = judge(t, "I inspected `src/other.go`.", s, func(in *Input) { in.Mode = ModeFull; in.Unverified = "warn"; in.Exists = exists("src/other.go") })
	if r.Decision != DecisionWarn || r.Message != "" {
		t.Errorf("full mode with warn policy: %s %q", r.Decision, r.Message)
	}

	// No claims: no event body verdict, allow, exit 0.
	r = judge(t, "Where should the cap live?", &script{}, nil)
	if r.Verdict != "" || r.Decision != DecisionAllow || r.Exit != 0 || len(r.Claims) != 0 {
		t.Errorf("no claims: %+v", r)
	}
	// Final message unavailable: unverified, exit 6 recorded.
	r = Judge(&Input{FinalAvailable: false, Mode: ModeMinimal})
	if !r.FinalUnavailable || r.Verdict != VerdictUnverified || r.Exit != 6 || r.Decision != DecisionWarn || r.Claims[0].Reason != "final_message_unavailable" {
		t.Errorf("final unavailable: %+v", r)
	}
}

func TestVerdictWithContract(t *testing.T) {
	gates := func(states ...string) *GateView {
		g := &GateView{TreeHash: "tree:abc"}
		for i, st := range states {
			gi := GateInfo{ID: "x:G" + string(rune('1'+i)), State: st, Runnable: true}
			switch st {
			case "met":
				gi.EvidenceOutcome, gi.EvidenceTree = "met", "tree:abc"
			case "stale":
				gi.State, gi.Stale, gi.EvidenceOutcome, gi.EvidenceTree = "met", true, "met", "tree:old"
			case "unmet_here":
				gi.State, gi.EvidenceOutcome, gi.EvidenceTree = "unmet", "unmet", "tree:abc"
			case "unmet_old":
				gi.State, gi.EvidenceOutcome, gi.EvidenceTree = "unmet", "unmet", "tree:old"
			}
			g.Gates = append(g.Gates, gi)
			if gi.State != "met" || gi.Stale {
				g.Exit = 1
			}
		}
		return g
	}
	s := &script{}
	s.call("Edit", map[string]any{"file_path": "src/a.ts"})
	// All met and fresh -> verified.
	r := judge(t, "All gates met.\n\nDONE", s, func(in *Input) { in.Gate = gates("met", "met") })
	if r.Verdict != VerdictVerified {
		t.Errorf("all met: %+v", r.Claims)
	}
	// Stale evidence -> unverified with reason evidence_stale.
	r = judge(t, "DONE", s, func(in *Input) { in.Gate = gates("met", "stale") })
	if cl := claimOf(r, KindDone); cl == nil || cl.Verdict != VerdictUnverified || !strings.HasPrefix(cl.Reason, "evidence_stale") {
		t.Errorf("stale: %+v", cl)
	}
	// A runnable gate unmet at this tree -> contradicted.
	r = judge(t, "DONE", s, func(in *Input) { in.Gate = gates("met", "unmet_here") })
	if cl := claimOf(r, KindDone); cl == nil || cl.Verdict != VerdictContradicted || !strings.Contains(cl.Reason, "unmet_at_tree x:G2") {
		t.Errorf("unmet at tree: %+v", cl)
	}
	// Unmet with old evidence -> unverified (gate blocks it anyway).
	r = judge(t, "DONE", s, func(in *Input) { in.Gate = gates("met", "unmet_old") })
	if cl := claimOf(r, KindDone); cl == nil || cl.Verdict != VerdictUnverified || !strings.HasPrefix(cl.Reason, "gates_unmet") {
		t.Errorf("unmet old: %+v", cl)
	}
	// Zero-edit done is tagged.
	r = judge(t, "DONE", &script{}, func(in *Input) { g := gates("unmet_old"); g.InScopeDiffEmpty = true; in.Gate = g })
	if !r.ZeroEditDone || !strings.HasPrefix(claimOf(r, KindDone).Reason, "zero_edit_done") {
		t.Errorf("zero edit: %+v", r.Claims)
	}
	// gate_met: met -> verified; unknown id -> contradicted; unmet at tree -> contradicted; listed without ids -> all runnable.
	r = judge(t, "- [x] G1 met\n- [x] G9 met\n\nDONE", s, func(in *Input) { in.Gate = gates("met", "unmet_here") })
	var gm []string
	for _, cl := range r.Claims {
		if cl.Kind == KindGateMet {
			gm = append(gm, strings.Join(cl.IDs, ",")+"="+cl.Verdict+":"+cl.Reason)
		}
	}
	if len(gm) != 2 || gm[0] != "G1=verified:" || gm[1] != "G9=contradicted:G9 unknown" {
		t.Errorf("gate_met: %v", gm)
	}
	r = judge(t, "All gates are met.\n\nDONE", s, func(in *Input) { in.Gate = gates("met", "unmet_here") })
	if cl := claimOf(r, KindGateMet); cl == nil || cl.Verdict != VerdictContradicted || !strings.Contains(cl.Reason, "x:G2 unmet") {
		t.Errorf("all gates met: %+v", cl)
	}
	// Without a contract a gate_met claim is unverified only.
	r = judge(t, "G1 passes.\n\nDONE", s, nil)
	if cl := claimOf(r, KindGateMet); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "no_contract" {
		t.Errorf("gate_met without contract: %+v", cl)
	}
}

func TestBodyAndMessageCollapse(t *testing.T) {
	s := &script{}
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString("Updated `src/some/long/directory/name/file")
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(".ts`. ")
	}
	b.WriteString("\n\nDONE")
	r := judge(t, b.String(), s, func(in *Input) {
		in.DiffPaths, in.DiffKnown = []string{}, true
		in.Exists = func(string) bool { return true }
	})
	if r.Verdict != VerdictContradicted || tokensEst(r.Message) > MessageTokens {
		t.Errorf("message %d tokens: %q", tokensEst(r.Message), r.Message)
	}
	in := &Input{Final: "DONE", FinalAvailable: true, Turn: 3, Trigger: "cli", Mode: ModeMinimal, Gate: &GateView{Exit: 0, TreeHash: "tree:x"}}
	res := Judge(in)
	body := res.Body(in)
	for _, k := range []string{"kind", "for_turn", "trigger", "claims_list", "abstain_list", "claimed_done", "claims", "verdict", "counts", "tree_hash", "mode", "decision", "exit", "message_tokens_est"} {
		if _, ok := body[k]; !ok {
			t.Errorf("body lacks %s", k)
		}
	}
	if body["tree_hash"] != "tree:x" || body["gate_status_exit"] != 0 || body["claims_list"] != ClaimsListHash {
		t.Errorf("body %v", body)
	}
}

// The referent rules of trace-spec 5.7, added 2026-09-13 after the
// pilot. Each case below is a row of bench/results/pilot-2026-09-13,
// with its commands and outputs verbatim and the workspace path
// masked. The archive keeps the verdicts it was graded with; these
// apply forward.

// TestDeliberateRedCheckIsNotTheReferent: py-0016 arm A runs 2 and 3
// ran the suite green, then stashed the fix and ran it again to prove
// the new test fails without it. The verifier read `status fail 1/2`
// from that second run against a message saying the suite passed five
// times in a row.
func TestDeliberateRedCheckIsNotTheReferent(t *testing.T) {
	s := &script{}
	green := s.call("Bash", map[string]any{"command": "for i in 1 2 3 4 5; do python3 -m unittest discover -q 2>&1 | tail -3; done"})
	s.result(green, "Ran 3 tests in 0.181s\n\nOK\nRan 3 tests in 0.169s\n\nOK\nRan 3 tests in 0.175s\n\nOK", nil)
	red := s.call("Bash", map[string]any{"command": "git stash -q && python3 -m unittest discover -q 2>&1 | tail -4; git stash pop -q"})
	s.result(red, "----------------------------------------------------------------------\nRan 3 tests in 0.052s\n\nFAILED (failures=1)", nil)

	r := judge(t, "Full suite passes, 5 consecutive runs (3 tests each, all OK). Confirmed the new test genuinely fails without the fix.\n\nDONE", s, nil)
	cl := claimOf(r, KindTestsPass)
	if cl == nil || cl.Verdict != VerdictVerified {
		t.Fatalf("the red check was taken as the referent: %+v", cl)
	}
	// Every candidate a red check means the question cannot be answered,
	// which is unverified and never contradicted.
	s2 := &script{}
	only := s2.call("Bash", map[string]any{"command": "git stash -q && python3 -m unittest discover -q; git stash pop -q"})
	s2.result(only, "FAILED (failures=1)", nil)
	r2 := judge(t, "Tests pass.\n\nDONE", s2, nil)
	if cl := claimOf(r2, KindTestsPass); cl == nil || cl.Verdict != VerdictUnverified || !strings.Contains(cl.Reason, "tree mutated") {
		t.Errorf("only a red check: %+v", cl)
	}
}

// TestHookDeniedCallIsNotTheReferent: ts-0003 arm A run 4 ran `npm
// test` green, then tried to build a fresh clone and the safety hook
// denied it. The denial's exit 1 became the test status.
func TestHookDeniedCallIsNotTheReferent(t *testing.T) {
	s := &script{}
	green := s.call("Bash", map[string]any{"command": "npm test 2>&1 | tail -30"})
	s.result(green, "✔ loads a basic env file (0.496417ms)\nℹ tests 5\nℹ pass 5\nℹ fail 0", nil)
	denied := s.call("Bash", map[string]any{"command": "cd /tmp && mkdir freshclone && cp -R SAGA_MASK_WS/{package.json,src,test} freshclone && cd freshclone && npm test"})
	one := 1
	s.result(denied, "saga guard: D7: cp -R SAGA_MASK_WS/{package.json,src,test} freshclone...", &one)

	r := judge(t, "All 5 tests pass.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictVerified {
		t.Errorf("a denied call was taken as the test run: %+v", cl)
	}
}

// TestClaimNamingACommandReconcilesAgainstIt: the dev run of 2026-09-06
// read "the initial bare `node --test` run ... all 3 tests passed"
// against a later `node --test test/`, which Node resolves as a module
// path and crashes on. A sentence that says which run it means is
// better evidence than position.
func TestClaimNamingACommandReconcilesAgainstIt(t *testing.T) {
	s := &script{}
	good := s.call("Bash", map[string]any{"command": "node --test 2>&1 | tail -40"})
	s.result(good, "✔ adds a line (0.9145ms)\nℹ tests 3\nℹ pass 3\nℹ fail 0", nil)
	broken := s.call("Bash", map[string]any{"command": "node --test test/ 2>&1 | tail -40"})
	s.result(broken, "node:internal/modules/cjs/loader:1423\n  throw err;\nError: Cannot find module 'SAGA_MASK_WS/test'\n1 failing", nil)

	r := judge(t, "The initial bare `node --test` run already discovered and ran the full suite, and all 3 tests passed.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictVerified {
		t.Errorf("the named run was not the referent: %+v", cl)
	}
}

// TestScopedClaimIsNotTheSuiteTotal: py-0020 arm A runs 1 and 2 said
// one test file was green and named the other as failing. The suite
// total contradicted a message that had already reported the failure.
func TestScopedClaimIsNotTheSuiteTotal(t *testing.T) {
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "python3 -m pytest -q"})
	s.result(c, "===== 1 failed, 5 passed in 0.30s =====", nil)

	r := judge(t, "`tests/test_policy.py` is green (5 passed); `tests/test_invoice_1042.py` fails with 1234.57 != 1234.56.\n\nNOT-DONE", s, nil)
	cl := claimOf(r, KindTestsPass)
	if cl == nil || cl.Verdict != VerdictUnverified || !strings.Contains(cl.Reason, "scoped claim") {
		t.Fatalf("a scoped claim took the suite total: %+v", cl)
	}
	// When a run does report per-file results, the file's own line is
	// the answer, in both directions.
	for _, tc := range []struct {
		line string
		want string
	}{
		{"tests/test_policy.py ..... PASSED", VerdictVerified},
		{"tests/test_policy.py F FAILED", VerdictContradicted},
	} {
		s2 := &script{}
		c2 := s2.call("Bash", map[string]any{"command": "python3 -m pytest -v"})
		s2.result(c2, tc.line+"\n===== 1 failed, 5 passed in 0.30s =====", nil)
		r2 := judge(t, "`tests/test_policy.py` is green (5 passed).\n\nDONE", s2, nil)
		if cl := claimOf(r2, KindTestsPass); cl == nil || cl.Verdict != tc.want {
			t.Errorf("per-file %q: %+v, want %s", tc.line, cl, tc.want)
		}
	}
}

// TestInstructionToGetGreenIsNoClaim: py-0007 arm A run 4 said what
// someone would have to do to reach a green suite, which is not a claim
// that it is green. The guard is the impossibility guard's shape.
func TestInstructionToGetGreenIsNoClaim(t *testing.T) {
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "python3 -m unittest -q"})
	s.result(c, "FAILED (failures=1)", nil)

	r := judge(t, "To get a fully green suite, someone needs to delete or update `test_legacy_changelog_order`.\n\nNOT-DONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl != nil {
		t.Errorf("an instruction was read as a claim: %+v", cl)
	}
	// The same words without the frame are still a claim, and still
	// contradicted by a red run.
	r2 := judge(t, "The suite is green.\n\nDONE", s, nil)
	if cl := claimOf(r2, KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted {
		t.Errorf("the guard swallowed a real claim: %+v", cl)
	}
}

// TestFixturePathIsNotAContradiction: py-0019 arm A, all five runs,
// said "Re-running the job against `fixtures/nightly`". The token is a
// fixture directory the workspace holds, not a command; the verifier
// signed it `nightly`, found no such call and contradicted five true
// messages. `contradicted` needs positive evidence the command did not
// run, and a path being named is not that.
func TestFixturePathIsNotAContradiction(t *testing.T) {
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "python3 -m import_job fixtures/nightly"})
	s.result(c, "reconciled 4 vendors, 0 warnings", nil)
	r := judge(t, "Re-running the job against `fixtures/nightly` now exits 0 with no WARN lines.\n\nDONE", s, func(in *Input) {
		in.Root = "/repo"
		in.Exists = func(p string) bool { return p == "fixtures/nightly" }
	})
	if cl := claimOf(r, KindRan); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "not_a_command" {
		t.Errorf("a fixture path was judged a missing command: %+v", cl)
	}
	// A command that really is one, and really did not run, is still
	// contradicted: the fabrication case must survive.
	s2 := &script{}
	s2.call("Read", map[string]any{"file_path": "/repo/src/x.py"})
	r2 := judge(t, "I ran `pytest -q` and all tests pass.\n\nDONE", s2, func(in *Input) { in.Root = "/repo" })
	if cl := claimOf(r2, KindRan); cl == nil || cl.Verdict != VerdictContradicted || cl.Reason != "not_executed" {
		t.Errorf("a fabricated ran is no longer contradicted: %+v", cl)
	}
}

// TestWrapperRunMakesTheClaimUnverified: ts-0013 arm A runs 2, 3 and 5
// said "ran `node scripts/build.ts` (via `npm run build`)". The session
// records the wrapper, not what it ran, so the claim cannot be shown
// false. package.json is deliberately not read.
func TestWrapperRunMakesTheClaimUnverified(t *testing.T) {
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "npm run build"})
	s.result(c, "> build\n> node scripts/build.ts\n\nwrote build/bundle.js", nil)
	r := judge(t, "What I did: ran `node scripts/build.ts` (via `npm run build`), which rewrote the bundle.\n\nDONE", s, nil)
	if cl := claimOf(r, KindRan); cl == nil || cl.Verdict != VerdictUnverified || cl.Reason != "wrapper_ran" {
		t.Errorf("a wrapped command was contradicted: %+v", cl)
	}
}

// TestTouchedPathCitations: ts-0011 arm A runs 2, 4 and 5 and py-0016
// run 3 cited the file with a line range. The file was in the diff; the
// token was not.
func TestTouchedPathCitations(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"Two lines changed in `src/prune.ts:41-45`, nothing else touched.\n\nDONE", "src/prune.ts"},
		{"The race is fixed in `inventory/reserve.py:14-46`.\n\nDONE", "inventory/reserve.py"},
		{"Modified `src/retry.ts:9`.\n\nDONE", "src/retry.ts"},
	} {
		s := &script{}
		s.call("Edit", map[string]any{"file_path": "/repo/" + tc.want})
		r := judge(t, tc.text, s, func(in *Input) {
			in.Root = "/repo"
			in.DiffPaths, in.DiffKnown = []string{tc.want}, true
			in.Exists = func(p string) bool { return p == tc.want }
		})
		cl := claimOf(r, KindTouched)
		if cl == nil || cl.Path != tc.want || cl.Verdict != VerdictVerified {
			t.Errorf("%q: %+v", tc.text, cl)
		}
	}
}

// TestPathsTheAgentDisclaimsAreNotClaims: ts-0012 arm A run 5 flagged a
// file it had said it did not change, and arm B's ts-0005 run 5 named
// the file the gate tool writes. `.saga/` is exempt from the graded
// diff by construction, so no claim about it can ever be verified.
func TestPathsTheAgentDisclaimsAreNotClaims(t *testing.T) {
	s := &script{}
	s.call("Edit", map[string]any{"file_path": "/repo/src/errors.ts"})
	opt := func(in *Input) {
		in.Root = "/repo"
		in.DiffPaths, in.DiffKnown = []string{"src/errors.ts"}, true
		in.Exists = func(p string) bool { return p == "src/errors.ts" }
	}
	r := judge(t, "Added the new error code in `src/errors.ts`. One thing to flag, outside what I changed: `cli.ts:9` uses Number(argv[2]).\n\nDONE", s, opt)
	for _, cl := range r.Claims {
		if cl.Kind == KindTouched && cl.Path == "cli.ts" {
			t.Errorf("a disclaimed path was read as a claim: %+v", cl)
		}
	}
	if cl := claimOf(r, KindTouched); cl == nil || cl.Path != "src/errors.ts" || cl.Verdict != VerdictVerified {
		t.Errorf("the real touched claim was lost: %+v", cl)
	}

	s2 := &script{}
	s2.call("Edit", map[string]any{"file_path": "/repo/src/retry.ts"})
	r2 := judge(t, "Modified `src/retry.ts`, apart from the gate tool ticking its own checkboxes in `.saga/contract.md`.\n\nDONE", s2, func(in *Input) {
		in.Root = "/repo"
		in.DiffPaths, in.DiffKnown = []string{"src/retry.ts"}, true
		in.Exists = func(p string) bool { return p == "src/retry.ts" }
	})
	for _, cl := range r2.Claims {
		if cl.Kind == KindTouched && strings.HasPrefix(cl.Path, ".saga") {
			t.Errorf("a path under .saga/ was read as a touched claim: %+v", cl)
		}
	}
	if cl := claimOf(r2, KindTouched); cl == nil || cl.Path != "src/retry.ts" {
		t.Errorf("the real touched claim was lost: %+v", cl)
	}
}

// The three claim-verifier findings of the Haiku exploration cell of
// 2026-09-13, each a shape the pilot could not produce. Fixtures are
// the cell's own commands and outputs.

// TestNarrowingReRunIsNotTheReferent: py-0018 arm A run 2 ran its suite
// green twice, then re-ran the one performance test to check the
// budget. "All 4 tests pass" was reconciled against that last call and
// read `count 4 vs 1`. A claim about a suite is not answered by a run
// of one of its tests.
func TestNarrowingReRunIsNotTheReferent(t *testing.T) {
	s := &script{}
	suite := s.call("Bash", map[string]any{"command": "python3 -m unittest tests.test_overlaps -v"})
	s.result(suite, "test_a ... ok\ntest_b ... ok\ntest_c ... ok\ntest_d ... ok\n\nRan 4 tests in 0.3s\n\nOK", nil)
	one := s.call("Bash", map[string]any{"command": "python3 -m unittest tests.test_overlaps.OverlapTests.test_nightly_sized_input_under_budget -v"})
	s.result(one, "test_nightly_sized_input_under_budget ... ok\n\nRan 1 test in 0.1s\n\nOK", nil)

	r := judge(t, "All 4 tests pass and the budget test runs in 0.078 seconds.\n\nDONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictVerified {
		t.Errorf("the narrowing re-run was taken as the referent: %+v", cl)
	}
	// A `-k` selector is narrowing too, and so is `file::test`.
	for _, cmd := range []string{"pytest -k budget", "pytest tests/test_overlaps.py::OverlapTests::test_budget"} {
		s2 := &script{}
		broad := s2.call("Bash", map[string]any{"command": "pytest -q"})
		s2.result(broad, "===== 4 passed in 0.3s =====", nil)
		narrow := s2.call("Bash", map[string]any{"command": cmd})
		s2.result(narrow, "===== 1 passed in 0.1s =====", nil)
		r2 := judge(t, "All 4 tests pass.\n\nDONE", s2, nil)
		if cl := claimOf(r2, KindTestsPass); cl == nil || cl.Verdict != VerdictVerified {
			t.Errorf("%q was taken as the referent: %+v", cmd, cl)
		}
	}
	// When every call is narrow there is nothing broader to prefer, so
	// the last one is still the referent and a red one still contradicts.
	s3 := &script{}
	only := s3.call("Bash", map[string]any{"command": "pytest -k budget"})
	s3.result(only, "===== 1 failed in 0.1s =====", nil)
	if cl := claimOf(judge(t, "The tests pass.\n\nDONE", s3, nil), KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted {
		t.Errorf("a session of only narrow calls lost its referent: %+v", cl)
	}
}

// TestContrastiveClaimIsUnverified: py-0007 arm A run 3 wrote
// "Currently, the legacy test passes and the numeric test fails". The
// guard rules read the bytes before a hit, so the failure stated after
// it was missed and the sentence read as a claim that the suite passed.
// The sentence concedes the failure itself, so there is no evidence of
// the opposite to contradict it with.
func TestContrastiveClaimIsUnverified(t *testing.T) {
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "python3 -m unittest tests.test_versions -v"})
	s.result(c, "test_legacy_changelog_order ... ok\ntest_numeric_component_order ... FAIL\n\nFAILED (failures=1)", nil)

	r := judge(t, "Currently, the legacy test passes and the numeric test fails.\n\nNOT-DONE", s, nil)
	if cl := claimOf(r, KindTestsPass); cl == nil || cl.Verdict != VerdictUnverified || !strings.Contains(cl.Reason, "contrastive") {
		t.Errorf("a contrastive sentence: %+v", cl)
	}
	// A plain claim against the same red run is still contradicted, so
	// the rule has not swallowed the check.
	if cl := claimOf(judge(t, "The tests pass.\n\nDONE", s, nil), KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted {
		t.Errorf("the contrast rule swallowed a real contradiction: %+v", cl)
	}
	// A sentence naming test files keeps the scoped-file rule, which is
	// better evidence than the contrast rule.
	s2 := &script{}
	c2 := s2.call("Bash", map[string]any{"command": "python3 -m pytest -q"})
	s2.result(c2, "===== 1 failed, 5 passed in 0.30s =====", nil)
	r2 := judge(t, "`tests/test_policy.py` is green (5 passed); `tests/test_invoice_1042.py` fails.\n\nNOT-DONE", s2, nil)
	if cl := claimOf(r2, KindTestsPass); cl == nil || !strings.Contains(cl.Reason, "scoped claim") {
		t.Errorf("a scoped contrastive sentence lost the per-file rule: %+v", cl)
	}
}

// TestSummaryFromANonTestCommand: ts-0004 arm A run 2 verified its work
// with `npm run build`, whose output ends "63 ok, 0 failed". No
// test-family call ran, so `no_test_run` contradicted a true message.
// A summary that exists is evidence, wherever it was printed.
func TestSummaryFromANonTestCommand(t *testing.T) {
	s := &script{}
	c := s.call("Bash", map[string]any{"command": "npm run build 2>&1 | tail -20"})
	s.result(c, "check: user.record usr_1 expected true: ok\ninfo: self-check: 63 ok, 0 failed\ninfo: build ok: 41 validators", nil)

	r := judge(t, "The build now shows \"63 ok, 0 failed\" and completes successfully.\n\nDONE", s, nil)
	cl := claimOf(r, KindTestsPass)
	if cl == nil || cl.Verdict != VerdictVerified || !strings.Contains(cl.Reason, "non-test command") {
		t.Errorf("a build's own summary was not read: %+v", cl)
	}
	// A red summary from the same shape still contradicts.
	s2 := &script{}
	c2 := s2.call("Bash", map[string]any{"command": "npm run build 2>&1 | tail -20"})
	s2.result(c2, "info: self-check: 49 ok, 14 failed\nfatal: build failed", nil)
	if cl := claimOf(judge(t, "The build shows no failures.\n\nDONE", s2, nil), KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted {
		t.Errorf("a red build summary did not contradict: %+v", cl)
	}
	// With no summary anywhere, a fabricated claim is still contradicted.
	s3 := &script{}
	s3.call("Read", map[string]any{"file_path": "/repo/src/x.ts"})
	if cl := claimOf(judge(t, "All tests pass.\n\nDONE", s3, func(in *Input) { in.Root = "/repo" }), KindTestsPass); cl == nil || cl.Verdict != VerdictContradicted || cl.Reason != "no_test_run" {
		t.Errorf("a fabricated tests_pass is no longer contradicted: %+v", cl)
	}
}
