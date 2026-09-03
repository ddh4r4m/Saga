package canon

import (
	"strings"
	"testing"
)

func TestCanonicalJSONGolden(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"empty object", map[string]any{}, `{}`},
		{"sorted keys", map[string]any{"b": 2, "a": 1}, `{"a":1,"b":2}`},
		{"nested", map[string]any{"z": map[string]any{"y": []any{1, "x", nil}}, "a": true}, `{"a":true,"z":{"y":[1,"x",null]}}`},
		{"no html escape", map[string]any{"s": "<a&b>"}, `{"s":"<a&b>"}`},
		{"struct field order ignored", struct {
			B int `json:"b"`
			A int `json:"a"`
		}{1, 2}, `{"a":2,"b":1}`},
		{"unicode kept", map[string]any{"k": OutsideRepo}, `{"k":"` + OutsideRepo + `"}`},
	}
	for _, c := range cases {
		got, err := JSON(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(got) != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestCanonicalizePreservesNumbers(t *testing.T) {
	got, err := Canonicalize([]byte(`{ "b" : 1.50, "a": 12345678901234567890 }`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":12345678901234567890,"b":1.50}` {
		t.Errorf("got %s", got)
	}
	if _, err := Canonicalize([]byte(`{"a":1} x`)); err == nil {
		t.Error("trailing data accepted")
	}
}

func TestHashGolden(t *testing.T) {
	if got := SHA256(nil); got != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("sha256 empty: %s", got)
	}
	if got := SHA256([]byte("{}")); got != "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a" {
		t.Errorf("sha256 {}: %s", got)
	}
	h, err := SHA256JSON(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if h != "sha256:43258cff783fe7036d8a43033f830adfc60ec037382473548ac742b888292777" {
		t.Errorf("sha256 json: %s", h)
	}
	// Reference vectors from the BLAKE3 specification test suite.
	if got := Blake3(nil); got != "blake3:af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262" {
		t.Errorf("blake3 empty: %s", got)
	}
	if got := Blake3([]byte("abc")); got != "blake3:6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85" {
		t.Errorf("blake3 abc: %s", got)
	}
}

func TestTokensEst(t *testing.T) {
	for in, want := range map[int]int{0: 0, 1: 1, 4: 1, 5: 2, 8: 2, 9: 3} {
		if got := TokensEst(make([]byte, in)); got != want {
			t.Errorf("TokensEst(%d) = %d want %d", in, got, want)
		}
	}
}

func TestRelPath(t *testing.T) {
	root := "/repo"
	if got := RelPath(root, "/repo/src/a.go"); got != "src/a.go" {
		t.Errorf("inside: %s", got)
	}
	if got := RelPath(root, "src/a.go"); got != "src/a.go" {
		t.Errorf("relative: %s", got)
	}
	got := RelPath(root, "/etc/passwd")
	if !strings.HasPrefix(got, OutsideRepo) || len(got) != len(OutsideRepo)+12 {
		t.Errorf("outside: %s", got)
	}
	if RelPath(root, "/repo") != "." {
		t.Errorf("root itself")
	}
}

func TestCleanText(t *testing.T) {
	in := "a\u202ebc\u2028d\te\x07\n"
	if got := CleanText(in); got != "abcd\te\n" {
		t.Errorf("got %q", got)
	}
}
