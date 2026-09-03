package gate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
)

// The fake shell script: exit code and output are chosen by the CHECK
// arguments, so pass and fail semantics can be exercised without a
// toolchain.
func TestRunSemantics(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e, _ := CompileExpect("MARKER")
	cases := []struct {
		name    string
		cmd     string
		pass    bool
		timeout bool
	}{
		{"exit 0 with marker passes", "echo MARKER", true, false},
		{"exit 0 without marker fails", "echo nothing", false, false},
		{"marker with non-zero exit fails", "echo MARKER; exit 3", false, false},
		{"marker on stderr counts", "echo MARKER 1>&2", true, false},
		{"timeout kills", "echo MARKER; sleep 5", false, true},
	}
	for _, tc := range cases {
		r := Run(ctx, "/bin/sh", tc.cmd, dir, 300*time.Millisecond, OutputCap)
		m, _ := e.Match(r.Output)
		pass := r.Exit == 0 && m && !r.TimedOut && !r.Truncated
		if pass != tc.pass || r.TimedOut != tc.timeout {
			t.Errorf("%s: exit %d matched %t timeout %t", tc.name, r.Exit, m, r.TimedOut)
		}
	}
	// The process group dies with the timeout: a background child must
	// not keep the pipe open past the deadline.
	start := time.Now()
	r := Run(ctx, "/bin/sh", "(sleep 5 & wait)", dir, 200*time.Millisecond, OutputCap)
	if !r.TimedOut || time.Since(start) > 3*time.Second {
		t.Errorf("process group not killed: %+v after %s", r, time.Since(start))
	}
	// Output cap breach fails and stops the process.
	r = Run(ctx, "/bin/sh", "yes MARKER | head -c 200000", dir, 5*time.Second, 1000)
	if !r.Truncated || r.Bytes > 1000 {
		t.Errorf("cap: truncated %t bytes %d", r.Truncated, r.Bytes)
	}
	// Missing command is exit 127 and rejected as a red.
	r = Run(ctx, "/bin/sh", "definitely-not-a-command-xyz", dir, time.Second, OutputCap)
	if ok, _ := RealRed(r, false, DefaultConfig()); ok || r.Exit != 127 {
		t.Errorf("exit 127 accepted as red: %+v", r)
	}
	// Regex on a 1 MiB adversarial input stays linear (the bound is loose
	// because -race slows RE2 by an order of magnitude; a backtracking
	// engine would not finish at all).
	big := []byte(strings.Repeat("a", 1<<20))
	e2, _ := CompileExpect(`/(a+)+b/`)
	start = time.Now()
	e2.Match(big)
	if time.Since(start) > 5*time.Second {
		t.Errorf("regex took %s", time.Since(start))
	}
}

func TestExpectDialect(t *testing.T) {
	e, err := CompileExpect("/^ok$/mi")
	if err != nil || e.Kind != "regex" {
		t.Fatal(err)
	}
	if m, span := e.Match([]byte("x\nOK\ny")); !m || span != [2]int{2, 4} {
		t.Errorf("match %t %v", m, span)
	}
	if _, err := CompileExpect(`/(?P<n>a)\1/`); err == nil {
		t.Error("backreference must be rejected")
	}
	p, _ := CompileExpect("plain / text")
	if p.Kind != "substring" {
		t.Error("slash inside is a substring")
	}
	if e3, _ := CompileExpect("/src/foo.ts/"); !e3.PathShaped() {
		t.Error("path-shaped regex not detected")
	}
}

