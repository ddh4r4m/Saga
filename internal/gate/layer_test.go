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
	// released with HANDOFF REQUIRED (below Claude Code's cap of 8).
	var released bool
	for i := 0; i < 8; i++ {
		out, _ = layer.Run(ctx, hookIn(hookio.EventStop, "", nil, true))
		if out.Decision == hookio.DecisionAllow {
			released = true
			if i != 5 || len(out.AdditionalContext) == 0 || !strings.Contains(out.AdditionalContext[0], "HANDOFF REQUIRED") {
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
