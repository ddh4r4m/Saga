package gate

import (
	"fmt"
	"regexp"
	"strings"
)

// SegmenterVersion names the fixed sentence-numbering rule set of
// gate-spec section 2.4.
const SegmenterVersion = "v1"

// Sentence is one numbered sentence of the request.
type Sentence struct {
	ID   string // "R<n>"
	N    int
	Text string
}

// abbreviations never end a sentence when followed by a space.
var abbreviations = map[string]bool{
	"e.g.": true, "i.e.": true, "etc.": true, "vs.": true, "cf.": true, "mr.": true, "mrs.": true, "ms.": true, "dr.": true,
	"no.": true, "fig.": true, "approx.": true, "incl.": true, "min.": true, "max.": true, "sec.": true, "st.": true,
}

var (
	reHeading  = regexp.MustCompile(`^\s{0,3}#{1,6}\s`)
	reListItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	reSpaces   = regexp.MustCompile(`\s+`)
)

// Segment numbers the sentences of text under SEGMENTER v1: terminal
// `.?!` followed by whitespace and an uppercase letter or EOF; the
// abbreviation list; list items and headings are their own sentences; a
// fenced block is one sentence.
func Segment(text string) []Sentence {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var units []string
	var para []string
	flush := func() {
		if len(para) > 0 {
			units = append(units, splitSentences(strings.Join(para, " "))...)
			para = nil
		}
	}
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if fm := reFence.FindStringSubmatch(l); fm != nil {
			flush()
			block := []string{l}
			for i++; i < len(lines); i++ {
				block = append(block, lines[i])
				if cm := reFence.FindStringSubmatch(lines[i]); cm != nil && cm[1][0] == fm[1][0] && len(cm[1]) >= len(fm[1]) && strings.TrimSpace(cm[2]) == "" {
					break
				}
			}
			units = append(units, strings.Join(block, "\n"))
			continue
		}
		switch {
		case strings.TrimSpace(l) == "":
			flush()
		case reHeading.MatchString(l), reListItem.MatchString(l):
			flush()
			units = append(units, strings.TrimSpace(l))
		default:
			para = append(para, strings.TrimSpace(l))
		}
	}
	flush()
	out := make([]Sentence, 0, len(units))
	for _, u := range units {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		n := len(out) + 1
		out = append(out, Sentence{ID: fmt.Sprintf("R%d", n), N: n, Text: u})
	}
	return out
}

func splitSentences(s string) []string {
	var out []string
	runes := []rune(s)
	start := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '.' && r != '?' && r != '!' {
			continue
		}
		// Terminal iff followed by whitespace then an uppercase letter, or EOF.
		j := i + 1
		for j < len(runes) && (runes[j] == '.' || runes[j] == '?' || runes[j] == '!') {
			j++
		}
		if j < len(runes) {
			if runes[j] != ' ' && runes[j] != '\t' {
				continue
			}
			k := j
			for k < len(runes) && (runes[k] == ' ' || runes[k] == '\t') {
				k++
			}
			if k >= len(runes) || !isUpper(runes[k]) {
				continue
			}
		}
		if r == '.' {
			word := lastWord(runes[start : i+1])
			if abbreviations[strings.ToLower(word)] || isInitial(word) {
				continue
			}
		}
		out = append(out, strings.TrimSpace(string(runes[start:j])))
		start = j
		i = j - 1
	}
	if rest := strings.TrimSpace(string(runes[start:])); rest != "" {
		out = append(out, rest)
	}
	return out
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z' || (r > 127 && strings.ToUpper(string(r)) == string(r) && strings.ToLower(string(r)) != string(r))
}

func lastWord(rs []rune) string {
	s := string(rs)
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// isInitial reports a single-letter initial such as "J." in a name.
func isInitial(w string) bool {
	return len(w) == 2 && w[1] == '.' && isUpper(rune(w[0]))
}

// Numbered renders the request in the section 2.4 form.
func Numbered(text string) string {
	var b strings.Builder
	b.WriteString("SEGMENTER: " + SegmenterVersion + "\n")
	for _, s := range Segment(text) {
		fmt.Fprintf(&b, "%-3s %s\n", s.ID, strings.ReplaceAll(s.Text, "\n", "\n    "))
	}
	return b.String()
}

// NormalizeSpace collapses whitespace runs to one space and trims.
func NormalizeSpace(s string) string { return strings.TrimSpace(reSpaces.ReplaceAllString(s, " ")) }

// VerifyRequest applies rows 11 and 12 of section 2.6: the REQUEST: hash
// must equal the hash of the request bytes and every FROM: quote must
// occur verbatim (after whitespace normalisation) inside its sentence.
// It returns the numbered sentences for the coverage report.
func VerifyRequest(c *Contract, request []byte, requestHash string) ([]Sentence, error) {
	if c.Request == "" {
		return nil, nil
	}
	if request == nil {
		return nil, perr(12, 0, "REQUEST: present but %s is missing", RequestPath)
	}
	if c.Request != requestHash {
		return nil, perr(12, 0, "REQUEST: %s does not match %s (%s)", c.Request, RequestPath, requestHash)
	}
	sents := Segment(string(request))
	for _, g := range c.Gates {
		for _, f := range g.From {
			if f.Sentence < 1 || f.Sentence > len(sents) {
				return nil, perr(11, f.Line+1, "gate %s FROM: R%d does not exist (%d sentences)", g.ID, f.Sentence, len(sents))
			}
			if !strings.Contains(NormalizeSpace(sents[f.Sentence-1].Text), NormalizeSpace(f.Quote)) {
				return nil, perr(11, f.Line+1, "gate %s FROM: quote not found in R%d", g.ID, f.Sentence)
			}
		}
	}
	return sents, nil
}

// Coverage classes.
const (
	Covered   = "covered"
	Uncovered = "uncovered"
	Ignored   = "ignored"
)

var reGreeting = regexp.MustCompile(`(?i)^(hi|hello|hey|thanks|thank you|cheers|please|ok|okay)\b[!., ]*$`)

// Classify labels a sentence for the coverage report. The classifier is
// lexical and experimental (gate-spec section 2.4): interrogatives,
// greetings and verb-less fragments are ignored.
func Classify(s Sentence, covered bool) string {
	if covered {
		return Covered
	}
	t := strings.TrimSpace(s.Text)
	if strings.HasSuffix(t, "?") || reGreeting.MatchString(t) || len(strings.Fields(t)) < 3 {
		return Ignored
	}
	return Uncovered
}
