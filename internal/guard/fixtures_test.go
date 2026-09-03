package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

type fixture struct {
	ID             string   `toml:"id"`
	Source         string   `toml:"source"`
	Shell          string   `toml:"shell"`
	Cwd            string   `toml:"cwd"`
	Env            []string `toml:"env"`
	Raw            string   `toml:"raw"`
	Expect         string   `toml:"expect"`
	Rules          []string `toml:"rules"`
	Skip           string   `toml:"skip"`
	PermissionMode string   `toml:"permission_mode"`
	Segment        *struct {
		Index int    `toml:"index"`
		Rule  string `toml:"rule"`
	} `toml:"segment"`
	Class *struct {
		Index int    `toml:"index"`
		Class string `toml:"class"`
	} `toml:"class"`
	Argv *struct {
		Segment int    `toml:"segment"`
		Index   int    `toml:"index"`
		Value   string `toml:"value"`
	} `toml:"argv"`
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "guard", "incidents.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Fixture []fixture `toml:"fixture"`
	}
	if _, err := toml.Decode(string(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Fixture
}

// sandbox lays out the fixture filesystem: a git repository, a vault, a
// home directory.
type sandbox struct {
	tmp, home, repo string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	tmp := t.TempDir()
	if r, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = r
	}
	sb := &sandbox{tmp: tmp, home: filepath.Join(tmp, "home", "u"), repo: filepath.Join(tmp, "repo")}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{
		filepath.Join(sb.home, ".ssh"), filepath.Join(sb.home, "Documents"), filepath.Join(sb.home, ".aws"), filepath.Join(sb.home, ".gemini"),
		filepath.Join(sb.repo, "src"), filepath.Join(sb.repo, "build"), filepath.Join(sb.repo, "node_modules", "x"), filepath.Join(sb.repo, "tests"),
		filepath.Join(sb.repo, ".saga"), filepath.Join(sb.repo, ".gemini"),
		filepath.Join(tmp, "vault", "notes"),
	} {
		must(os.MkdirAll(d, 0o755))
	}
	files := map[string]string{
		filepath.Join(sb.home, ".ssh", "id_rsa"):       "key",
		filepath.Join(sb.home, ".aws", "credentials"):  "[default]",
		filepath.Join(sb.home, "Documents", "a.txt"):   "a",
		filepath.Join(sb.repo, "src", "main.go"):       "package main\n",
		filepath.Join(sb.repo, "src", "util.go"):       "package main\n",
		filepath.Join(sb.repo, "script.py"):            "print(1)\n",
		filepath.Join(sb.repo, "README.md"):            "# x\n",
		filepath.Join(sb.repo, "requirements.txt"):     "requests\n",
		filepath.Join(sb.repo, "build", "a.txt"):       "a",
		filepath.Join(sb.repo, ".gemini", ".env"):      "KEY=1\n",
		filepath.Join(sb.repo, ".saga", "policy.toml"): "schema = \"saga.guard.policy/1\"\n",
		filepath.Join(sb.repo, "package.json"):         `{"name":"x","scripts":{"test":"jest","build":"tsc -p .","leak":"cat ~/.ssh/id_rsa"}}`,
		filepath.Join(sb.repo, "Makefile"):             "build:\n\tgo build ./...\n\ntest:\n\tgo test ./...\n",
		filepath.Join(tmp, "vault", "notes", "a.md"):   "# a",
		filepath.Join(tmp, "vault", "b.md"):            "# b",
	}
	for p, c := range files {
		must(os.WriteFile(p, []byte(c), 0o644))
	}
	if _, err := exec.LookPath("git"); err == nil {
		run := func(args ...string) {
			cmd := exec.Command("git", append([]string{"-C", sb.repo}, args...)...)
			cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "HOME="+sb.home)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
		run("init", "-q", "-b", "main")
		run("add", "src", "script.py", "README.md", "package.json", "Makefile")
		run("commit", "-q", "-m", "init")
	} else {
		must(os.MkdirAll(filepath.Join(sb.repo, ".git"), 0o755))
	}
	return sb
}

func (sb *sandbox) expand(s string) string {
	s = strings.ReplaceAll(s, "{HOME}", sb.home)
	s = strings.ReplaceAll(s, "{REPO}", sb.repo)
	s = strings.ReplaceAll(s, "{TMP}", sb.tmp)
	if strings.Contains(s, "{TRUE51}") {
		s = strings.ReplaceAll(s, "{TRUE51}", strings.Repeat("true; ", 51))
	}
	return s
}

