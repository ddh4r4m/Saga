// Package canon implements the identity and encoding rules of
// docs/specs/00-cross-spec-contracts.md section 3: canonical JSON (sorted
// keys, no insignificant whitespace, UTF-8), sha256 ids for every
// cross-layer field, blake3 ids for opaque in-layer caches, the token
// estimate, and the repo-relative path rule.
package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/zeebo/blake3"
)

// Genesis is the prev value of the first event in a session chain
// (trace-spec section 2.8).
const Genesis = "sha256:genesis"

// EmptyHash is the before_hash of a file that does not exist
// (trace-spec section 2.3).
const EmptyHash = "sha256:empty"

// OutsideRepo is the prefix that replaces an absolute path outside the
// repository root (contracts section 3).
const OutsideRepo = "«outside-repo»/"

// JSON returns the canonical JSON encoding of v: object keys sorted
// recursively, no insignificant whitespace, no HTML escaping, numbers
// preserved as written by encoding/json.
func JSON(v any) ([]byte, error) {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return nil, err
	}
	return Canonicalize(raw)
}

// Canonicalize re-encodes an arbitrary JSON document in canonical form.
// It fails on invalid JSON.
func Canonicalize(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("canon: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("canon: trailing data after JSON value")
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("canon: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := marshalNoEscape(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case json.Number:
		buf.WriteString(t.String())
	default:
		b, err := marshalNoEscape(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

// SHA256 returns "sha256:<64 hex>" of b, the form every cross-layer hash
// field uses.
func SHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SHA256JSON returns the sha256 id of the canonical JSON encoding of v.
func SHA256JSON(v any) (string, error) {
	b, err := JSON(v)
	if err != nil {
		return "", err
	}
	return SHA256(b), nil
}

// Blake3 returns "blake3:<64 hex>" of b. Only index and shape use blake3,
// and only as opaque ids that never cross a layer boundary.
func Blake3(b []byte) string {
	sum := blake3.Sum256(b)
	return "blake3:" + hex.EncodeToString(sum[:])
}

// TokensEst is the contracts section 3 estimate: ceil(utf8_bytes / 4).
func TokensEst(b []byte) int {
	return (len(b) + 3) / 4
}

// TokensEstString is TokensEst over the UTF-8 bytes of s.
func TokensEstString(s string) int {
	return TokensEst([]byte(s))
}

// RelPath returns the repo-relative, slash-separated form of p. An
// absolute path outside root becomes OutsideRepo followed by the first 12
// hex of sha256 of the basename.
func RelPath(root, p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(filepath.Clean(p))
	}
	rel, err := filepath.Rel(root, p)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	sum := sha256.Sum256([]byte(filepath.Base(p)))
	return OutsideRepo + hex.EncodeToString(sum[:])[:12]
}

// CleanText strips control characters, line and paragraph separators and
// bidi control points from s (contracts section 3, text fields). Tab,
// newline and carriage return are kept.
func CleanText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case unicode.IsControl(r):
			return -1
		case r == 0x2028 || r == 0x2029:
			return -1
		case r >= 0x200B && r <= 0x200F:
			return -1
		case r >= 0x202A && r <= 0x202E:
			return -1
		case r >= 0x2066 && r <= 0x2069:
			return -1
		case r == 0xFEFF:
			return -1
		}
		return r
	}, s)
}
