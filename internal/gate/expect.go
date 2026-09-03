package gate

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// Expect is a compiled EXPECT: or RED-EXPECT: oracle: a plain substring
// or a `/pattern/flags` regex in the linear-time RE2 dialect with flags
// i, m and s (gate-spec section 4.1).
type Expect struct {
	Kind    string // "substring" or "regex"
	Source  string
	pattern string
	flags   string
	re      *regexp.Regexp
}

var reSlashed = regexp.MustCompile(`^/(.*)/([a-z]*)$`)

// CompileExpect parses and compiles an EXPECT: value.
func CompileExpect(v string) (*Expect, error) {
	m := reSlashed.FindStringSubmatch(v)
	if m == nil {
		return &Expect{Kind: "substring", Source: v}, nil
	}
	for _, f := range m[2] {
		if !strings.ContainsRune("ims", f) {
			return nil, fmt.Errorf("unknown regex flag %q (i, m, s are accepted)", string(f))
		}
	}
	src := m[1]
	if m[2] != "" {
		src = "(?" + m[2] + ")" + src
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %v", err)
	}
	return &Expect{Kind: "regex", Source: v, pattern: m[1], flags: m[2], re: re}, nil
}

// Match applies the oracle to the byte stream (invalid UTF-8 is replaced)
// and returns the matched span.
func (e *Expect) Match(out []byte) (matched bool, span [2]int) {
	out = bytes.ToValidUTF8(out, []byte("�"))
	if e.Kind == "substring" {
		i := bytes.Index(out, []byte(e.Source))
		if i < 0 {
			return false, span
		}
		return true, [2]int{i, i + len(e.Source)}
	}
	loc := e.re.FindIndex(out)
	if loc == nil {
		return false, span
	}
	return true, [2]int{loc[0], loc[1]}
}

// PathShaped reports whether a slash-wrapped regex looks like a file path
// (lint warning: the author probably meant a substring).
func (e *Expect) PathShaped() bool {
	if e.Kind != "regex" {
		return false
	}
	p := e.pattern
	if !strings.Contains(p, "/") {
		return false
	}
	return !strings.ContainsAny(p, "^$*+?()[]{}|\\")
}
