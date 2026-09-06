package gate

import (
	"context"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/hookio"
)

// unprovenContract has one plainly met gate, one whose red is required
// by declaration (RED: mutation, which nothing here can produce), and
// one whose baseline red would be rejected. All three CHECK lines pass.
const unprovenContract = `# Contract: three gates

IN: **
OUT: nothing/**

- [ ] G1: the marker says done
    CHECK: cat marker.txt
    EXPECT: CANARY-DONE
    RED: none
- [ ] G2: the marker is still readable
    CHECK: cat marker.txt
    EXPECT: CANARY-DONE
    RED: mutation
- [ ] G3: the file exists
    CHECK: test -f marker.txt && echo present
    EXPECT: present
    RED: mutation
`

// requireRed writes the workspace config and commits it, because the
// gate reads config from the base commit and not from the working tree.
func requireRed(t *testing.T, r *repo, on bool) {
	t.Helper()
	v := "false"
	if on {
		v = "true"
	}
	r.write(".saga/config.toml", "[gate]\nrequire_red = "+v+"\n")
	// The gate reads config from the base commit, so it has to be there;
	// .saga is gitignored, hence the force.
	r.git("add", "-f", ".saga/config.toml")
	r.git("commit", "-q", "-m", "config")
}

// TestUnprovenNeverBlocksStopWithRequireRedOff (2026-09-06 dev run
// finding 1): three arm B runs spent four minutes each trying to produce
// a red proof they had no way to produce, because the Stop step counted
// an unproven gate as unmet. With require_red off, a gate that is met
// without a red is met: it does not block, and it is reported as
// met-unproven so the archive still says it passed without a red.
func TestUnprovenNeverBlocksStopWithRequireRedOff(t *testing.T) {
	r := newRepo(t)
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	r.write("marker.txt", "CANARY-DONE\n")
	r.write(".saga/contract.md", unprovenContract)
	r.commit("base")
	requireRed(t, r, false)
	r.check(CheckOptions{Approve: true})

	layer := &Layer{Store: r.store}
	out, err := layer.Run(context.Background(), hookIn(hookio.EventStop, "", nil, false))
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != hookio.DecisionAllow {
		t.Fatalf("an unproven gate blocked Stop under require_red = false: %+v", out)
	}
	rep := r.status()
	// The states are visible: met for the declared-none gate, and
	// met-unproven for the two whose red is required by declaration.
	var unproven int
	for _, gs := range rep.Gates {
		switch gs.ReportedState() {
		case StateMetUnproven:
			unproven++
		case StateMet:
		default:
			t.Errorf("gate %s is %s, want met or met-unproven", gs.ID, gs.ReportedState())
		}
	}
	if unproven == 0 {
		t.Error("no gate reported met-unproven; the state is invisible again")
	}
	if len(rep.UnprovenIDs()) != unproven {
		t.Errorf("UnprovenIDs %v, want %d entries", rep.UnprovenIDs(), unproven)
	}
	if rep.Exit != 0 {
		t.Errorf("exit %d under require_red = false, want 0", rep.Exit)
	}
}

// TestUnprovenBlocksStopWithRequireRedOn: the product default is
// unchanged. A gate whose red is required and absent still blocks, and
// the message still names it.
func TestUnprovenBlocksStopWithRequireRedOn(t *testing.T) {
	r := newRepo(t)
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	r.write("marker.txt", "CANARY-DONE\n")
	r.write(".saga/contract.md", unprovenContract)
	r.commit("base")
	requireRed(t, r, true)
	r.check(CheckOptions{Approve: true})

	layer := &Layer{Store: r.store}
	out, _ := layer.Run(context.Background(), hookIn(hookio.EventStop, "", nil, false))
	if out.Decision != hookio.DecisionBlock {
		t.Fatalf("require_red = true must still block an unproven gate: %+v", out)
	}
	for _, id := range []string{"G2", "G3"} {
		if !strings.Contains(out.Reason, id) {
			t.Errorf("the block does not name %s: %q", id, out.Reason)
		}
	}
}

// TestStopMessageNamesOnlyBlockingGates: a mixed contract names the gate
// the agent can act on and not the one it cannot, and the advisory
// coverage note is not carried on a block that has a real reason.
func TestStopMessageNamesOnlyBlockingGates(t *testing.T) {
	r := newRepo(t)
	obs, _ := r.store.ReadObserved("sess1")
	obs.MaskSalt = "salt"
	_ = r.store.WriteObserved(obs)
	// G1 fails, G2 and G3 pass without a red.
	r.write("marker.txt", "not-yet\n")
	r.write(".saga/contract.md", unprovenContract)
	r.commit("base")
	requireRed(t, r, false)
	r.check(CheckOptions{Approve: true})

	layer := &Layer{Store: r.store}
	out, _ := layer.Run(context.Background(), hookIn(hookio.EventStop, "", nil, false))
	if out.Decision != hookio.DecisionBlock {
		t.Fatalf("an unmet gate must block: %+v", out)
	}
	if !strings.Contains(out.Reason, "G1") {
		t.Errorf("the block does not name the unmet gate: %q", out.Reason)
	}
	for _, id := range []string{"G2(unproven)", "G3(unproven)"} {
		if strings.Contains(out.Reason, id) {
			t.Errorf("the block names a gate the agent cannot clear: %q", out.Reason)
		}
	}
	if strings.Contains(out.Reason, "uncovered") {
		t.Errorf("the advisory coverage note is carried on a block with a real reason: %q", out.Reason)
	}
}
