// Package figure renders the one-glance figure for a bench archive from
// its runner-written `compare.json`. It reads only, and every number on
// the figure is the report's own: nothing is recomputed here, so the
// figure cannot disagree with the report beside it.
//
// The figure shows the pre-registered primary first and labels every
// other row as secondary, a cost condition, or exploratory, so a reader
// cannot take a scan number for the result. Ported from
// `scripts/pilot-figure.py`, which needed matplotlib and drew the
// confidence interval as prose; drawing Δ against a zero hairline and
// against the kill floor makes a null visible rather than merely
// stated, and a Go writer regenerates under `go test` the way
// `report.json` does.
//
// The output is deterministic: fixed sizes in px, no timestamps, no map
// iteration in the emitted order, numbers formatted with an explicit
// precision. Two renders of one archive are byte-equal, which is what
// the golden test holds.
package figure

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/report"
	"github.com/ddh4r4m/saga/internal/cli"
)

// The two categorical slots and the neutral ink, from the validated
// palette: blue is arm A (bare), orange arm B (with the component).
// They are the only two hues on the figure; everything else is ink,
// muted ink, a hairline grey and the surface.
const (
	colBlue   = "#2a78d6"
	colOrange = "#eb6834"
	colInk    = "#0b0b0b"
	colInk2   = "#52514e"
	colGrid   = "#e6e5e1"
	colSurf   = "#fcfcfb"
)

// Layout, all in px so the golden file is stable across machines.
const (
	width      = 1180
	labelX     = 48  // left margin, where row labels start
	dumbbellX  = 470 // the A-to-B panel
	dumbbellW  = 380
	deltaX     = 890 // the delta panel
	deltaW     = 240
	headerTop  = 92
	rowsTop    = 150
	rowH       = 104
	footerGap  = 26
	fontStack  = "'Helvetica Neue', Helvetica, Arial, 'DejaVu Sans', sans-serif"
	killFloor  = -0.05 // docs/12 section 8 condition 1
	minDeltaHW = 0.06  // half-width of the delta axis when the data is tighter
)

// row is one line of the figure.
type row struct {
	name string
	kind string
	// exploratory rows carry no delta panel and say so; a scan that was
	// never registered has no interim-look protection and must not be
	// drawn as though it had been tested.
	exploratory bool
	a, b        float64
	format      func(float64) string
	note        string
	// delta and ci are drawn only when hasDelta; killFloorLine adds the
	// second hairline on the primary row.
	hasDelta      bool
	delta         float64
	ciLo, ciHi    float64
	killFloorLine bool
	// ratio and bound draw the cost condition, which is a ratio against
	// a ceiling rather than a difference against zero. The report
	// carries no interval for tokens per solved task, so this panel is a
	// mark against a baseline and nothing more: inventing an interval
	// would be the one thing this package must never do.
	hasRatio     bool
	ratio, bound float64
}

// Render writes figure.svg for an archive directory and returns the
// path it wrote.
func Render(archive, out string) (string, error) {
	r, err := load(archive)
	if err != nil {
		return "", err
	}
	svg, err := build(r)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = filepath.Join(archive, "figure.svg")
	}
	if err := os.WriteFile(out, []byte(svg), 0o644); err != nil {
		return "", cli.Wrap(cli.ExitEnvironment, "figure", err)
	}
	return out, nil
}

// load reads the archive's compare.json through report.Report, so a
// schema change fails the build rather than the figure.
func load(archive string) (*report.Report, error) {
	raw, err := os.ReadFile(filepath.Join(archive, "compare.json"))
	if err != nil {
		return nil, cli.Errorf(cli.ExitUsage, "figure: %s carries no compare.json; the figure is drawn from a paired report", archive)
	}
	var r report.Report
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "figure: compare.json", err)
	}
	if r.Primary == nil {
		return nil, cli.Errorf(cli.ExitUsage, "figure: the report has no primary metric")
	}
	for _, arm := range []string{"A", "B"} {
		if r.Arms[arm] == nil {
			return nil, cli.Errorf(cli.ExitUsage, "figure: the report has no arm %s", arm)
		}
	}
	return &r, nil
}

