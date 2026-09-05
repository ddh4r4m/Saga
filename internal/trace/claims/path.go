package claims

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reLineSuffix = regexp.MustCompile(`:\d+(?::\d+)?$`)
	reExtension  = regexp.MustCompile(`\.[A-Za-z][A-Za-z0-9]{0,9}$`)
)

// NormalizePath reduces a backticked mention to a slash-separated
// repo-relative path, or "" when the mention does not normalise to a
// location inside the repository root (trace-spec section 5.6: anything
// else is dropped). root may be "" when the repository is unknown, in
// which case absolute paths are dropped and relative ones kept.
func NormalizePath(root, mention string) string {
	p := strings.TrimSpace(mention)
	p = strings.Trim(p, `"'`)
	p = strings.TrimRight(p, ".,;:)")
	p = reLineSuffix.ReplaceAllString(p, "")
	if p == "" || len(p) > 256 {
		return ""
	}
	if strings.ContainsAny(p, " \t\r\n$<>|*?\"'`") || strings.Contains(p, "://") {
		return ""
	}
	if strings.HasPrefix(p, "-") || strings.HasPrefix(p, "~") || strings.HasPrefix(p, "@") {
		return ""
	}
	p = filepath.ToSlash(p)
	if strings.HasSuffix(p, "/") {
		return ""
	}
	if filepath.IsAbs(filepath.FromSlash(p)) || strings.HasPrefix(p, "/") {
		if root == "" {
			return ""
		}
		rel, err := filepath.Rel(root, filepath.FromSlash(p))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		p = filepath.ToSlash(rel)
	}
	p = path.Clean(p)
	if p == "." || p == "" || p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	// A path is something with a directory or a file extension; a bare
	// word (`pytest`, `DONE`, `Makefile`) is not.
	if !strings.Contains(p, "/") && !reExtension.MatchString(p) {
		return ""
	}
	return p
}
