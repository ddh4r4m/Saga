package gate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
)

func hookIn(event, tool string, input map[string]any, active bool) *hookio.Input {
	raw, _ := json.Marshal(input)
	return &hookio.Input{Harness: "claude-code", Event: event, SessionID: "sess1", ToolName: tool, ToolInput: raw, StopHookActive: active}
}

func TestLayerStop(t *testing.T) {
	r := newRepo(t)
	layer := &Layer{Store: r.store}
	// Seed the observed file the way trace does (mask salt present).
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	ctx := context.Background()

	// No contract anywhere: inactive.
	out, err := layer.Run(ctx, hookIn(hookio.EventStop, "", nil, false))
	if err != nil || out.Decision != hookio.DecisionAllow {
		t.Fatalf("inactive: %+v %v", out, err)
	}
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	r.check(CheckOptions{Approve: true}) // baseline red on the empty diff
	out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, false))
	if out.Decision != hookio.DecisionBlock || !strings.Contains(out.Reason, "scratch:G1(unmet)") || !strings.Contains(out.Reason, "run: saga gate status") {
		t.Fatalf("unmet should block with ids only: %+v", out)
	}
	if strings.Contains(out.Reason, "marker says done") {
		t.Error("contract text leaked into a hook message")
	}
	// Consecutive blocks without progress count toward max_blocks (6):
	// the first block plus five active ones, then the seventh Stop is
	// released. The release carries nothing in additionalContext: on Stop
	// that field continues the conversation (harness-facts C11), so a
	// release carrying it is a block by another name.
	var released bool
	for i := 0; i < 8; i++ {
		out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, true))
		if out.Decision == hookio.DecisionAllow {
			released = true
			if i != 5 || len(out.AdditionalContext) != 0 {
				t.Errorf("release at block %d: %+v", i, out)
			}
			break
		}
	}
	if !released {
		t.Error("never released at max_blocks")
	}
	obs, _ = r.store.ReadObserved("sess1")
	if obs.Tokens["gate"] == 0 || obs.Tokens["gate"] > SessionShare {
		t.Errorf("token accounting: %d", obs.Tokens["gate"])
	}
	// Progress resets the counter.
	r.write("marker.txt", "CANARY-DONE\n")
	r.check(CheckOptions{})
	out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, true))
	if out.Decision != hookio.DecisionAllow {
		t.Errorf("all met should allow: %+v", out)
	}
	obs, _ = r.store.ReadObserved("sess1")
	if obs.GateBlocks != 0 {
		t.Errorf("blocks not reset: %d", obs.GateBlocks)
	}
	// Invalid contract fails closed.
	r.write(".saga/contract.md", "# T\nIN: a\n")
	out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, false))
	if out.Decision != hookio.DecisionBlock || !strings.Contains(out.Reason, "contract invalid") {
		t.Errorf("invalid: %+v", out)
	}
	// Contract tracked at HEAD but deleted.
	r.remove(".saga/contract.md")
	out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, false))
	if out.Decision != hookio.DecisionBlock || !strings.Contains(out.Reason, "contract deleted") {
		t.Errorf("deleted: %+v", out)
	}
}

func TestLayerPreAndPostTool(t *testing.T) {
	r := newRepo(t)
	layer := &Layer{Store: r.store}
	ctx := context.Background()
	r.write(".saga/contract.md", guardContract)
	r.write("tests/a.test.ts", "it('x', () => {});\n")
	r.commit("base")
	for _, cmd := range []string{"saga gate approve --gate G1", "saga   gate attest G2 --note x", "saga gate check --approve"} {
		out, _ := layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Bash", map[string]any{"command": cmd}, false))
		if out.Decision != hookio.DecisionDeny {
			t.Errorf("%q not denied: %+v", cmd, out)
		}
	}
	out, _ := layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Bash", map[string]any{"command": "saga gate check"}, false))
	if out.Decision != hookio.DecisionAllow {
		t.Errorf("plain check denied: %+v", out)
	}
	for _, p := range []string{r.root + "/.saga/evidence/g/G1.json", r.root + "/.saga/red/g/G1.json", r.root + "/.saga/request.md", r.root + "/docs/x.md"} {
		out, _ := layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Edit", map[string]any{"file_path": p}, false))
		if out.Decision != hookio.DecisionDeny {
			t.Errorf("edit of %s not denied: %+v", p, out)
		}
	}
	out, _ = layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Write", map[string]any{"file_path": r.root + "/src/new.ts"}, false))
	if out.Decision != hookio.DecisionAllow {
		t.Errorf("in-scope write denied: %+v", out)
	}
	// PostToolUse: a Bash edit that adds a skip marker is caught post hoc.
	r.write("tests/a.test.ts", "it.skip('x', () => {});\n")
	out, _ = layer.Run(ctx, hookIn(hookio.EventPostToolUse, "Bash", map[string]any{"command": "sed -i ..."}, false))
	if out.Decision != hookio.DecisionBlock || !strings.Contains(out.Reason, "G-SKIP tests/a.test.ts") || !strings.Contains(out.Reason, "WAIVE:") {
		t.Errorf("post-tool feedback: %+v", out)
	}
	obs, _ := r.store.ReadObserved("sess1")
	if obs.Tokens["gate"] == 0 {
		t.Error("post-tool feedback not charged to gate's share")
	}
	_ = store.Open
}

// hookInFinal is hookIn plus the final assistant message the Stop step
// reads for a terminal.
func hookInFinal(final string, active bool) *hookio.Input {
	in := hookIn(hookio.EventStop, "", nil, active)
	in.LastAssistantMessage = final
	return in
}

