package claims

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/harness/claude"
	"github.com/ddh4r4m/saga/internal/hook"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// repo is a scratch git repository with .saga initialised and the
// composed entry wired the way `saga hook claude-code` wires it.
type repo struct {
	t     *testing.T
	root  string
	store *store.Store
	gate  *gate.Layer
	trace *trace.Layer
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	r := &repo{t: t, root: root}
	r.git("init", "-q")
	r.git("config", "user.email", "t@t")
	r.git("config", "user.name", "t")
	r.git("config", "commit.gpgsign", "false")
	if _, err := store.Init(root); err != nil {
		t.Fatal(err)
	}
	r.store = store.Open(root)
	// `saga init` writes a config skeleton; minimal mode is its absence
	// at BASE:, so the scratch repository commits without it.
	os.Remove(r.store.Path("config.toml"))
	r.write("README.md", "hello\n")
	r.commit("base")
	for _, m := range gate.AgentShellMarkers {
		t.Setenv(m, "")
	}
	prev := gate.ParentHarness
	gate.ParentHarness = func() string { return "" }
	t.Cleanup(func() { gate.ParentHarness = prev })
	r.gate = &gate.Layer{Store: r.store}
	r.trace = &trace.Layer{Store: r.store, Version: "test", Config: store.DefaultConfig(), Components: []string{"trace", "gate"}, ClaimedDone: func(final string) (*bool, string) {
		l, _ := Default()
		d := Detect(l, final, "")
		return d.ClaimedDone, d.Reason
	}}
	return r
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

// hook drives one recorded Claude Code payload through the composed
// entry (trace, gate, claims) and returns the rendered JSON and code.
func (r *repo) hook(event, payload string) (map[string]any, cli.Code) {
	r.t.Helper()
	var out, errb bytes.Buffer
	e := &hook.Entry{Harness: "claude-code", Parse: claude.Parse, Render: claude.Render,
		Layers: []hookio.Layer{r.trace, r.gate, &Layer{Store: r.store, Gate: r.gate}}, Deadline: 20 * time.Second,
		Stdin: strings.NewReader(payload), Stdout: &out, Stderr: &errb}
	code := e.Run(event)
	if errb.Len() > 0 {
		r.t.Logf("%s stderr: %s", event, strings.TrimSpace(errb.String()))
	}
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &m); err != nil {
		r.t.Fatalf("%s: stdout not JSON: %q", event, out.String())
	}
	return m, code
}

func (r *repo) common(session string) string {
	return `"session_id":"` + session + `","cwd":"` + r.root + `"`
}

func (r *repo) toolCall(session, id, tool string, input map[string]any, response string) {
	r.t.Helper()
	ij, _ := json.Marshal(input)
	r.hook(hookio.EventPreToolUse, `{`+r.common(session)+`,"hook_event_name":"PreToolUse","tool_name":"`+tool+`","tool_input":`+string(ij)+`,"tool_use_id":"`+id+`"}`)
	if response != "" {
		r.hook(hookio.EventPostToolUse, `{`+r.common(session)+`,"hook_event_name":"PostToolUse","tool_name":"`+tool+`","tool_input":`+string(ij)+`,"tool_response":`+response+`,"tool_use_id":"`+id+`","duration_ms":10}`)
	}
}

func (r *repo) stop(session, final string, active bool) (map[string]any, cli.Code) {
	r.t.Helper()
	fj, _ := json.Marshal(final)
	act := "false"
	if active {
		act = "true"
	}
	return r.hook(hookio.EventStop, `{`+r.common(session)+`,"hook_event_name":"Stop","stop_hook_active":`+act+`,"last_assistant_message":`+string(fj)+`}`)
}

func (r *repo) claimEvents(session string) []trace.Event {
	r.t.Helper()
	events, err := trace.ReadAll(trace.SessionDir(r.store, session))
	if err != nil {
		r.t.Fatal(err)
	}
	var out []trace.Event
	for _, ev := range events {
		if ev.Type == trace.TypeGate && ev.Body["kind"] == "claim" {
			out = append(out, ev)
		}
	}
	return out
}

