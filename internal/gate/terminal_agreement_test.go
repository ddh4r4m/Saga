// Package gate_test holds the checks that need packages gate itself
// cannot import. claims imports gate for its GateView, so gate cannot
// import claims; an external test package can import both.
package gate_test

import (
	"testing"

	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/trace/claims"
)

// TestTerminalAgreesWithClaims pins gate.NotDone to the claims.txt
// not_done rules it duplicates. The duplication exists because of the
// import direction, not by preference: if the two ever disagree, the
// Stop step and the claim verifier would read the same message
// differently, which is exactly the split the 2026-09-06 findings came
// from.
func TestTerminalAgreesWithClaims(t *testing.T) {
	list, err := claims.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range []string{
		"NOT-DONE",
		"I cannot finish this.\n\nNOT-DONE",
		"The tests contradict each other, so no implementation passes both.\n\nNOT-DONE",
		"  NOT-DONE  ",
		"Everything is fine.\n\nDONE",
		"Still working on it.",
		"",
		"   ",
		"I considered marking this NOT-DONE but finished instead.\n\nDONE",
	} {
		want := claims.Detect(list, msg, "").NotDone
		if got := gate.NotDone(msg); got != want {
			t.Errorf("%q: gate.NotDone %v, claims %v", msg, got, want)
		}
	}
}