// TestStopReleaseEndsTheTurn (decision 1 of the 2026-09-06 brief): a
// release carries nothing in additionalContext, which on Stop would
// continue the conversation (harness-facts C11), and it is remembered,
// so every later Stop allows at once without counting.
func TestStopReleaseEndsTheTurn(t *testing.T) {
	r := newRepo(t)
	layer := &Layer{Store: r.store}
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	ctx := context.Background()
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	r.check(CheckOptions{Approve: true})

	var releasedAt = -1
	for i := 0; i < 10; i++ {
		out, err := layer.Run(ctx, hookIn(hookio.EventStop, "", nil, i > 0))
		if err != nil {
			t.Fatal(err)
		}
		if out.Decision == hookio.DecisionAllow {
			releasedAt = i
			if len(out.AdditionalContext) != 0 {
				t.Errorf("release carries additionalContext, which continues the turn: %v", out.AdditionalContext)
			}
			break
		}
	}
	if releasedAt < 0 {
		t.Fatal("never released")
	}
	obs, _ = r.store.ReadObserved("sess1")
	if !obs.GateReleased {
		t.Error("the release was not remembered for the session")
	}
	blocksAt := obs.GateBlocks
	// Every later Stop allows at once and counts nothing: continuing to
	// block after a release is what turned one handoff into a 29-turn loop.
	for i := 0; i < 3; i++ {
		out, err := layer.Run(ctx, hookIn(hookio.EventStop, "", nil, true))
		if err != nil {
			t.Fatal(err)
		}
		if out.Decision != hookio.DecisionAllow || len(out.AdditionalContext) != 0 {
			t.Fatalf("Stop %d after the release: %+v", i, out)
		}
	}
	obs, _ = r.store.ReadObserved("sess1")
	if obs.GateBlocks != blocksAt {
		t.Errorf("blocks moved after the release: %d then %d", blocksAt, obs.GateBlocks)
	}
}

// TestStopHonoursNotDoneTerminal (decision 2): an agent that says it
// cannot finish is the honest outcome the gate exists to protect. The
// Stop step allows on a NOT-DONE last line even with a gate unmet, and
// the terminal never accrues a block.
func TestStopHonoursNotDoneTerminal(t *testing.T) {
	r := newRepo(t)
	layer := &Layer{Store: r.store}
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	ctx := context.Background()
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	r.check(CheckOptions{Approve: true})

	// The gate blocks an ordinary stop on the same tree.
	out, err := layer.Run(ctx, hookIn(hookio.EventStop, "", nil, false))
	if err != nil || out.Decision != hookio.DecisionBlock {
		t.Fatalf("unmet contract should block: %+v %v", out, err)
	}
	before, _ := r.store.ReadObserved("sess1")

	for _, final := range []string{
		"I cannot do this: the two tests contradict each other.\n\nNOT-DONE",
		"NOT-DONE",
	} {
		out, err = layer.Run(ctx, hookInFinal(final, true))
		if err != nil {
			t.Fatal(err)
		}
		if out.Decision != hookio.DecisionAllow {
			t.Errorf("NOT-DONE terminal was held: %+v", out)
		}
		if len(out.AdditionalContext) != 0 {
			t.Errorf("terminal carries additionalContext: %v", out.AdditionalContext)
		}
	}
	after, _ := r.store.ReadObserved("sess1")
	if after.GateBlocks > before.GateBlocks {
		t.Errorf("a terminal accrued a block: %d then %d", before.GateBlocks, after.GateBlocks)
	}
}

// TestStopFirstBlockNamesTheWayOut (decision 3): the first block of a
// session tells the agent how to stop honestly, once, and the hint does
// not repeat on later blocks.
func TestStopFirstBlockNamesTheWayOut(t *testing.T) {
	r := newRepo(t)
	layer := &Layer{Store: r.store}
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	ctx := context.Background()
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	r.check(CheckOptions{Approve: true})

	out, err := layer.Run(ctx, hookIn(hookio.EventStop, "", nil, false))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Reason, "NOT-DONE") || !strings.Contains(out.Reason, "ABANDON:") {
		t.Errorf("the first block does not name the way out: %q", out.Reason)
	}
	seen := 0
	for i := 0; i < 3; i++ {
		out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, true))
		if strings.Contains(out.Reason, "ABANDON:") {
			seen++
		}
	}
	if seen != 0 {
		t.Errorf("the hint repeated on %d later blocks", seen)
	}
}

// TestScopeGuardIgnoresCaches (decision 4): a byte-code cache is not an
// edit, and a real out-of-scope edit still is.
func TestScopeGuardIgnoresCaches(t *testing.T) {
	for _, p := range []string{
		"__pycache__/versions.cpython-313.pyc",
		"versions/__pycache__/x.pyc",
		"src/a.pyc",
		"node_modules/left-pad/index.js",
		".pytest_cache/v/cache/lastfailed",
		".oracle-run/x.test.ts",
		".saga-oracle-out.txt",
	} {
		if !IsCachePath(p) {
			t.Errorf("%q should be treated as a cache, not an edit", p)
		}
	}
	for _, p := range []string{
		"versions.py",
		"src/index.ts",
		"docs/readme.md",
		"pycache/notreally.py",
		"src/pyc.py",
	} {
		if IsCachePath(p) {
			t.Errorf("%q is real work and must not be ignored", p)
		}
	}
}
