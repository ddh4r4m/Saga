package figure

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/report"
	"github.com/ddh4r4m/saga/internal/cli"
)

// The per-task figure answers the one question the report's tables
// hide: where arm B's extra tokens come from. The arm-level figure says
// the ratio is 1.17 and the cost condition failed; this one says on
// which tasks, and shows that the answer is not "all of them".
//
// It is descriptive and says so. There is no test here and none is
// implied: per-task medians at K=5 are five runs each, the report
// computes no interval for them, and the ordering is a presentation
// choice rather than a ranking anyone registered.

// Per-task layout, px, so the golden file is stable.
const (
	ptWidth   = 1180
	ptRowsTop = 150
	ptRowH    = 30
	ptLabelX  = 48
	ptBarX    = 330
	ptBarW    = 600
	ptTextX   = ptBarX + ptBarW + 24
)

// RenderPerTask writes figure-per-task.svg for an archive directory.
func RenderPerTask(archive, out string) (string, error) {
	r, err := load(archive)
	if err != nil {
		return "", err
	}
	svg, err := BuildPerTask(r)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = filepath.Join(archive, "figure-per-task.svg")
	}
	if err := os.WriteFile(out, []byte(svg), 0o644); err != nil {
		return "", cli.Wrap(cli.ExitEnvironment, "figure", err)
	}
	return out, nil
}

// BuildPerTask renders the per-task SVG for a loaded report.
func BuildPerTask(r *report.Report) (string, error) {
	A, B := r.Arms["A"], r.Arms["B"]
	if len(A.PerTask) == 0 || len(B.PerTask) == 0 {
		return "", cli.Errorf(cli.ExitUsage, "figure: the report carries no per-task rows")
	}
	a := byTask(A.PerTask)
	b := byTask(B.PerTask)
	names := sortedTasks(A.PerTask, B.PerTask)

	// Tasks whose pass rate moved within the arm are marked hollow: a
	// median over five runs that were not all the same outcome is a
	// weaker summary than one over five that were.
	unstable := map[string]bool{}
	if raw, ok := r.Instability["unstable_tasks"].([]any); ok {
		for _, t := range raw {
			if s, ok := t.(string); ok {
				unstable[s] = true
			}
		}
	}

	hi := 0.0
	for _, n := range names {
		hi = math.Max(hi, math.Max(a[n].MedianTokens, b[n].MedianTokens))
	}
	if hi <= 0 {
		hi = 1
	}
	hi *= 1.06

	var s strings.Builder
	height := ptRowsTop + len(names)*ptRowH + 112
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="%s">`+"\n",
		ptWidth, height, ptWidth, height, fontStack)
	fmt.Fprintf(&s, `<rect width="%d" height="%d" fill="%s"/>`+"\n", ptWidth, height, colSurf)

	text(&s, ptLabelX, 46, 22, "700", colInk, "Median tokens per task, arm A against arm B")
	text(&s, ptLabelX, 72, 13, "400", colInk2,
		fmt.Sprintf("%d tasks, %d runs each, model %s. Sorted by B minus A, so the tasks the gate cost most are at the foot.", len(names), A.K, A.Model))
	fmt.Fprintf(&s, `<circle cx="%d" cy="%d" r="6" fill="%s"/>`+"\n", ptLabelX+5, 96, colBlue)
	text(&s, ptLabelX+20, 101, 13, "400", colInk, "A, bare")
	fmt.Fprintf(&s, `<circle cx="%d" cy="%d" r="6" fill="%s"/>`+"\n", ptLabelX+100, 96, colOrange)
	text(&s, ptLabelX+115, 101, 13, "400", colInk, "B, with the gate")
	fmt.Fprintf(&s, `<circle cx="%d" cy="%d" r="6" fill="%s" stroke="%s" stroke-width="2"/>`+"\n", ptLabelX+265, 96, colSurf, colInk2)
	text(&s, ptLabelX+280, 101, 13, "400", colInk2, "hollow: pass rate not the same on every run of the arm")
	text(&s, ptTextX, 101, 11, "400", colInk2, "passes / runs")

	for i, n := range names {
		y := ptRowsTop + i*ptRowH
		ta, tb := a[n], b[n]
		xa := ptBarX + int(ta.MedianTokens/hi*float64(ptBarW))
		xb := ptBarX + int(tb.MedianTokens/hi*float64(ptBarW))
		text(&s, ptLabelX, y+4, 12, "400", colInk, n)
		line(&s, ptBarX, y, ptBarX+ptBarW, y, colGrid, 1, "")
		line(&s, xa, y, xb, y, colInk2, 2, ` stroke-linecap="round"`)
		// When the two medians are within a few pixels the second dot
		// draws over the first and the row reads as a single value, or
		// worse as a missing one. A split disc keeps both colours visible
		// at the one position, which is what "no difference" looks like.
		if abs(xa-xb) < 6 {
			split(&s, (xa+xb)/2, y, unstable[n])
		} else {
			mark(&s, xa, y, colBlue, unstable[n])
			mark(&s, xb, y, colOrange, unstable[n])
		}
		fmt.Fprintf(&s, `<text x="%d" y="%d" font-size="11" fill="%s" font-family="ui-monospace, SFMono-Regular, Menlo, monospace">%s</text>`+"\n",
			ptTextX, y+4, colInk2, esc(fmt.Sprintf("A %d/%d   B %d/%d", ta.C, ta.N, tb.C, tb.N)))
	}

	fy := ptRowsTop + len(names)*ptRowH + 30
	for j, l := range []string{
		"descriptive; medians per task; no test computed",
		"The arm-level figure says the ratio is 1.17 and the cost condition failed. This one says the extra tokens are not spread evenly: most tasks sit close to the diagonal and a few carry the difference.",
		"Source: the runner-written report in this archive. Rendered by `saga bench figure --per-task`.",
	} {
		w, col := "400", colInk2
		if j == 0 {
			w, col = "700", colInk
		}
		text(&s, ptLabelX, fy+j*20, 12, w, col, l)
	}
	s.WriteString("</svg>\n")
	return s.String(), nil
}

// split draws one disc in both colours, left half A and right half B,
// for a task whose two medians land on the same pixel.
func split(s *strings.Builder, x, y int, hollow bool) {
	const r = 5
	fillA, fillB := colBlue, colOrange
	if hollow {
		fillA, fillB = colSurf, colSurf
	}
	fmt.Fprintf(s, `<path d="M %d %d A %d %d 0 0 0 %d %d Z" fill="%s" stroke="%s" stroke-width="2"/>`+"\n",
		x, y-r, r, r, x, y+r, fillA, colBlue)
	fmt.Fprintf(s, `<path d="M %d %d A %d %d 0 0 1 %d %d Z" fill="%s" stroke="%s" stroke-width="2"/>`+"\n",
		x, y-r, r, r, x, y+r, fillB, colOrange)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// mark draws a task's dot, hollow when the arm's pass rate moved.
func mark(s *strings.Builder, x, y int, fill string, hollow bool) {
	if hollow {
		fmt.Fprintf(s, `<circle cx="%d" cy="%d" r="5" fill="%s" stroke="%s" stroke-width="2"/>`+"\n", x, y, colSurf, fill)
		return
	}
	dot(s, x, y, 5, fill)
}

func byTask(rows []report.PerTask) map[string]report.PerTask {
	m := make(map[string]report.PerTask, len(rows))
	for _, r := range rows {
		m[r.Task] = r
	}
	return m
}
