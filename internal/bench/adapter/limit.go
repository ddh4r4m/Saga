package adapter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The account's session window, not the run's. When it is exhausted the
// harness answers every invocation at once with a `result` event whose
// entire final message is one line naming the limit and the reset, and
// it does so with `subtype: "success"` and `is_error: true`, which is
// why nothing in the outcome table caught it: the pilot of 2026-09-13
// graded 132 such replies `completed` with the oracle red, recording
// the harness declining to run as the model failing.
//
// Two shapes were captured, both in fixtures/harness/: the clean one
// (1 turn, no tool calls, 0.6 s, cost 0) and the transition row, where
// the window was exhausted after 12 turns and 11 tool calls and 0.1250
// usd of real work. The message is byte-identical in both, so the
// detector reads the message alone and takes no interest in cost or
// tool use; a rule keyed on "cost 0 and no tool use" would have missed
// the one row of the 132 where the model had actually worked.
var harnessLimitRe = regexp.MustCompile(
	`^(?i)you've hit your (?:session|usage) limit(?:\s*[·\-|,]\s*resets\s+(.+?))?\s*$`)

// IsHarnessLimit reports whether a final message is, in its entirety, a
// harness limit reply, and returns the line for the outcome reason. The
// whole trimmed message must be that line: a model that merely mentions
// hitting a limit inside a longer message is a normal run and is graded
// as one.
func IsHarnessLimit(finalMessage string) (bool, string) {
	line := strings.TrimSpace(finalMessage)
	if line == "" || strings.ContainsAny(line, "\n\r") {
		return false, ""
	}
	if !harnessLimitRe.MatchString(line) {
		return false, ""
	}
	return true, line
}

// LimitResetAt returns the wall-clock instant a limit reply says the
// window reopens, resolved against now. `resets 4:40am (Asia/Calcutta)`
// gives both a time and an IANA zone; a reply with neither, or with a
// zone this machine does not know, is not a failure of the run and the
// caller falls back to a short retry (bench-spec 4.5).
func LimitResetAt(finalMessage string, now time.Time) (time.Time, bool) {
	m := harnessLimitRe.FindStringSubmatch(strings.TrimSpace(finalMessage))
	if m == nil || m[1] == "" {
		return time.Time{}, false
	}
	rest := strings.TrimSpace(m[1])
	loc := now.Location()
	if z := zoneRe.FindStringSubmatch(rest); z != nil {
		if l, err := time.LoadLocation(z[1]); err == nil {
			loc = l
		}
		rest = strings.TrimSpace(zoneRe.ReplaceAllString(rest, ""))
	}
	hm := clockRe.FindStringSubmatch(rest)
	if hm == nil {
		return time.Time{}, false
	}
	hour, err := strconv.Atoi(hm[1])
	if err != nil || hour > 23 {
		return time.Time{}, false
	}
	min := 0
	if hm[2] != "" {
		if min, err = strconv.Atoi(hm[2]); err != nil || min > 59 {
			return time.Time{}, false
		}
	}
	switch strings.ToLower(hm[3]) {
	case "am":
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour < 12 {
			hour += 12
		}
	default:
		if hour > 23 {
			return time.Time{}, false
		}
	}
	local := now.In(loc)
	at := time.Date(local.Year(), local.Month(), local.Day(), hour, min, 0, 0, loc)
	// A reset is always ahead: "resets 4:40am" read at 03:11 is today and
	// read at 23:50 is tomorrow.
	if !at.After(local) {
		at = at.AddDate(0, 0, 1)
	}
	return at, true
}

var (
	zoneRe  = regexp.MustCompile(`\(([A-Za-z]+(?:/[A-Za-z_\-+0-9]+)+)\)`)
	clockRe = regexp.MustCompile(`(?i)\b(\d{1,2})(?::(\d{2}))?\s*(am|pm)?\b`)
)

// LimitWait is one interruption recorded in the manifest: which row was
// hit, when, and when the runner resumed it. The cost and wall of the
// abandoned attempt stay with the attempt, not with the row.
type LimitWait struct {
	Task      string `json:"task"`
	Arm       string `json:"arm"`
	K         int    `json:"k"`
	HitAt     string `json:"hit_at"`
	ResumedAt string `json:"resumed_at"`
	Message   string `json:"message"`
}

// OnLimit is the policy for a harness limit reply.
type OnLimit string

const (
	// OnLimitWait sleeps until the announced reset and retries the row
	// once. It is the default: an interrupted pilot is worth resuming and
	// a limit is not a property of the model.
	OnLimitWait OnLimit = "wait"
	// OnLimitStop ends the run at the first limit reply.
	OnLimitStop OnLimit = "stop"
)

// ParseOnLimit validates the flag value.
func ParseOnLimit(s string) (OnLimit, error) {
	switch OnLimit(s) {
	case OnLimitWait, OnLimitStop:
		return OnLimit(s), nil
	case "":
		return OnLimitWait, nil
	}
	return "", fmt.Errorf("--on-limit %q: want wait or stop", s)
}

// LimitRetryDelay is how long to wait when the reply names no reset time
// this machine can read. One short retry distinguishes a transient reply
// from an exhausted window without guessing at a schedule.
const LimitRetryDelay = 5 * time.Minute

// LimitResetGrace is added to the announced reset before retrying, since
// the announced minute is when the window reopens, not when it is safe
// to assume it has.
const LimitResetGrace = 90 * time.Second
