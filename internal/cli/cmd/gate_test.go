package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// benchWorkspace copies a bench task's repo into a scratch git
// repository the way bench/tasks/_tools/verify-task.sh does.
func benchWorkspace(t *testing.T, task string) (string, string) {
	t.Helper()
	root := repoRoot(t)
	taskDir := filepath.Join(root, "bench", "tasks", task)
	if _, err := os.Stat(taskDir); err != nil {
		t.Skipf("bench task %s not present", task)
	}
	ws := t.TempDir()
	if r, err := filepath.EvalSymlinks(ws); err == nil {
		ws = r
	}
	if out, err := exec.Command("cp", "-R", filepath.Join(taskDir, "repo")+"/.", ws+"/").CombinedOutput(); err != nil {
		t.Fatalf("copy: %v %s", err, out)
	}
	gitIn(t, ws, "init", "-q")
	gitIn(t, ws, "config", "user.email", "t@t")
	gitIn(t, ws, "config", "user.name", "t")
	gitIn(t, ws, "config", "commit.gpgsign", "false")
	gitIn(t, ws, "add", "-A")
	gitIn(t, ws, "commit", "-q", "-m", "base")
	setup := exec.Command("bash", filepath.Join(taskDir, "setup.sh"))
	setup.Dir = ws
	if out, err := setup.CombinedOutput(); err != nil {
		t.Skipf("setup.sh failed (toolchain missing?): %v %s", err, out)
	}
	return ws, taskDir
}

func humanEnv(t *testing.T) {
	t.Helper()
	adir := filepath.Join(t.TempDir(), "approved")
	if err := os.MkdirAll(adir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(gate.ApprovalEnv, adir)
	for _, m := range gate.AgentShellMarkers {
		t.Setenv(m, "")
	}
}

func statusJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("not JSON: %q", out)
	}
	return m
}

func gateStates(m map[string]any) map[string]string {
	states := map[string]string{}
	for _, g := range m["gates"].([]any) {
		gm := g.(map[string]any)
		id := gm["id"].(string)
		states[id[strings.LastIndex(id, ":")+1:]] = gm["state"].(string)
	}
	return states
}

func TestEndToEndBenchTasks(t *testing.T) {
	for _, tc := range []struct {
		task string
		bin  string
	}{
		{"ts-0001-slug-collapse", "node"},
		{"py-0008-money-exact-cents", "python3"},
	} {
		t.Run(tc.task, func(t *testing.T) {
			if _, err := exec.LookPath(tc.bin); err != nil {
				t.Skipf("%s not on PATH", tc.bin)
			}
			humanEnv(t)
			ws, taskDir := benchWorkspace(t, tc.task)
			out, errs, code := runIn(t, ws, "", "gate", "init", "--request", filepath.Join(taskDir, "prompt.md"))
			if code != cli.ExitOK || !strings.HasPrefix(out, "SEGMENTER: v1\nR1  ") {
				t.Fatalf("init: %d %s %s", code, out, errs)
			}
			raw, err := os.ReadFile(filepath.Join(taskDir, "contract.md"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(ws, ".saga", "contract.md"), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if out, errs, code := runIn(t, ws, "", "gate", "lint", "--strict"); code != cli.ExitOK {
				t.Fatalf("lint: %d %s %s", code, out, errs)
			}
			// Status never executes: no evidence appears.
			if _, _, code := runIn(t, ws, "", "gate", "status"); code == cli.ExitOK {
				t.Fatal("status green before any check")
			}
			if _, err := os.Stat(filepath.Join(ws, ".saga", "evidence")); err == nil {
				t.Fatal("status wrote evidence")
			}
			// Baseline: approve and check on the untouched tree; at least one
			// gate must be red and the reds are recorded.
			out, errs, code = runIn(t, ws, "", "gate", "check", "--approve", "--json")
			if code == cli.ExitOK || code == cli.ExitUsage || code == cli.ExitEnvironment {
				t.Fatalf("baseline check: %d %s %s", code, out, errs)
			}
			base := gateStates(statusJSON(t, out))
			if base["G1"] != gate.StateUnmet {
				t.Errorf("baseline G1 %s", base["G1"])
			}
			// Gold applied: the visible gates go green.
			gitIn(t, ws, "apply", filepath.Join(taskDir, "controls", "gold.patch"))
			out, errs, code = runIn(t, ws, "", "gate", "check", "--json")
			m := statusJSON(t, out)
			states := gateStates(m)
			for _, id := range []string{"G1", "G2"} {
				if states[id] != gate.StateMet {
					t.Errorf("gold %s: %s (%s)", id, states[id], errs)
				}
			}
			// G3 is a regression gate (green at baseline): met but unproven
			// under require_red, so exit 5; without require_red the run is ALL MET.
			if states["G3"] != gate.StateUnproven || code != cli.ExitIntegrity {
				t.Errorf("gold G3: %s exit %d", states["G3"], code)
			}
			out, errs, code = runIn(t, ws, "", "gate", "check", "--no-require-red")
			if code != cli.ExitOK || !strings.Contains(out, "ALL MET") {
				t.Errorf("--no-require-red: %d %s %s", code, out, errs)
			}
			contract, _ := os.ReadFile(filepath.Join(ws, ".saga", "contract.md"))
			if strings.Count(string(contract), "- [x]") != 3 || strings.Count(string(contract), "EVIDENCE: sha256:") != 3 {
				t.Errorf("contract not rewritten:\n%s", contract)
			}
			// Guards are clean on gold and reverify in CI mode recomputes.
			if out, errs, code := runIn(t, ws, "", "gate", "guard-diff"); code != cli.ExitOK {
				t.Errorf("guard-diff: %d %s %s", code, out, errs)
			}
			if _, errs, code := runIn(t, ws, "", "gate", "reverify", "--ci", "--json"); code != cli.ExitIntegrity {
				t.Errorf("reverify --ci: %d %s", code, errs)
			}
			// The Stop hook cites ids only and allows once the ledger is green.
			payload := `{"session_id":"e2e","hook_event_name":"Stop","stop_hook_active":false,"cwd":"` + ws + `"}`
			hookJSON(t, ws, "SessionStart", `{"session_id":"e2e","hook_event_name":"SessionStart","source":"startup","cwd":"`+ws+`"}`)
			res, code := hookJSON(t, ws, "Stop", payload)
			if code != cli.ExitIntegrity || res["decision"] != "block" || !strings.Contains(res["reason"].(string), "G3(unproven)") {
				t.Errorf("stop under require_red: %v %d", res, code)
			}
			for _, g := range m["gates"].([]any) {
				if strings.Contains(res["reason"].(string), g.(map[string]any)["outcome"].(string)) {
					t.Error("outcome text leaked into the Stop reason")
				}
			}
			// A cheat patch trips a guard at Stop.
			cheat := filepath.Join(taskDir, "controls", "cheat-1.patch")
			if _, err := os.Stat(cheat); err == nil {
				gitIn(t, ws, "apply", "--reverse", filepath.Join(taskDir, "controls", "gold.patch"))
				gitIn(t, ws, "apply", cheat)
				out, _, code := runIn(t, ws, "", "gate", "guard-diff")
				t.Logf("cheat-1 guard-diff (%d): %s", code, strings.TrimSpace(out))
			}
		})
	}
}
