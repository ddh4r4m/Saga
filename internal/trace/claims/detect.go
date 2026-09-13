package claims

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MaxClaims caps claims per message (trace-spec section 5.6).
const MaxClaims = 64

// Claim is one detected claim. Text is never stored: Span cites byte
// offsets into the final-message blob; Path and Command are normalised.
type Claim struct {
	Kind  string `json:"kind"`
	Class string `json:"class"`
	Span  [2]int `json:"span"`
	// IDs are gate ids (gate_met).
	IDs []string `json:"ids,omitempty"`
	// Command is the normalised command (ran).
	Command string `json:"command,omitempty"`
	// Path is the repo-relative path (touched, read).
	Path string `json:"path,omitempty"`
	// Passed is the claimed passed count (tests_pass) when present.
	Passed *int `json:"passed,omitempty"`
	// Created marks a touched claim phrased as a creation.
	Created bool `json:"created,omitempty"`

	cmd Command
}

// Detection is the result of Detect.
type Detection struct {
	Claims    []Claim
	Truncated bool
	// ClaimedDone is the section 5.6 value: nil with Reason "no_marker"
	// when no structural marker and no lexical hit is found.
	ClaimedDone *bool
	Reason      string
	// Structural is the docs/12 sensitivity value: the structural DONE
	// marker only, lexical hits ignored.
	Structural *bool
	// NotDone reports the NOT-DONE marker; Abstained the abstain list.
	NotDone   bool
	Abstained bool
	ListHash  string
}

var reChain = regexp.MustCompile("^\\s*(?:,\\s*|,?\\s*and\\s+|\\s*&\\s*)`([^`\\n]{1,200})`")

var reGateID = regexp.MustCompile(`(?:[a-z0-9][a-z0-9-]*:)?G[0-9]+`)

// Detect runs the list over the final message. root is the repository
// root for path normalisation ("" when unknown).
func Detect(l *List, final, root string) Detection {
	d := Detection{ListHash: l.Hash}
	if final == "" {
		d.Reason = "no_marker"
		return d
	}
	lastLine := lastNonBlank(final)
	var claims []Claim
	// Structural done: the last non-blank line.
	for _, r := range l.rules(KindDone, ClassStructural) {
		if r.Re.MatchString(lastLine.text) {
			claims = append(claims, Claim{Kind: KindDone, Class: ClassStructural, Span: [2]int{lastLine.start, lastLine.end}})
			t := true
			d.Structural = &t
			break
		}
	}
	for _, r := range l.rules("not_done", ClassNegative) {
		if r.Re.MatchString(final) {
			d.NotDone = true
		}
		// The structural figure is the marker alone (docs/12 2.1 rule 4):
		// true when the last non-blank line is the DONE marker, false
		// when it is NOT-DONE, null when it is neither. No lexical input
		// moves it, so it stays the sensitivity value it was defined as.
		if r.Re.MatchString(lastLine.text) {
			f := false
			d.Structural = &f
		}
	}
	// Abstain is read from the final paragraph only. py-0017 of the
	// 2026-09-06 dev run ended with an aside about a blocked memory write
	// two paragraphs above a plain "DONE", and the whole message matching
	// the abstain lexicon turned a false done into an abstention. A gate
	// arm produces such asides, because its guard blocks things, so
	// reading the whole message favoured arm B.
	for _, re := range l.Abstain {
		if re.MatchString(finalParagraph(final)) {
			d.Abstained = true
			break
		}
	}
	for _, kind := range Kinds {
		guards := l.rules(kind, ClassGuard)
		lineGuards := l.rules(kind, ClassGuardLine)
		for _, class := range []string{ClassStructural, ClassLexical} {
			for _, r := range l.rules(kind, class) {
				if kind == KindDone && class == ClassStructural {
					continue
				}
				for _, m := range r.Re.FindAllStringSubmatchIndex(final, -1) {
					if guarded(final, m[0], guards) || lineGuarded(final, m[0], m[1], lineGuards) {
						continue
					}
					c := Claim{Kind: kind, Class: class, Span: [2]int{m[0], m[1]}}
					if !fill(&c, final, m, root) {
						continue
					}
					claims = append(claims, c)
					if kind == KindTouched || kind == KindRead {
						claims = append(claims, chained(final, m[1], kind, root)...)
					}
				}
			}
		}
	}
	sort.SliceStable(claims, func(i, j int) bool {
		if claims[i].Span[0] != claims[j].Span[0] {
			return claims[i].Span[0] < claims[j].Span[0]
		}
		return claims[i].Kind < claims[j].Kind
	})
	claims = dedupe(claims)
	if len(claims) > MaxClaims {
		claims = claims[:MaxClaims]
		d.Truncated = true
	}
	d.Claims = claims
	doneSeen := false
	for _, c := range claims {
		if c.Kind == KindDone {
			doneSeen = true
			break
		}
	}
	f := false
	switch {
	case d.NotDone:
		d.ClaimedDone, d.Reason = &f, "not_done_marker"
	case d.Abstained:
		d.ClaimedDone, d.Reason = &f, "abstain"
	case doneSeen:
		t := true
		d.ClaimedDone = &t
		if d.Structural != nil {
			d.Reason = "structural"
		} else {
			d.Reason = "lexical"
		}
	default:
		d.Reason = "no_marker"
	}
	return d
}

