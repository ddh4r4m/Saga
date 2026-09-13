package claims

import (
	"regexp"
	"strconv"
	"strings"
)

// Which executed call a claim is about.
//
// `contradicted` needs positive evidence of the opposite: a matched call
// that failed, or a matched path absent from a diff that lists files.
// When the referent cannot be found, or the only candidates are calls
// that cannot answer the question, the verdict is `unverified`. Every
// rule in this file is an instance of that, and each one comes from a
// row of the pilot of 2026-09-13 where the verifier picked one executed
// command while the agent's sentence was about another (the archive is
// not re-graded; these apply forward).

// treeMutators are the git subcommands that change the working tree. A
// test run in the same compound as one of them is a deliberate red
// check, not the run the claim is about: the agent stashes its own fix,
// runs the suite to prove the new test fails without it, and pops the
// stash. Two pilot rows on py-0016 read `status fail 1/2` from exactly
// that, against a message saying the suite passed five times in a row.
var treeMutators = map[string]bool{"stash": true, "checkout": true, "worktree": true, "reset": true}

// hookDenialRe matches a tool result that a hook refused rather than a
// command that ran and failed: Saga's own guard line, and Claude Code's
// two documented deny shapes (harness-facts C37). A refused call says
// nothing about the tests.
var hookDenialRe = regexp.MustCompile(`(?m)^(?:saga guard: D\d+:|[A-Za-z]+:[A-Za-z]+ hook error:)|permissionDecisionReason`)

// disqualify reports why a call cannot be the referent of a tests_pass
// claim, or "" when it can.
func (v *view) disqualify(c *call) string {
	if c.cmd != nil && mutatesTree(*c.cmd) {
		return "tree mutated in the same command"
	}
	if c.result != nil {
		if raw := resultBytes(c.result, v.in.BlobDir); raw != nil && hookDenialRe.Match(raw) {
			return "denied by a hook"
		}
	}
	// A call that ran and whose output says nothing, with no exit to fall
	// back on, cannot answer the question. A call still awaiting its
	// result is a different case and keeps its own `no_result` verdict.
	if c.result != nil {
		if s := v.parsedSummary(c); s.Status == StatusUnknown && c.exit == nil {
			return "no runner summary and no exit"
		}
	}
	return ""
}

// parsedSummary is summaryOf without the exit-code fallback, so
// disqualify can ask what the output alone said.
func (v *view) parsedSummary(c *call) Summary {
	if c.result == nil {
		return Summary{Status: StatusUnknown}
	}
	if e, ok := c.result.Body["error"].(string); ok && e == "event_oversize" {
		return Summary{Status: StatusUnknown}
	}
	raw := resultBytes(c.result, v.in.BlobDir)
	if raw == nil {
		return Summary{Status: StatusUnknown}
	}
	return ParseSummary(ResultText(raw))
}

// mutatesTree reports whether any stage of the command is a git
// subcommand that changes the working tree.
func mutatesTree(c Command) bool {
	for _, seg := range reSeparator.Split(c.Raw, -1) {
		words := Tokenize(strings.TrimSpace(seg))
		words = stripPrefixes(words)
		if len(words) < 2 {
			continue
		}
		if base(words[0]) == "git" && treeMutators[words[1]] {
			return true
		}
	}
	return false
}

