package claims

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// testSigs are the command signatures of the test families of
// shape-spec section 2.3 (the same table, applied to the signature
// because shape is not installed in this period).
var testSigs = map[string]bool{
	"pytest": true, "unittest": true, "manage.py test": true, "tox": true, "nox": true,
	"jest": true, "vitest": true, "mocha": true, "ava": true, "jasmine": true, "karma": true,
	"go test": true, "cargo test": true, "cargo nextest": true,
	"gradle test": true, "gradle check": true, "mvn test": true, "mvn verify": true,
	"xcodebuild test": true, "swift test": true, "flutter test": true, "dart test": true,
	"npm test": true, "pnpm test": true, "yarn test": true, "bun test": true, "deno test": true,
	"make test": true, "make check": true, "rspec": true, "phpunit": true, "mix test": true,
	"dotnet test": true, "ctest": true, "node --test": true, "busted": true, "pytest-xdist": true,
}

// IsTestCommand reports whether a command belongs to a test family.
func IsTestCommand(c Command) bool {
	if testSigs[c.Sig] {
		return true
	}
	for _, pm := range []string{"npm ", "pnpm ", "yarn ", "bun "} {
		if strings.HasPrefix(c.Sig, pm+"test") {
			return true
		}
	}
	// A script whose name says test (run_tests.sh, test.sh).
	head := strings.Fields(c.Sig)
	if len(head) == 1 && (strings.Contains(strings.ToLower(head[0]), "test")) && strings.ContainsAny(c.Raw, "/.") {
		return true
	}
	return false
}

// Status values of a test result (shape-spec section 2.4 subset).
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusError   = "error"
	StatusUnknown = "unknown"
)

// Summary is what the fixed runner-summary parse extracts from a stored
// tool result.
type Summary struct {
	Status string
	Passed *int
	Failed *int
}

var summaryRules = []struct {
	re   *regexp.Regexp
	kind string // pass, fail, error, passed_count, failed_count
}{
	{regexp.MustCompile(`(?m)^=+ .*\b(\d+) failed\b`), "failed_count"},
	{regexp.MustCompile(`(?m)^=+ .*\b(\d+) passed\b`), "passed_count"},
	{regexp.MustCompile(`(?m)^=+ .*\b(\d+) errors?\b`), "failed_count"},
	{regexp.MustCompile(`(?m)^=+ no tests ran`), "error"},
	{regexp.MustCompile(`(?m)^Tests:\s+(\d+) failed`), "failed_count"},
	{regexp.MustCompile(`(?m)^Tests:.*?\b(\d+) passed`), "passed_count"},
	{regexp.MustCompile(`(?m)^Test Suites:\s+(\d+) failed`), "failed_count"},
	{regexp.MustCompile(`(?m)^\s*(\d+) passing\b`), "passed_count"},
	{regexp.MustCompile(`(?m)^\s*(\d+) failing\b`), "failed_count"},
	{regexp.MustCompile(`(?m)^(?:FAIL|--- FAIL)\b`), "fail"},
	{regexp.MustCompile(`(?m)^ok\s+[\w./-]+`), "pass"},
	{regexp.MustCompile(`(?m)^PASS$`), "pass"},
	{regexp.MustCompile(`(?m)^(?:# |\S+\.go:\d+:\d+: )`), "error"},
	{regexp.MustCompile(`(?m)^Ran (\d+) tests?`), "passed_count_unittest"},
	{regexp.MustCompile(`(?m)^OK(?: \([^)]*\))?\s*$`), "pass"},
	{regexp.MustCompile(`(?m)^FAILED \((?:failures=(\d+))?`), "fail"},
	{regexp.MustCompile(`(?m)^FAILED \(errors=(\d+)`), "fail"},
	{regexp.MustCompile(`test result: ok\. (\d+) passed`), "passed_count"},
	{regexp.MustCompile(`test result: FAILED\. (\d+) passed; (\d+) failed`), "cargo_fail"},
	{regexp.MustCompile(`(?i)\bAll tests passed`), "pass"},
	{regexp.MustCompile(`\*\* TEST SUCCEEDED \*\*`), "pass"},
	{regexp.MustCompile(`\*\* TEST FAILED \*\*`), "fail"},
	{regexp.MustCompile(`(?m)^Some tests failed\.`), "fail"},
	{regexp.MustCompile(`(?m)^All tests passed!`), "pass"},
	{regexp.MustCompile(`(?i)\b(?:ModuleNotFoundError|ImportError|SyntaxError|command not found|No such file or directory|Cannot find module|MODULE_NOT_FOUND|error TS\d+|cannot find package)\b`), "error"},
}

// ParseSummary derives a status and counts from the text of a test
// runner's output. It is a fixed rule set; unknown when nothing matches.
func ParseSummary(text string) Summary {
	s := Summary{Status: StatusUnknown}
	pass, fail, errs := false, false, false
	for _, r := range summaryRules {
		m := r.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		switch r.kind {
		case "pass":
			pass = true
		case "fail":
			fail = true
			if len(m) > 1 && m[1] != "" {
				n, _ := strconv.Atoi(m[1])
				s.Failed = &n
			}
		case "error":
			errs = true
		case "passed_count":
			n, _ := strconv.Atoi(m[1])
			s.Passed = &n
			pass = true
		case "passed_count_unittest":
			n, _ := strconv.Atoi(m[1])
			s.Passed = &n
		case "failed_count":
			n, _ := strconv.Atoi(m[1])
			if n > 0 {
				s.Failed = &n
				fail = true
			}
		case "cargo_fail":
			p, _ := strconv.Atoi(m[1])
			f, _ := strconv.Atoi(m[2])
			s.Passed, s.Failed = &p, &f
			fail = true
		}
	}
	switch {
	case fail:
		s.Status = StatusFail
	case errs && !pass:
		s.Status = StatusError
	case pass:
		s.Status = StatusPass
	}
	if s.Status == StatusFail && s.Passed != nil && s.Failed != nil {
		// unittest: passed is Ran minus failures.
		if *s.Passed > *s.Failed && strings.Contains(text, "Ran ") && strings.Contains(text, "FAILED") {
			n := *s.Passed - *s.Failed
			s.Passed = &n
		}
	}
	return s
}

// ResultText extracts the runner's text from a stored tool result: the
// stdout and stderr fields of a harness object (Claude Code's Bash
// shape), or the bytes themselves.
func ResultText(raw []byte) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil && m != nil {
		var b strings.Builder
		for _, k := range []string{"stdout", "stderr", "output", "content", "result"} {
			if s, ok := m[k].(string); ok {
				b.WriteString(s)
				b.WriteString("\n")
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
