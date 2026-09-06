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

// SourceExtensions are the file extensions a claim's token may end in to
// count as a path when it names no directory. The list is the corpus's
// languages plus the data and document files the tasks carry.
var SourceExtensions = []string{
	".py", ".ts", ".mts", ".mjs", ".js", ".json", ".toml", ".md",
	".txt", ".sh", ".yaml", ".yml", ".csv", ".html", ".css",
}

// PathShaped reports whether a token can be a path at all: it names a
// directory, or it ends in a source extension. A dotted identifier is
// not a path however much it looks like one. "I added a per-SKU
// `threading.Lock` inside `Inventory`" was read as a claim to have
// touched a file named threading.Lock, found absent, and contradicted;
// so was `Object.freeze`. Both messages were true (2026-09-06 dev run
// finding 3).
func PathShaped(p string) bool {
	if strings.Contains(p, "/") {
		return true
	}
	lower := strings.ToLower(p)
	for _, ext := range SourceExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
