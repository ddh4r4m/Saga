package gate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/store"
)

// repo is a scratch git repository with .saga initialised.
type repo struct {
	t     *testing.T
	root  string
	store *store.Store
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	r := &repo{t: t, root: root}
	r.git("init", "-q")
	r.git("config", "user.email", "t@t")
	r.git("config", "user.name", "t")
	r.git("config", "commit.gpgsign", "false")
	if _, err := store.Init(root); err != nil {
		t.Fatal(err)
	}
	r.store = store.Open(root)
	// Approval store outside the repo, owner-private; not an agent shell.
	adir := filepath.Join(t.TempDir(), "approved")
	if err := os.MkdirAll(adir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ApprovalEnv, adir)
	for _, m := range AgentShellMarkers {
		t.Setenv(m, "")
	}
	// The suite itself may run under a harness; stand the parent-chain
	// detector down so the human-act tests exercise the other signals.
	prev := ParentHarness
	ParentHarness = func() string { return "" }
	t.Cleanup(func() { ParentHarness = prev })
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) read(rel string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *repo) remove(rel string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(r.root, filepath.FromSlash(rel))); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func (r *repo) load() *Loaded {
	r.t.Helper()
	l, err := Load(r.root, r.store)
	if err != nil {
		r.t.Fatalf("load: %v", err)
	}
	return l
}

func (r *repo) check(opts CheckOptions) *Report {
	r.t.Helper()
	rep, err := Check(r.load(), opts)
	if err != nil {
		r.t.Fatalf("check: %v", err)
	}
	return rep
}

func (r *repo) status() *Report {
	r.t.Helper()
	rep, err := Status(r.load(), StatusOptions{})
	if err != nil {
		r.t.Fatalf("status: %v", err)
	}
	return rep
}

func gateState(rep *Report, id string) *GateStatus {
	for _, g := range rep.Gates {
		if strings.HasSuffix(g.ID, ":"+id) {
			return g
		}
	}
	return nil
}

// minimalContract is a one-gate contract whose oracle reads marker.txt.
const minimalContract = `# Contract: scratch

IN: src/**, marker.txt

- [ ] G1: the marker says done
    CHECK: cat marker.txt
    EXPECT: CANARY-DONE
`