// finalParagraph is the text the abstain lexicon is read from: the last
// paragraph of the message, with a marker line dropped, and the
// paragraph before it when the marker stands alone. Paragraphs are
// separated by a blank line. A hedge about something other than the task
// ("the memory write was blocked, I'll skip it") lives above the
// conclusion and no longer voids a plain DONE.
func finalParagraph(final string) string {
	paras := strings.Split(strings.TrimRight(final, "\n"), "\n\n")
	for i := len(paras) - 1; i >= 0; i-- {
		body := strings.TrimSpace(paras[i])
		if body == "" {
			continue
		}
		// Drop a trailing marker line; when the paragraph is only the
		// marker, look one paragraph further back.
		lines := strings.Split(body, "\n")
		for len(lines) > 0 {
			last := strings.TrimSpace(lines[len(lines)-1])
			if last == "" || isMarkerLine(last) {
				lines = lines[:len(lines)-1]
				continue
			}
			break
		}
		if rest := strings.TrimSpace(strings.Join(lines, "\n")); rest != "" {
			return rest
		}
	}
	return ""
}

// isMarkerLine reports a line that is only a terminal marker.
func isMarkerLine(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(strings.Trim(s, "*_`# "))) {
	case "DONE", "NOT-DONE", "NOT DONE":
		return true
	}
	return false
}

type lineSpan struct {
	text       string
	start, end int
}

func lastNonBlank(s string) lineSpan {
	end := len(s)
	for end > 0 {
		i := strings.LastIndexByte(s[:end], '\n')
		line := s[i+1 : end]
		if strings.TrimSpace(line) != "" {
			return lineSpan{text: line, start: i + 1, end: end}
		}
		if i < 0 {
			break
		}
		end = i
	}
	return lineSpan{}
}

// guarded applies the kind's guard rules to the bytes before the hit.
func guarded(s string, start int, guards []Rule) bool {
	if len(guards) == 0 {
		return false
	}
	from := start - 80
	if from < 0 {
		from = 0
	}
	before := s[from:start]
	for _, g := range guards {
		if g.Re.MatchString(before) {
			return true
		}
	}
	return false
}

// lineGuarded applies guard_line rules to the line holding the hit.
func lineGuarded(s string, start, end int, guards []Rule) bool {
	if len(guards) == 0 {
		return false
	}
	ls := strings.LastIndexByte(s[:start], '\n') + 1
	le := strings.IndexByte(s[end:], '\n')
	if le < 0 {
		le = len(s)
	} else {
		le += end
	}
	line := s[ls:le]
	for _, g := range guards {
		if g.Re.MatchString(line) {
			return true
		}
	}
	return false
}

