package cli

import (
	"errors"
	"testing"
)

func TestPrecedence(t *testing.T) {
	cases := []struct {
		in   []Code
		want Code
	}{
		{nil, ExitOK},
		{[]Code{ExitOK, ExitOK}, ExitOK},
		{[]Code{ExitFinding, ExitIntegrity}, ExitIntegrity},
		{[]Code{ExitFinding, ExitUsage, ExitRefusal}, ExitUsage},
		{[]Code{ExitContamination, ExitEnvironment}, ExitEnvironment},
		{[]Code{ExitApproval, ExitRefusal}, ExitRefusal},
		{[]Code{ExitIntegrity, ExitApproval}, ExitApproval},
		{[]Code{ExitContamination, ExitUsage}, ExitContamination},
	}
	for _, c := range cases {
		if got := Precedence(c.in...); got != c.want {
			t.Errorf("Precedence(%v) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestCodeOf(t *testing.T) {
	if CodeOf(nil) != ExitOK {
		t.Error("nil")
	}
	if CodeOf(errors.New("x")) != ExitFinding {
		t.Error("plain error")
	}
	wrapped := Wrap(ExitEnvironment, "store", errors.New("unreadable"))
	if CodeOf(wrapped) != ExitEnvironment || wrapped.Error() != "store: unreadable" {
		t.Errorf("wrapped: %v %v", CodeOf(wrapped), wrapped)
	}
	if Wrap(ExitUsage, "x", nil) != nil {
		t.Error("wrap nil")
	}
}
