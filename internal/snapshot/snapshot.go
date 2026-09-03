// Package snapshot is the one tree-snapshot primitive of
// docs/specs/00-cross-spec-contracts.md section 5 in its git-tree mode:
// the tree object id over tracked plus untracked-not-ignored files,
// written through a temporary index and pinned under
// refs/saga/snap/<session>/<turn>. Guard owns the full command surface
// (guard-spec section 3); this is the minimal implementation gate's
// evidence records need for worktree_hash.
package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EmptyTree is git's well-known empty tree id, the base when HEAD does
// not resolve (a repository with no commits).
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// Kind is the only snapshot mode implemented at M0.
const Kind = "git-tree"

// Snapshot is the section 5 result of `saga snapshot take`.
type Snapshot struct {
	ID       string `json:"id"`
	TreeHash string `json:"tree_hash"`
	Kind     string `json:"kind"`
	Session  string `json:"session"`
	Turn     int    `json:"turn"`
	Taken    bool   `json:"taken"`
	Reason   string `json:"reason"`
	At       string `json:"at"`
}

// ErrNotRepo is returned when root is not inside a git work tree.
var ErrNotRepo = errors.New("snapshot: not a git repository")

func git(root string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// IsRepo reports whether root is inside a git work tree.
func IsRepo(root string) bool {
	out, err := git(root, nil, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// Tree computes the tree id over tracked plus untracked-not-ignored
// files through a temporary index, without touching the real index.
// Paths under exclude are dropped from the tree; gate excludes its own
// ledger so a checker write does not move the hash it just recorded.
func Tree(root string, exclude ...string) (string, error) {
	if !IsRepo(root) {
		return "", ErrNotRepo
	}
	dir, err := os.MkdirTemp("", "saga-snap-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	tmpIndex := filepath.Join(dir, "index")
	env := []string{"GIT_INDEX_FILE=" + tmpIndex}
	// Seed the temporary index from a copy of the real index: its stat
	// cache lets `git add -A` re-hash only the files that changed, which
	// on a 20k-file tree is the difference between about 0.5 s and 4 s per
	// snapshot. Without a real index (or when the copy fails) fall back to
	// HEAD's tree, which represents deletions and modes but hashes
	// everything.
	seeded := false
	if out, err := git(root, nil, "rev-parse", "--git-path", "index"); err == nil {
		src := strings.TrimSpace(string(out))
		if !filepath.IsAbs(src) {
			src = filepath.Join(root, src)
		}
		if raw, err := os.ReadFile(src); err == nil && len(raw) > 0 {
			seeded = os.WriteFile(tmpIndex, raw, 0o600) == nil
		}
	}
	if !seeded {
		if _, err := git(root, nil, "rev-parse", "--verify", "HEAD^{tree}"); err == nil {
			if _, err := git(root, env, "read-tree", "HEAD"); err != nil {
				return "", err
			}
		}
	}
	if _, err := git(root, env, "add", "-A", "--", "."); err != nil {
		return "", err
	}
	if len(exclude) > 0 {
		args := append([]string{"rm", "-r", "-q", "--cached", "--ignore-unmatch", "--"}, exclude...)
		if _, err := git(root, env, args...); err != nil {
			return "", err
		}
	}
	out, err := git(root, env, "write-tree")
	if err != nil {
		return "", err
	}
	return "tree:" + strings.TrimSpace(string(out)), nil
}

// Take snapshots the working tree for session and turn. An unchanged tree
// (same tree id as the previous snapshot of the session) returns the
// previous snapshot with Taken = false.
func Take(root, reason, session string, turn int, exclude ...string) (*Snapshot, error) {
	tree, err := Tree(root, exclude...)
	if err != nil {
		return nil, err
	}
	if session == "" {
		session = "local"
	}
	oid := strings.TrimPrefix(tree, "tree:")
	snap := &Snapshot{TreeHash: tree, Kind: Kind, Session: session, Turn: turn, Reason: reason, At: time.Now().UTC().Format(time.RFC3339)}
	snap.ID = fmt.Sprintf("snap:%s:%d:%s", session, turn, oid[:12])
	last := filepath.Join(root, ".saga", "snap", "last-"+safe(session)+".json")
	if raw, err := os.ReadFile(last); err == nil {
		var prev Snapshot
		if json.Unmarshal(raw, &prev) == nil && prev.TreeHash == tree {
			prev.Taken = false
			return &prev, nil
		}
	}
	ref := fmt.Sprintf("refs/saga/snap/%s/%d", safe(session), turn)
	if _, err := git(root, nil, "update-ref", ref, oid); err != nil {
		return nil, err
	}
	snap.Taken = true
	if err := os.MkdirAll(filepath.Dir(last), 0o700); err == nil {
		if raw, err := json.Marshal(snap); err == nil {
			_ = os.WriteFile(last, raw, 0o600)
		}
	}
	return snap, nil
}

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

// Head returns the abbreviated HEAD commit, or "" when there is none.
func Head(root string) string {
	out, err := git(root, nil, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