func TestStopChainMinimalMode(t *testing.T) {
	r := newRepo(t)
	s := "sess-min"
	r.hook(hookio.EventSessionStart, `{`+r.common(s)+`,"hook_event_name":"SessionStart","source":"startup"}`)
	r.hook(hookio.EventUserPromptSubmit, `{`+r.common(s)+`,"hook_event_name":"UserPromptSubmit","prompt":"add a helper"}`)
	r.toolCall(s, "tu1", "Write", map[string]any{"file_path": filepath.Join(r.root, "src", "helper.py"), "content": "x"}, `{"filePath":"src/helper.py"}`)
	r.write("src/helper.py", "def f(): pass\n")

	// Verified: work observed, touched path in the diff. Zero tokens injected.
	m, code := r.stop(s, "Added `src/helper.py` with the helper.\n\nDONE", false)
	if code != cli.ExitOK || m["decision"] != nil {
		t.Fatalf("verified stop: %v %v", m, code)
	}
	evs := r.claimEvents(s)
	if len(evs) != 1 || evs[0].Body["verdict"] != "verified" || evs[0].Body["decision"] != "allow" || evs[0].Body["claimed_done"] != true || evs[0].Body["mode"] != "minimal" {
		t.Fatalf("claim event: %+v", evs)
	}
	if evs[0].Source != "hook:Stop" || evs[0].Body["trigger"] != "stop" || evs[0].Body["claims_list"] != ClaimsListHash {
		t.Errorf("claim event envelope: %+v", evs[0])
	}
	obs, _ := r.store.ReadObserved(s)
	if obs.Tokens["trace"] != 0 {
		t.Errorf("verified verdict injected %d tokens", obs.Tokens["trace"])
	}
	// The turn event carries claimed_done and the final message inline.
	events, _ := trace.ReadAll(trace.SessionDir(r.store, s))
	var turnEnd *trace.Event
	for i := range events {
		if events[i].Type == trace.TypeTurn && events[i].Body["phase"] == "assistant_end" {
			turnEnd = &events[i]
		}
	}
	if turnEnd == nil || turnEnd.Body["claimed_done"] != true || turnEnd.Body["claimed_done_reason"] != "structural" || turnEnd.Body["final_message_inline"] == nil {
		t.Errorf("turn end: %+v", turnEnd)
	}

	// Unverified in minimal mode: warn, exit 1, nothing injected.
	m, code = r.stop(s, "I inspected `src/nothere.py`.", false)
	if code != cli.ExitFinding || m["decision"] != nil {
		t.Errorf("unverified in minimal mode should warn: %v %v", m, code)
	}
	evs = r.claimEvents(s)
	if last := evs[len(evs)-1]; last.Body["verdict"] != "unverified" || last.Body["decision"] != "warn" || last.Body["exit"] != float64(1) {
		t.Errorf("warn event: %v", last.Body)
	}

	// Contradicted: fabricated test run. Block with the fixed line, exit 5.
	m, code = r.stop(s, "I ran `pytest -q` and all tests pass.\n\nDONE", false)
	reason, _ := m["reason"].(string)
	if code != cli.ExitIntegrity || m["decision"] != "block" || !strings.HasPrefix(reason, "saga trace: claim contradicted: ") || !strings.Contains(reason, "no_test_run") || !strings.Contains(reason, "ran pytest -q") {
		t.Fatalf("contradicted: %v %v", m, code)
	}
	if strings.Contains(reason, "all tests pass") {
		t.Error("claim text leaked into the block line")
	}
	obs, _ = r.store.ReadObserved(s)
	if obs.Tokens["trace"] == 0 || obs.Tokens["trace"] > MessageTokens || obs.GateBlocks != 1 {
		t.Errorf("accounting: tokens %d blocks %d", obs.Tokens["trace"], obs.GateBlocks)
	}
	// Six consecutive no-progress blocks, then release with HANDOFF REQUIRED
	// and a drift event; the cap stays under Claude Code's 8.
	released := -1
	for i := 0; i < 8; i++ {
		m, code = r.stop(s, "I ran `pytest -q` and all tests pass.\n\nDONE", true)
		if m["decision"] != "block" {
			released = i
			hso, _ := m["hookSpecificOutput"].(map[string]any)
			ctx, _ := hso["additionalContext"].(string)
			if !strings.Contains(ctx, "HANDOFF REQUIRED") || code != cli.ExitOK {
				t.Errorf("release: %v %v", m, code)
			}
			break
		}
	}
	if released != 5 {
		t.Errorf("released at block %d, want 5 (max_blocks 6)", released)
	}
	events, _ = trace.ReadAll(trace.SessionDir(r.store, s))
	var drift *trace.Event
	for i := range events {
		if events[i].Type == trace.TypeDrift {
			drift = &events[i]
		}
	}
	if drift == nil || drift.Body["signal"] != "claim" || drift.Body["action"] != "released" {
		t.Errorf("drift release event: %+v", drift)
	}
	// Progress (a better verdict) resets the counter.
	m, code = r.stop(s, "Added `src/helper.py`.\n\nDONE", true)
	if m["decision"] != nil || code != cli.ExitOK {
		t.Errorf("progress: %v %v", m, code)
	}
	obs, _ = r.store.ReadObserved(s)
	if obs.GateBlocks != 0 {
		t.Errorf("blocks not reset: %d", obs.GateBlocks)
	}
	// SubagentStop records with the agent id and never blocks.
	m, code = r.hook(hookio.EventSubagentStop, `{`+r.common(s)+`,"hook_event_name":"SubagentStop","agent_id":"ag1","agent_type":"Explore","last_assistant_message":"I ran `+"`pytest`"+` and everything passes."}`)
	if len(m) != 0 || code != cli.ExitOK {
		t.Errorf("subagent stop must not block: %v %v", m, code)
	}
	evs = r.claimEvents(s)
	if last := evs[len(evs)-1]; last.Agent != "ag1" || last.Body["decision"] != "record" || last.Body["verdict"] != "contradicted" || last.Body["trigger"] != "subagent_stop" {
		t.Errorf("subagent claim event: %v", last)
	}
	// The parent's next Stop lists it as evidence.
	r.stop(s, "Added `src/helper.py`.\n\nDONE", false)
	evs = r.claimEvents(s)
	if last := evs[len(evs)-1]; last.Body["subagent_claims"] == nil {
		t.Errorf("parent stop lacks subagent_claims: %v", last.Body)
	}
	// The whole chain verifies.
	if _, err := trace.Verify(trace.SessionDir(r.store, s)); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Every claim event body validates as saga.trace.claims/1 with schema and session.
	for _, ev := range evs {
		body := map[string]any{}
		for k, v := range ev.Body {
			body[k] = v
		}
		body["schema"], body["session"] = Schema, s
		v, _ := schema.Normalize(body)
		if err := schema.ValidateID(Schema, v); err != nil {
			t.Errorf("claim event seq %d: %v", ev.Seq, err)
		}
	}
}

