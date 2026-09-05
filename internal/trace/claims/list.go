// Package claims implements claim verification (trace-spec sections 5.5
// to 5.9): detection of completion claims in the harness-visible final
// assistant message against the versioned claims.txt, reconciliation of
// every claim against the session record (tool calls, tool results,
// gate evidence, the working tree diff), the verdict and the Stop-chain
// decision. Nothing here runs a command or consults a model: every
// verdict is a lookup in the record.
package claims

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
)

//go:embed claims.txt
var claimsListText string

//go:embed abstain.txt
var abstainListText string

// ClaimsList is the embedded claims.txt, verbatim.
func ClaimsList() string { return claimsListText }

// AbstainList is the embedded abstain.txt, verbatim.
func AbstainList() string { return abstainListText }

// ClaimsListHash is sha256 of claims.txt, recorded in every claim event
// and in the bench manifest.
var ClaimsListHash = canon.SHA256([]byte(claimsListText))

// AbstainListHash is sha256 of abstain.txt.
var AbstainListHash = canon.SHA256([]byte(abstainListText))

// Claim kinds of trace-spec section 5.6.
const (
	KindDone      = "done"
	KindGateMet   = "gate_met"
	KindTestsPass = "tests_pass"
	KindRan       = "ran"
	KindTouched   = "touched"
	KindRead      = "read"
)

// Kinds lists the six claim kinds in table order.
var Kinds = []string{KindDone, KindGateMet, KindTestsPass, KindRan, KindTouched, KindRead}

// Pattern classes of claims.txt.
const (
	ClassStructural = "structural"
	ClassLexical    = "lexical"
	ClassNegative   = "negative"
	ClassGuard      = "guard"
	ClassGuardLine  = "guard_line"
)

// Rule is one parsed line of claims.txt.
type Rule struct {
	Kind  string
	Class string
	Re    *regexp.Regexp
	// Line is the 1-based line in claims.txt, for diagnostics.
	Line int
}

// List is the parsed claims.txt and abstain.txt.
type List struct {
	Rules   []Rule
	Abstain []*regexp.Regexp
	Hash    string
	// AbstainHash is sha256 of abstain.txt.
	AbstainHash string
}

// ParseList parses a claims list and an abstain list. An unreadable or
// invalid list is the exit-2 case of section 5.9 (fail closed).
func ParseList(claimsText, abstainText string) (*List, error) {
	l := &List{Hash: canon.SHA256([]byte(claimsText)), AbstainHash: canon.SHA256([]byte(abstainText))}
	kinds := map[string]bool{}
	for _, k := range Kinds {
		kinds[k] = true
	}
	kinds["not_done"] = true
	for i, line := range strings.Split(claimsText, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("claims.txt line %d: want <kind>\\t<class>\\t<regex>", i+1)
		}
		kind, class, pat := parts[0], parts[1], parts[2]
		if !kinds[kind] {
			return nil, fmt.Errorf("claims.txt line %d: unknown kind %q", i+1, kind)
		}
		switch class {
		case ClassStructural, ClassLexical, ClassNegative, ClassGuard, ClassGuardLine:
		default:
			return nil, fmt.Errorf("claims.txt line %d: unknown class %q", i+1, class)
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("claims.txt line %d: %v", i+1, err)
		}
		l.Rules = append(l.Rules, Rule{Kind: kind, Class: class, Re: re, Line: i + 1})
	}
	if len(l.Rules) == 0 {
		return nil, fmt.Errorf("claims.txt: no rules")
	}
	for i, line := range strings.Split(abstainText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		re, err := regexp.Compile(line)
		if err != nil {
			return nil, fmt.Errorf("abstain.txt line %d: %v", i+1, err)
		}
		l.Abstain = append(l.Abstain, re)
	}
	return l, nil
}

var (
	defaultList    *List
	defaultListErr error
)

func init() {
	defaultList, defaultListErr = ParseList(claimsListText, abstainListText)
}

// Default returns the embedded lists, or the parse error that makes every
// verdict exit 2.
func Default() (*List, error) { return defaultList, defaultListErr }

func (l *List) rules(kind, class string) []Rule {
	var out []Rule
	for _, r := range l.Rules {
		if r.Kind == kind && r.Class == class {
			out = append(out, r)
		}
	}
	return out
}
