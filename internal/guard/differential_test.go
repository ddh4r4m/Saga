package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// differentialCommands are run through the real shells with stub
// binaries on PATH that record their argv; guard's resolved argv for the
// stub segments must agree (guard-spec 11.3: a differing resolved argv is
// a P0 bug, unresolvable is the only permitted divergence).
var differentialCommands = []string{
	`foo a b c`,
	`foo "a b" 'c d'`,
	`foo a\ b`,
	`foo *.txt`,
	`foo sub/*.go`,
	`foo {a,b}.txt`,
	`foo x{1..3}`,
	`foo $HOME/x`,
	`foo ~/y`,
	`foo ${UNSET_VAR:-def}`,
	`foo $UNSET_VAR x`,
	`foo "$(basename /a/b)"`,
	`foo $(pwd)`,
	`foo $((1+2))`,
	`foo a; bar b`,
	`~foo a && bar b || baz c`,
	`foo a | bar b`,
	`if true; then foo yes; fi`,
	`~if false; then foo no; else foo yes; fi`,
	`for i in 1 2; do foo $i; done`,
	`(foo sub); { bar grp; }`,
	`foo 'single $HOME'`,
	`foo "quoted $VAR1 mid"`,
	`VAR2=zzz; foo $VAR2`,
	`foo -- --flag`,
	`foo "" empty`,
	`env FOO=1 foo envd`,
	`sh -c 'foo inner a'`,
	`foo a >/dev/null`,
	`echo hi | foo -`,
	`foo "$(echo nested)"`,
	`foo a\;b`,
	`foo "$HOME"/"x y"`,
	`foo ./*.txt`,
	`foo sub/../a.txt`,
	`case x in x) foo casex;; esac`,
	`~while false; do foo never; done; foo after`,
}

// The stub appends one record per invocation to its own file (pipe
// stages run concurrently, so a shared file would interleave).
const stubScript = `#!/bin/sh
out="$SAGA_STUB_OUT/$$.$(date +%N 2>/dev/null)"
{ printf '%s' "$(basename "$0")"; for a in "$@"; do printf '\037%s' "$a"; done; printf '\n'; } >> "$out"
`

func TestDifferentialAgainstShells(t *testing.T) {
	shells := map[string]string{"sh": "/bin/sh"}
	if p, err := exec.LookPath("zsh"); err == nil {
		shells["zsh"] = p
	}
	if p, err := exec.LookPath("bash"); err == nil {
		shells["bash"] = p
	}
	tmp := t.TempDir()
	if r, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = r
	}
	cwd := filepath.Join(tmp, "work")
	home := filepath.Join(tmp, "home")
	stubs := filepath.Join(tmp, "stubs")
	for _, d := range []string{cwd, home, stubs, filepath.Join(cwd, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"a.txt", "b.txt", "sub/x.go", "sub/y.go"} {
		if err := os.WriteFile(filepath.Join(cwd, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"foo", "bar", "baz"} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte(stubScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(tmp, "argv")
	env := []string{"HOME=" + home, "PATH=" + stubs + ":/usr/bin:/bin", "VAR1=hello", "SAGA_STUB_OUT=" + out}
	compared := 0
	for shellName, shellPath := range shells {
		for _, cmd := range differentialCommands {
			// A leading ~ marks control flow the shell may not take: guard
			// classifies every branch, so its argv set is a superset.
			superset := strings.HasPrefix(cmd, "~")
			cmd = strings.TrimPrefix(cmd, "~")
			t.Run(shellName+"/"+cmd, func(t *testing.T) {
				_ = os.RemoveAll(out)
				if err := os.MkdirAll(out, 0o755); err != nil {
					t.Fatal(err)
				}
				args := []string{"-c", cmd}
				if shellName == "zsh" {
					args = []string{"-f", "-c", cmd}
				}
				sh := exec.Command(shellPath, args...)
				sh.Dir = cwd
				sh.Env = env
				if b, err := sh.CombinedOutput(); err != nil {
					t.Fatalf("shell: %v: %s", err, b)
				}
				var want []string
				entries, _ := os.ReadDir(out)
				for _, e := range entries {
					raw, _ := os.ReadFile(filepath.Join(out, e.Name()))
					for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
						if line != "" {
							want = append(want, strings.Join(strings.Split(line, "\x1f"), " \x00 "))
						}
					}
				}
				d, err := Check(Request{Command: cmd, Shell: shellName, Cwd: cwd, Env: env, Policy: DefaultPolicy(), RepoRoot: cwd})
				if err != nil {
					t.Fatalf("guard: %v", err)
				}
				var got []string
				for _, s := range d.Segments {
					if len(s.Argv) == 0 {
						continue
					}
					switch baseName(s.Argv[0]) {
					case "foo", "bar", "baz":
						if s.Class == ClassUnresolvable && len(s.Unresolved) > 0 {
							// permitted divergence: report it
							t.Logf("unresolvable: %v (%v)", s.Argv, s.Unresolved)
						}
						got = append(got, strings.Join(s.Argv, " \x00 "))
					}
				}
				sort.Strings(want)
				sort.Strings(got)
				if superset {
					for _, w := range want {
						found := false
						for _, g := range got {
							if g == w {
								found = true
							}
						}
						if !found {
							t.Errorf("shell ran %q, guard has %q", w, got)
						}
					}
				} else if strings.Join(want, "\n") != strings.Join(got, "\n") {
					t.Errorf("argv mismatch\n shell: %q\n guard: %q", want, got)
				}
				compared++
			})
		}
	}
	t.Logf("compared %d command runs across %d shells", compared, len(shells))
}
