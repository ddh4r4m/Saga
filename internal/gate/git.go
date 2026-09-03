package gate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ddh4r4m/saga/internal/snapshot"
)

// gitOut runs git in root and returns stdout.
func gitOut(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// ResolveBase turns BASE: (or HEAD when absent) into a full object id.
// A repository with no commits resolves to the empty tree.
func ResolveBase(root, base string) (string, error) {
	if base == "" {
		base = "HEAD"
	}
	out, err := gitOut(root, "rev-parse", "--verify", "--quiet", base+"^{tree}")
	if err != nil {
		if base == "HEAD" {
			return snapshot.EmptyTree, nil
		}
		return "", fmt.Errorf("BASE: %s does not resolve", base)
	}
	// Keep the commit id when base is a commit so `git show base:path`
	// and rev-list work; the tree is used only as a fallback.
	if c, err := gitOut(root, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err == nil {
		return strings.TrimSpace(string(c)), nil
	}
	return strings.TrimSpace(string(out)), nil
}

// ShortRev abbreviates an object id for display.
func ShortRev(id string) string {
	if len(id) > 7 {
		return id[:7]
	}
	return id
}

// ShowAt returns the content of path at rev, and ok = false when the path
// is not tracked there.
func ShowAt(root, rev, path string) ([]byte, bool) {
	out, err := gitOut(root, "show", rev+":"+path)
	if err != nil {
		return nil, false
	}
	return out, true
}

// TrackedAt reports whether path exists in rev's tree.
func TrackedAt(root, rev, path string) bool {
	_, ok := ShowAt(root, rev, path)
	return ok
}

// DiffEntry is one changed path between base and the working tree.
type DiffEntry struct {
	Status  string // A, M, D, R, T (untracked files are A)
	Path    string
	OldPath string // for renames
	// Untracked marks an untracked-not-ignored file (never in the base):
	// work for the guards, but not a tracked change for the baseline
	// red-proof condition.
	Untracked bool
}

// TrackedDiffEmpty reports whether no tracked change in entries touches an
// IN: glob (the baseline condition of section 3.2 over a cached diff).
func TrackedDiffEmpty(entries []DiffEntry, globs []string, fold bool) bool {
	for _, e := range entries {
		if e.Untracked {
			continue
		}
		for _, p := range []string{e.OldPath, e.Path} {
			if p != "" && MatchAny(globs, p, fold) {
				return false
			}
		}
	}
	return true
}

// DiffPaths lists changed paths against base: `git diff --find-renames
// --name-status -z <base>` plus untracked-not-ignored files as added.
// Paths are slash-separated repo-relative.
func DiffPaths(root, base string, pathspec ...string) ([]DiffEntry, error) {
	args := []string{"diff", "--find-renames", "--name-status", "-z", base}
	if len(pathspec) > 0 {
		args = append(args, "--")
		args = append(args, pathspec...)
	}
	out, err := gitOut(root, args...)
	if err != nil {
		return nil, err
	}
	var entries []DiffEntry
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		st := fields[i]
		if st == "" {
			continue
		}
		code := st[:1]
		if code == "R" || code == "C" {
			if i+2 >= len(fields) {
				break
			}
			entries = append(entries, DiffEntry{Status: "R", OldPath: fields[i+1], Path: fields[i+2]})
			i += 2
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		entries = append(entries, DiffEntry{Status: code, Path: fields[i+1]})
		i++
	}
	args = []string{"ls-files", "--others", "--exclude-standard", "-z"}
	if len(pathspec) > 0 {
		args = append(args, "--")
		args = append(args, pathspec...)
	}
	out, err = gitOut(root, args...)
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			entries = append(entries, DiffEntry{Status: "A", Path: p, Untracked: true})
		}
	}
	return entries, nil
}

// DiffEmpty reports whether `git diff <base> -- <IN globs>` is empty, the
// baseline red-proof condition of section 3.2. It is over tracked
// changes only, as the spec states it: an oracle's own by-products
// (caches, build output) must not count as work done.
func DiffEmpty(root, base string, globs []string, fold bool) (bool, error) {
	out, err := gitOut(root, "diff", "--name-only", "-z", base, "--", ".")
	if err != nil {
		return false, err
	}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" && MatchAny(globs, p, fold) {
			return false, nil
		}
	}
	return true, nil
}

// CommitsBehind counts commits from rev to HEAD (red_ttl_commits).
func CommitsBehind(root, rev string) int {
	out, err := gitOut(root, "rev-list", "--count", rev+"..HEAD")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

// IsSymlink reports whether the working-tree path is a symlink.
func IsSymlink(root, rel string) bool {
	fi, err := os.Lstat(join(root, rel))
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// errNotRepo wraps snapshot.ErrNotRepo for callers that map to exit 6.
var errNotRepo = errors.New("gate: not a git repository")
