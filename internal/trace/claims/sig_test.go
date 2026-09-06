package claims

import (
	"strings"
	"testing"
)

func TestSignatureRule(t *testing.T) {
	cases := []struct {
		raw, sig, norm string
		args           []string
	}{
		{"pytest -q tests/auth", "pytest", "pytest -q tests/auth", []string{"tests/auth"}},
		{"cd packages/x && FOO=1 python3 -m pytest tests/auth -q 2>&1 | tail -n 1", "pytest", "pytest tests/auth -q | tail -n 1", []string{"tests/auth", "tail", "1"}},
		{"$ npx jest tests/retry", "jest", "jest tests/retry", []string{"tests/retry"}},
		{"npm run test:unit -- --watch=false", "npm test:unit", "npm test:unit -- --watch=false", []string{}},
		{"npm test -- --runInBand", "npm test", "npm test -- --runinband", nil},
		{"go test ./...", "go test", "go test ./...", []string{"./..."}},
		{"./gradlew test --info", "gradle test", "gradle test --info", nil},
		{"uv run pytest", "pytest", "pytest", nil},
		{"timeout 30 cargo test -p retry", "cargo test", "cargo test -p retry", []string{"retry"}},
		{"python manage.py test contacts", "python", "python manage.py test contacts", []string{"manage.py", "test", "contacts"}},
		{"bundle exec rspec spec/x_spec.rb", "rspec", "rspec spec/x_spec.rb", []string{"spec/x_spec.rb"}},
		{"node --test tests/", "node --test", "node --test tests/", []string{"tests/"}},
		{"make", "make", "make", nil},
		{"", "", "", nil},
	}
	for _, c := range cases {
		got := ParseCommand(c.raw)
		if got.Sig != c.sig {
			t.Errorf("%q: sig %q, want %q", c.raw, got.Sig, c.sig)
		}
		wantNorm := strings.Fields(c.norm)
		gotNorm := strings.Fields(got.Norm)
		if strings.Join(gotNorm, " ") != strings.Join(wantNorm, " ") && c.norm != "" && !strings.EqualFold(got.Norm, c.norm) {
			t.Errorf("%q: norm %q, want %q", c.raw, got.Norm, c.norm)
		}
		if c.args != nil && !sameSet(got.Args, c.args) {
			t.Errorf("%q: args %v, want %v", c.raw, got.Args, c.args)
		}
	}
}

func TestMatches(t *testing.T) {
	yes := [][2]string{
		{"pytest -q tests/auth", "cd x && pytest tests/auth -q 2>&1 | tail -n 1"},
		{"pytest", "python3 -m pytest -x tests/"},
		{"go test ./...", "GOFLAGS=-mod=mod go test ./... -count=1"},
		{"npm test", "npm run test -- --runInBand"},
		{"jest tests/retry", "npx jest --ci tests/retry"},
		{"cargo test -P retry", "cargo test -p retry"},
	}
	no := [][2]string{
		{"pytest tests/auth", "pytest tests/other"},
		{"go test ./...", "go build ./..."},
		{"pytest", "npm test"},
		{"make test", "make build"},
		{"cargo test -p RETRY", "cargo test -p retry"},
	}
	for _, p := range yes {
		if !Matches(ParseCommand(p[0]), ParseCommand(p[1])) {
			t.Errorf("%q should match %q", p[0], p[1])
		}
	}
	for _, p := range no {
		if Matches(ParseCommand(p[0]), ParseCommand(p[1])) {
			t.Errorf("%q should not match %q", p[0], p[1])
		}
	}
}

