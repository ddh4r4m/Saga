package report

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/run"
)

// The three defects below were all found by reading the pilot archive of
// 2026-09-13 against its own report. None of them changed a number in
// that archive, which is not re-graded; the fixes apply to reports
// written from 2026-09-13 on.

// TestFalseDoneSurvivesARunWithoutAVerdict (pilot report defect 1): one
// run of 100 carried no claimed_done verdict, because it ended at its
// cost cap before the harness emitted a final message, and the secondary
// table nulled the whole column for the arm while section 3 computed the
// primary from the same rows. A run with no verdict is simply not a run
// that claimed done; it belongs outside the denominator, not in place of
// it.
func TestFalseDoneSurvivesARunWithoutAVerdict(t *testing.T) {
	rows := fixture("A", map[string][]bool{"a": {true, false, false, false}})
	// Three claims, one of them false; the fourth run has no verdict.
	rows[3].ClaimedDone = nil
	rows[3].ClaimedDoneReason = strp("trace claim event: final_message_unavailable")

	r := Build(manifest(4, "A"), "sha256:"+strings.Repeat("1", 64), rows, t.TempDir())
	a := r.Arms["A"]
	if a.FalseDone == nil {
		t.Fatalf("the column was nulled by one run without a verdict: %v", deref(a.FalseDoneReason))
	}
	// Three runs claimed done, two of them failed the oracle.
	if *a.FalseDone < 0.666 || *a.FalseDone > 0.667 {
		t.Errorf("false_done %v, want 2 of 3", *a.FalseDone)
	}
	if a.FalseDoneUnknown != 1 || a.FalseDoneTotal != 4 {
		t.Errorf("coverage counts: unknown %d of %d", a.FalseDoneUnknown, a.FalseDoneTotal)
	}
	// The coverage is printed beside the value rather than left implicit.
	if got := fmtFalseDone(a); !strings.Contains(got, "3 of 4 runs carry a verdict") {
		t.Errorf("rendered %q", got)
	}
	if md := r.Markdown(); !strings.Contains(md, "3 of 4 runs carry a verdict") {
		t.Errorf("the table does not print the coverage:\n%s", md)
	}
	// With every run carrying a verdict the value is bare, as before.
	clean := Build(manifest(4, "A"), "sha256:"+strings.Repeat("1", 64), fixture("A", map[string][]bool{"a": {true, false}}), t.TempDir())
	if got := fmtFalseDone(clean.Arms["A"]); strings.Contains(got, "carry a verdict") {
		t.Errorf("a fully covered arm should print a bare value, got %q", got)
	}
}

// TestThreatsRowReadsTheControlBlocks (pilot report defect 2): the
// threats table said "not applicable: no component blocks are
// configured in this runner" while section 2 of the same report listed
// four control blocks for arm A. The line was fixed text that read
// nothing; it now answers from the manifest and the rows.
func TestThreatsRowReadsTheControlBlocks(t *testing.T) {
	rows := fixture("A", map[string][]bool{"a": {true, true}})
	rows[0].BlockedReachAttempts = 2
	rows[1].BlockedReachAttempts = 1
	m := manifest(2, "A")
	m.Arms[0].BlocksInControl = []string{"path-shim:saga", "settings:no-mcp"}

	r := Build(m, "sha256:"+strings.Repeat("1", 64), rows, t.TempDir())
	note := controlReachNote(r)
	for _, want := range []string{"path-shim:saga", "settings:no-mcp", "3 reach attempts"} {
		if !strings.Contains(note, want) {
			t.Errorf("the threats row does not name %q: %q", want, note)
		}
	}
	if strings.Contains(note, "no component blocks are configured") {
		t.Errorf("the stale wording survived: %q", note)
	}
	// An arm the manifest gives no blocks still says so, which is the
	// only case the old text was right about.
	m2 := manifest(2, "A")
	r2 := Build(m2, "sha256:"+strings.Repeat("1", 64), fixture("A", map[string][]bool{"a": {true}}), t.TempDir())
	if !strings.Contains(controlReachNote(r2), "no control blocks") {
		t.Errorf("an unblocked arm: %q", controlReachNote(r2))
	}
}

// TestComponentExposureIsReadFromTheHookTraces (pilot report defect 3):
// kill-rule condition 2 asks whether the treatment arm ever reached the
// component. The pilot's report answered "not evaluated" although every
// one of its 100 arm B runs carried gate Stop events, because nothing
// read them.
func TestComponentExposureIsReadFromTheHookTraces(t *testing.T) {
	dir := t.TempDir()
	rows := fixture("B", map[string][]bool{"a": {true, true, true}})
	// Two of the three runs show the gate acting; the third is a run
	// whose hook trace holds only session and turn events.
	traces := []string{
		`{"type":"session"}` + "\n" + `{"type":"gate","body":{"kind":"stop"}}` + "\n",
		`{"type":"gate","body":{"kind":"claim"}}` + "\n",
		`{"type":"session"}` + "\n" + `{"type":"turn"}` + "\n",
	}
	for i, body := range traces {
		d := filepath.Join(dir, "a", "m", "replay", "B", strconv.Itoa(i+1))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "hook-trace.jsonl"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exposed, traced := ComponentExposure(dir, rows)
	if exposed != 2 || traced != 3 {
		t.Errorf("exposure %d of %d, want 2 of 3", exposed, traced)
	}

	// And an arm with no hook trace at all is the one case that stays
	// unevaluated, which is what the old text always said.
	e2, t2 := ComponentExposure(t.TempDir(), rows)
	if e2 != 0 || t2 != 0 {
		t.Errorf("an archive with no hook traces reported %d of %d", e2, t2)
	}
}

// TestComponentUsageNoteNamesTheShare: the kill-rule entry says what was
// measured, and only an arm without a hook trace reads "not evaluated".
func TestComponentUsageNoteNamesTheShare(t *testing.T) {
	r := newReport(manifest(1, "B"), "sha256:"+strings.Repeat("1", 64))
	r.armMeta = map[string]run.Arm{"B": {ID: "B", Components: []string{"gate"}}}
	r.Arms["B"] = &ArmStats{ExposedRuns: 5, TracedRuns: 5}
	note, _ := componentUsage(r, nil)["note"].(string)
	if !strings.Contains(note, "arm B exposed in 5 of 5 runs") {
		t.Errorf("note %q", note)
	}
	r.Arms["B"] = &ArmStats{}
	note, _ = componentUsage(r, nil)["note"].(string)
	if !strings.Contains(note, "not evaluated") {
		t.Errorf("an arm with no hook trace should stay unevaluated: %q", note)
	}
	// A bare arm is not a treatment arm and is never counted.
	r.armMeta = map[string]run.Arm{"A": {ID: "A", Components: []string{}}}
	r.Arms = map[string]*ArmStats{"A": {}}
	if arms, _ := componentUsage(r, nil)["arms"].(map[string]any); len(arms) != 0 {
		t.Errorf("a bare arm was counted: %v", arms)
	}
}
