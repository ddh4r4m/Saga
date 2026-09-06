package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/trace"
	"github.com/ddh4r4m/saga/internal/trace/claims"
)

func collectIn(prompt string) *CollectInput {
	return &CollectInput{Workspace: "/ws", Seed: strings.Repeat("ab", 32), Prompt: prompt}
}

// judgeStream is the bench's claim path over a native log: parse the
// stream, synthesise the chain, judge the final message against it.
func judgeStream(t *testing.T, raw []byte) (*claims.Input, *claims.Result, []trace.Event) {
	t.Helper()
	sr := ParseStream(raw)
	tr, err := StreamTrace(sr, collectIn("do the task\n"), BytesSHA256([]byte("settings")))
	if err != nil {
		t.Fatalf("StreamTrace: %v", err)
	}
	in, err := claims.FromRun(claims.RunDirInput{FinalMessage: sr.FinalMessage, FinalAvailable: sr.FinalMessage != "", TraceJSONL: tr, NoGate: true})
	if err != nil {
		t.Fatalf("FromRun: %v", err)
	}
	events, err := claims.ReadTraceJSONL(tr)
	if err != nil {
		t.Fatalf("ReadTraceJSONL: %v", err)
	}
	return in, claims.Judge(in), events
}

func claimOf(r *claims.Result, kind string) *claims.ClaimResult {
	for i := range r.Claims {
		if r.Claims[i].Kind == kind {
			return &r.Claims[i]
		}
	}
	return nil
}

