package cli

import (
	"errors"
	"fmt"
)

// Code is a Saga process exit code. The table is the uniform one of
// docs/specs/00-cross-spec-contracts.md section 4 and every subcommand
// uses it; only `saga shape run` propagates a child's code instead.
type Code int

// The uniform exit codes.
const (
	// ExitOK: ok, allow, all met.
	ExitOK Code = 0
	// ExitFinding: unmet gate, ask, stale record, unverified claim, canary
	// non-pass, runtime budget exhausted, empty result, doctor check failed.
	ExitFinding Code = 1
	// ExitUsage: usage, parse or schema failure; fail closed.
	ExitUsage Code = 2
	// ExitRefusal: hard deny, unwaived guard violation, bench estimate over
	// budget, masked write rejected.
	ExitRefusal Code = 3
	// ExitApproval: approval or trust required.
	ExitApproval Code = 4
	// ExitIntegrity: integrity or proof missing; hash chain mismatch.
	ExitIntegrity Code = 5
	// ExitEnvironment: environment refusal; hostile file shape, unreadable
	// store, harness or runtime missing.
	ExitEnvironment Code = 6
	// ExitContamination: contamination, bench only.
	ExitContamination Code = 7
)

// precedence is the order in which codes win when several apply:
// 6, 7, 2, 3, 4, 5, 1 (contracts section 4).
var precedence = []Code{ExitEnvironment, ExitContamination, ExitUsage, ExitRefusal, ExitApproval, ExitIntegrity, ExitFinding}

// Precedence returns the single code to exit with when several codes
// apply at once. It returns ExitOK when no non-zero code is present.
func Precedence(codes ...Code) Code {
	for _, p := range precedence {
		for _, c := range codes {
			if c == p {
				return p
			}
		}
	}
	return ExitOK
}

// String names the code for humans.
func (c Code) String() string {
	switch c {
	case ExitOK:
		return "ok"
	case ExitFinding:
		return "finding"
	case ExitUsage:
		return "usage"
	case ExitRefusal:
		return "refusal"
	case ExitApproval:
		return "approval"
	case ExitIntegrity:
		return "integrity"
	case ExitEnvironment:
		return "environment"
	case ExitContamination:
		return "contamination"
	}
	return fmt.Sprintf("code(%d)", int(c))
}

// Error is an error that carries the exit code the process must use.
type Error struct {
	Code Code
	Msg  string
	Err  error
}

// Error implements error.
func (e *Error) Error() string {
	if e.Err != nil {
		if e.Msg == "" {
			return e.Err.Error()
		}
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

// Unwrap returns the wrapped cause.
func (e *Error) Unwrap() error { return e.Err }

// Errorf builds an Error with a formatted message.
func Errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap attaches a code to an existing error. A nil err returns nil.
func Wrap(code Code, msg string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Msg: msg, Err: err}
}

// CodeOf extracts the exit code from err. A nil error is ExitOK; an error
// without a code is ExitFinding.
func CodeOf(err error) Code {
	if err == nil {
		return ExitOK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ExitFinding
}
