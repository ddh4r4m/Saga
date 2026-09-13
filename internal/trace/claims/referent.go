package claims

import (
	"regexp"
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