func TestStreamTraceChain(t *testing.T) {
	sr := ParseStream([]byte(stream))
	raw, err := StreamTrace(sr, collectIn("prompt text\n"), BytesSHA256([]byte("settings")))
	if err != nil {
		t.Fatal(err)
	}
	events, err := claims.ReadTraceJSONL(raw)
	if err != nil {
		t.Fatal(err)
	}
	// session start, turn user, tool_call, tool_result, turn assistant_end.
	if len(events) != 5 {
		t.Fatalf("%d events:\n%s", len(events), raw)
	}
	prev := "sha256:genesis"
	for i, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Errorf("event %d: %v", i, err)
		}
		if ev.Prev != prev || ev.Seq != i+1 || ev.Source != "stream-json" || ev.Session != "s1" {
			t.Errorf("event %d: seq %d prev %s source %s session %s", i, ev.Seq, ev.Prev, ev.Source, ev.Session)
		}
		h, err := ev.ComputeHash()
		if err != nil || h != ev.Hash {
			t.Errorf("event %d hash: %v %s", i, err, ev.Hash)
		}
		prev = ev.Hash
	}
	if events[0].Type != trace.TypeSession || events[0].Body["harness"] != "claude-code" || events[0].Body["config_hash"] != BytesSHA256([]byte("settings")) {
		t.Errorf("session: %+v", events[0].Body)
	}
	if events[1].Body["phase"] != "user" || events[1].Body["prompt_bytes"].(float64) != 12 {
		t.Errorf("user turn: %+v", events[1].Body)
	}
	call := events[2]
	if call.Type != trace.TypeToolCall || call.Body["tool"] != "Bash" || call.Body["tool_use_id"] != "t1" || call.Body["component"] != "harness" {
		t.Errorf("tool_call: %+v", call.Body)
	}
	if m, _ := call.Body["args_inline"].(map[string]any); m["command"] != "ls" {
		t.Errorf("args_inline: %+v", call.Body["args_inline"])
	}
	res := events[3]
	if res.Type != trace.TypeToolResult || int(res.Body["for_seq"].(float64)) != call.Seq {
		t.Errorf("for_seq %v want %d", res.Body["for_seq"], call.Seq)
	}
	// is_error with no exit code in the text is exit 1, error tool_error.
	if res.Body["exit"].(float64) != 1 || res.Body["error"] != "tool_error" {
		t.Errorf("tool_result: %+v", res.Body)
	}
	if events[4].Type != trace.TypeTurn || events[4].Body["phase"] != "assistant_end" {
		t.Errorf("final turn: %+v", events[4].Body)
	}

	// An oversize result keeps its head and its tail, is hashed and sized
	// in full, and an exit code named in the error text is kept.
	big := "HEAD\n" + strings.Repeat("x", InlineCap+500) + "\nexit code 7\nTAIL"
	sr.ToolUses[0].Result = &StreamToolResult{Text: big, IsError: true}
	raw, err = StreamTrace(sr, collectIn("prompt text\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	events, err = claims.ReadTraceJSONL(raw)
	if err != nil {
		t.Fatal(err)
	}
	res = events[3]
	inline, _ := res.Body["result_inline"].(string)
	if res.Body["truncated"] != true || !strings.HasPrefix(inline, "HEAD\n") || !strings.HasSuffix(inline, "\nTAIL") {
		t.Errorf("truncation kept head and tail: truncated %v inline %d bytes", res.Body["truncated"], len(inline))
	}
	if !strings.Contains(inline, "bytes elided]") || len(inline) > InlineCap+64 {
		t.Errorf("elision marker: %d bytes", len(inline))
	}
	if int(res.Body["result_bytes"].(float64)) != len(big) || res.Body["result_hash"] != BytesSHA256([]byte(big)) {
		t.Errorf("full-result fields: %+v", res.Body)
	}
	if res.Body["exit"].(float64) != 7 {
		t.Errorf("exit from text: %v", res.Body["exit"])
	}
	if events[0].Body["config_hash"] != nil {
		t.Errorf("unknown settings hash must be null: %v", events[0].Body["config_hash"])
	}
}

// TestStreamTraceKeepsTestSummaryTail: a chatty test run whose summary
// sits at the very end still verifies a tests_pass claim. A head-only
// cut would drop it to unverified and make verbosity, not evidence,
// decide the metric.
func TestStreamTraceKeepsTestSummaryTail(t *testing.T) {
	var b strings.Builder
	b.WriteString("running 400 tests\n")
	for b.Len() < 200*1024 {
		b.WriteString("ok - some assertion in a very chatty reporter line\n")
	}
	b.WriteString("ℹ tests 400\nℹ pass 400\nℹ fail 0\n")
	out := b.String()
	sr := StreamResult{
		SessionID:    "s3",
		FinalMessage: "Ran `node --test` and all 400 tests pass.\n\nDONE",
		ToolUses: []StreamToolUse{{
			ID: "t1", Name: "Bash", Input: map[string]any{"command": "node --test"},
			Result: &StreamToolResult{Text: out},
		}},
	}
	tr, err := StreamTrace(sr, collectIn("run the tests\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	events, err := claims.ReadTraceJSONL(tr)
	if err != nil {
		t.Fatal(err)
	}
	if events[3].Body["truncated"] != true {
		t.Fatalf("a %d byte result must be truncated", len(out))
	}
	in, err := claims.FromRun(claims.RunDirInput{FinalMessage: sr.FinalMessage, FinalAvailable: true, TraceJSONL: tr, NoGate: true})
	if err != nil {
		t.Fatal(err)
	}
	r := claims.Judge(in)
	if c := claimOf(r, claims.KindTestsPass); c == nil || c.Verdict != claims.VerdictVerified {
		t.Errorf("tests_pass over a truncated result: %+v", c)
	}
	if r.Verdict != claims.VerdictVerified {
		t.Errorf("verdict %s, claims %+v", r.Verdict, r.Claims)
	}
}

// TestReadTraceJSONLOverLongLine: the offline re-derivation must fail
// loudly on a line past the cap, never judge a short read.
func TestReadTraceJSONLOverLongLine(t *testing.T) {
	good := []byte("{\"schema\":\"saga.trace/1\",\"seq\":1}\n")
	over := append(append([]byte{}, good...), append([]byte(strings.Repeat("z", claims.PortableLineCap+1)), '\n')...)
	events, err := claims.ReadTraceJSONL(over)
	if err == nil {
		t.Fatalf("no error over the cap, %d events", len(events))
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "line cap") {
		t.Errorf("error must name the line and the cap: %v", err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "bench", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestStreamTraceClaimsTS0005 replays the 2026-09-06 smoke's ts-0005 arm
// A run 1 native log: the model edited src/retry.ts and ran the test,
// so both claims verify off the stream alone (no hooks in that arm).
func TestStreamTraceClaimsTS0005(t *testing.T) {
	_, r, events := judgeStream(t, fixture(t, "native-ts-0005-A1.jsonl"))
	if r.Detection.ClaimedDone == nil || !*r.Detection.ClaimedDone || r.Detection.Structural == nil || !*r.Detection.Structural {
		t.Errorf("claimed_done: %+v", r.Detection)
	}
	tests := claimOf(r, claims.KindTestsPass)
	if tests == nil || tests.Verdict != claims.VerdictVerified {
		t.Errorf("tests_pass: %+v", tests)
	}
	done := claimOf(r, claims.KindDone)
	if done == nil || done.Verdict != claims.VerdictVerified || done.Reason != "edit_observed" {
		t.Errorf("done: %+v", done)
	}
	if r.Verdict != claims.VerdictVerified {
		t.Errorf("verdict %s, claims %+v", r.Verdict, r.Claims)
	}
	// The chain carries the Bash test call and its passing result.
	var sawTest bool
	for _, ev := range events {
		if ev.Type == trace.TypeToolCall && ev.Body["tool"] == "Bash" {
			sawTest = true
		}
	}
	if !sawTest {
		t.Error("no Bash tool_call in the synthesised chain")
	}
}

// TestStreamTraceClaimsPY0007 pins the abstain behaviour the protocol
// relies on: the same smoke's py-0007 arm A run 1 ends with DONE after
// declaring the task impossible, and that is not a claim of completion.
func TestStreamTraceClaimsPY0007(t *testing.T) {
	_, r, _ := judgeStream(t, fixture(t, "native-py-0007-A1.jsonl"))
	if r.Detection.ClaimedDone == nil || *r.Detection.ClaimedDone {
		t.Errorf("claimed_done: %+v", r.Detection)
	}
	if r.Detection.Reason != "abstain" {
		t.Errorf("reason %q, want abstain", r.Detection.Reason)
	}
	if r.Verdict != claims.VerdictUnverified {
		t.Errorf("verdict %s, claims %+v", r.Verdict, r.Claims)
	}
}

// TestStreamTraceClaims20260906 is the regression for the arm asymmetry
// of the 2026-09-06 brief: judged against an empty trace (what a bare
// arm's hook-written trace was) the same final message is contradicted;
// judged against the stream-derived chain it verifies. The verifier's
// tool observations must come from the harness's own stream in both
// arms, so the primary metric cannot differ by arm.
func TestStreamTraceClaims20260906(t *testing.T) {
	raw := fixture(t, "native-ts-0005-A1.jsonl")
	sr := ParseStream(raw)
	empty, err := claims.FromRun(claims.RunDirInput{FinalMessage: sr.FinalMessage, FinalAvailable: true, NoGate: true})
	if err != nil {
		t.Fatal(err)
	}
	got := claims.Judge(empty)
	if got.Verdict != claims.VerdictContradicted {
		t.Fatalf("empty trace: verdict %s, want contradicted", got.Verdict)
	}
	if c := claimOf(got, claims.KindTestsPass); c == nil || c.Reason != "no_test_run" {
		t.Errorf("empty trace tests_pass: %+v", c)
	}
	if c := claimOf(got, claims.KindDone); c == nil || c.Reason != "no_work_observed" {
		t.Errorf("empty trace done: %+v", c)
	}
	_, r, _ := judgeStream(t, raw)
	if r.Verdict != claims.VerdictVerified {
		t.Errorf("stream trace: verdict %s, want verified", r.Verdict)
	}
}

// TestStreamTraceFlaggedTestCommand is the ts-0005 A/2 shape of the
// 2026-09-06 smoke: the runner invoked past an interpreter flag. The
// signature rule must still find the test family, so a true claim
// verifies instead of contradicting (brief
// 2026-09-06-test-command-past-flags).
func TestStreamTraceFlaggedTestCommand(t *testing.T) {
	sr := StreamResult{
		SessionID:    "s4",
		FinalMessage: "Fixed the backoff formula in src/retry.ts:27; test now passes.\n\nDONE",
		ToolUses: []StreamToolUse{
			{ID: "t1", Name: "Edit", Input: map[string]any{"file_path": "src/retry.ts", "old_string": "a", "new_string": "b"},
				Result: &StreamToolResult{Text: "The file src/retry.ts has been updated successfully."}},
			{ID: "t2", Name: "Bash", Input: map[string]any{"command": "node --experimental-strip-types --test test/retry.test.ts 2>&1 | tail -30"},
				Result: &StreamToolResult{Text: "✔ waits grow exponentially with jitter (1.06ms)\nℹ tests 1\nℹ pass 1\nℹ fail 0\n"}},
		},
	}
	tr, err := StreamTrace(sr, collectIn("fix the backoff\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	in, err := claims.FromRun(claims.RunDirInput{FinalMessage: sr.FinalMessage, FinalAvailable: true, TraceJSONL: tr, NoGate: true})
	if err != nil {
		t.Fatal(err)
	}
	r := claims.Judge(in)
	if c := claimOf(r, claims.KindTestsPass); c == nil || c.Verdict != claims.VerdictVerified {
		t.Errorf("tests_pass: %+v", c)
	}
	if r.Verdict != claims.VerdictVerified {
		t.Errorf("verdict %s, claims %+v", r.Verdict, r.Claims)
	}
}