func pct(x float64) string  { return fmt.Sprintf("%.1f%%", 100*x) }
func kilo(x float64) string { return fmt.Sprintf("%.0fk", x/1000) }

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Build renders the SVG for a loaded report. It is exported for tests
// that hold a report in hand.
func Build(r *report.Report) (string, error) { return build(r) }

func build(r *report.Report) (string, error) {
	A, B := r.Arms["A"], r.Arms["B"]
	prim := r.Primary

	// pass@1 comes from the `secondary` comparison, which carries A, B,
	// delta and the interval together; the `negative` entry of the same
	// metric carries the interval but not the arm values.
	var pass *report.Comparison
	for i := range r.Secondary {
		if r.Secondary[i].Metric == "pass_at_1" {
			pass = &r.Secondary[i]
			break
		}
	}

	rows := []row{{
		name: "False “done” claims", kind: "primary, pre-registered",
		a: deref(prim.A), b: deref(prim.B), format: pct,
		note: fmt.Sprintf("Δ %+.1f pp, 95%% CI %+.1f to %+.1f pp. Null: the −5 pp floor was not cleared.",
			100*deref(prim.Delta), 100*ciAt(prim.CI95, 0), 100*ciAt(prim.CI95, 1)),
		hasDelta: true, delta: deref(prim.Delta),
		ciLo: ciAt(prim.CI95, 0), ciHi: ciAt(prim.CI95, 1), killFloorLine: true,
	}}
	if pass != nil {
		rows = append(rows, row{
			name: "Tasks solved (pass@1)", kind: "secondary, pre-registered",
			a: deref(pass.A), b: deref(pass.B), format: pct,
			note: fmt.Sprintf("Δ %+.1f pp, CI %+.1f to %+.1f pp. Not lowered.",
				100*deref(pass.Delta), 100*ciAt(pass.CI95, 0), 100*ciAt(pass.CI95, 1)),
			hasDelta: true, delta: deref(pass.Delta),
			ciLo: ciAt(pass.CI95, 0), ciHi: ciAt(pass.CI95, 1),
		})
	}
	ta, tb := r.PerSolved["A"].Tokens, r.PerSolved["B"].Tokens
	if ta != nil && tb != nil && *ta > 0 {
		ratio := *tb / *ta
		rows = append(rows, row{
			name: "Tokens per solved task", kind: "cost condition, pre-registered",
			a: *ta, b: *tb, format: kilo,
			note:     fmt.Sprintf("Ratio %.2f, above the 1.10 bound. Cost condition failed.", ratio),
			hasRatio: true, ratio: ratio, bound: 1.10,
		})
	}
	rows = append(rows,
		row{
			name: "Runs with out-of-scope edits", kind: "exploratory scan, not a result",
			exploratory: true, a: A.ScopeViolationRate, b: B.ScopeViolationRate, format: pct,
			note: "not tested, no interim-look protection. The gated agent edited outside the task's files far less often.",
		},
		row{
			name: "Passing runs that edited a visible test file", kind: "exploratory scan, not a result",
			exploratory: true, a: deref(A.CheatRate), b: deref(B.CheatRate), format: pct,
			note: "not tested, no interim-look protection. The scan calls this the cheat rate; on the pilot all five were strengthened tests, unseen by the clean-checkout oracle (docs/15).",
		},
	)

	var s strings.Builder
	height := rowsTop + len(rows)*rowH + 140
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="%s">`+"\n",
		width, height, width, height, fontStack)
	fmt.Fprintf(&s, `<rect width="%d" height="%d" fill="%s"/>`+"\n", width, height, colSurf)

	// Header: the question, the cell, and the two colour slots named.
	text(&s, labelX, 46, 25, "700", colInk, "Does a Stop gate make a coding agent honest about “done”? A pilot.")
	text(&s, labelX, headerTop-16, 14, "400", colInk2,
		fmt.Sprintf("%d tasks × %d runs × 2 arms on Claude Code, model %s.  Arm A runs bare; arm B runs with the Saga gate hook.  Pre-registered before any run.",
			A.Tasks, A.K, A.Model))
	fmt.Fprintf(&s, `<circle cx="%d" cy="%d" r="6" fill="%s"/>`+"\n", labelX+5, headerTop+14, colBlue)
	text(&s, labelX+20, headerTop+19, 14, "400", colInk, "A, bare")
	fmt.Fprintf(&s, `<circle cx="%d" cy="%d" r="6" fill="%s"/>`+"\n", labelX+115, headerTop+14, colOrange)
	text(&s, labelX+130, headerTop+19, 14, "400", colInk, "B, with the gate")

	for i, rw := range rows {
		y := rowsTop + i*rowH
		drawRow(&s, y, rw)
		if i < len(rows)-1 {
			line(&s, labelX, y+rowH-18, width-labelX, y+rowH-18, colGrid, 1, "")
		}
	}

	// Footer: what holds, what does not, and where every number came from.
	fy := rowsTop + len(rows)*rowH + footerGap
	prereg := "none"
	if r.PreregistrationSHA256 != nil {
		prereg = strings.TrimPrefix(*r.PreregistrationSHA256, "sha256:")
		if len(prereg) > 16 {
			prereg = prereg[:16]
		}
	}
	for j, l := range []string{
		"What holds: the gate ran in every arm B run, no run approved its own baselines, hidden oracles graded in a clean checkout, and the harness's own cost figure reconciles.",
		"What does not: the pre-registered effect. By the kill rule the experiment ended here and this pilot is the result. Exploratory rows are a hint for a differently registered experiment, not evidence.",
		fmt.Sprintf("Source: the runner-written report in this archive (pre-registration sha256 %s…), %d bootstrap resamples over tasks. Rendered by `saga bench figure`.", prereg, r.BootstrapResamples),
	} {
		text(&s, labelX, fy+j*20, 12, "400", colInk2, l)
	}
	s.WriteString("</svg>\n")
	return s.String(), nil
}

// drawRow writes one row: the label block, the A-to-B dumbbell, and the
// delta panel when the row is one that was tested.
func drawRow(s *strings.Builder, y int, rw row) {
	kindCol, kindStyle := colInk2, ""
	if rw.exploratory {
		kindCol, kindStyle = colOrange, ` font-style="italic"`
	}
	textStyled(s, labelX, y+16, 16, "700", colInk, rw.name, "")
	textStyled(s, labelX, y+38, 12, "400", kindCol, rw.kind, kindStyle)
	// Three lines of note at most; the report carries the rest. The
	// first render cut the last row mid-sentence at two.
	for j, l := range wrap(rw.note, 64) {
		if j > 2 {
			break
		}
		text(s, labelX, y+58+j*15, 12, "400", colInk2, l)
	}

	// Dumbbell: A and B on a shared scale from zero.
	hi := math.Max(rw.a, rw.b) * 1.25
	if hi <= 0 {
		hi = 1
	}
	mid := y + 34
	xa := dumbbellX + int(rw.a/hi*float64(dumbbellW))
	xb := dumbbellX + int(rw.b/hi*float64(dumbbellW))
	line(s, dumbbellX, mid, dumbbellX+dumbbellW, mid, colGrid, 1, "")
	line(s, xa, mid, xb, mid, colInk2, 2, ` stroke-linecap="round"`)
	dot(s, xa, mid, 7, colBlue)
	dot(s, xb, mid, 7, colOrange)
	// The larger value's label goes above, the smaller below, so the two
	// never collide however close the dots are.
	aAbove := rw.a >= rw.b
	anchor(s, xa, mid, rw.format(rw.a), aAbove)
	anchor(s, xb, mid, rw.format(rw.b), !aAbove)

	if rw.hasRatio {
		// The cost condition: the ratio as a dot against its ceiling, on
		// its own axis. H2 is a ratio and the report carries no interval
		// for it, so there is no CI line here and none is implied.
		lo, hiR := math.Min(rw.ratio, rw.bound), math.Max(rw.ratio, rw.bound)
		pad := math.Max((hiR-lo)*0.9, 0.08)
		lo, hiR = lo-pad, hiR+pad
		at := func(v float64) int { return deltaX + int((v-lo)/(hiR-lo)*float64(deltaW)) }
		line(s, deltaX, mid, deltaX+deltaW, mid, colGrid, 1, "")
		line(s, at(rw.bound), y+18, at(rw.bound), y+50, colOrange, 1, ` stroke-dasharray="3 3"`)
		textStyled(s, at(rw.bound)-18, y+64, 10, "400", colOrange, "1.10 bound", "")
		dot(s, at(rw.ratio), mid, 5, colInk)
		text(s, deltaX, y+16, 11, "400", colInk2, "ratio, no interval reported")
		return
	}
	if !rw.hasDelta {
		return
	}
	// Delta panel: Δ as a dot with its interval, against a zero hairline,
	// and on the primary row against the kill floor as well.
	lo, hiD := rw.ciLo, rw.ciHi
	lo = math.Min(lo, math.Min(rw.delta, 0))
	hiD = math.Max(hiD, math.Max(rw.delta, 0))
	if rw.killFloorLine {
		lo = math.Min(lo, killFloor)
	}
	pad := math.Max((hiD-lo)*0.18, minDeltaHW*0.2)
	lo, hiD = lo-pad, hiD+pad
	at := func(v float64) int {
		return deltaX + int((v-lo)/(hiD-lo)*float64(deltaW))
	}
	line(s, at(0), y+18, at(0), y+50, colGrid, 1, "")
	text(s, at(0)-3, y+64, 10, "400", colInk2, "0")
	if rw.killFloorLine {
		line(s, at(killFloor), y+18, at(killFloor), y+50, colOrange, 1, ` stroke-dasharray="3 3"`)
		textStyled(s, at(killFloor)-26, y+64, 10, "400", colOrange, "kill floor", "")
	}
	line(s, at(lo+pad), mid, at(hiD-pad), mid, colGrid, 1, "")
	fmt.Fprintf(s, `<line class="ci" x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="2" stroke-linecap="round"/>`+"\n",
		at(rw.ciLo), mid, at(rw.ciHi), mid, colInk2)
	dot(s, at(rw.delta), mid, 5, colInk)
	text(s, deltaX, y+16, 11, "400", colInk2, "Δ with 95% CI")
}

// anchor places a value label above or below its dot.
func anchor(s *strings.Builder, x, y int, label string, above bool) {
	dy := 18
	if above {
		dy = -12
	}
	fmt.Fprintf(s, `<text x="%d" y="%d" font-size="13" fill="%s" text-anchor="middle">%s</text>`+"\n",
		x, y+dy, colInk, esc(label))
}

func text(s *strings.Builder, x, y, size int, weight, fill, body string) {
	textStyled(s, x, y, size, weight, fill, body, "")
}

func textStyled(s *strings.Builder, x, y, size int, weight, fill, body, style string) {
	fmt.Fprintf(s, `<text x="%d" y="%d" font-size="%d" font-weight="%s" fill="%s"%s>%s</text>`+"\n",
		x, y, size, weight, fill, style, esc(body))
}

func line(s *strings.Builder, x1, y1, x2, y2 int, stroke string, w int, extra string) {
	fmt.Fprintf(s, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="%d"%s/>`+"\n",
		x1, y1, x2, y2, stroke, w, extra)
}

