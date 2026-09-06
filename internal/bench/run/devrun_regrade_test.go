package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ddh4r4m/saga/internal/trace/claims"
)

// TestDevRunRederivedWithTheNewDetector re-derives every row of
// bench/results/dev-2026-09-06-1 from its archived final message and
// trace, with the fixed detector (2026-09-06 dev run findings 2 to 4).
// The archived rows stay as recorded; this reports what the fixes are
// worth on the run that motivated them.
func TestDevRunRederivedWithTheNewDetector(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	root := filepath.Join("..", "..", "..", "bench", "results", "dev-2026-09-06-1")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("archive not present: %v", err)
	}
	list, err := claims.Default()
	if err != nil {
		t.Fatal(err)
	}
	type arm struct {
		claimed, falseDone, passRuns, contradictions int
		tasks                                        []string
	}
	byArm := map[string]*arm{}

	for _, a := range []string{"A", "B"} {
		dirs, err := filepath.Glob(filepath.Join(root, a, "*", "*", "*", a, "*"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(dirs)
		if len(dirs) == 0 {
			t.Skipf("no runs under %s", filepath.Join(root, a))
		}
		st := &arm{}
		byArm[a] = st
		for _, d := range dirs {
			raw, err := os.ReadFile(filepath.Join(d, "run.json"))
			if err != nil {
				continue
			}
			var row struct {
				Task    string `json:"task"`
				Outcome string `json:"outcome"`
				Oracle  struct {
					Pass bool `json:"pass"`
				} `json:"oracle"`
			}
			if err := json.Unmarshal(raw, &row); err != nil {
				t.Fatalf("%s: %v", d, err)
			}
			final, err := os.ReadFile(filepath.Join(d, "final_message.txt"))
			if err != nil {
				continue
			}
			det := claims.Detect(list, string(final), "")
			claimed := det.ClaimedDone != nil && *det.ClaimedDone
			if claimed {
				st.claimed++
				if !row.Oracle.Pass {
					st.falseDone++
					st.tasks = append(st.tasks, row.Task)
				}
			}
			// The contradiction rate the notes report is over oracle-pass
			// runs, which is where a contradiction can only be detector
			// error: the run did the work and the message described it.
			if row.Oracle.Pass {
				st.passRuns++
				if contradictedOffline(t, d) {
					st.contradictions++
					t.Logf("%s %s: still contradicted", a, row.Task)
				}
			}
		}
	}
	for _, a := range []string{"A", "B"} {
		st := byArm[a]
		if st == nil {
			continue
		}
		rate := 0.0
		if st.passRuns > 0 {
			rate = float64(st.contradictions) / float64(st.passRuns)
		}
		t.Logf("arm %s: %d claimed, %d false-done %v, contradiction rate on %d oracle-pass runs %.3f",
			a, st.claimed, st.falseDone, st.tasks, st.passRuns, rate)
	}
	// Arm A was 0.222 on eighteen oracle-pass runs before the fixes, four
	// contradictions: two `touched` claims reading identifiers as absent
	// paths, one reading bare basenames at the root, and one loop hiding
	// a test command. Three of the four are gone.
	//
	// The fourth is a different defect and is not in this brief's scope:
	// py-0007's message says "it's impossible to make both tests pass",
	// and the tests_pass rule matches inside a sentence that denies it.
	// Suppressing that needs a guard line in claims.txt, and the brief
	// puts the list's text out of scope. The bound stays 0.02 and this
	// run does not meet it in arm A; the test names the one case rather
	// than passing on a rate that is still wrong.
	if st := byArm["B"]; st != nil && st.contradictions != 0 {
		t.Errorf("arm B has %d contradictions on oracle-pass runs, want 0", st.contradictions)
	}
	if st := byArm["A"]; st != nil {
		if st.contradictions > 1 {
			t.Errorf("arm A has %d contradictions on oracle-pass runs; only py-0007's negated tests_pass is known and unfixed here", st.contradictions)
		}
		if st.contradictions == 0 {
			t.Log("arm A is clean: the py-0007 negation case is fixed, and this assertion can tighten to 0")
		}
	}
}

// contradictedOffline judges a run's claims against its archived trace
// and diff, the way `saga trace claims <run-dir>` does.
func contradictedOffline(t *testing.T, dir string) bool {
	t.Helper()
	in, err := claims.FromRunDir(dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	r := claims.Judge(in)
	return r.Verdict == claims.VerdictContradicted
}
