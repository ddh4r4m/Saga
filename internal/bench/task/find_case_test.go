package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// caseInsensitiveFS reports whether the filesystem under dir opens a
// name spelled with a different case. It is asked at test time rather
// than assumed from runtime.GOOS: a macOS volume can be case-sensitive
// and a Linux one can be mounted case-insensitive.
func caseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// TestFindResolvesThroughTheFilesystem pins the behaviour the 2026-09-06
// launcher failure was blamed on: a task referenced through a path whose
// case differs from the on-disk name resolves wherever the filesystem
// itself would open it. A task set is identified by ids and content
// hashes, so the spelling of the path a run was launched from must not
// change what runs. The behaviour already held on the machine this was
// written on, and the test exists so it cannot quietly stop holding.
func TestFindResolvesThroughTheFilesystem(t *testing.T) {
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if !caseInsensitiveFS(t, root) {
		t.Skip("case-sensitive filesystem: a differently spelled path is a different path here, and must stay one")
	}
	// A corpus of one task, under a directory with an upper-case letter.
	corpus := filepath.Join(root, "Bench", "tasks")
	dir := filepath.Join(corpus, "py-0001-example")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.toml"), []byte("id = \"py-0001-example\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lower := strings.Replace(dir, filepath.Join(root, "Bench"), filepath.Join(root, "bench"), 1)
	if lower == dir {
		t.Fatal("the test built no differently spelled path")
	}
	for _, pattern := range []string{
		dir,                                      // as spelled on disk
		lower,                                    // a parent segment in another case
		filepath.Join(filepath.Dir(lower), "*"),  // a glob under that parent
		filepath.Join(root, "bench", "tasks"),    // the corpus directory itself
		filepath.Join(corpus, "PY-0001-EXAMPLE"), // the task's own name in another case
		dir + "/",                                // a trailing separator
	} {
		found, err := Find(pattern)
		if err != nil {
			t.Errorf("%s: %v", pattern, err)
			continue
		}
		if len(found) != 1 {
			t.Errorf("%s: found %v, want the one task", pattern, found)
			continue
		}
		// Whatever the spelling, it is the same directory on disk.
		a, err := os.Stat(found[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(a, b) {
			t.Errorf("%s resolved to %s, which is not the task directory", pattern, found[0])
		}
	}

	// A name that is wrong rather than differently spelled still fails.
	if found, _ := Find(filepath.Join(corpus, "py-0002-absent")); len(found) != 0 {
		t.Errorf("an absent task resolved to %v", found)
	}
}

// TestWhyNoMatchNamesTheCause: the refusal has to say what it looked at,
// or the next launcher failure needs a reproduction to diagnose.
func TestWhyNoMatchNamesTheCause(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ path, want string }{
		{filepath.Join(root, "absent"), "does not exist"},
		{file, "a file, not a task directory"},
		{empty, "no task.toml"},
		{filepath.Join(root, "*", "nothing"), "matched nothing"},
	} {
		if got := WhyNoMatch(c.path); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q does not mention %q", c.path, got, c.want)
		}
	}
}
