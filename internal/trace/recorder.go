package trace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
)

// ForbiddenCommands is the contracts section 8 list, enforced by string
// match in the composed hook until guard's post-expansion classifier is
// installed (M1).
var ForbiddenCommands = []string{
	"saga gate approve", "saga gate attest", "saga gate check --approve",
	"saga trace budget --raise", "saga trace ack", "saga trace pin --set", "saga trace prices use", "saga trace prune",
	"saga route policy trust", "saga route validate --write", "saga route budget --raise",
	"saga mem confirm", "saga mem review", "saga mem prune",
	"saga guard policy trust", "saga guard policy set",
	"saga snapshot gc", "saga snapshot prune",
	"saga install", "saga uninstall",
}

// ForbiddenPaths are files the agent may not edit (contracts section 8).
var ForbiddenPaths = []string{".saga/policy.toml", ".saga/route.toml", ".saga/manifest.json", ".saga/.gitignore", ".claude/settings.json", ".claude/settings.local.json"}

// MatchForbidden returns the forbidden command that cmd invokes, or "".
func MatchForbidden(cmd string) string {
	norm := strings.Join(strings.Fields(cmd), " ")
	for _, f := range ForbiddenCommands {
		if strings.Contains(norm, f) {
			return f
		}
	}
	return ""
}

// Layer is trace as a member of the composed hook chain: it records every
// event first and the merged decision last (contracts section 1).
type Layer struct {
	Store   *store.Store
	Version string
	Config  store.Config
	// Components is what saga.trace.pins/1 reports as installed.
	Components []string
	// Stderr receives one-line notes that never reach the harness.
	Stderr func(string)
	// ClaimedDone is the trace-spec section 5.6 detector over the final
	// message, wired by the entry (internal/trace/claims); nil leaves
	// claimed_done null with a reason.
	ClaimedDone func(final string) (claimed *bool, reason string)

	pending *toolCall
}

type toolCall struct {
	ev *Event
}

// Name implements hookio.Layer.
func (l *Layer) Name() string { return "trace" }

func (l *Layer) note(format string, args ...any) {
	if l.Stderr != nil {
		l.Stderr(fmt.Sprintf(format, args...))
	}
}

// session bundles what one hook invocation opens.
type session struct {
	st     *store.Store
	obs    *store.Observed
	w      *Writer
	dir    string
	prices *PriceTable
	l      *Layer
}

func (l *Layer) open(in *hookio.Input) (*session, error) {
	id := in.SessionID
	if id == "" {
		id = canon.ULID()
	}
	obs, err := l.Store.ReadObserved(id)
	if err != nil {
		return nil, err
	}
	if obs.MaskSalt == "" {
		obs.MaskSalt = canon.ULID()
	}
	dir := SessionDir(l.Store, id)
	w, err := OpenWriter(dir, id, NewMasker(obs.MaskSalt))
	if err != nil {
		return nil, err
	}
	return &session{st: l.Store, obs: obs, w: w, dir: dir, l: l}, nil
}