// Check: baseline red on the empty diff, then green after the work; the
// evidence record round-trips, the EVIDENCE: line matches the file hash,
// and the successful output never lands anywhere under .saga.
func TestCheckBaselineThenGreenAndEvidenceCanary(t *testing.T) {
	r := newRepo(t)
	const canary = "CANARY-DONE-7f3e9a1c"
	r.write("marker.txt", "not yet\n")
	r.write(".saga/contract.md", strings.Replace(minimalContract, "CANARY-DONE", canary, 1))
	r.commit("base")

	rep := r.check(CheckOptions{Approve: true})
	g := gateState(rep, "G1")
	if g.State != StateUnmet || g.Failure == nil || *g.Failure != "no-match" {
		t.Fatalf("baseline: %+v", g)
	}
	if !g.Red.Valid || g.Red.Mode != RedBaseline {
		t.Fatalf("baseline red not recorded: %+v", g.Red)
	}
	if rep.Exit != int(cli.ExitFinding) {
		t.Errorf("exit %d", rep.Exit)
	}

	r.write("marker.txt", canary+"\n")
	rep = r.check(CheckOptions{})
	g = gateState(rep, "G1")
	if g.State != StateMet || rep.Exit != 0 {
		t.Fatalf("after work: %+v exit %d lines %v", g, rep.Exit, rep.Lines)
	}
	if rep.Lines[len(rep.Lines)-1] != "ALL MET" {
		t.Errorf("lines: %v", rep.Lines)
	}
	contract := r.read(".saga/contract.md")
	if !strings.Contains(contract, "- [x] G1:") || !strings.Contains(contract, "EVIDENCE: "+*g.Evidence) {
		t.Errorf("contract not rewritten: %s", contract)
	}
	rec, hash, err := LoadEvidence(r.store, "scratch", "G1")
	if err != nil || rec == nil || hash != *g.Evidence || rec.Outcome != OutcomeMet || rec.OutputSHA256 == nil || rec.Matched == nil {
		t.Fatalf("evidence: %+v %s %v", rec, hash, err)
	}
	if rec.Tree == nil || rec.Tree.WorktreeHash == nil || !strings.HasPrefix(*rec.Tree.WorktreeHash, "tree:") || rec.Tree.SnapshotID == nil {
		t.Errorf("tree: %+v", rec.Tree)
	}
	if rec.RedProof == nil || rec.Approval == nil {
		t.Errorf("red_proof/approval missing: %+v", rec)
	}
	// Canary: the raw success output is in no artefact under .saga.
	filepath.WalkDir(r.store.Dir(), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "contract.md") {
			return nil
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), canary) {
			t.Errorf("success output persisted in %s", p)
		}
		return nil
	})
	// Status never executes and agrees.
	if st := r.status(); gateState(st, "G1").State != StateMet || st.Exit != 0 {
		t.Errorf("status: %+v", gateState(st, "G1"))
	}
	// An edit after the check makes the met record stale.
	r.write("src/x.txt", "later\n")
	if st := r.status(); gateState(st, "G1").State != StateUnmet {
		t.Errorf("stale met record not demoted: %+v", gateState(st, "G1"))
	}
}

func TestBaselineRefusedOnNonEmptyDiffAndWrongReasonReds(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "no\n")
	r.write("src/work.txt", "untouched\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	r.write("src/work.txt", "started\n") // a tracked in-scope change is work
	rep := r.check(CheckOptions{Approve: true})
	g := gateState(rep, "G1")
	if g.Red.Valid || g.Red.Reason == nil || *g.Red.Reason != ReasonBaselineMissed {
		t.Errorf("baseline should be missed on a non-empty diff: %+v", g.Red)
	}

	// Wrong-reason reds: exit 127, a missing-module pattern, a timeout.
	for _, tc := range []struct{ name, check string }{
		{"exit127", "no-such-binary-abc"},
		{"module", "echo ModuleNotFoundError: No module named x; exit 1"},
		{"timeout", "sleep 3"},
	} {
		r2 := newRepo(t)
		r2.write("marker.txt", "no\n")
		r2.write(".saga/contract.md", "# Contract: w\n\nIN: marker.txt\n\n- [ ] G1: o\n    CHECK: "+tc.check+"\n    EXPECT: CANARY\n")
		r2.commit("base")
		rep := r2.check(CheckOptions{Approve: true, TimeoutS: 1})
		if g := gateState(rep, "G1"); g.Red.Valid {
			t.Errorf("%s: wrong-reason red accepted", tc.name)
		}
	}
	// A green oracle on the empty diff cannot establish a red either.
	r3 := newRepo(t)
	r3.write("marker.txt", "CANARY-DONE\n")
	r3.write(".saga/contract.md", minimalContract)
	r3.commit("base")
	rep = r3.check(CheckOptions{Approve: true})
	g = gateState(rep, "G1")
	if g.State != StateUnproven || g.Red.Valid || rep.Exit != int(cli.ExitIntegrity) {
		t.Errorf("tautological gate: %+v exit %d", g, rep.Exit)
	}
	if rep2 := r3.check(CheckOptions{NoRequireRed: true}); rep2.Exit != 0 {
		t.Errorf("--no-require-red should downgrade: %d", rep2.Exit)
	}
}

