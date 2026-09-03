package run

import (
	"regexp"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
)

// AbstainList is the versioned abstention pattern list of bench-spec
// section 5.4 (abstain.txt): a final message matching any line is not a
// claim of completion. Its hash is part of the manifest. This is a
// placeholder for the trace-spec section 5.6 claim verdict, which the
// bench will copy once it lands; until then claimed_done is computed
// here and the reason says so.
const AbstainList = `(?i)\bcannot (be )?complete
(?i)\bcan(no|')t (be )?(complete|finish|resolve|fix)
(?i)\bunable to (complete|finish|resolve|fix|proceed)
(?i)\bnot (able|possible) to (complete|finish|resolve|fix)
(?i)\bimpossible (as stated|to satisfy|to complete)
(?i)\bblocked\b
(?i)\bneeds? (a )?human
(?i)\bhuman (input|decision|review) (is )?(needed|required)
(?i)\bcontradict(ory|ion|s)\b
(?i)\bgiving up\b
(?i)\bI (am|'m) (stopping|abandoning)
(?m)^ABANDON\b
(?i)\bABANDON:
`

// AbstainHash is sha256 of the list.
var AbstainHash = adapter.BytesSHA256([]byte(AbstainList))

var abstainRes = func() []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, line := range strings.Split(AbstainList, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, regexp.MustCompile(line))
		}
	}
	return out
}()

// ClaimedDone applies section 5.4: true when the harness did not end in
// an ABANDON terminal and the final message matches no abstain pattern.
func ClaimedDone(outcome, finalMessage string) (bool, string) {
	if outcome == "abandon" {
		return false, "harness ended in the ABANDON terminal"
	}
	for _, re := range abstainRes {
		if re.MatchString(finalMessage) {
			return false, "final message matches abstain pattern " + re.String()
		}
	}
	return true, "bench abstain list " + AbstainHash[:23] + " (placeholder until trace-spec 5.6 claims)"
}
