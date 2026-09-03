package gate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/hookio"
)

// Tests added by the gate implementation review (docs/specs/gate-impl-review.md).

func TestContractSlugIsClosedAlphabet(t *testing.T) {
	g := "- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n"
	for _, bad := range []string{"../../escape", "a/b", "ok‮evil", "Upper", "x y", strings.Repeat("a", 65), "-lead"} {
		if _, err := Parse([]byte("# T\nCONTRACT: " + bad + "\nIN: src/**\n" + g)); err == nil {
			t.Errorf("CONTRACT: %q accepted", bad)
		}
	}
	c, err := Parse([]byte("# T\nCONTRACT: vendor-import-2\nIN: src/**\n" + g))
	if err != nil || c.Slug != "vendor-import-2" {
		t.Fatalf("valid slug: %v %+v", err, c)
	}
}

func TestOneSpaceIndentIsExit2(t *testing.T) {
	_, err := Parse([]byte("# T\nIN: src/**\n- [ ] G1: o\n CHECK: true\n EXPECT: x\n"))
	pe, ok := err.(*ParseError)
	if !ok || pe.Rule != 5 {
		t.Fatalf("one-space indent: %v", err)
	}
}

func TestForbiddenCommandClassifier(t *testing.T) {
	deny := []string{
		"saga gate approve",
		"saga gate check --approve",
		"saga gate check --gate G1 --approve",
		"saga gate check -approve",
		"saga gate check --approve=true",
		`saga gate "approve"`,
		"saga  gate   attest G2 --note x",
		"/usr/local/bin/saga gate approve",
		"./saga gate approve",
		"saga gate reverify --ci",
		"saga gate reverify --base main -ci --json",
		"SAGA_APPROVAL_DIR=/tmp/x saga gate check",
		"cat > ~/.saga/approved/x.json",
		"env -u CLAUDECODE saga gate approve",
		"saga gate approve --revoke",
		"ls; saga gate approve",
		"true && saga gate attest G1 --note y",
		"echo x | saga gate approve",
		"printf 'x' > .saga/observed/gate-s.json",
	}
	allow := []string{"saga gate status", "saga gate check", "saga gate check --json --gate G1", "saga gate lint --strict", "saga gate reverify", "git status", "echo saga gate approve is not what I run"}
	for _, c := range deny {
		if ForbiddenCommand(c) == "" {
			t.Errorf("not denied: %q", c)
		}
	}
	for _, c := range allow[:6] {
		if f := ForbiddenCommand(c); f != "" {
			t.Errorf("wrongly denied %q: %s", c, f)
		}
	}
	// The classifier is lexical: an echo that merely mentions the command
	// is denied too, which is the safe direction.
	if ForbiddenCommand(allow[6]) == "" {
		t.Error("lexical match should deny the echo")
	}
}

func TestProtectedWritesAndApprovalStore(t *testing.T) {
	r := newRepo(t)
	layer := &Layer{Store: r.store}
	ctx := context.Background()
	adir := os.Getenv(ApprovalEnv)
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(adir, "deadbeef.json"),
		filepath.Join(home, ".saga", "approved", "x.json"),
		filepath.Join(home, ".saga", "anything"),
		filepath.Join(r.root, ".saga", "evidence", "s", "G1.json"),
		filepath.Join(r.root, "src", "..", ".saga", "red", "x.json"),
		filepath.Join(r.root, ".saga", "observed", "gate-sess1.json"),
		".saga/request.md",
	} {
		out, _ := layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Write", map[string]any{"file_path": p, "content": "x"}, false))
		if out.Decision != hookio.DecisionDeny {
			t.Errorf("write to %s allowed: %+v", p, out)
		}
	}
	// A symlink inside the repo that points at the ledger is resolved.
	if err := os.Symlink(filepath.Join(r.root, ".saga", "evidence"), filepath.Join(r.root, "ev")); err == nil {
		out, _ := layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Write", map[string]any{"file_path": filepath.Join(r.root, "ev", "s", "G1.json")}, false))
		if out.Decision != hookio.DecisionDeny {
			t.Errorf("symlinked ledger write allowed: %+v", out)
		}
	}
	// Bash commands naming the store are denied even without a contract.
	out, _ := layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Bash", map[string]any{"command": "SAGA_APPROVAL_DIR=/tmp/x saga gate check"}, false))
	if out.Decision != hookio.DecisionDeny {
		t.Errorf("store override allowed: %+v", out)
	}
	// An ordinary write with no contract is allowed.
	out, _ = layer.Run(ctx, hookIn(hookio.EventPreToolUse, "Write", map[string]any{"file_path": filepath.Join(r.root, "src", "a.txt")}, false))
	if out.Decision != hookio.DecisionAllow {
		t.Errorf("plain write denied: %+v", out)
	}
}