func TestRedInvalidation(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "no\n")
	r.write("scripts/oracle.sh", "cat marker.txt\n")
	r.write(".saga/contract.md", "# Contract: w\n\nIN: marker.txt\n\n- [ ] G1: o\n    CHECK: sh scripts/oracle.sh\n    EXPECT: CANARY-DONE\n")
	r.commit("base")
	rep := r.check(CheckOptions{Approve: true})
	if !gateState(rep, "G1").Red.Valid {
		t.Fatal("no baseline red")
	}
	// A witness byte change voids the proof.
	r.write("scripts/oracle.sh", "cat marker.txt # changed\n")
	st := r.status()
	if g := gateState(st, "G1"); g.Red.Valid || *g.Red.Reason != ReasonInvalidated {
		t.Errorf("witness change: %+v", g.Red)
	}
	// The approval identity moved with it.
	if gateState(st, "G1").Approval != "missing" {
		t.Error("approval should require re-consent after a witness change")
	}
	r.write("scripts/oracle.sh", "cat marker.txt\n")
	// An in-scope file named in CHECK: is the subject, not a witness.
	if paths, _ := WitnessesIn(r.root, r.load().Contract, r.load().Contract.Gates[0]); len(paths) != 1 || paths[0] != "scripts/oracle.sh" {
		t.Errorf("witnesses: %v", paths)
	}
	// An oracle edit voids it too.
	r.write(".saga/contract.md", strings.Replace(r.read(".saga/contract.md"), "EXPECT: CANARY-DONE", "EXPECT: CANARY-OTHER", 1))
	if g := gateState(r.status(), "G1"); g.Red.Valid {
		t.Error("oracle edit did not invalidate")
	}
}

func TestControlRedProof(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "CANARY-DONE\n")
	r.write("control.txt", "nothing here\n")
	r.write(".saga/contract.md", "# Contract: c\n\nIN: marker.txt\n\n- [ ] G1: o\n    CHECK: cat marker.txt\n    EXPECT: CANARY-DONE\n    RED: control\n    RED-CHECK: cat control.txt\n    RED-EXPECT: nothing here\n")
	r.commit("base")
	rep := r.check(CheckOptions{Approve: true})
	if g := gateState(rep, "G1"); g.State != StateMet || !g.Red.Valid || g.Red.Mode != RedControl {
		t.Errorf("control: %+v", g)
	}
	// Control output that also matches EXPECT: proves nothing.
	r2 := newRepo(t)
	r2.write("marker.txt", "CANARY-DONE\n")
	r2.write(".saga/contract.md", "# Contract: c\n\nIN: marker.txt\n\n- [ ] G1: o\n    CHECK: cat marker.txt\n    EXPECT: CANARY-DONE\n    RED: control\n    RED-CHECK: cat marker.txt\n    RED-EXPECT: CANARY\n")
	r2.commit("base")
	rep = r2.check(CheckOptions{Approve: true})
	if g := gateState(rep, "G1"); g.State != StateUnproven || g.Red.Valid {
		t.Errorf("tautological control accepted: %+v", g)
	}
}

