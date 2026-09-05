package claims

import (
	"fmt"
	"strings"

	"github.com/ddh4r4m/saga/internal/cli"
)

// Decisions of the section 5.9 table.
const (
	DecisionAllow = "allow"
	DecisionWarn  = "warn"
	DecisionBlock = "block"
)

// Modes of gate-spec section 1.2.
const (
	ModeMinimal = "minimal"
	ModeFull    = "full"
)

// MessageTokens is the claim block line ceiling (contracts section 7.3).
const MessageTokens = 120

// Collapsed is the line at the session share.
const Collapsed = "saga trace: claim %s; run saga trace claims %s"

// decide applies the section 5.9 decision table.
func decide(r *Result, in *Input) (string, int) {
	if r.ListInvalid {
		return DecisionBlock, int(cli.ExitUsage)
	}
	switch r.Verdict {
	case "", VerdictVerified:
		return DecisionAllow, int(cli.ExitOK)
	case VerdictContradicted:
		return DecisionBlock, int(cli.ExitIntegrity)
	}
	exit := int(cli.ExitFinding)
	if r.FinalUnavailable {
		exit = int(cli.ExitEnvironment)
	}
	if in.Mode == ModeFull && in.Unverified != "warn" {
		return DecisionBlock, exit
	}
	return DecisionWarn, exit
}

// message is the fixed block line: ids, seqs and normalised paths only,
// at most MessageTokens estimated tokens. Empty unless the decision is
// block.
func message(r *Result, in *Input) string {
	if r.Decision != DecisionBlock {
		return ""
	}
	session := in.Session
	if session == "" {
		session = "<session>"
	}
	if r.ListInvalid {
		return "saga trace: claims list invalid; run saga trace claims " + session
	}
	var items []string
	for _, c := range r.Claims {
		if c.Verdict != r.Verdict {
			continue
		}
		items = append(items, describe(c))
	}
	head := "saga trace: claim " + r.Verdict + ": "
	tail := ". run saga trace claims " + session
	for len(items) > 0 {
		msg := head + strings.Join(items, "; ") + tail
		if tokensEst(msg) <= MessageTokens {
			return msg
		}
		items = items[:len(items)-1]
	}
	return fmt.Sprintf(Collapsed, r.Verdict, session)
}

func describe(c ClaimResult) string {
	ev := ""
	if len(c.Evidence) > 0 {
		ev = fmt.Sprintf(" vs #%d", c.Evidence[len(c.Evidence)-1])
	}
	what := c.Kind
	switch c.Kind {
	case KindRan:
		what = "ran " + clip(c.Command, 60)
	case KindTouched, KindRead:
		what = c.Kind + " " + clip(c.Path, 80)
	case KindGateMet:
		if len(c.IDs) > 0 {
			what = "gate_met " + strings.Join(c.IDs, ",")
		}
	}
	reason := c.Reason
	if reason == "" {
		return what + ev
	}
	return what + ev + " (" + clip(reason, 60) + ")"
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// Schema is the id of the `--json` output.
const Schema = "saga.trace.claims/1"

// Render is the human view: one line per claim, then the verdict.
func Render(in *Input, r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "turn %d  claimed_done=%s (%s)  claims=%d\n", in.Turn, boolStr(r.Detection.ClaimedDone), r.Detection.Reason, len(r.Claims))
	for _, c := range r.Claims {
		fmt.Fprintf(&b, "  %-12s %-12s %s\n", c.Kind, c.Verdict, describe(c))
	}
	verdict := r.Verdict
	if verdict == "" {
		verdict = "none"
	}
	fmt.Fprintf(&b, "verdict %s  decision %s  exit %d  mode %s\n", verdict, r.Decision, r.Exit, in.Mode)
	return b.String()
}

func boolStr(p *bool) string {
	if p == nil {
		return "null"
	}
	return fmt.Sprint(*p)
}
