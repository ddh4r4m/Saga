package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// limitFixture reads one captured `result` event from the pilot of
// 2026-09-13.
func limitFixture(t *testing.T, name string) *StreamResult {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "harness", name))
	if err != nil {
		t.Fatal(err)
	}
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	return &StreamResult{
		Subtype:      str(ev["subtype"]),
		IsError:      ev["is_error"] == true,
		FinalMessage: str(ev["result"]),
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// TestHarnessLimitIsInfraInBothShapes: the pilot of 2026-09-13 graded
// 132 limit replies `completed` with the oracle red, which recorded the
// harness declining to run as the model failing. Both captured shapes
// are infra: the clean one (1 turn, cost 0) and the transition row,
// where the window was exhausted after 12 turns, 11 tool calls and
// 0.1250 usd of real work. The message is what decides; a rule keyed on
// cost or tool use would miss the second.
func TestHarnessLimitIsInfraInBothShapes(t *testing.T) {
	for _, name := range []string{"session-limit-clean.json", "session-limit-after-work.json"} {
		sr := limitFixture(t, name)
		if sr.Subtype != "success" || !sr.IsError {
			t.Fatalf("%s: fixture shape changed: subtype %q is_error %v", name, sr.Subtype, sr.IsError)
		}
		ok, line := IsHarnessLimit(sr.FinalMessage)
		if !ok {
			t.Fatalf("%s: not read as a harness limit: %q", name, sr.FinalMessage)
		}
		if line != "You've hit your session limit · resets 4:40am (Asia/Calcutta)" {
			t.Errorf("%s: reason line %q", name, line)
		}
	}
}

// TestHarnessLimitNeedsTheWholeMessage: a model that mentions a limit
// inside a longer message has run, and is graded as one.
func TestHarnessLimitNeedsTheWholeMessage(t *testing.T) {
	no := []string{
		"I hit a rate limit while fetching, so I retried.\n\nYou've hit your session limit · resets 4:40am\n\nDONE",
		"The error was: You've hit your session limit · resets 4:40am (Asia/Calcutta). I worked around it.",
		"",
		"DONE",
	}
	for _, m := range no {
		if ok, _ := IsHarnessLimit(m); ok {
			t.Errorf("read as a harness limit: %q", m)
		}
	}
	yes := []string{
		"You've hit your session limit · resets 4:40am (Asia/Calcutta)",
		"  You've hit your usage limit · resets 11:05pm (America/New_York)  ",
		"You've hit your session limit",
	}
	for _, m := range yes {
		if ok, _ := IsHarnessLimit(m); !ok {
			t.Errorf("not read as a harness limit: %q", m)
		}
	}
}

// TestLimitResetAtReadsTheAnnouncedTime: the reset is resolved in the
// zone the message names, and is always ahead of now.
func TestLimitResetAtReadsTheAnnouncedTime(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Calcutta")
	if err != nil {
		t.Skip("no tzdata")
	}
	msg := "You've hit your session limit · resets 4:40am (Asia/Calcutta)"
	// The pilot's own clock: hit at 03:11:53 IST, reset announced 4:40am.
	now := time.Date(2026, 9, 13, 3, 11, 53, 0, ist)
	at, ok := LimitResetAt(msg, now)
	if !ok {
		t.Fatal("reset not parsed")
	}
	if want := time.Date(2026, 9, 13, 4, 40, 0, 0, ist); !at.Equal(want) {
		t.Errorf("reset %s, want %s", at, want)
	}
	// Read at 23:50 the same announcement is tomorrow's.
	at2, _ := LimitResetAt(msg, time.Date(2026, 9, 13, 23, 50, 0, 0, ist))
	if want := time.Date(2026, 9, 14, 4, 40, 0, 0, ist); !at2.Equal(want) {
		t.Errorf("reset %s, want %s", at2, want)
	}
	// A pm time and a bare hour.
	if at, ok := LimitResetAt("You've hit your usage limit · resets 11pm (Asia/Calcutta)", now); !ok || at.Hour() != 23 {
		t.Errorf("11pm read as %s (ok %v)", at, ok)
	}
}

// TestLimitResetUnparsableFallsBack: a reply with no readable reset is
// not a run failure; the caller takes the short retry instead.
func TestLimitResetUnparsableFallsBack(t *testing.T) {
	for _, m := range []string{
		"You've hit your session limit",
		"You've hit your session limit · resets soon",
		"You've hit your session limit · resets 99:99am (Nowhere/Nothing)",
	} {
		if ok, _ := IsHarnessLimit(m); !ok {
			t.Fatalf("%q is a limit reply", m)
		}
		if _, ok := LimitResetAt(m, time.Now()); ok {
			t.Errorf("%q: reset should not parse", m)
		}
	}
}

// TestParseOnLimit: the flag takes two values and defaults to wait.
func TestParseOnLimit(t *testing.T) {
	for in, want := range map[string]OnLimit{"": OnLimitWait, "wait": OnLimitWait, "stop": OnLimitStop} {
		got, err := ParseOnLimit(in)
		if err != nil || got != want {
			t.Errorf("ParseOnLimit(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseOnLimit("retry"); err == nil {
		t.Error("an unknown policy must be a usage error")
	}
}