func TestStopChainFullModeAndGateOrder(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/config.toml", "[gate]\nrequire_red = false\nmax_blocks = 4\n")
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", "# Contract: scratch\n\nIN: src/**, marker.txt\n\n- [ ] G1: the marker says done\n    CHECK: cat marker.txt\n    EXPECT: CANARY-DONE\n")
	r.commit("contract")
	adir := filepath.Join(t.TempDir(), "approved")
	os.MkdirAll(adir, 0o700)
	t.Setenv(gate.ApprovalEnv, adir)
	ld, err := gate.Load(r.root, r.store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Check(ld, gate.CheckOptions{Approve: true}); err != nil {
		t.Fatal(err)
	}
	s := "sess-full"
	r.hook(hookio.EventSessionStart, `{`+r.common(s)+`,"hook_event_name":"SessionStart","source":"startup"}`)
	r.hook(hookio.EventUserPromptSubmit, `{`+r.common(s)+`,"hook_event_name":"UserPromptSubmit","prompt":"make the marker say done"}`)

	// Gate unmet at this very tree (the baseline check ran here) and the
	// message claims done with no edit: contradicted (unmet_at_tree),
	// tagged zero_edit_done; gate's reason first, trace's claim line
	// second; the merged exit is 5 over gate's 1.
	m, code := r.stop(s, "Everything is complete.\n\nDONE", false)
	reason, _ := m["reason"].(string)
	if m["decision"] != "block" || code != cli.ExitIntegrity {
		t.Fatalf("gate unmet + done: %v %v", m, code)
	}
	gi, ci := strings.Index(reason, "saga gate: "), strings.Index(reason, "saga trace: claim contradicted")
	if gi < 0 || ci < 0 || gi > ci {
		t.Fatalf("order of reasons: %q", reason)
	}
	if !strings.Contains(reason, "zero_edit_done") || !strings.Contains(reason, "unmet_at_tree scratch:G1") {
		t.Errorf("zero-edit done not tagged: %q", reason)
	}
	evs := r.claimEvents(s)
	if last := evs[len(evs)-1]; last.Body["mode"] != "full" || last.Body["gate_status_exit"] != float64(1) || last.Body["tree_hash"] == nil || last.Body["zero_edit_done"] != true {
		t.Errorf("full-mode event: %v", last.Body)
	}
	// The tree moves without a re-check: the unmet evidence is stale, so a
	// done claim is unverified; full mode blocks it with exit 1.
	r.write("marker.txt", "still no\n")
	m, code = r.stop(s, "Updated `marker.txt`; everything is complete.\n\nDONE", false)
	reason, _ = m["reason"].(string)
	if m["decision"] != "block" || code != cli.ExitFinding || !strings.Contains(reason, "saga trace: claim unverified: done (gates_unmet scratch:G1(unmet))") {
		t.Errorf("unverified done in full mode: %v %v", m, code)
	}
	// Gate met: a done claim verifies against fresh evidence; read-only
	// unverified claims still block in full mode, and the warn policy lifts it.
	r.write("marker.txt", "CANARY-DONE\n")
	ld, _ = gate.Load(r.root, r.store)
	if _, err := gate.Check(ld, gate.CheckOptions{}); err != nil {
		t.Fatal(err)
	}
	m, code = r.stop(s, "Updated `marker.txt`; G1 is met.\n\nDONE", false)
	if m["decision"] != nil || code != cli.ExitOK {
		t.Errorf("all met and fresh: %v %v", m, code)
	}
	evs = r.claimEvents(s)
	if last := evs[len(evs)-1]; last.Body["verdict"] != "verified" {
		t.Errorf("met event: %v", last.Body)
	}
	m, code = r.stop(s, "I reviewed `src/unknown.py`.", false)
	if m["decision"] != "block" || code != cli.ExitFinding {
		t.Errorf("unverified in full mode: %v %v", m, code)
	}
	r.write(".saga/config.toml", "[gate]\nrequire_red = false\n\n[trace.claims]\nunverified = \"warn\"\n")
	r.commit("warn policy")
	ld, _ = gate.Load(r.root, r.store)
	gate.Check(ld, gate.CheckOptions{})
	m, code = r.stop(s, "I reviewed `src/unknown.py`.", false)
	if m["decision"] != nil || code != cli.ExitFinding {
		t.Errorf("warn policy: %v %v", m, code)
	}
	// Offline: saga trace claims over the session agrees with the last online verdict.
	in, err := FromSession(r.store, s, 0)
	if err != nil {
		t.Fatal(err)
	}
	res := Judge(in)
	if res.Verdict != VerdictUnverified || in.Mode != ModeFull || in.Unverified != "warn" || in.Gate == nil {
		t.Errorf("offline: verdict %s mode %s policy %s gate %v", res.Verdict, in.Mode, in.Unverified, in.Gate != nil)
	}
	body := res.Body(in)
	body["schema"], body["session"] = Schema, s
	v, _ := schema.Normalize(body)
	if err := schema.ValidateID(Schema, v); err != nil {
		t.Errorf("offline body: %v", err)
	}
}