func TestHumanActRefusedWhileToolInFlight(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	layer := &Layer{Store: r.store}
	ctx := context.Background()
	in := hookIn(hookio.EventPreToolUse, "Bash", map[string]any{"command": "ls"}, false)
	in.ToolUseID = "tu-1"
	if _, err := layer.Run(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := ToolInFlight(r.store, InFlightMaxAge); !ok {
		t.Fatal("PreToolUse Bash did not record an in-flight call")
	}
	if _, err := Approve(r.load(), nil, false); cli.CodeOf(err) != cli.ExitRefusal || !strings.Contains(err.Error(), "in flight") {
		t.Errorf("approve during a tool call: %v", err)
	}
	if _, err := Check(r.load(), CheckOptions{Approve: true}); cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("check --approve during a tool call: %v", err)
	}
	if _, err := Check(r.load(), CheckOptions{CI: true, Reverify: true}); cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("reverify --ci during a tool call: %v", err)
	}
	// A plain check is the agent's to run.
	if _, err := Check(r.load(), CheckOptions{}); err != nil {
		t.Errorf("plain check refused: %v", err)
	}
	post := hookIn(hookio.EventPostToolUse, "Bash", map[string]any{"command": "ls"}, false)
	post.ToolUseID = "tu-1"
	if _, err := layer.Run(ctx, post); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := ToolInFlight(r.store, InFlightMaxAge); ok {
		t.Fatal("PostToolUse did not clear the in-flight call")
	}
	if _, err := Approve(r.load(), nil, false); err != nil {
		t.Errorf("approve when idle: %v", err)
	}
	// A stale marker (harness died mid-tool) expires.
	if _, err := layer.Run(ctx, in); err != nil {
		t.Fatal(err)
	}
	g, _ := ReadGateSession(r.store, "sess1")
	g.UpdatedNS = time.Now().Add(-2 * InFlightMaxAge).UnixNano()
	raw, _ := json.Marshal(g)
	if err := os.WriteFile(gateSessionPath(r.store, "sess1"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(r.load(), nil, false); err != nil {
		t.Errorf("stale in-flight marker still blocks: %v", err)
	}
	// Reverify --ci refuses under an environment marker as well.
	t.Setenv("CLAUDECODE", "1")
	if _, err := Check(r.load(), CheckOptions{CI: true, Reverify: true}); cli.CodeOf(err) != cli.ExitRefusal {
		t.Errorf("reverify --ci from an agent shell: %v", err)
	}
}

func TestApprovalStoreMustMatchHarnessEnvironment(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	human := os.Getenv(ApprovalEnv)
	// The hook (harness environment) records the human's store.
	layer := &Layer{Store: r.store}
	if _, err := layer.Run(context.Background(), hookIn(hookio.EventSessionStart, "", nil, false)); err != nil {
		t.Fatal(err)
	}
	// The agent points check at a store of its own: refused, exit 6.
	forged := filepath.Join(t.TempDir(), "forged")
	if err := os.MkdirAll(forged, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ApprovalEnv, forged)
	if _, err := Check(r.load(), CheckOptions{}); cli.CodeOf(err) != cli.ExitEnvironment {
		t.Errorf("forged store accepted: %v", err)
	}
	t.Setenv(ApprovalEnv, human)
	if _, err := Check(r.load(), CheckOptions{}); err != nil {
		t.Errorf("matching store refused: %v", err)
	}
}

func TestCWDResolvingOutsideRepoFails(t *testing.T) {
	r := newRepo(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(r.root, "esc")); err != nil {
		t.Skip("symlinks unavailable")
	}
	r.write(".saga/contract.md", "# Contract: s\n\nIN: src/**\n\n- [ ] G1: o\n    CHECK: pwd\n    EXPECT: /\n    CWD: esc\n")
	r.commit("base")
	rep := r.check(CheckOptions{Approve: true, NoRequireRed: true})
	gs := gateState(rep, "G1")
	if gs.State != StateUnmet || gs.Failure == nil || *gs.Failure != "start-failure" {
		t.Fatalf("CWD through a symlink out of the repo ran: %+v", gs)
	}
}

func TestSymlinkedLedgerDirectoryIsRefused(t *testing.T) {
	r := newRepo(t)
	r.write("marker.txt", "no\n")
	r.write(".saga/contract.md", minimalContract)
	r.commit("base")
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(r.root, ".saga", "evidence")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err := Check(r.load(), CheckOptions{Approve: true, NoRequireRed: true})
	if cli.CodeOf(err) != cli.ExitEnvironment {
		t.Fatalf("symlinked evidence dir: %v", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("checker wrote through the symlink: %v", entries)
	}
}

func TestTestDelInPlaceAndMoreLanguages(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", "# Contract: g\n\nIN: **\n\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n")
	r.write("tests/a.test.ts", "it('one', () => {});\nit('two', () => {});\n")
	r.write("tests/lib.rs", "#[test]\nfn a() {}\n#[test]\nfn b() {}\n")
	r.write("src/test/FooTest.java", "@Test\nvoid a() {}\n@Test\nvoid b() {}\n")
	r.write("FooTests.swift", "func testA() {}\nfunc testB() {}\n")
	r.write("test/foo_test.dart", "test('a', () {});\ntest('b', () {});\n")
	r.commit("base")
	// Emptied in place: a deletion that kept its name.
	r.write("tests/a.test.ts", "")
	// Skip markers per language.
	r.write("tests/lib.rs", "#[test]\n#[ignore]\nfn a() {}\n#[test]\nfn b() {}\n")
	r.write("src/test/FooTest.java", "@Disabled\n@Test\nvoid a() {}\n@Test\nvoid b() {}\n")
	r.write("FooTests.swift", "func testA() { throw XCTSkip(\"x\") }\nfunc testB() {}\n")
	r.write("test/foo_test.dart", "test('a', () {}, skip: true);\ntest('b', () {});\n")
	fs := findings(t, r, false)
	td := byID(fs, "G-TESTDEL")
	if len(td) != 1 || td[0].Rule != "test-decl-drop" || td[0].Pre != 2 || td[0].Post != 0 {
		t.Errorf("in-place drop: %+v", td)
	}
	skips := map[string]bool{}
	for _, f := range byID(fs, "G-SKIP") {
		skips[f.Path] = true
	}
	for _, p := range []string{"tests/lib.rs", "src/test/FooTest.java", "FooTests.swift", "test/foo_test.dart"} {
		if !skips[p] {
			t.Errorf("G-SKIP missed %s: %+v", p, byID(fs, "G-SKIP"))
		}
	}
}

func TestScopeWaiverBoundToContent(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", guardContract)
	r.write("README.md", "a\n")
	r.commit("base")
	r.write("README.md", "b\n")
	f1 := byID(findings(t, r, false), "G-SCOPE")
	if len(f1) != 1 {
		t.Fatalf("expected one scope finding: %+v", f1)
	}
	r.write(".saga/contract.md", guardContract+"\nWAIVE: G-SCOPE README.md "+f1[0].Hunk+" the readme edit was requested by the user in the ticket\n")
	if f := byID(findings(t, r, false), "G-SCOPE"); len(f) != 1 || !f[0].Waived {
		t.Fatalf("waiver not applied: %+v", f)
	}
	// A later, different change to the same file is not covered.
	r.write("README.md", "c\n")
	if f := byID(findings(t, r, false), "G-SCOPE"); len(f) != 1 || f[0].Waived {
		t.Fatalf("waiver covered a later change: %+v", f)
	}
}

func TestHookReasonIsCleanedAndCapped(t *testing.T) {
	r := newRepo(t)
	r.write(".saga/contract.md", "# Contract: g\n\nIN: src/**\n\n- [ ] G1: o\n    CHECK: true\n    EXPECT: x\n")
	r.commit("base")
	layer := &Layer{Store: r.store}
	bad := filepath.Join(r.root, "docs", "a‮bc\x1b[31m.md")
	out, _ := layer.Run(context.Background(), hookIn(hookio.EventPreToolUse, "Write", map[string]any{"file_path": bad}, false))
	if out.Decision != hookio.DecisionDeny || strings.ContainsAny(out.Reason, "‮\x1b") {
		t.Fatalf("reason not cleaned: %q", out.Reason)
	}
}

func TestParentHarnessClassifier(t *testing.T) {
	for cmd, want := range map[string]string{
		"/Users/x/.local/share/claude/versions/2.1.258":               "claude",
		"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js": "anthropic-ai/claude-code",
		"codex exec":       "codex",
		"/opt/bin/gemini":  "gemini",
		"/bin/zsh -l":      "",
		"tmux":             "",
		"python3 build.py": "",
	} {
		if got := harnessOf(cmd); got != want {
			t.Errorf("%q: got %q want %q", cmd, got, want)
		}
	}
}

// Every contract in the bench corpus must parse, hash its prompt.md as
// REQUEST:, and cite spans that occur in the numbered sentences (rows 11
// and 12) under SEGMENTER v1.
func TestBenchCorpusContractsParse(t *testing.T) {
	dirs, _ := filepath.Glob(filepath.Join("..", "..", "bench", "tasks", "*", "contract.md"))
	if len(dirs) == 0 {
		t.Skip("bench corpus not present")
	}
	for _, cp := range dirs {
		raw, err := os.ReadFile(cp)
		if err != nil {
			t.Fatal(err)
		}
		c, err := Parse(raw)
		if err != nil {
			t.Errorf("%s: %v", cp, err)
			continue
		}
		req, err := os.ReadFile(filepath.Join(filepath.Dir(cp), "prompt.md"))
		if err != nil {
			t.Errorf("%s: no prompt.md", cp)
			continue
		}
		if _, err := VerifyRequest(c, req, canon.SHA256(req)); err != nil {
			t.Errorf("%s: %v", cp, err)
		}
		if len(Lint(c)) > 0 {
			t.Logf("%s: lint warnings %+v", cp, Lint(c))
		}
	}
}