func dot(s *strings.Builder, x, y, r int, fill string) {
	fmt.Fprintf(s, `<circle cx="%d" cy="%d" r="%d" fill="%s" stroke="%s" stroke-width="2"/>`+"\n",
		x, y, r, fill, colSurf)
}

// esc escapes the five XML entities. Every string on the figure comes
// from the archive, so none of it is trusted to be markup-safe.
func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// wrap breaks a note into lines of at most n characters on word
// boundaries, deterministically.
func wrap(s string, n int) []string {
	words := strings.Fields(s)
	var out []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= n:
			cur += " " + w
		default:
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func ciAt(ci []*float64, i int) float64 {
	if i < len(ci) && ci[i] != nil {
		return *ci[i]
	}
	return 0
}

// RenderPerTask is defined in pertask.go.

// sortedTasks is used by the per-task figure; kept here so both figures
// share one deterministic ordering rule.
func sortedTasks(a, b []report.PerTask) []string {
	byTask := map[string]float64{}
	for _, t := range a {
		byTask[t.Task] -= t.MedianTokens
	}
	for _, t := range b {
		byTask[t.Task] += t.MedianTokens
	}
	names := make([]string, 0, len(byTask))
	for n := range byTask {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if byTask[names[i]] != byTask[names[j]] {
			return byTask[names[i]] < byTask[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}
