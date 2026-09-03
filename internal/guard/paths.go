package guard

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ddh4r4m/saga/internal/gate"
)

// scope is the resolved path model of guard-spec 2.4: the repository
// root plus scope.extra are writable; $HOME, /, drive roots, the parent
// of the repo root and credential paths are always out of scope.
type scope struct {
	root    string // canonical repo root, "" when the root itself is protected
	rawRoot string // repo root before the protection check
	home    string // canonical $HOME
	extra   []string
	creds   []string
	allowRd []string
	fold    bool
	saga    []string // D11 paths, canonical
}

// foldCase reports whether paths compare case-insensitively.
func foldCase() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

// canon makes p absolute against cwd, cleans it and follows symlinks on
// the longest existing prefix, so /tmp and /private/tmp compare equal.
func canonPath(cwd, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	dir, base := filepath.Split(p)
	dir = filepath.Clean(dir)
	if dir == p {
		return p
	}
	return filepath.Join(canonPath(cwd, dir), base)
}

func newScope(cwd, home, root string, pol *Policy) *scope {
	s := &scope{fold: foldCase()}
	s.home = canonPath(cwd, home)
	s.rawRoot = canonPath(cwd, root)
	// The repository itself is writable unless the "repository" is HOME
	// or a filesystem root (no .git or .saga found above cwd).
	if !isDriveOrFSRoot(s.rawRoot) && !s.eq(s.rawRoot, s.home) {
		s.root = s.rawRoot
	}
	for _, e := range pol.Scope.Extra {
		s.extra = append(s.extra, expandTilde(e, home))
	}
	for _, c := range pol.CredentialPaths() {
		s.creds = append(s.creds, expandTilde(c, home))
	}
	for _, a := range pol.Credentials.AllowRead {
		s.allowRd = append(s.allowRd, canonPath(s.rawRoot, expandTilde(a, home)))
	}
	for _, rel := range []string{".saga/policy.toml", ".saga/guard.toml", ".saga/route.toml", ".saga/manifest.json", ".saga/.gitignore", ".claude/settings.json", ".claude/settings.local.json", ".gemini/settings.json", ".codex/hooks.json", ".git/refs/saga"} {
		s.saga = append(s.saga, filepath.Join(s.rawRoot, filepath.FromSlash(rel)))
	}
	s.saga = append(s.saga, filepath.Join(s.home, ".claude", "settings.json"), filepath.Join(s.home, ".codex", "hooks.json"), filepath.Join(s.home, ".gemini", "settings.json"))
	return s
}

func expandTilde(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func (s *scope) eq(a, b string) bool {
	if s.fold {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// under reports whether p is dir or lies below it.
func (s *scope) under(p, dir string) bool {
	if s.eq(p, dir) {
		return true
	}
	if dir == string(filepath.Separator) {
		return strings.HasPrefix(p, dir)
	}
	pre := dir + string(filepath.Separator)
	if s.fold {
		return len(p) >= len(pre) && strings.EqualFold(p[:len(pre)], pre)
	}
	return strings.HasPrefix(p, pre)
}

// isDriveOrFSRoot reports "/" and Windows drive roots.
func isDriveOrFSRoot(p string) bool {
	if p == "/" {
		return true
	}
	if vol := filepath.VolumeName(p); vol != "" {
		rest := strings.TrimPrefix(p, vol)
		return rest == "" || rest == `\` || rest == "/"
	}
	return false
}

// isProtectedRoot reports the D1 roots: /, a drive root, $HOME, the repo
// root and every ancestor of the repo root.
func (s *scope) isProtectedRoot(p string) bool {
	if isDriveOrFSRoot(p) || s.eq(p, s.home) {
		return true
	}
	if s.rawRoot != "" && !isDriveOrFSRoot(s.rawRoot) && !s.eq(s.rawRoot, s.home) {
		if s.eq(p, s.rawRoot) || s.under(s.rawRoot, p) {
			return true
		}
	}
	return false
}

// protectedName says which D1 root p is, for the reason string.
func (s *scope) protectedName(p string) string {
	switch {
	case isDriveOrFSRoot(p):
		return "filesystem root"
	case s.eq(p, s.home):
		return "HOME"
	case s.eq(p, s.rawRoot):
		return "repo root"
	case s.under(s.rawRoot, p):
		return "ancestor of repo root"
	}
	return ""
}

// inScope reports whether p may be written without a prompt.
func (s *scope) inScope(p string) bool {
	if s.isCredential(p) {
		return false
	}
	if s.root != "" && s.under(p, s.root) {
		return true
	}
	for _, e := range s.extra {
		if s.matchesRoot(e, p) {
			return true
		}
	}
	return false
}

// matchesRoot reports whether p is at or below a directory matching the
// glob pattern (scope.extra entries such as /tmp/saga-*).
func (s *scope) matchesRoot(pattern, p string) bool {
	for q := p; ; q = filepath.Dir(q) {
		if s.globPath(pattern, q) {
			return true
		}
		if filepath.Dir(q) == q {
			return false
		}
	}
}

// globPath matches an absolute or **-prefixed slash pattern against an
// absolute path.
func (s *scope) globPath(pattern, p string) bool {
	pattern = filepath.ToSlash(pattern)
	q := filepath.ToSlash(p)
	if vol := filepath.VolumeName(p); vol != "" {
		q = strings.TrimPrefix(q, filepath.ToSlash(vol))
	}
	q = strings.TrimPrefix(q, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	return gate.MatchGlob(pattern, q, s.fold)
}

// isCredential reports a credential_paths match not covered by allow_read.
func (s *scope) isCredential(p string) bool {
	for _, a := range s.allowRd {
		if s.eq(a, p) {
			return false
		}
	}
	for _, c := range s.creds {
		if s.globPath(c, p) {
			return true
		}
	}
	return false
}

// credentialPattern reports whether a basename pattern (from find -name)
// would select credential files: id_rsa, *.env, *.pem, credentials.
func (s *scope) credentialPattern(pat string) bool {
	for _, c := range s.creds {
		base := path.Base(filepath.ToSlash(c))
		if hasMeta(pat) && !hasMeta(base) {
			if ok, _ := path.Match(pat, base); ok {
				return true
			}
		} else if !hasMeta(pat) {
			if ok, _ := path.Match(base, pat); ok {
				return true
			}
		}
	}
	return false
}

// isSagaProtected reports the D11 paths.
func (s *scope) isSagaProtected(p string) bool {
	for _, q := range s.saga {
		if s.eq(p, q) || s.under(p, q) {
			return true
		}
	}
	return false
}

// isDevice reports block or raw device paths for D9.
func isDevice(p string) bool {
	if !strings.HasPrefix(p, "/dev/") {
		return false
	}
	base := strings.TrimPrefix(p, "/dev/")
	for _, pre := range []string{"sd", "hd", "vd", "xvd", "nvme", "disk", "rdisk", "mmcblk", "md", "dm-", "mapper/", "loop", "zd"} {
		if strings.HasPrefix(base, pre) {
			return true
		}
	}
	return false
}

func hasMeta(s string) bool { return strings.ContainsAny(s, "*?[") }

// isDir reports an existing directory (read-only stat).
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// repoRootFor finds the repository containing cwd: the first ancestor
// with .saga or .git, else cwd itself (store.RepoRoot semantics without
// the import cycle risk).
func repoRootFor(cwd string) string {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".saga")); err == nil {
			return d
		}
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

// RepoRoot is the exported repository discovery used by the CLI.
func RepoRoot(cwd string) string { return repoRootFor(cwd) }