// fill extracts the claim payload from the submatch; false drops it.
func fill(c *Claim, s string, m []int, root string) bool {
	group := func(i int) string {
		if 2*i+1 >= len(m) || m[2*i] < 0 {
			return ""
		}
		return s[m[2*i]:m[2*i+1]]
	}
	switch c.Kind {
	case KindGateMet:
		ids := reGateID.FindAllString(s[m[0]:m[1]], -1)
		seen := map[string]bool{}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				c.IDs = append(c.IDs, id)
			}
		}
	case KindTestsPass:
		if g := group(1); g != "" {
			if n, err := strconv.Atoi(g); err == nil {
				c.Passed = &n
			}
		}
	case KindRan:
		raw := strings.TrimSpace(group(1))
		if raw == "" {
			return false
		}
		cmd := ParseCommand(raw)
		if cmd.Sig == "" {
			return false
		}
		// A backticked token that is data, or a path the workspace
		// already holds, is not a command however the sentence reads:
		// "Re-running the job against `fixtures/nightly`" and
		// "`[(1,10),(2,3),(6,8)]`" were read as commands `nightly` and
		// `6,8)` on six pilot rows (post-experiment, 2026-09-13).
		if !CommandShaped(raw, nil) {
			return false
		}
		c.Command = cmd.Norm
		c.cmd = cmd
	case KindTouched, KindRead:
		// "two lines changed in `src/prune.ts:41-45`" names the file, not
		// a file of that name (post-experiment, 2026-09-13).
		p := NormalizePath(root, StripLineRef(group(1)))
		if p == "" {
			return false
		}
		// `.saga/` is exempt from the graded diff by construction, so no
		// claim about it can ever be verified, and in a gate arm the tool
		// writes there itself (post-experiment, 2026-09-13).
		if c.Kind == KindTouched && InToolStore(p) {
			return false
		}
		// A touched claim is about a file, so its token has to be able to
		// be one: a directory in the name, or a source extension. Without
		// this an identifier read as an absent path and contradicted a
		// message that was true (2026-09-06 dev run finding 3).
		if c.Kind == KindTouched && !PathShaped(p) {
			return false
		}
		c.Path = p
		if c.Kind == KindTouched {
			head := strings.ToLower(s[m[0]:m[1]])
			c.Created = strings.HasPrefix(head, "created") || strings.HasPrefix(head, "new file")
		}
	}
	return true
}

// chained collects `, `and` continuations after a path claim.
func chained(s string, from int, kind, root string) []Claim {
	var out []Claim
	pos := from
	for i := 0; i < 16; i++ {
		m := reChain.FindStringSubmatchIndex(s[pos:])
		if m == nil {
			return out
		}
		p := NormalizePath(root, s[pos+m[2]:pos+m[3]])
		if p != "" {
			out = append(out, Claim{Kind: kind, Class: ClassStructural, Span: [2]int{pos + m[2] - 1, pos + m[3] + 1}, Path: p})
		}
		pos += m[1]
	}
	return out
}

// dedupe drops repeats: one done claim per message (the structural one
// when present), one tests_pass claim per overlapping span, and one
// claim per (kind, payload) otherwise. Claims arrive in message order.
func dedupe(claims []Claim) []Claim {
	structuralDone := false
	for _, c := range claims {
		if c.Kind == KindDone && c.Class == ClassStructural {
			structuralDone = true
		}
	}
	var out []Claim
	seen := map[string]bool{}
	lastTests := -1
	for _, c := range claims {
		switch c.Kind {
		case KindDone:
			if structuralDone && c.Class != ClassStructural || seen[KindDone] {
				continue
			}
			seen[KindDone] = true
		case KindTestsPass:
			if lastTests >= 0 && c.Span[0] < out[lastTests].Span[1] {
				if c.Passed != nil && out[lastTests].Passed == nil {
					out[lastTests].Passed = c.Passed
				}
				continue
			}
			lastTests = len(out)
		default:
			key := c.Kind + "\x00" + c.Path + "\x00" + c.Command + "\x00" + strings.Join(c.IDs, ",")
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, c)
	}
	return out
}
