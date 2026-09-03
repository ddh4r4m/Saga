package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePolicyRejectsUnknownKeys(t *testing.T) {
	if _, err := ParsePolicy([]byte("schema = \"saga.guard.policy/1\"\n[bogus]\nx = 1\n")); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("want unknown key error, got %v", err)
	}
	if _, err := ParsePolicy([]byte("schema = \"saga.guard.other/1\"\n")); err == nil {
		t.Fatal("want schema error")
	}
	if _, err := ParsePolicy([]byte("[allow]\nread = [\"* foo\"]\n")); err == nil {
		t.Fatal("want first-token-literal error")
	}
}

func TestLoosenings(t *testing.T) {
	p, err := ParsePolicy([]byte(`
schema = "saga.guard.policy/1"
[allow]
read = ["ls *"]
[scope]
extra = ["/tmp"]
[net]
fetch = true
[ask]
unresolvable = false
[deny]
extra = ["terraform apply *"]
[git]
protected = ["staging"]
`))
	if err != nil {
		t.Fatal(err)
	}
	l := Loosenings(p)
	var fields []string
	for _, x := range l {
		fields = append(fields, x.Field)
	}
	want := "allow,ask.unresolvable,net.fetch,scope.extra"
	if got := strings.Join(fields, ","); got != want {
		t.Fatalf("loosenings %q, want %q", got, want)
	}
	tight, _ := ParsePolicy([]byte("[deny]\nextra = [\"terraform apply *\"]\n[git]\nprotected = [\"staging\"]\n[net]\nfetch = false\n"))
	if l := Loosenings(tight); len(l) != 0 {
		t.Fatalf("tightening policy flagged: %v", l)
	}
	m := Merge(DefaultPolicy(), tight)
	if !containsAny(m.Deny.Extra, "terraform apply *") || !containsAny(m.Git.Protected, "staging", "main") || m.NetFetch() {
		t.Fatalf("merge lost a tightening: %+v", m)
	}
}

func TestLoadPolicyTrust(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "saga-home")
	root := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(root, ".saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"SAGA_HOME": home, "HOME": tmp}
	repoPolicy := "schema = \"saga.guard.policy/1\"\n[deny]\nextra = [\"kubectl apply *\"]\n"
	if err := os.WriteFile(filepath.Join(root, ".saga", "policy.toml"), []byte(repoPolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	lp, err := LoadPolicy(env, root)
	if err != nil {
		t.Fatal(err)
	}
	if lp.RepoApplied || lp.RepoTrusted || len(lp.Notes) == 0 {
		t.Fatalf("untrusted repo policy applied: %+v", lp)
	}
	if containsAny(lp.Policy.Deny.Extra, "kubectl apply *") {
		t.Fatal("untrusted deny.extra merged")
	}
	if err := Trust(env, root, lp.RepoHash); err != nil {
		t.Fatal(err)
	}
	lp, err = LoadPolicy(env, root)
	if err != nil {
		t.Fatal(err)
	}
	if !lp.RepoApplied || !containsAny(lp.Policy.Deny.Extra, "kubectl apply *") {
		t.Fatalf("trusted repo policy not applied: %+v", lp)
	}
	// A trusted repo policy that loosens is exit 2 material.
	if err := os.WriteFile(filepath.Join(root, ".saga", "policy.toml"), []byte(repoPolicy+"[scope]\nextra = [\"/\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".saga", "policy.toml"))
	_ = Trust(env, root, "sha256:"+strings.Repeat("0", 64))
	lp2, err := LoadPolicy(env, root)
	if err != nil || lp2.RepoApplied {
		t.Fatalf("hash mismatch must ignore the policy: %v %+v", err, lp2)
	}
	if err := Trust(env, root, hashBytes(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(env, root); err == nil || !strings.Contains(err.Error(), "fixed table") {
		t.Fatalf("want fixed-table error, got %v", err)
	}
	// User policy loosens freely and deny.extra from the repo denies.
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "policy.toml"), []byte("[net]\nfetch = false\n[allow]\nmutate_out_of_scope = [\"docker run *\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".saga", "policy.toml"), []byte(repoPolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = Trust(env, root, hashBytes([]byte(repoPolicy)))
	lp, err = LoadPolicy(env, root)
	if err != nil {
		t.Fatal(err)
	}
	if lp.Policy.NetFetch() || !containsAny(lp.Policy.Allow.MutateOutOfScope, "docker run *") || !lp.RepoApplied {
		t.Fatalf("user override or repo merge missing: %+v", lp.Policy)
	}
	d, err := Check(Request{Command: "kubectl apply -f x.yaml", Shell: "bash", Cwd: root, Env: []string{"SAGA_HOME=" + home, "HOME=" + tmp}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision != Deny || d.Segments[0].Rule != "policy" {
		t.Fatalf("deny.extra did not deny: %+v", d)
	}
	d, _ = Check(Request{Command: "curl -s https://x", Shell: "bash", Cwd: root, Env: []string{"SAGA_HOME=" + home, "HOME=" + tmp}})
	if d.Decision != Ask {
		t.Fatalf("net.fetch=false must ask, got %s", d.Decision)
	}
}

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pat  string
		argv []string
		want bool
	}{
		{"git *", []string{"git"}, true},
		{"git *", []string{"git", "status"}, true},
		{"npm test", []string{"npm", "test"}, true},
		{"npm test", []string{"npm", "test", "--", "x"}, false},
		{"pytest *", []string{"pytest", "-q", "tests"}, true},
		{"kubectl apply -f prod/*", []string{"kubectl", "apply", "-f", "prod/a.yaml"}, true},
		{"kubectl apply -f prod/*", []string{"kubectl", "apply", "-f", "dev/a.yaml"}, false},
		{"pip install -r requirements.txt", []string{"pip", "install", "-r", "requirements.txt"}, true},
	}
	for _, c := range cases {
		if got := matchPattern(c.pat, c.argv); got != c.want {
			t.Errorf("%q vs %v: %v", c.pat, c.argv, got)
		}
	}
}

func TestShellAssumedAndParseFailure(t *testing.T) {
	root := t.TempDir()
	d, err := Check(Request{Command: "git status", Cwd: root, Env: []string{"HOME=" + root}, Policy: DefaultPolicy(), RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !d.ShellAssumed || d.Decision != Ask {
		t.Fatalf("assumed shell must downgrade allow to ask: %+v", d)
	}
	d, err = Check(Request{Command: "echo 'unterminated", Shell: "bash", Cwd: root, Env: []string{"HOME=" + root}, Policy: DefaultPolicy(), RepoRoot: root})
	if err == nil || d == nil || d.Decision != Deny {
		t.Fatalf("parse failure must fail closed: %v %+v", err, d)
	}
	if _, err := Check(Request{Command: "ls", Shell: "pwsh", Cwd: root}); err == nil {
		t.Fatal("unknown shell must be a usage error")
	}
}

func hashBytes(b []byte) string {
	return canonSHA(b)
}

func canonSHA(b []byte) string { return canonPkgSHA(b) }