func TestApprovalRequiredAndAgentShellRefusal(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "x\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	rep := r.check(CheckOptions{})
	if rep.Exit != int(cli.ExitApproval) || len(rep.ApprovalMissing) != 1 {
		t.Fatalf("unapproved oracle executed or wrong code: %d %v", rep.Exit, rep.ApprovalMissing)
	}
	if rec, _, _ := LoadEvidence(r.store, "scratch", "G1"); rec != nil {
		t.Error("evidence written without approval")
	}
	t.Setenv("CLAUDECODE", "1")
	if _, err := Check(r.load(), CheckOptions{Approve: true}); cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("check --approve from an agent shell: %v", err)
	}
	if _, err := Approve(r.load(), nil, false); cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("approve from an agent shell: %v", err)
	}
	t.Setenv("CLAUDECODE", "")
	lines, err := Approve(r.load(), []string{"G1"}, false)
	if err != nil || len(lines) != 1 {
		t.Fatalf("approve: %v %v", lines, err)
	}
	if rep := r.check(CheckOptions{}); rep.Exit == int(cli.ExitApproval) {
		t.Error("approval not honoured")
	}
	// A different PATH is a different identity.
	t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+t.TempDir())
	if g := gateState(r.status(), "G1"); g.Approval != "missing" {
		t.Error("PATH change should invalidate approval")
	}
	// CI mode needs no store.
	t.Setenv(ApprovalEnv, filepath.Join(r.root, "inside"))
	if _, err := ApprovalDir(r.root); err == nil {
		t.Error("approval store inside the repo accepted")
	}
	if rep, err := Check(r.load(), CheckOptions{CI: true, Reverify: true}); err != nil || rep.Exit == int(cli.ExitApproval) {
		t.Errorf("--ci: %v %d", err, rep.Exit)
	}
}

func TestAttestManualGateAndAbandon(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", "# Contract: m\n\nIN: src/**\n\n- [ ] G1: reviewed by a human\n- [ ] G2: impossible bit\n\nABANDON: G2 owner unavailable\n")
	r.commit("base")
	st := r.status()
	if gateState(st, "G1").State != StateManual || gateState(st, "G2").State != StateAbandoned || len(st.Handoff) != 1 {
		t.Fatalf("status: %+v", st.Gates)
	}
	if st.Lines[len(st.Lines)-1] != "HANDOFF REQUIRED" || st.Exit != int(cli.ExitFinding) {
		t.Errorf("handoff: %v %d", st.Lines, st.Exit)
	}
	t.Setenv("CLAUDECODE", "1")
	if _, err := Attest(r.load(), "G1", "seen it"); cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("attest from agent shell: %v", err)
	}
	t.Setenv("CLAUDECODE", "")
	rep, err := Attest(r.load(), "G1", "seen it")
	if err != nil || gateState(rep, "G1").State != StateAttested {
		t.Fatalf("attest: %v %+v", err, rep.Gates)
	}
	if _, err := Attest(r.load(), "G2", "x"); cli.CodeOf(err) != cli.ExitFinding {
		t.Errorf("attesting an abandoned gate: %v", err)
	}
}

func TestLedgerRule16(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", "# Contract: l\n\nIN: src/**\n\n- [x] G1: o\n    CHECK: true\n    EXPECT: x\n    EVIDENCE: sha256:"+strings.Repeat("d", 64)+"\n")
	r.commit("base")
	err := r.load().CheckLedger()
	pe, ok := err.(interface{ Unwrap() error })
	if err == nil || !ok || cli.CodeOf(err) != cli.ExitUsage {
		t.Fatalf("dangling EVIDENCE: %v", err)
	}
	if inner, ok := pe.Unwrap().(*ParseError); !ok || inner.Rule != 16 {
		t.Errorf("rule: %v", pe.Unwrap())
	}
}

func TestLint(t *testing.T) {
	c, err := Parse([]byte("# T\nIN: a\n- [ ] G1: improve the 3 handlers\n    CHECK: echo passed\n    EXPECT: passed\n- [ ] G2: manual one\n- [ ] G3: manual two\n    RED: none\n- [ ] G4: p\n    CHECK: ls\n    EXPECT: /src/foo.ts/\n- [ ] G5: manual three\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, w := range Lint(c) {
		got[w.Rule] = true
	}
	for _, want := range []string{LintActivity, LintNumber, LintVocab, LintTrivial, LintManual, LintRedNone, LintPathRegex} {
		if !got[want] {
			t.Errorf("missing %s: %v", want, Lint(c))
		}
	}
	if c2, _ := Parse([]byte(minimalContract)); len(Lint(c2)) != 0 {
		t.Errorf("minimal contract should lint clean: %v", Lint(c2))
	}
}
