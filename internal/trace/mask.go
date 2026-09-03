package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

// Masker is the M0 built-in redaction of trace-spec section 2.6: a small
// gitleaks-class rule set applied to every string before hashing or
// writing. Placeholders use guard's form, SAGA_MASK_<TYPE>_<8 hex>, stable
// within a session through the salt.
type Masker struct {
	salt string
}

// NewMasker returns a masker whose placeholders are stable for salt.
func NewMasker(salt string) *Masker { return &Masker{salt: salt} }

type maskRule struct {
	typ string
	re  *regexp.Regexp
	// group is the capture group holding the secret; 0 masks the whole match.
	group int
}

var maskRules = []maskRule{
	{"PRIVATEKEY", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), 0},
	{"ANTHROPIC", regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`), 0},
	{"OPENAI", regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_\-]{20,}`), 0},
	{"GITHUB", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`), 0},
	{"AWS", regexp.MustCompile(`AKIA[0-9A-Z]{16}`), 0},
	{"GOOGLE", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`), 0},
	{"SLACK", regexp.MustCompile(`xox[baprs]-[0-9A-Za-z\-]{10,}`), 0},
	{"JWT", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`), 0},
	{"ENV", regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?[A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|API_KEY|APIKEY|PRIVATE_KEY)[A-Z0-9_]*[ \t]*=[ \t]*["']?([^\s"']{8,})`), 1},
}

func (m *Masker) placeholder(typ string, secret []byte) []byte {
	h := sha256.New()
	h.Write([]byte(m.salt))
	h.Write(secret)
	return []byte("SAGA_MASK_" + typ + "_" + hex.EncodeToString(h.Sum(nil))[:8])
}

// Mask redacts every rule match in b and returns the masked bytes and the
// number of replacements.
func (m *Masker) Mask(b []byte) ([]byte, int) {
	count := 0
	for _, r := range maskRules {
		idx := r.re.FindAllSubmatchIndex(b, -1)
		if len(idx) == 0 {
			continue
		}
		out := make([]byte, 0, len(b))
		last := 0
		for _, loc := range idx {
			s, e := loc[2*r.group], loc[2*r.group+1]
			if s < 0 {
				continue
			}
			out = append(out, b[last:s]...)
			out = append(out, m.placeholder(r.typ, b[s:e])...)
			last = e
			count++
		}
		out = append(out, b[last:]...)
		b = out
	}
	return b, count
}

// MaskString is Mask over a string.
func (m *Masker) MaskString(s string) (string, int) {
	b, n := m.Mask([]byte(s))
	return string(b), n
}
