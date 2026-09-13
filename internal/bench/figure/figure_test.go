package figure

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden SVG from the pilot archive")

const pilot = "../../../bench/results/pilot-2026-09-13"

func render(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(pilot, "compare.json")); err != nil {
		t.Skipf("no pilot archive: %v", err)
	}
	r, err := load(pilot)
	if err != nil {
		t.Fatal(err)
	}
	svg, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	return svg
}

// TestGoldenSVG holds the whole figure byte for byte against the pilot
// archive's own compare.json. The figure is a published artefact in a
// closed experiment's archive, so a change to it should be a change
// someone made on purpose; `go test ./internal/bench/figure -update`
// rewrites it.
func TestGoldenSVG(t *testing.T) {
	got := render(t)
	golden := filepath.Join("testdata", "pilot.golden.svg")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("golden rewritten")
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("the figure changed; run with -update if that was intended.\nfirst difference at byte %d", firstDiff(got, string(want)))
	}
}

func firstDiff(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// TestDeterministic: two renders of one archive are byte-equal. The
// figure carries no timestamp and iterates no map in its output order,
// so it regenerates the way report.json does (README week-6 exit
// criterion).
func TestDeterministic(t *testing.T) {
	if render(t) != render(t) {
		t.Error("two renders of the same archive differ")
	}
}

// TestExploratoryRowsAreNotDrawnAsTested: a scan nobody registered has
// no interim-look protection, so it gets no interval and says so. This
// is the honesty guardrail the figure exists for: a reader must not be
// able to take the scope-violation row for the result.
func TestExploratoryRowsAreNotDrawnAsTested(t *testing.T) {
	svg := render(t)
	rows := strings.Split(svg, "exploratory scan, not a result")
	if len(rows) != 3 {
		t.Fatalf("%d exploratory rows, want 2", len(rows)-1)
	}
	// Everything after the first exploratory label is exploratory
	// territory: no CI line may appear there.
	tail := "exploratory scan, not a result" + strings.Join(rows[1:], "exploratory scan, not a result")
	if strings.Contains(tail, `class="ci"`) {
		t.Error("an exploratory row carries a confidence interval")
	}
	if n := strings.Count(svg, "not tested, no interim-look protection"); n != 2 {
		t.Errorf("%d rows say they were not tested, want 2", n)
	}
	if !strings.Contains(svg, "not a result") {
		t.Error("the figure never says `not a result`")
	}
}

// TestPrimaryRowDrawsTheNull: the point of the port. The primary row
// carries the kill floor as a second hairline, so a reader sees that
// the interval did not clear it rather than reading that it did not.
func TestPrimaryRowDrawsTheNull(t *testing.T) {
	svg := render(t)
	head := svg
	if i := strings.Index(svg, "secondary, pre-registered"); i > 0 {
		head = svg[:i]
	}
	for _, want := range []string{"primary, pre-registered", "kill floor", `class="ci"`, "Δ with 95% CI"} {
		if !strings.Contains(head, want) {
			t.Errorf("the primary row has no %q", want)
		}
	}
	if strings.Count(svg, "kill floor") != 1 {
		t.Error("the kill floor is drawn on more than the primary row")
	}
	// Two tested rows, so two intervals, and no more: the cost condition
	// is a ratio with no interval in the report and must not get one.
	if n := strings.Count(svg, `class="ci"`); n != 2 {
		t.Errorf("%d confidence intervals, want 2", n)
	}
	if !strings.Contains(svg, "ratio, no interval reported") {
		t.Error("the cost-condition panel does not say it has no interval")
	}
}

// TestEveryNumberIsEscaped: every string on the figure comes from an
// archive, so none is trusted to be markup-safe.
func TestEveryNumberIsEscaped(t *testing.T) {
	if got := esc(`a<b>&"'`); got != `a&lt;b&gt;&amp;&quot;&apos;` {
		t.Errorf("esc: %q", got)
	}
	svg := render(t)
	if strings.Count(svg, "<svg") != 1 || !strings.HasSuffix(svg, "</svg>\n") {
		t.Error("the document is not one well-formed svg element")
	}
}

// TestREADMEEmbedsTheFigureThatExists: the README points at the file the
// command writes, and that file is in the tree.
func TestREADMEEmbedsTheFigureThatExists(t *testing.T) {
	raw, err := os.ReadFile("../../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	const want = "bench/results/pilot-2026-09-13/figure.svg"
	if !strings.Contains(string(raw), want) {
		t.Fatalf("the README does not embed %s", want)
	}
	if strings.Contains(string(raw), "figure.png") {
		t.Error("the README still points at the removed PNG")
	}
	if _, err := os.Stat(filepath.Join("../../..", want)); err != nil {
		t.Errorf("the README embeds a figure that is not in the tree: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
