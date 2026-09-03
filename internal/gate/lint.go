package gate

import (
	"fmt"
	"regexp"
	"strings"
)

// LintWarning is one section 2.6 warning; exit 0 unless --strict.
type LintWarning struct {
	Rule string `json:"rule"`
	Gate string `json:"gate,omitempty"`
	Msg  string `json:"msg"`
}

// Lint rule ids.
const (
	LintPathRegex = "L-PATH-REGEX"
	LintVocab     = "L-VOCAB"
	LintActivity  = "L-ACTIVITY"
	LintNumber    = "L-NUMBER"
	LintManual    = "L-MANUAL"
	LintRedNone   = "L-RED-NONE"
	LintTrivial   = "L-TRIVIAL"
)

var (
	reActivity = regexp.MustCompile(`(?i)\b(improve|ensure|refactor|enhance|clean ?up|optimi[sz]e)\b`)
	reNumber   = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	reTrivial  = regexp.MustCompile(`^\s*(?:(?:echo|printf)\b[^&|;]*|true|exit\s+0|:)\s*$`)
)

// Lint returns the warnings for a parsed contract.
func Lint(c *Contract) []LintWarning {
	var out []LintWarning
	manual := 0
	for _, g := range c.Gates {
		if !g.Runnable() {
			manual++
		}
		if reActivity.MatchString(g.Outcome) {
			out = append(out, LintWarning{LintActivity, g.ID, "outcome is phrased as an activity, not a state that can fail"})
		}
		for _, n := range reNumber.FindAllString(g.Outcome, -1) {
			if !strings.Contains(g.Check, n) && !strings.Contains(g.Expect, n) {
				out = append(out, LintWarning{LintNumber, g.ID, fmt.Sprintf("outcome mentions %s but no CHECK: or EXPECT: measures it", n)})
			}
		}
		if g.Red == RedNone {
			out = append(out, LintWarning{LintRedNone, g.ID, "RED: none declares the gate unproven"})
		}
		if !g.Runnable() {
			continue
		}
		if e, err := CompileExpect(g.Expect); err == nil {
			if e.PathShaped() {
				out = append(out, LintWarning{LintPathRegex, g.ID, "EXPECT: is a slash-wrapped path-shaped regex; quote a substring instead"})
			}
			switch strings.ToLower(strings.TrimSpace(g.Expect)) {
			case "ok", "done", "pass", "passed":
				out = append(out, LintWarning{LintVocab, g.ID, "EXPECT: uses a word failure output also prints (" + g.Expect + ")"})
			}
		}
		if reTrivial.MatchString(g.Check) {
			out = append(out, LintWarning{LintTrivial, g.ID, "CHECK: is echo, printf, true or exit 0 only"})
		}
	}
	if len(c.Gates) > 0 && manual*2 > len(c.Gates) {
		out = append(out, LintWarning{Rule: LintManual, Msg: fmt.Sprintf("%d of %d gates are manual", manual, len(c.Gates))})
	}
	return out
}
