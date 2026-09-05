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
