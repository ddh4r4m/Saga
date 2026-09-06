package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/run"
)

// withOverhead attaches a measured block to every row of an arm.
func withOverhead(rows []run.Row, invocations, p50, p95, injected int, share float64) []run.Row {
	out := make([]run.Row, 0, len(rows))
	for _, r := range rows {
		s := share
		r.Overhead = &run.Overhead{
			Invocations: invocations,
			ByEvent: map[string]run.EventOverhead{
				"PreToolUse": {N: invocations - 1, P50MS: p50, P95MS: p95, MaxMS: p95, TotalMS: p50 * (invocations - 1)},
				"Stop":       {N: 1, P50MS: p95, P95MS: p95, MaxMS: p95, TotalMS: p95},
			},
			WallMS: p50*(invocations-1) + p95, WallShare: &s, InjectedTokensEst: injected,
		}
		out = append(out, r)
	}
	return out
}

// TestReportOverheadBesideThePrimary (docs/12 commitment 7): the table is
// in section 3, next to the primary, and carries the per-event p50 and
// p95, the wall share, the injected tokens and the paired delta.
func TestReportOverheadBesideThePrimary(t *testing.T) {
	a := fixture("A", map[string][]bool{"x": {true, false}, "y": {true, true}})
	b := fixture("B", map[string][]bool{"x": {true, true}, "y": {true, true}})
	a = withOverhead(a, 5, 3, 9, 0, 0.002)
	b = withOverhead(b, 11, 14, 62, 480, 0.031)

	rep, err := Compare(
		&Archive{Dir: t.TempDir(), Manifest: manifest(2, "A"), Hash: "sha256:" + strings.Repeat("1", 64), Rows: a},
		&Archive{Dir: t.TempDir(), Manifest: manifest(2, "B"), Hash: "sha256:" + strings.Repeat("2", 64), Rows: b}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("report schema: %v", err)
	}
	armA, armB := rep.Arms["A"], rep.Arms["B"]
	if armA.Overhead == nil || armB.Overhead == nil {
		t.Fatalf("no overhead: %+v %+v", armA.OverheadReason, armB.OverheadReason)
	}
	if armB.Overhead.MedianInvocations != 11 || armA.Overhead.MedianInvocations != 5 {
		t.Errorf("median invocations %v / %v", armA.Overhead.MedianInvocations, armB.Overhead.MedianInvocations)
	}
	if e := armB.Overhead.ByEvent["PreToolUse"]; e.P50MS != 14 || e.P95MS != 62 {
		t.Errorf("arm B PreToolUse %+v", e)
	}
	if armB.Overhead.MedianInjectedTokensEst != 480 || armA.Overhead.MedianInjectedTokensEst != 0 {
		t.Errorf("injected tokens %v / %v", armA.Overhead.MedianInjectedTokensEst, armB.Overhead.MedianInjectedTokensEst)
	}

	md := rep.Markdown()
	primary := strings.Index(md, "## 3. Primary outcome")
	table := strings.Index(md, "Measured hook overhead")
	secondary := strings.Index(md, "## 4. Secondary outcomes")
	if primary < 0 || table < 0 || secondary < 0 || !(primary < table && table < secondary) {
		t.Fatalf("the overhead table is not between the primary and section 4: %d %d %d", primary, table, secondary)
	}
	for _, want := range []string{"| PreToolUse | 40 | 14 | 62 | 62 |", "B minus A: injected tokens +480", "hook wall share +0.0290"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md[table:secondary])
		}
	}
}

// TestReportOverheadNullOnOldArchive: the archive committed before hook
// wall time was recorded reports null with the reason, and still
// validates. Nothing is back-filled.
func TestReportOverheadNullOnOldArchive(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "bench", "results", "smoke-2026-09-06-2", "A")
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Skipf("archive not present: %v", err)
	}
	m, hash, rows, err := run.ReadArchive(dir)
	if err != nil {
		t.Fatal(err)
	}
	rep := Build(m, hash, rows, dir)
	if err := rep.Validate(); err != nil {
		t.Fatalf("report schema on a pre-recording archive: %v", err)
	}
	for id, a := range rep.Arms {
		if a.Overhead != nil {
			t.Errorf("arm %s invented overhead for an archive that recorded none: %+v", id, a.Overhead)
		}
		if a.OverheadReason == nil || !strings.Contains(*a.OverheadReason, "recorded") {
			t.Errorf("arm %s reason %v", id, a.OverheadReason)
		}
	}
	md := rep.Markdown()
	if !strings.Contains(md, "Measured hook overhead") {
		t.Error("the table is absent instead of saying it has nothing to show")
	}
	if !strings.Contains(md, "recorded") {
		t.Errorf("the table does not say why it is empty:\n%s", md)
	}
}