func TestTestFamilyAndSummary(t *testing.T) {
	for _, c := range []string{"pytest -q", "python -m pytest", "python3 -m unittest tests.test_merge", "npm test", "pnpm run test:unit", "go test ./...", "cargo nextest run", "./run_tests.sh", "npx vitest run", "flutter test"} {
		if !IsTestCommand(ParseCommand(c)) {
			t.Errorf("%q should be a test command", c)
		}
	}
	for _, c := range []string{"go build ./...", "npm run build", "cat README.md", "python script.py", "make lint", "ls tests"} {
		if IsTestCommand(ParseCommand(c)) {
			t.Errorf("%q should not be a test command", c)
		}
	}
	cases := []struct {
		text   string
		status string
		passed int
		failed int
	}{
		{"============ 3 failed, 120 passed in 4.2s ============", StatusFail, 120, 3},
		{"====== 12 passed in 0.3s ======", StatusPass, 12, -1},
		{"Tests:       2 failed, 8 passed, 10 total", StatusFail, 8, 2},
		{"Tests:       8 passed, 8 total", StatusPass, 8, -1},
		{"ok  \tgithub.com/x/y\t0.4s\nok  \tgithub.com/x/z\t0.1s", StatusPass, -1, -1},
		{"--- FAIL: TestX (0.00s)\nFAIL\nFAIL\tgithub.com/x/y\t0.4s", StatusFail, -1, -1},
		{"Ran 7 tests in 0.004s\n\nOK", StatusPass, 7, -1},
		{"Ran 7 tests in 0.004s\n\nFAILED (failures=2)", StatusFail, 5, 2},
		{"test result: ok. 14 passed; 0 failed; 0 ignored", StatusPass, 14, -1},
		{"test result: FAILED. 12 passed; 2 failed; 0 ignored", StatusFail, 12, 2},
		{"ModuleNotFoundError: No module named 'contacts'", StatusError, -1, -1},
		{"nothing to see here", StatusUnknown, -1, -1},
		{"  5 passing (40ms)\n  1 failing", StatusFail, 5, 1},
	}
	for _, c := range cases {
		s := ParseSummary(c.text)
		if s.Status != c.status {
			t.Errorf("%q: status %s, want %s", c.text, s.Status, c.status)
		}
		if c.passed >= 0 && (s.Passed == nil || *s.Passed != c.passed) {
			t.Errorf("%q: passed %v, want %d", c.text, s.Passed, c.passed)
		}
		if c.failed >= 0 && (s.Failed == nil || *s.Failed != c.failed) {
			t.Errorf("%q: failed %v, want %d", c.text, s.Failed, c.failed)
		}
	}
	if got := ResultText([]byte(`{"stdout":"ok\n","stderr":"","interrupted":false,"isImage":false}`)); !strings.Contains(got, "ok") {
		t.Errorf("result text %q", got)
	}
}

// TestSignaturePastLeadingFlags (brief 2026-09-06-test-command-past-flags):
// an interpreter's own flags must not hide the subcommand. ts-0005 A/2 of
// the 2026-09-06 smoke ran `node --experimental-strip-types --test …` and
// was read as "no test run", a false contradiction on an oracle-pass run.
func TestSignaturePastLeadingFlags(t *testing.T) {
	for _, c := range []struct {
		raw, sig string
		args     []string
		test     bool
	}{
		{"node --experimental-strip-types --test test/retry.test.ts 2>&1 | tail -30", "node --test", []string{"test/retry.test.ts", "tail"}, true},
		{`node --test --test-reporter=tap "test/**/*.test.ts"`, "node --test", []string{"test/**/*.test.ts"}, true},
		{"node --test", "node --test", []string{}, true},
		{"node --experimental-strip-types src/index.ts", "node", []string{"src/index.ts"}, false},
		{"node --experimental-strip-types --test-only src/x.ts", "node", []string{"src/x.ts"}, false},
		{"python -W error -m pytest -q", "pytest", []string{}, true},
		{"python3 -X dev -m pytest tests/", "pytest", []string{"tests/"}, true},
		{"python -u -m unittest discover", "unittest", []string{"discover"}, true},
		{"python -m http.server 8000", "http.server", []string{"8000"}, false},
		{`python -c "import x"`, "python", []string{"import x"}, false},
		{"python script.py -m pytest", "python", []string{"script.py", "pytest"}, false},
	} {
		got := ParseCommand(c.raw)
		if got.Sig != c.sig {
			t.Errorf("%q: sig %q, want %q", c.raw, got.Sig, c.sig)
		}
		if !sameSet(got.Args, c.args) {
			t.Errorf("%q: args %v, want %v", c.raw, got.Args, c.args)
		}
		if IsTestCommand(got) != c.test {
			t.Errorf("%q: test family %v, want %v", c.raw, !c.test, c.test)
		}
	}
}
