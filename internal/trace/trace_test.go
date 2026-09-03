package trace

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/store"
)

func newWriter(t *testing.T) (*Writer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sess")
	w, err := OpenWriter(dir, "sess", NewMasker("salt"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w, dir
}

func sessionBody() map[string]any {
	return map[string]any{"phase": "start", "harness": "claude-code", "cwd_hash": canon.SHA256([]byte("/x")), "config_hash": nil, "changed": []string{}}
}

func TestChainIntegrityAndRecovery(t *testing.T) {
	w, dir := newWriter(t)
	for i := 0; i < 5; i++ {
		if err := w.Append(&Event{Type: TypeSession, Source: "hook:SessionStart", Body: sessionBody()}); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	// Reopen: the head must be recovered from the file, not from memory.
	w2, err := OpenWriter(dir, "sess", NewMasker("salt"))
	if err != nil {
		t.Fatal(err)
	}
	if w2.Seq() != 5 {
		t.Fatalf("recovered seq %d", w2.Seq())
	}
	if err := w2.Append(&Event{Type: TypeCompaction, Source: "hook:PreCompact", Body: map[string]any{"phase": "pre", "trigger": "auto"}}); err != nil {
		t.Fatal(err)
	}
	w2.Close()
	res, err := Verify(dir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.Events != 6 || res.Segments != 1 {
		t.Errorf("verify result %+v", res)
	}
	events, _ := ReadAll(dir)
	if events[0].Prev != canon.Genesis {
		t.Errorf("first prev %s", events[0].Prev)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Prev != events[i-1].Hash || events[i].Seq != i+1 {
			t.Errorf("chain broken at %d", i)
		}
	}
}

func TestVerifyDetectsTamper(t *testing.T) {
	w, dir := newWriter(t)
	for i := 0; i < 3; i++ {
		if err := w.Append(&Event{Type: TypeSession, Source: "hook:SessionStart", Body: sessionBody()}); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	seg := filepath.Join(dir, "events.000001.jsonl")
	raw, _ := os.ReadFile(seg)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	lines[1] = strings.Replace(lines[1], `"harness":"claude-code"`, `"harness":"codex"`, 1)
	os.WriteFile(seg, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	_, err := Verify(dir)
	var ve *VerifyError
	if !errors.As(err, &ve) || ve.Seq != 2 {
		t.Fatalf("expected VerifyError at seq 2, got %v", err)
	}
}

func TestRotation(t *testing.T) {
	w, dir := newWriter(t)
	w.SegmentCap = 2048
	for i := 0; i < 20; i++ {
		if err := w.Append(&Event{Type: TypeSession, Source: "hook:SessionStart", Body: sessionBody()}); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	segs, _ := Segments(dir)
	if len(segs) < 3 {
		t.Fatalf("expected rotation, got %d segments", len(segs))
	}
	res, err := Verify(dir)
	if err != nil {
		t.Fatalf("verify across segments: %v", err)
	}
	if res.Events != 20 || res.Segments != len(segs) {
		t.Errorf("%+v", res)
	}
	// Recovery after rotation continues the chain.
	w2, err := OpenWriter(dir, "sess", NewMasker("salt"))
	if err != nil {
		t.Fatal(err)
	}
	if w2.Seq() != 20 {
		t.Errorf("seq after rotation %d", w2.Seq())
	}
	w2.Close()
}

func TestOversizeEventBecomesToolResultError(t *testing.T) {
	w, _ := newWriter(t)
	big := strings.Repeat("x", LineCap+10)
	ev := &Event{Type: TypeTurn, Source: "hook:UserPromptSubmit", Body: map[string]any{"phase": "user", "prompt_hash": canon.SHA256(nil), "prompt_bytes": 1, "prompt_ref": big}}
	if err := w.Append(ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != TypeToolResult || ev.Body["error"] != "event_oversize" {
		t.Errorf("oversize not replaced: %s %v", ev.Type, ev.Body["error"])
	}
}

func TestPayloadInlineAndBlob(t *testing.T) {
	w, dir := newWriter(t)
	small, err := w.StorePayload([]byte("hello"))
	if err != nil || small.Inline == nil || small.Ref != nil {
		t.Fatalf("small: %+v %v", small, err)
	}
	large, err := w.StorePayload([]byte(strings.Repeat("y", InlineCap+1)))
	if err != nil || large.Ref == nil || large.Inline != nil {
		t.Fatalf("large: %+v %v", large, err)
	}
	if _, err := os.Stat(filepath.Join(dir, *large.Ref)); err != nil {
		t.Errorf("blob missing: %v", err)
	}
}

func TestMasker(t *testing.T) {
	m := NewMasker("s")
	in := "key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789 and AKIAABCDEFGHIJKLMNOP\nexport OPENAI_API_KEY=abcdefgh12345678\n"
	out, n := m.MaskString(in)
	if n != 3 {
		t.Errorf("masked %d, want 3: %s", n, out)
	}
	if strings.Contains(out, "sk-ant-") || strings.Contains(out, "AKIA") || strings.Contains(out, "abcdefgh12345678") {
		t.Errorf("secret survived: %s", out)
	}
	if !strings.Contains(out, "SAGA_MASK_ANTHROPIC_") || !strings.Contains(out, "SAGA_MASK_AWS_") || !strings.Contains(out, "OPENAI_API_KEY=SAGA_MASK_ENV_") {
		t.Errorf("placeholders: %s", out)
	}
	again, _ := m.MaskString(in)
	if again != out {
		t.Error("placeholders not stable within a session")
	}
	clean, n := m.MaskString("func skip(a, b int) int { return a + b } // AKIA is not a key here")
	if n != 0 {
		t.Errorf("negative control masked: %s", clean)
	}
}

func TestUsageMapping(t *testing.T) {
	u := UsageFromAnthropic(map[string]any{"input_tokens": 1820.0, "cache_read_input_tokens": 141200.0, "cache_creation_input_tokens": 9100.0, "output_tokens": 412.0, "cache_creation": map[string]any{"ephemeral_5m_input_tokens": 0.0, "ephemeral_1h_input_tokens": 9100.0}}, "transcript", "1h")
	if u.InputFresh != 1820 || u.CacheRead != 141200 || u.CacheWrite1h != 9100 || u.CacheWrite5m != 0 || u.Output != 412 || u.Reasoning != nil || u.SplitReason != nil {
		t.Errorf("%+v", u)
	}
	u = UsageFromAnthropic(map[string]any{"input_tokens": 1.0, "cache_creation_input_tokens": 50.0}, "transcript", "5m")
	if u.CacheWrite5m != 50 || u.SplitReason == nil {
		t.Errorf("unsplit: %+v", u)
	}
	o := UsageFromOpenAI(map[string]any{"input_tokens": 1000.0, "input_tokens_details": map[string]any{"cached_tokens": 600.0}, "output_tokens": 50.0, "output_tokens_details": map[string]any{"reasoning_tokens": 20.0}}, "stream-json")
	if o.InputFresh != 400 || o.CacheRead != 600 || o.Reasoning == nil || *o.Reasoning != 20 {
		t.Errorf("openai: %+v", o)
	}
	g := UsageFromGoogle(map[string]any{"prompt_token_count": 500.0, "cached_content_token_count": 100.0, "candidates_token_count": 30.0, "thoughts_token_count": 9.0}, "stream-json")
	if g.InputFresh != 400 || g.CacheRead != 100 || g.Output != 30 || *g.Reasoning != 9 {
		t.Errorf("google: %+v", g)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestLedgerArithmeticAgainstFixture(t *testing.T) {
	calls, off, err := ReadClaudeTranscript(filepath.Join("..", "..", "fixtures", "trace", "claude-transcript.jsonl"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if off == 0 || len(calls) != 3 {
		t.Fatalf("calls %d offset %d (dedupe on message.id expected)", len(calls), off)
	}
	if calls[0].Usage["output_tokens"].(float64) != 412 {
		t.Errorf("dedupe kept the first line, want the last: %v", calls[0].Usage)
	}
	prices := DefaultPrices()
	// Call 1: opus 5 at in 5, out 25, cache_read 0.50, write_1h 10.
	u1 := UsageFromAnthropic(calls[0].Usage, "transcript", "1h")
	row1 := BuildRow(RowInput{Session: "fx", Turn: 1, Seq: 2, Model: calls[0].Model, Usage: u1, Prices: prices, OriginTokens: map[string]int{"user": 100}})
	if row1.USD.Total == nil {
		t.Fatalf("row1 unpriced: %v", *row1.USDReason)
	}
	wantIn := 1820 * 5.0 / 1e6
	wantRead := 141200 * 0.50 / 1e6
	wantWrite := 9100 * 10.0 / 1e6
	wantOut := 412 * 25.0 / 1e6
	if !near(*row1.USD.Input, wantIn) || !near(*row1.USD.CacheRead, wantRead) || !near(*row1.USD.CacheWrite, wantWrite) || !near(*row1.USD.Output, wantOut) || !near(*row1.USD.Total, wantIn+wantRead+wantWrite+wantOut) {
		t.Errorf("row1 usd %+v", row1.USD)
	}
	if row1.ContextTokens != 1820+141200+9100 || row1.ContextDelta != 1820+9100 {
		t.Errorf("row1 context %d delta %d", row1.ContextTokens, row1.ContextDelta)
	}
	if !near(*row1.CacheHitRatio, 141200.0/(1820+141200+9100)) {
		t.Errorf("hit ratio %v", *row1.CacheHitRatio)
	}
	if row1.Attribution["user"] != 1 && row1.Attribution["harness"]+row1.Attribution["user"] < 0.999 {
		t.Errorf("attribution %v", row1.Attribution)
	}
	if err := row1.Validate(); err != nil {
		t.Errorf("row1 schema: %v", err)
	}
	// Call 2: 5m write bucket, cumulative usd carried.
	u2 := UsageFromAnthropic(calls[1].Usage, "transcript", "1h")
	row2 := BuildRow(RowInput{Session: "fx", Turn: 1, Seq: 4, Model: calls[1].Model, Usage: u2, Prices: prices, PrevContextGrowth: u1.Growth(), CumUSD: row1.CumUSD})
	if !near(*row2.USD.CacheWrite, 2000*6.25/1e6) || row2.ContextDelta != (300+2000)-(1820+9100) {
		t.Errorf("row2 %+v delta %d", row2.USD, row2.ContextDelta)
	}
	if !near(row2.CumUSD, *row1.USD.Total+*row2.USD.Total) {
		t.Errorf("cum %v", row2.CumUSD)
	}
	// Call 3: unpriced model stays null and does not move cum_usd.
	u3 := UsageFromAnthropic(calls[2].Usage, "transcript", "1h")
	row3 := BuildRow(RowInput{Session: "fx", Turn: 1, Seq: 6, Model: calls[2].Model, Usage: u3, Prices: prices, CumUSD: row2.CumUSD})
	if row3.USD.Total != nil || row3.USDReason == nil || row3.CumUSD != row2.CumUSD {
		t.Errorf("row3 %+v reason %v cum %v", row3.USD, row3.USDReason, row3.CumUSD)
	}
	if err := row3.Validate(); err != nil {
		t.Errorf("row3 schema: %v", err)
	}
	rep := Summarize("fx", []LedgerRow{row1, row2, row3}, nil)
	if rep.Calls != 3 || rep.Unpriced != 1 || !near(rep.TotalUSD, row2.CumUSD) {
		t.Errorf("report %+v", rep)
	}
	if !strings.Contains(rep.Text(), "unpriced calls: 1") {
		t.Errorf("report text:\n%s", rep.Text())
	}
	// Incremental read: nothing new from the same offset.
	more, off2, err := ReadClaudeTranscript(filepath.Join("..", "..", "fixtures", "trace", "claude-transcript.jsonl"), off)
	if err != nil || len(more) != 0 || off2 != off {
		t.Errorf("incremental: %d %d %v", len(more), off2, err)
	}
}

func TestLongContextTierAndPartialPrice(t *testing.T) {
	prices := DefaultPrices()
	u := Usage{InputFresh: 250000, Output: 10, Source: "transcript"}
	usd, reason := prices.Cost("gemini-3.1-pro", u)
	if reason != "" || !near(*usd.Input, 250000*4.0/1e6) || !near(*usd.Output, 10*18.0/1e6) {
		t.Errorf("long context: %+v %s", usd, reason)
	}
	u = Usage{InputFresh: 10, CacheRead: 5, Source: "transcript"}
	if _, reason := prices.Cost("gemini-3-pro", u); !strings.Contains(reason, "cache_read unpriced") {
		t.Errorf("partial price: %q", reason)
	}
	if _, reason := prices.Cost("nope", u); reason == "" {
		t.Error("unknown model priced")
	}
}

func TestBudgetCrossings(t *testing.T) {
	limit := 10.0
	cfg := store.DefaultConfig().Trace.Budget
	cfg.SessionUSD = &limit
	crossed := map[string]bool{}
	if c := CheckBudget(cfg, 1, crossed); c != nil {
		t.Errorf("crossed early: %+v", c)
	}
	c := CheckBudget(cfg, 8.5, crossed)
	if c == nil || c.Level != "soft" || c.Action != "warn" {
		t.Fatalf("soft: %+v", c)
	}
	if c := CheckBudget(cfg, 8.6, crossed); c != nil {
		t.Errorf("soft reported twice: %+v", c)
	}
	c = CheckBudget(cfg, 10.5, crossed)
	if c == nil || c.Level != "hard" || c.Action != "stop" || !strings.Contains(c.Message, "budget --raise") {
		t.Fatalf("hard: %+v", c)
	}
	if !Exhausted(cfg, 10.5) || Exhausted(cfg, 9) {
		t.Error("exhausted")
	}
	if canon.TokensEstString(c.Message) > 60 {
		t.Errorf("hard message over 60 est tokens: %d", canon.TokensEstString(c.Message))
	}
}

func TestPinsChangedKeys(t *testing.T) {
	a := NewPins("claude-code", "0.1.0", nil, nil, "", "sha256:a", []string{"trace"})
	b := NewPins("claude-code", "0.1.0", nil, nil, "high", "sha256:b", []string{"trace"})
	got := ChangedKeys(a, b)
	if strings.Join(got, ",") != "effort,price_table" {
		t.Errorf("changed %v", got)
	}
	raw, _ := json.Marshal(a)
	var v any
	json.Unmarshal(raw, &v)
	if err := validatePins(v); err != nil {
		t.Errorf("pins schema: %v", err)
	}
}

func TestForbiddenMatch(t *testing.T) {
	if MatchForbidden("cd x &&  saga   trace budget --raise session") == "" {
		t.Error("whitespace-normalised match failed")
	}
	if MatchForbidden("saga trace ledger") != "" {
		t.Error("false positive")
	}
}