func (sb *sandbox) request(f fixture) Request {
	cwd := sb.repo
	if f.Cwd != "" {
		cwd = sb.expand(f.Cwd)
	}
	env := []string{"HOME=" + sb.home, "PATH=/usr/bin:/bin", "USER=u", "SHELL=/bin/" + f.Shell}
	for _, kv := range f.Env {
		env = append(env, sb.expand(kv))
	}
	mode := f.PermissionMode
	if mode == "" {
		mode = "acceptEdits"
	}
	return Request{Command: sb.expand(f.Raw), Shell: f.Shell, Cwd: cwd, Env: env, PermissionMode: mode, Policy: DefaultPolicy()}
}

func TestIncidentFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	sb := newSandbox(t)
	var escapes, controlDenies, skipped int
	for _, f := range fixtures {
		f := f
		t.Run(f.ID, func(t *testing.T) {
			if f.Skip != "" {
				skipped++
				t.Skip(f.Skip)
			}
			d, err := Check(sb.request(f))
			if err != nil {
				t.Fatalf("%s: %v", f.ID, err)
			}
			if verr := d.Validate(); verr != nil {
				t.Errorf("%s: decision record invalid: %v", f.ID, verr)
			}
			switch f.Expect {
			case "deny":
				if d.Decision != Deny {
					escapes++
					t.Errorf("%s ESCAPE: got %s (%s)", f.ID, d.Decision, d.Reason)
				}
			case "allow", "ask":
				if d.Decision == Deny {
					controlDenies++
					t.Errorf("%s CONTROL DENIED: %s", f.ID, d.Reason)
				} else if string(d.Decision) != f.Expect {
					t.Errorf("%s: got %s, want %s (%s)", f.ID, d.Decision, f.Expect, d.Reason)
				}
			case "allow_or_ask":
				if d.Decision == Deny {
					controlDenies++
					t.Errorf("%s CONTROL DENIED: %s", f.ID, d.Reason)
				}
			default:
				t.Fatalf("%s: bad expect %q", f.ID, f.Expect)
			}
			for _, r := range f.Rules {
				found := false
				for _, got := range d.Rules {
					if got == r {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: rule %s missing, got %v (%s)", f.ID, r, d.Rules, d.Reason)
				}
			}
			if f.Segment != nil {
				if f.Segment.Index >= len(d.Segments) {
					t.Errorf("%s: segment %d missing (have %d)", f.ID, f.Segment.Index, len(d.Segments))
				} else if got := d.Segments[f.Segment.Index].Rule; got != f.Segment.Rule {
					t.Errorf("%s: segment %d rule %q, want %q", f.ID, f.Segment.Index, got, f.Segment.Rule)
				}
			}
			if f.Class != nil {
				if f.Class.Index >= len(d.Segments) {
					t.Errorf("%s: segment %d missing", f.ID, f.Class.Index)
				} else if got := d.Segments[f.Class.Index].Class; string(got) != f.Class.Class {
					t.Errorf("%s: segment %d class %q, want %q", f.ID, f.Class.Index, got, f.Class.Class)
				}
			}
			if f.Argv != nil {
				want := sb.expand(f.Argv.Value)
				if f.Argv.Segment >= len(d.Segments) || f.Argv.Index >= len(d.Segments[f.Argv.Segment].Argv) {
					t.Errorf("%s: argv[%d] of segment %d missing", f.ID, f.Argv.Index, f.Argv.Segment)
				} else if got := d.Segments[f.Argv.Segment].Argv[f.Argv.Index]; got != want {
					t.Errorf("%s: argv[%d] = %q, want %q", f.ID, f.Argv.Index, got, want)
				}
			}
			t.Logf("%s %s %v %s", f.ID, d.Decision, d.Rules, d.Reason)
		})
	}
	t.Logf("fixtures: %d rows, %d skipped, %d escapes, %d control denies", len(fixtures), skipped, escapes, controlDenies)
	if escapes > 0 || controlDenies > 0 {
		t.Fatalf("exit criterion failed: %d escapes, %d control denies", escapes, controlDenies)
	}
}

func TestFixtureCounts(t *testing.T) {
	fixtures := loadFixtures(t)
	var incidents, controls int
	for _, f := range fixtures {
		switch {
		case strings.HasPrefix(f.ID, "I-"):
			incidents++
		case strings.HasPrefix(f.ID, "C-"):
			controls++
		}
	}
	if incidents < 18 {
		t.Errorf("want 18 incident rows, have %d", incidents)
	}
	if controls < 40 {
		t.Errorf("want 40 control rows, have %d", controls)
	}
}