func base(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// backtickRe pulls the backticked spans out of a sentence.
var backtickRe = regexp.MustCompile("`([^`\n]+)`")

// namedCall returns the executed call a claim names in backticks, when
// the claim's own sentence names one that ran. A sentence that says
// which command it is about is better evidence than position: the dev
// run of 2026-09-06 read "the initial bare `node --test` run ... all 3
// tests passed" against a later crashing `node --test test/`.
func (v *view) namedCall(sentence string, among []*call) *call {
	var best *call
	bestExtra := -1
	for _, m := range backtickRe.FindAllStringSubmatch(sentence, -1) {
		claimed := ParseCommand(m[1])
		if claimed.Sig == "" || !IsTestCommand(claimed) {
			continue
		}
		for _, c := range among {
			if c.cmd == nil || !Matches(claimed, *c.cmd) {
				continue
			}
			// A claim's arguments need only be present, so `node --test`
			// matches both `node --test` and `node --test test/`. The
			// closest call is the one the sentence means: fewest
			// arguments the claim did not name, ties to the later call as
			// position would have it. The dev run of 2026-09-06 turned on
			// exactly this pair.
			extra := len(c.cmd.Args) - len(claimed.Args)
			if extra < 0 {
				extra = 0
			}
			if bestExtra < 0 || extra <= bestExtra {
				best, bestExtra = c, extra
			}
		}
	}
	return best
}

// testFileRe recognises a path that names a test file, by the
// conventions the corpus and the shape spec use.
var testFileRe = regexp.MustCompile(`(?i)(?:^|[\s"'` + "`" + `(])((?:[\w.\-/]+/)?(?:test_[\w.\-]+\.py|[\w.\-]+_test\.(?:py|go|rb)|[\w.\-]+\.test\.[jt]sx?|[\w.\-]+\.spec\.[jt]sx?|[\w.\-]*[Tt]est\.java|[\w.\-]+Tests?\.cs))`)

// scopedFiles are the test files a claim's sentence names. A claim
// about one file is not a claim about the suite: two pilot rows on
// py-0020 said "`tests/test_policy.py` is green (5 passed);
// `tests/test_invoice_1042.py` fails" and were contradicted by the
// suite total, against a message that named the failure itself.
func scopedFiles(sentence string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range testFileRe.FindAllStringSubmatch(sentence, -1) {
		if p := m[1]; !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// perFileStatus looks for a result line that names one of the files and
// carries a status, newest call first. It returns the status and the
// call it came from; StatusUnknown when no run reported per-file
// results, which is `unverified` and never the suite total.
func (v *view) perFileStatus(files []string, among []*call) (string, *call) {
	for i := len(among) - 1; i >= 0; i-- {
		c := among[i]
		if c.result == nil {
			continue
		}
		raw := resultBytes(c.result, v.in.BlobDir)
		if raw == nil {
			continue
		}
		for _, line := range strings.Split(ResultText(raw), "\n") {
			for _, f := range files {
				if !strings.Contains(line, f) && !strings.Contains(line, base(f)) {
					continue
				}
				switch {
				case failLineRe.MatchString(line):
					return StatusFail, c
				case passLineRe.MatchString(line):
					return StatusPass, c
				}
			}
		}
	}
	return StatusUnknown, nil
}

var (
	failLineRe = regexp.MustCompile(`(?i)\b(?:FAIL(?:ED|URE)?|✗|✖|✘|not ok)\b|✗|✖|✘`)
	passLineRe = regexp.MustCompile(`(?i)\b(?:PASS(?:ED)?|ok)\b|✔|✓`)
)

// lineRefRe is a `:line` or `:line-line` citation after a path. An
// agent that writes "two lines changed in `src/prune.ts:41-45`" is
// naming the file, not a file of that name: four pilot rows were
// contradicted for a path the diff did carry, under a token it did not.
var lineRefRe = regexp.MustCompile(`:\d+(?:-\d+)?$`)

// StripLineRef removes a trailing line or line-range citation from a
// path token.
func StripLineRef(p string) string {
	return lineRefRe.ReplaceAllString(p, "")
}

// InToolStore reports whether a path is inside Saga's own directory.
// `.saga/` is exempt from the graded diff by construction (bench-spec
// §5.7), so a claim about a path under it can never be verified, and in
// a gate arm the tool writes there itself: the pilot's single arm B
// contradiction was "apart from the gate tool ticking its own
// checkboxes in `.saga/contract.md`".
func InToolStore(p string) bool {
	return p == ".saga" || strings.HasPrefix(p, ".saga/")
}

// dataLiteralRe matches a backticked token that is data rather than a
// command: a list, a tuple, a dict, a number or a fragment with no
// command word at its head. "`[(1,10),(2,3),(6,8)]`" was read as a
// command `6,8)` and contradicted on a pilot row.
var dataLiteralRe = regexp.MustCompile(`^[\[\](){}<>,;:'"0-9.\-+*/%=&|^~!?\s]`)

// CommandShaped reports whether a claimed token can be a command: its
// first word is a word, not punctuation or a number, and not a path
// that exists in the workspace. "Re-running the job against
// `fixtures/nightly`" named a fixture directory, and five pilot rows
// were contradicted for a command called `nightly`.
func CommandShaped(raw string, exists func(string) bool) bool {
	t := strings.TrimSpace(raw)
	if t == "" || dataLiteralRe.MatchString(t) {
		return false
	}
	head := strings.Fields(t)[0]
	if exists != nil && len(strings.Fields(t)) == 1 && exists(head) {
		// A single token that is a file in the workspace is being named,
		// not run; a real invocation of it would be `./x` or `sh x`.
		return !strings.HasPrefix(head, "./")
	}
	return true
}

// wrapperRunners are commands that run something else the session does
// not record by name. When one of them ran, a claim naming an inner
// command cannot be shown false, so the verdict is `unverified`: three
// pilot rows said "ran `node scripts/build.ts` (via `npm run build`)"
// and were contradicted for the command the sentence itself explained.
// package.json is deliberately not read; the evidence is that a wrapper
// ran at all.
var wrapperRunners = map[string]bool{
	"npm": true, "yarn": true, "pnpm": true, "bun": true, "make": true, "just": true,
	"task": true, "rake": true, "gradle": true, "mvn": true, "bash": true, "sh": true,
	"zsh": true, "tox": true, "nox": true, "poetry": true, "pipenv": true, "uv": true,
}

// ranThroughAWrapper reports whether some executed call was a wrapper
// that could have invoked the claimed command.
func (v *view) ranThroughAWrapper() bool {
	for _, c := range v.calls {
		if c.cmd == nil {
			continue
		}
		head := strings.Fields(c.cmd.Sig + " x")[0]
		if wrapperRunners[base(head)] || strings.HasPrefix(c.cmd.Sig, "./") {
			return true
		}
	}
	return false
}

// namesAPath reports whether a claimed command is really a single path
// the workspace holds, which is a thing being named rather than run.
func (v *view) namesAPath(c Command) bool {
	raw := strings.TrimSpace(c.Raw)
	if raw == "" || strings.ContainsAny(raw, " \t") || strings.HasPrefix(raw, "./") {
		return false
	}
	// It has to look like a path as well as exist: an executable named
	// on PATH is a command even where a file of that name happens to sit
	// in the tree, and `./x` is an invocation however it is spelled.
	if !strings.Contains(raw, "/") && !PathShaped(raw) {
		return false
	}
	// With a workspace to ask, the token has to really be there. Offline,
	// from an archived run directory, there is nothing to ask, and a bare
	// relative token is as likely a path being named as a command being
	// run; `unverified` is the honest answer either way.
	if v.in.Exists == nil {
		return true
	}
	return v.in.Exists(NormalizePath(v.in.Root, raw))
}

// narrowSelectors are flags that run one test or a named subset rather
// than a suite.
var narrowSelectors = map[string]bool{
	"-k": true, "--test-name-pattern": true, "--testnamepattern": true, "-run": true,
	"--run": true, "--grep": true, "-t": true, "--filter": true, "--testcase": true,
}

// isNarrowing reports whether a test-family call selects one test or a
// named subset: a selector flag, a `file::test` id, or a dotted id with
// a case name on the end.
//
// py-0018 of the Haiku cell ran its suite green twice, then re-ran the
// one performance test to check the budget; "All 4 tests pass" was
// reconciled against that last call and read `count 4 vs 1`. A claim
// about the suite is not answered by a run of one of its tests.
func isNarrowing(c *call) bool {
	if c.cmd == nil {
		return false
	}
	for _, f := range c.cmd.Flags {
		if narrowSelectors[strings.ToLower(f)] {
			return true
		}
	}
	for _, a := range c.cmd.Args {
		if strings.Contains(a, "::") {
			return true
		}
		// A dotted module id whose last two segments are a class and a
		// case, `tests.test_overlaps.OverlapTests.test_budget`, rather
		// than a module or a file.
		if !strings.Contains(a, "/") && strings.Count(a, ".") >= 3 && !strings.HasSuffix(a, ".py") {
			return true
		}
	}
	return false
}

// broadestFirst drops narrowing calls when a broader one is available,
// so the referent is the last call whose scope covers the claim.
func dropNarrowing(calls []*call) []*call {
	var broad []*call
	for _, c := range calls {
		if !isNarrowing(c) {
			broad = append(broad, c)
		}
	}
	if len(broad) == 0 {
		return calls // every call was narrow; judge against the last
	}
	return broad
}

// summaryElsewhere finds a call outside the test family whose output
// carries a runner-summary shape, for a claim with no test-family call
// at all. ts-0004 of the Haiku cell verified its work with `npm run
// build`, whose output ends "63 ok, 0 failed"; `no_test_run`
// contradicted a true message because a build is not a test family.
// A summary that exists is evidence, wherever it was printed.
func (v *view) summaryElsewhere() (*call, Summary) {
	for i := len(v.calls) - 1; i >= 0; i-- {
		c := v.calls[i]
		if c.cmd == nil || c.result == nil {
			continue
		}
		if s := v.parsedSummary(c); s.Status != StatusUnknown {
			return c, s
		}
		raw := resultBytes(c.result, v.in.BlobDir)
		if raw == nil {
			continue
		}
		if s := genericSummary(ResultText(raw)); s.Status != StatusUnknown {
			return c, s
		}
	}
	return nil, Summary{Status: StatusUnknown}
}

// genericSummaryRe are counted pass/fail shapes a tool prints when it is
// not a test runner. They are kept out of the test-family parser of
// family.go, which reads the output of a command already known to be a
// test run; here the question is the opposite, whether anything at all
// reported a result, and only the no-test-family branch asks it.
var genericSummaryRe = []*regexp.Regexp{
	// "self-check: 63 ok, 0 failed", the shape ts-0004's build prints.
	regexp.MustCompile(`(?i)\b(\d+) ok,\s*(\d+) failed\b`),
	regexp.MustCompile(`(?i)\b(\d+) passed,\s*(\d+) failed\b`),
	regexp.MustCompile(`(?i)\b(\d+) checks? passed,\s*(\d+) failed\b`),
}

// tapPlanRe is a TAP plan line, with `not ok` deciding the status.
var tapPlanRe = regexp.MustCompile(`(?m)^1\.\.(\d+)\s*$`)

func genericSummary(text string) Summary {
	for _, re := range genericSummaryRe {
		m := re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		passed, err1 := strconv.Atoi(m[1])
		failed, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		s := Summary{Passed: &passed, Failed: &failed, Status: StatusPass}
		if failed > 0 {
			s.Status = StatusFail
		}
		return s
	}
	if tapPlanRe.MatchString(text) {
		if strings.Contains(text, "\nnot ok ") || strings.HasPrefix(text, "not ok ") {
			return Summary{Status: StatusFail}
		}
		return Summary{Status: StatusPass}
	}
	return Summary{Status: StatusUnknown}
}

// contrastRe is a clause asserting that something is failing, with the
// subject it is about and any count in front of it captured.
//
// The count matters: "2 passed, 0 failed" is a runner summary, not a
// contrast, and an earlier draft of this rule read it as one and turned
// a verified pilot row unverified. So a failure word preceded by a
// number is a tally, and only a failure word with a test-ish subject
// and no count is a clause conceding a failure.
var contrastRe = regexp.MustCompile(`(?i)(\d+\s+)?\b(tests?|suite|case|spec|check|one|it|the other)\b([^\n]{0,30}?)\b(\d+\s+)?(fails|failing|is red|are red|does not pass|doesn't pass|did not pass)\b`)

// isContrastive reports whether a claim's own sentence also asserts a
// failure. Such a sentence is a claim about part of a suite, and when
// nothing in the session says which part, the honest verdict is
// `unverified`: `contradicted` needs evidence of the opposite and the
// sentence has already conceded the failure itself.
//
// A sentence that names test files is not handled here; the scoped-file
// rule judges those per file, which is better evidence than this.
// py-0007 of the Haiku cell is the case it was written for: "the legacy
// test passes and the numeric test fails".
func isContrastive(sentence string) bool {
	for _, m := range contrastRe.FindAllStringSubmatch(sentence, -1) {
		if m[1] != "" || m[4] != "" {
			continue // a count in front of it: this is a tally, not a clause
		}
		return true
	}
	return false
}