func (s *session) close() error {
	werr := s.st.WriteObserved(s.obs)
	cerr := s.w.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

func (s *session) append(ev *Event) error {
	ev.Turn = s.obs.Turn
	if ev.Agent == "" {
		ev.Agent = "main"
	}
	if err := s.w.Append(ev); err != nil {
		return err
	}
	s.obs.Seq, s.obs.Prev = s.w.Seq(), s.w.Prev()
	return nil
}

func (s *session) loadPrices() error {
	if s.prices != nil {
		return nil
	}
	t, err := LoadPrices(s.st)
	if err != nil {
		return err
	}
	s.prices = t
	return nil
}

// Run implements hookio.Layer.
func (l *Layer) Run(ctx context.Context, in *hookio.Input) (*hookio.Output, error) {
	out := hookio.Allow("trace")
	if l.Store == nil || !l.Store.Exists() {
		l.note("saga trace: .saga missing, nothing recorded; run saga init")
		return out, nil
	}
	s, err := l.open(in)
	if err != nil {
		return out, err
	}
	defer func() {
		if cerr := s.close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	src := "hook:" + in.Event
	switch in.Event {
	case hookio.EventSessionStart:
		err = s.sessionEvent(in, src, "start", out)
	case hookio.EventSessionEnd:
		if terr := s.sweepTranscript(in, src); terr != nil {
			l.note("saga trace: transcript: %v", terr)
		}
		err = s.sessionEvent(in, src, "end", out)
	case hookio.EventUserPromptSubmit:
		err = s.userPrompt(in, src)
	case hookio.EventPreToolUse:
		err = s.preTool(in, src, out)
	case hookio.EventPostToolUse:
		err = s.postTool(in, src, out)
	case hookio.EventPostToolUseFailure:
		err = s.postToolFailure(in, src, out)
	case hookio.EventStop:
		err = s.stop(in, src, out)
	case hookio.EventSubagentStart, hookio.EventSubagentStop:
		err = s.subagent(in, src)
	case hookio.EventPreCompact, hookio.EventPostCompact:
		err = s.compaction(in, src)
	}
	return out, err
}

// Finalize implements hookio.Finalizer: it writes the pending tool_call
// with the merged decision.
func (l *Layer) Finalize(ctx context.Context, in *hookio.Input, merged *hookio.Output) error {
	if l.pending == nil || l.Store == nil || !l.Store.Exists() {
		return nil
	}
	ev := l.pending.ev
	l.pending = nil
	s, err := l.open(in)
	if err != nil {
		return err
	}
	defer s.close()
	ev.Body["decision"] = merged.Decision
	if merged.Reason != "" {
		ev.Body["decision_reason"] = canon.CleanText(merged.Reason)
	} else {
		ev.Body["decision_reason"] = nil
	}
	if err := s.append(ev); err != nil {
		return err
	}
	if in.ToolUseID != "" && merged.Decision != hookio.DecisionDeny {
		s.obs.Pending[in.ToolUseID] = ev.Seq
	}
	return nil
}

func (s *session) sessionEvent(in *hookio.Input, src, phase string, out *hookio.Output) error {
	if phase == "start" && (in.Source == "resume" || in.Source == "compact" || in.Source == "fork") {
		phase = "resume"
	}
	if err := s.loadPrices(); err != nil {
		return err
	}
	var settingsHash, hooksHash *string
	if raw, err := os.ReadFile(filepath.Join(in.Cwd, ".claude", "settings.json")); err == nil {
		h := canon.SHA256(raw)
		settingsHash = &h
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			if hb, err := canon.JSON(m["hooks"]); err == nil {
				hh := canon.SHA256(hb)
				hooksHash = &hh
			}
		}
	}
	pins := NewPins(in.Harness, s.l.Version, settingsHash, hooksHash, in.Effort, s.prices.Hash, s.l.Components)
	prev, had, err := ReadPins(s.st)
	if err != nil {
		return err
	}
	changed := []string{}
	if had {
		changed = ChangedKeys(prev, pins)
	}
	if err := WritePins(s.st, pins); err != nil {
		return err
	}
	var configHash any
	if raw, err := os.ReadFile(s.st.Path("config.toml")); err == nil {
		configHash = canon.SHA256(raw)
	}
	pm, _ := toMap(pins)
	body := map[string]any{
		"phase": phase, "pins": pm, "harness": in.Harness,
		"cwd_hash": canon.SHA256([]byte(in.Cwd)), "config_hash": configHash, "changed": changed,
	}
	if in.Source != "" {
		body["source"] = in.Source
	}
	// What this step put into additionalContext, so a run's injected
	// tokens can be summed from the trace alone (docs/12 commitment 7).
	// It counts this layer's own injection: a later layer that injects on
	// SessionStart would record its own, and none does today.
	ctxTokens := 0
	if out != nil {
		for _, c := range out.AdditionalContext {
			ctxTokens += canon.TokensEstString(c)
		}
	}
	body["context_tokens_est"] = ctxTokens
	// Resume input carries undocumented cache fields (harness-probes P7);
	// recorded when present, never assumed.
	for _, k := range []string{"context_tokens", "prompt_cache_likely_expired", "seconds_since_last_response", "estimated_cache_write_usd"} {
		if v, ok := in.Raw[k]; ok {
			body[k] = v
		}
	}
	return s.append(&Event{Type: TypeSession, Source: src, Body: body})
}

func (s *session) userPrompt(in *hookio.Input, src string) error {
	s.obs.Turn++
	s.obs.ToolOutputBytesTurn = 0
	if id, ok := in.Raw["turn_id"].(string); ok {
		s.obs.HarnessTurnID = id
	}
	p, err := s.w.StorePayload([]byte(in.Prompt))
	if err != nil {
		return err
	}
	s.obs.OriginTokens["user"] += canon.TokensEst([]byte(in.Prompt))
	body := map[string]any{"phase": "user", "prompt_hash": p.Hash, "prompt_bytes": p.Bytes, "prompt_ref": p.Ref}
	if s.obs.HarnessTurnID != "" {
		body["harness_turn_id"] = s.obs.HarnessTurnID
	}
	mc := p.MaskedCount
	return s.append(&Event{Type: TypeTurn, Source: src, Body: body, MaskedCount: &mc})
}

func (s *session) preTool(in *hookio.Input, src string, out *hookio.Output) error {
	args := in.ToolInput
	if len(args) == 0 {
		args = []byte("{}")
	}
	cargs, err := canon.Canonicalize(args)
	if err != nil {
		cargs = args
	}
	p, err := s.w.StorePayload(cargs)
	if err != nil {
		return err
	}
	body := map[string]any{
		"tool": in.ToolName, "args_hash": p.Hash, "args_ref": p.Ref, "args_inline": nil,
		"component": "harness", "cwd_rel": canon.RelPath(s.st.Root, in.Cwd), "index_version": nil,
	}
	if p.Inline != nil {
		var inline any
		if json.Unmarshal([]byte(*p.Inline), &inline) == nil {
			body["args_inline"] = inline
		} else {
			body["args_inline"] = *p.Inline
		}
	}
	if in.ToolUseID != "" {
		body["tool_use_id"] = in.ToolUseID
	}
	mc := p.MaskedCount
	s.l.pending = &toolCall{ev: &Event{Type: TypeToolCall, Source: src, Body: body, MaskedCount: &mc}}

	// Decisions: agent-forbidden commands and files, then the budget.
	if in.ToolName == "Bash" || in.ToolName == "PowerShell" {
		var ti struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		if f := MatchForbidden(ti.Command); f != "" {
			out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, "saga: agent-forbidden command: "+f, int(cli.ExitRefusal)
			return nil
		}
	}
	if in.ToolName == "Edit" || in.ToolName == "Write" || in.ToolName == "NotebookEdit" {
		var ti struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		rel := canon.RelPath(s.st.Root, ti.FilePath)
		for _, fp := range ForbiddenPaths {
			if rel == fp {
				out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, "saga: agent-forbidden edit: "+fp, int(cli.ExitRefusal)
				return nil
			}
		}
	}
	if Exhausted(s.l.Config.Trace.Budget, s.obs.CumUSD) {
		out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, HardStopMessage("session", s.obs.CumUSD, *s.l.Config.Trace.Budget.SessionUSD), int(cli.ExitFinding)
	}
	return nil
}

func (s *session) postTool(in *hookio.Input, src string, out *hookio.Output) error {
	resp := in.ToolResponse
	if len(resp) == 0 {
		resp = []byte("null")
	}
	cresp, err := canon.Canonicalize(resp)
	if err != nil {
		cresp = resp
	}
	p, err := s.w.StorePayload(cresp)
	if err != nil {
		return err
	}
	var forSeq any
	if seq, ok := s.obs.Pending[in.ToolUseID]; ok {
		forSeq = seq
		delete(s.obs.Pending, in.ToolUseID)
	}
	var toolErr any
	var rm map[string]any
	if json.Unmarshal(resp, &rm) == nil {
		if e, ok := rm["error"].(string); ok && e != "" {
			toolErr = canon.CleanText(e)
		} else if ie, ok := rm["is_error"].(bool); ok && ie {
			toolErr = "tool reported is_error"
		}
	}
	var wall any
	if in.DurationMS > 0 {
		wall = in.DurationMS
	}
	body := map[string]any{
		"for_seq": forSeq, "exit": nil, "error": toolErr, "result_hash": p.Hash, "result_bytes": p.Bytes,
		"result_inline": p.Inline, "result_ref": p.Ref, "truncated": p.Truncated, "wall_ms": wall, "served": "live",
	}
	mc := p.MaskedCount
	if err := s.append(&Event{Type: TypeToolResult, Source: src, Body: body, MaskedCount: &mc}); err != nil {
		return err
	}
	s.obs.ToolOutputBytesTurn += p.Bytes
	s.obs.OriginTokens["tool_results"] += canon.TokensEst(cresp)
	if err := s.sweepTranscript(in, src); err != nil {
		s.l.note("saga trace: transcript: %v", err)
	}
	return s.budgetFeedback(src, out)
}

// exitInError reads an exit status the harness's failure text names
// ("exit code 1", "exited with code 2", "exit status 3").
var exitInError = regexp.MustCompile(`(?i)\bexit(?:ed)?(?: with)?(?: code| status)?[: ]+(\d{1,3})\b`)

// postToolFailure records a failed tool as a tool_result (trace-spec
// 2.2) from the PostToolUseFailure payload (harness-facts C34): the
// failure text is the stored result so a runner summary inside it is
// readable by the claim check, `exit` is the code the text names, else 1
// (a failure is never exit 0), and `error` carries the first line. An
// interrupted tool is recorded with error "interrupted".
func (s *session) postToolFailure(in *hookio.Input, src string, out *hookio.Output) error {
	text := in.ToolError
	if in.Interrupted && text == "" {
		text = "interrupted"
	}
	raw, err := json.Marshal(map[string]any{"error": text, "is_interrupt": in.Interrupted})
	if err != nil {
		return err
	}
	p, err := s.w.StorePayload(raw)
	if err != nil {
		return err
	}
	var forSeq any
	if seq, ok := s.obs.Pending[in.ToolUseID]; ok {
		forSeq = seq
		delete(s.obs.Pending, in.ToolUseID)
	}
	exit := 1
	if m := exitInError.FindStringSubmatch(text); m != nil {
		fmt.Sscanf(m[1], "%d", &exit)
	}
	first := text
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	if in.Interrupted {
		first = "interrupted"
	}
	if first == "" {
		first = "tool failed"
	}
	var wall any
	if in.DurationMS > 0 {
		wall = in.DurationMS
	}
	body := map[string]any{
		"for_seq": forSeq, "exit": exit, "error": canon.CleanText(first), "result_hash": p.Hash, "result_bytes": p.Bytes,
		"result_inline": p.Inline, "result_ref": p.Ref, "truncated": p.Truncated, "wall_ms": wall, "served": "live",
	}
	mc := p.MaskedCount
	if err := s.append(&Event{Type: TypeToolResult, Source: src, Body: body, MaskedCount: &mc}); err != nil {
		return err
	}
	s.obs.ToolOutputBytesTurn += p.Bytes
	s.obs.OriginTokens["tool_results"] += canon.TokensEst(raw)
	return s.budgetFeedback(src, out)
}

func (s *session) budgetFeedback(src string, out *hookio.Output) error {
	c := CheckBudget(s.l.Config.Trace.Budget, s.obs.CumUSD, s.obs.BudgetCrossed)
	if c == nil {
		return nil
	}
	body := map[string]any{"scope": "session", "metric": "usd", "limit": c.Limit, "value": c.Value, "action": c.Action, "ack": nil}
	if err := s.append(&Event{Type: TypeBudget, Source: src, Body: body}); err != nil {
		return err
	}
	if s.obs.Tokens["trace"] < 400 {
		out.AdditionalContext = append(out.AdditionalContext, c.Message)
		s.obs.Tokens["trace"] += canon.TokensEstString(c.Message)
		s.obs.OriginTokens["trace"] += canon.TokensEstString(c.Message)
	}
	return nil
}

func (s *session) stop(in *hookio.Input, src string, out *hookio.Output) error {
	if err := s.sweepTranscript(in, src); err != nil {
		s.l.note("saga trace: transcript: %v", err)
	}
	p, err := s.w.StorePayload([]byte(in.LastAssistantMessage))
	if err != nil {
		return err
	}
	body := map[string]any{
		"phase": "assistant_end", "final_message_hash": p.Hash, "final_message_bytes": p.Bytes, "final_message_ref": p.Ref, "final_message_inline": p.Inline,
		"claimed_done": nil, "claimed_done_reason": "no claim detector wired",
	}
	if s.l.ClaimedDone != nil {
		claimed, reason := s.l.ClaimedDone(in.LastAssistantMessage)
		body["claimed_done"], body["claimed_done_reason"] = claimed, reason
	}
	mc := p.MaskedCount
	if err := s.append(&Event{Type: TypeTurn, Source: src, Body: body, MaskedCount: &mc}); err != nil {
		return err
	}
	if Exhausted(s.l.Config.Trace.Budget, s.obs.CumUSD) && s.obs.Blocks < 6 {
		s.obs.Blocks++
		out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, HardStopMessage("session", s.obs.CumUSD, *s.l.Config.Trace.Budget.SessionUSD), int(cli.ExitFinding)
	}
	return nil
}

func (s *session) subagent(in *hookio.Input, src string) error {
	phase := "start"
	body := map[string]any{"subagent_id": in.AgentID, "parent_agent": "main", "agent_type": in.AgentType}
	if in.Event == hookio.EventSubagentStop {
		phase = "stop"
		p, err := s.w.StorePayload([]byte(in.LastAssistantMessage))
		if err != nil {
			return err
		}
		body["final_message_hash"] = p.Hash
		body["usage"] = nil
		// Sub-agent usage is not in the Agent tool_response (harness-facts
		// C33); it is read from the sub-agent's own transcript.
		if in.AgentTranscriptPath != "" {
			cum, calls, err := s.sweepSubagentTranscript(in, src)
			if err != nil {
				s.l.note("saga trace: subagent transcript: %v", err)
			} else if calls > 0 {
				body["usage"] = cum
				body["model_calls"] = calls
			}
		}
	}
	body["phase"] = phase
	return s.append(&Event{Type: TypeSubagent, Source: src, Agent: firstNonEmpty(in.AgentID, "main"), Body: body})
}

func (s *session) compaction(in *hookio.Input, src string) error {
	phase := "pre"
	body := map[string]any{"trigger": nilIfEmpty(in.Trigger), "context_tokens_before": nil, "context_tokens_after": nil, "state_block_hash": nil}
	if in.Event == hookio.EventPostCompact {
		phase = "post"
		p, err := s.w.StorePayload([]byte(in.CompactSummary))
		if err != nil {
			return err
		}
		body["summary_hash"] = p.Hash
		body["summary_bytes"] = p.Bytes
	}
	body["phase"] = phase
	return s.append(&Event{Type: TypeCompaction, Source: src, Body: body})
}

// sweepTranscript turns new assistant messages in the harness transcript
// into model_call events and ledger rows.
func (s *session) sweepTranscript(in *hookio.Input, src string) error {
	if in.TranscriptPath == "" {
		return nil
	}
	calls, off, err := ReadClaudeTranscript(in.TranscriptPath, s.obs.TranscriptOffset)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	s.obs.TranscriptOffset = off
	if len(calls) == 0 {
		return nil
	}
	if err := s.loadPrices(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range s.obs.SeenMessages {
		seen[id] = true
	}
	for _, c := range calls {
		if seen[c.MessageID] {
			continue
		}
		seen[c.MessageID] = true
		s.obs.SeenMessages = append(s.obs.SeenMessages, c.MessageID)
		if len(s.obs.SeenMessages) > 512 {
			s.obs.SeenMessages = s.obs.SeenMessages[len(s.obs.SeenMessages)-512:]
		}
		if err := s.modelCall(c, in.Harness); err != nil {
			return err
		}
	}
	return nil
}

// sweepSubagentTranscript records a finished sub-agent's model calls under
// agent = <id> and returns the cumulative usage.
func (s *session) sweepSubagentTranscript(in *hookio.Input, src string) (Usage, int, error) {
	var cum Usage
	calls, _, err := ReadClaudeTranscript(in.AgentTranscriptPath, 0)
	if err != nil {
		return cum, 0, err
	}
	if len(calls) == 0 {
		return cum, 0, nil
	}
	if err := s.loadPrices(); err != nil {
		return cum, 0, err
	}
	cum.Source = "transcript"
	cum.ReasoningReason = strp("anthropic bills thinking inside output")
	prevGrowth := 0
	n := 0
	for _, c := range calls {
		u := UsageFromAnthropic(c.Usage, "transcript", "1h")
		ev := &Event{Type: TypeModelCall, Source: "transcript", Agent: in.AgentID, Body: modelCallBody(c, u, in.Harness, Attribute(map[string]int{"mem.preamble": 0}, u.Growth()-prevGrowth))}
		if err := s.append(ev); err != nil {
			return cum, n, err
		}
		row := BuildRow(RowInput{Session: s.w.Session, Turn: s.obs.Turn, Seq: ev.Seq, Model: c.Model, Usage: u, Prices: s.prices, PrevContextGrowth: prevGrowth, CumUSD: s.obs.CumUSD})
		if err := AppendLedger(s.dir, row); err != nil {
			return cum, n, err
		}
		if row.USD.Total == nil {
			s.obs.CumUnpriced++
		}
		s.obs.CumUSD = row.CumUSD
		prevGrowth = u.Growth()
		cum.InputFresh += u.InputFresh
		cum.CacheRead += u.CacheRead
		cum.CacheWrite5m += u.CacheWrite5m
		cum.CacheWrite1h += u.CacheWrite1h
		cum.Output += u.Output
		n++
	}
	return cum, n, nil
}

func modelCallBody(c TranscriptCall, u Usage, harness string, attr map[string]float64) map[string]any {
	body := map[string]any{
		"model_requested": nil, "model_requested_reason": "not in transcript",
		"model_served": c.Model, "request_id": nilIfEmpty(c.RequestID), "request_id_reason": nil,
		"fingerprint": nil, "fingerprint_reason": "anthropic does not expose one",
		"effort": nil, "effort_reason": "per-call value not in transcript",
		"usage": u, "call_key": nil, "call_key_reason": "system and messages not observable from hooks",
		"system_hash": nil, "system_hash_reason": "not exposed by " + harness + " hooks",
		"tools_hash": nil, "tools_hash_reason": "not exposed by " + harness + " hooks",
		"context_tokens_est": u.ContextTokens(), "latency_ms": nil, "latency_ms_reason": "not in transcript",
		"stop_reason": nilIfEmpty(c.StopReason), "status": "ok", "attribution": attr, "message_id": c.MessageID,
	}
	if c.RequestID == "" {
		body["request_id_reason"] = "not in transcript"
	}
	return body
}

func (s *session) modelCall(c TranscriptCall, harness string) error {
	u := UsageFromAnthropic(c.Usage, "transcript", "1h")
	delta := u.Growth() - s.obs.LastContext
	attr := Attribute(s.obs.OriginTokens, delta)
	body := modelCallBody(c, u, harness, attr)
	ev := &Event{Type: TypeModelCall, Source: "transcript", Body: body}
	if err := s.append(ev); err != nil {
		return err
	}
	row := BuildRow(RowInput{
		Session: s.w.Session, Turn: s.obs.Turn, Seq: ev.Seq, Model: c.Model, Usage: u, Prices: s.prices,
		PrevContextGrowth: s.obs.LastContext, ToolOutputBytesTurn: s.obs.ToolOutputBytesTurn,
		OriginTokens: s.obs.OriginTokens, CumUSD: s.obs.CumUSD,
	})
	if err := AppendLedger(s.dir, row); err != nil {
		return err
	}
	if row.USD.Total == nil {
		s.obs.CumUnpriced++
	}
	s.obs.CumUSD = row.CumUSD
	s.obs.LastContext = u.Growth()
	s.obs.OriginTokens = map[string]int{}
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
