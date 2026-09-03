package task

import (
	"regexp"
	"strings"
	"sync"
)

var (
	globMu    sync.Mutex
	globCache = map[string]*regexp.Regexp{}
)

// GlobMatch matches a repo-relative path against a contract glob:
// "**" spans directories, "*" and "?" stay inside one segment, and a
// bare directory prefix ("src/legacy") matches everything below it.
func GlobMatch(pattern, path string) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if pattern == "" {
		return false
	}
	if !strings.ContainsAny(pattern, "*?[") && !strings.Contains(pattern, ".") {
		pattern = strings.TrimSuffix(pattern, "/") + "/**"
	}
	globMu.Lock()
	re, ok := globCache[pattern]
	if !ok {
		re = regexp.MustCompile("^" + globToRegexp(pattern) + "$")
		globCache[pattern] = re
	}
	globMu.Unlock()
	return re.MatchString(path)
}

func globToRegexp(g string) string {
	var b strings.Builder
	for i := 0; i < len(g); i++ {
		c := g[i]
		switch {
		case c == '*' && i+1 < len(g) && g[i+1] == '*':
			i++
			if i+1 < len(g) && g[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// InScope reports whether path is allowed by the contract: it must not
// match any OUT glob, and when IN globs exist it must match one.
func InScope(in, out []string, path string) bool {
	for _, g := range out {
		if GlobMatch(g, path) {
			return false
		}
	}
	if len(in) == 0 {
		return true
	}
	for _, g := range in {
		if GlobMatch(g, path) {
			return true
		}
	}
	return false
}
