package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The tree hash must be the git tree over tracked plus untracked-not-
// ignored files, whether the temporary index was seeded from the real
// index (fast path) or from HEAD, and ignored files must never reach it
// (they would leak through refs/saga/snap into the object store).
func TestTreeMatchesGitAndSkipsIgnored(t *testing.T) {
	root := t.TempDir()
	run(t, root, "init", "-q")
	run(t, root, "config", "user.email", "t@t")
	run(t, root, "config", "user.name", "t")
	run(t, root, "config", "commit.gpgsign", "false")
	write(t, root, ".gitignore", "secret.env\nbuild/\n")
	// `saga init` ignores snap/ so the last-snapshot record never moves the tree.
	write(t, root, ".saga/.gitignore", "snap/\n")
	write(t, root, "a.txt", "a\n")
	write(t, root, "b.txt", "b\n")
	run(t, root, "add", "-A")
	run(t, root, "commit", "-q", "-m", "init")
	head := run(t, root, "rev-parse", "HEAD^{tree}")
	got, err := Tree(root)
	if err != nil || got != "tree:"+head {
		t.Fatalf("clean tree: %s %v want tree:%s", got, err, head)
	}
	// Modify, delete, add untracked, add ignored; the real index is stale.
	write(t, root, "a.txt", "a2\n")
	if err := os.Remove(filepath.Join(root, "b.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "new/c.txt", "c\n")
	write(t, root, "secret.env", "TOKEN=leak\n")
	write(t, root, "build/out.bin", "bin\n")
	got, err = Tree(root)
	if err != nil {
		t.Fatal(err)
	}
	oid := strings.TrimPrefix(got, "tree:")
	ls := run(t, root, "ls-tree", "-r", "--name-only", oid)
	names := strings.Split(ls, "\n")
	want := []string{".gitignore", ".saga/.gitignore", "a.txt", "new/c.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tree entries %v, want %v", names, want)
	}
	if strings.Contains(ls, "secret.env") || strings.Contains(ls, "build/") {
		t.Fatalf("ignored file leaked into the tree: %s", ls)
	}
	// The blob of a.txt is the modified content, not the stale index entry.
	if blob := run(t, root, "show", oid+":a.txt"); blob != "a2" {
		t.Fatalf("stale index content used: %q", blob)
	}
	// The real index was not touched.
	if st := run(t, root, "diff", "--cached", "--name-only"); st != "" {
		t.Fatalf("real index modified: %s", st)
	}
	// Excludes drop paths.
	got2, err := Tree(root, "new")
	if err != nil || got2 == got {
		t.Fatalf("exclude had no effect: %s %v", got2, err)
	}
	// Take pins a ref and dedupes an unchanged tree.
	s1, err := Take(root, "gate", "s", 1)
	if err != nil || !s1.Taken {
		t.Fatalf("take: %+v %v", s1, err)
	}
	s2, err := Take(root, "gate", "s", 2)
	if err != nil || s2.Taken || s2.ID != s1.ID {
		t.Fatalf("second take on an unchanged tree: %+v %v", s2, err)
	}
	if ref := run(t, root, "rev-parse", "refs/saga/snap/s/1"); ref != oid {
		t.Fatalf("ref %s want %s", ref, oid)
	}
}
