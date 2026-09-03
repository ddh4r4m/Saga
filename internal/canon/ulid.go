package canon

import (
	"crypto/rand"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ULID returns a 26-character Crockford base32 ULID (48-bit millisecond
// time, 80 random bits), the session id form of trace-spec section 2.1
// when the harness supplies none.
func ULID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		// crypto/rand failing is not recoverable in a meaningful way; fall
		// back to the clock so the id is still unique per millisecond.
		ns := uint64(time.Now().UnixNano())
		for i := 15; i >= 6; i-- {
			b[i] = byte(ns)
			ns >>= 8
		}
	}
	// Encode 128 bits as 26 base32 characters, first character 2 bits.
	out := make([]byte, 26)
	var acc uint64
	bits := 0
	pos := 25
	for i := 15; i >= 0; i-- {
		acc |= uint64(b[i]) << bits
		bits += 8
		for bits >= 5 && pos >= 0 {
			out[pos] = crockford[acc&31]
			acc >>= 5
			bits -= 5
			pos--
		}
	}
	for pos >= 0 {
		out[pos] = crockford[acc&31]
		acc >>= 5
		pos--
	}
	return string(out)
}
