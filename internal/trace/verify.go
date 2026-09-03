package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
)

// VerifyError reports the first chain, schema or blob failure in a
// session, naming the seq (trace-spec section 11.1). Callers map it to
// exit 5.
type VerifyError struct {
	Seq    int
	Reason string
}

// Error implements error.
func (e *VerifyError) Error() string { return fmt.Sprintf("trace verify: seq %d: %s", e.Seq, e.Reason) }

// VerifyResult summarises a passing verification.
type VerifyResult struct {
	Events   int    `json:"events"`
	Segments int    `json:"segments"`
	Blobs    int    `json:"blobs"`
	Head     string `json:"head"`
}

// Verify walks the session's segments checking that seq is gapless from
// 1, every prev equals the previous hash, every hash recomputes, every
// event validates against its schema, and every blob ref exists.
func Verify(dir string) (*VerifyResult, error) {
	segs, err := Segments(dir)
	if err != nil {
		return nil, err
	}
	res := &VerifyResult{Segments: len(segs)}
	prev := canon.Genesis
	expect := 1
	err = Walk(dir, func(ev Event, _ []byte) error {
		if ev.Seq != expect {
			return &VerifyError{Seq: ev.Seq, Reason: fmt.Sprintf("seq gap: expected %d", expect)}
		}
		if ev.Prev != prev {
			return &VerifyError{Seq: ev.Seq, Reason: "prev does not match previous hash"}
		}
		h, err := ev.ComputeHash()
		if err != nil {
			return &VerifyError{Seq: ev.Seq, Reason: err.Error()}
		}
		if h != ev.Hash {
			return &VerifyError{Seq: ev.Seq, Reason: "hash mismatch"}
		}
		if err := ev.Validate(); err != nil {
			return &VerifyError{Seq: ev.Seq, Reason: err.Error()}
		}
		for _, k := range []string{"args_ref", "result_ref", "prompt_ref", "final_message_ref"} {
			if ref, ok := ev.Body[k].(string); ok && strings.HasPrefix(ref, "blobs/") {
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ref))); err != nil {
					return &VerifyError{Seq: ev.Seq, Reason: "missing blob " + ref}
				}
				res.Blobs++
			}
		}
		prev = ev.Hash
		expect++
		res.Events++
		res.Head = ev.Hash
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
