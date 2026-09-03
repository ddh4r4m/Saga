package gate

import (
	"path"
	"strings"
)

// Glob matching for IN:, OUT:, WITNESS:, test_globs and scope_exempt.
// The dialect is the usual one: `*` and `?` inside one path segment,
// `**` for zero or more whole segments, `{a,b}` is not supported because
// glob lists are comma-separated (gate-spec section 2.2). Matching is on
// slash-separated repo-relative paths; fold makes it case-insensitive.

// MatchGlob reports whether p matches pattern.
func MatchGlob(pattern, p string, fold bool) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	p = strings.TrimPrefix(p, "./")
	if fold {
		pattern, p = strings.ToLower(pattern), strings.ToLower(p)
	}
	if pattern == "" {
		return false
	}
	// A bare directory pattern (no metacharacters, trailing slash or an
	// existing prefix) matches everything under it.
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], segs[0])
		if err != nil || !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// MatchAny reports whether p matches at least one pattern.
func MatchAny(patterns []string, p string, fold bool) bool {
	for _, g := range patterns {
		if MatchGlob(g, p, fold) {
			return true
		}
	}
	return false
}

// SplitGlobs splits a comma-separated glob list, trimming each entry and
// dropping empties.
func SplitGlobs(v string) []string {
	var out []string
	for _, g := range strings.Split(v, ",") {
		g = strings.TrimSpace(g)
		if g != "" {
			out = append(out, g)
		}
	}
	return out
}
