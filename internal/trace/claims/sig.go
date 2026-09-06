package claims

import (
	"path"
	"regexp"
	"strings"
)

// Command is a shell command reduced to shape-spec section 2.2's
// signature rule: wrapper prefixes, `cd x &&`, variable assignments and
// shell prefixes stripped, whitespace collapsed. Sig is argv[0] after
// stripping (plus the subcommand for multiplexers such as `go test`);
// Args are the non-flag arguments of every stage; Flags the flag
// tokens, lower-cased.
type Command struct {
	Raw   string
	Sig   string
	Args  []string
	Flags []string
	// Norm is the normalised command text stored in a claim.
	Norm string
}

var (
	reSeparator = regexp.MustCompile(`\s*(?:&&|\|\||;|\n)\s*`)
	reAssign    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	reRedirect  = regexp.MustCompile(`^(?:\d*>>?|<|&>|\d*>&\d*|2>&1)`)
)

// wrappers are stripped from the head of a stage; the value is how many
// following tokens (besides flags) go with the wrapper.
var wrappers = map[string]int{
	"env": 0, "nice": 0, "time": 0, "sudo": 0, "command": 0, "exec": 0,
	"timeout": 1, "npx": 0, "bunx": 0, "uvx": 0, "xcrun": 0,
}

// wrapperPairs are two-word wrappers.
var wrapperPairs = map[string]bool{
	"pnpm exec": true, "pnpm dlx": true, "yarn exec": true, "yarn dlx": true, "bun x": true,
	"uv run": true, "poetry run": true, "pipenv run": true, "pdm run": true, "hatch run": true,
	"bundle exec": true, "mise exec": true, "nix-shell --run": true, "cargo run": false,
}

// multiplexers carry their first subcommand in the signature.
var multiplexers = map[string]bool{
	"go": true, "npm": true, "pnpm": true, "yarn": true, "bun": true, "cargo": true, "dotnet": true,
	"git": true, "make": true, "dart": true, "flutter": true, "swift": true, "mix": true,
	"gradle": true, "mvn": true, "docker": true, "kubectl": true, "rails": true, "deno": true,
	"xcodebuild": true, "manage.py": true, "tox": false, "python": false,
}

// aliases fold spellings of one tool.
var aliases = map[string]string{
	"python3": "python", "python2": "python", "py.test": "pytest", "gradlew": "gradle", "nodejs": "node",
	"pip3": "pip", "ruby3": "ruby",
}

// Tokenize splits a shell line into words honouring single and double
// quotes and backslash escapes; quoting is dropped from the words.
func Tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	quote := rune(0)
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
			inWord = true
		case r == '\\' && quote != '\'':
			esc = true
			inWord = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t' || r == '\r':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// ParseCommand applies the signature rule to raw.
func ParseCommand(raw string) Command {
	c := Command{Raw: raw}
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "$ ")
	text = strings.TrimPrefix(text, "> ")
	var sigSet bool
	var normParts []string
	for _, seg := range reSeparator.Split(text, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		for si, stage := range strings.Split(seg, "|") {
			words := Tokenize(strings.TrimSpace(stage))
			words = stripPrefixes(words)
			if len(words) == 0 {
				continue
			}
			if words[0] == "cd" || words[0] == "pushd" || words[0] == "popd" || words[0] == "export" || words[0] == "source" || words[0] == "set" {
				continue
			}
			sig, rest := signatureOf(words)
			if !sigSet {
				c.Sig = sig
				sigSet = true
			}
			normWords := append([]string{}, strings.Fields(sig)...)
			if si > 0 {
				// A later pipe stage's own name is an argument of the
				// whole command for matching purposes.
				c.Args = append(c.Args, strings.Fields(sig)...)
			}
			for _, w := range rest {
				if reRedirect.MatchString(w) {
					continue
				}
				if strings.HasPrefix(w, "-") && len(w) > 1 {
					c.Flags = append(c.Flags, strings.ToLower(w))
				} else {
					c.Args = append(c.Args, w)
				}
				normWords = append(normWords, w)
			}
			if si > 0 {
				normParts = append(normParts, "|")
			}
			normParts = append(normParts, strings.Join(normWords, " "))
		}
	}
	c.Norm = strings.Join(strings.Fields(strings.Join(normParts, " ")), " ")
	return c
}

// stripPrefixes drops variable assignments and wrapper prefixes.
func stripPrefixes(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		switch {
		case reAssign.MatchString(w):
			words = words[1:]
		case len(words) >= 2 && wrapperPairs[w+" "+words[1]]:
			words = words[2:]
			words = skipFlags(words)
			if len(words) > 0 && words[0] == "--" {
				words = words[1:]
			}
		case isWrapper(w):
			n := wrappers[path.Base(w)]
			words = skipFlags(words[1:])
			if n > 0 && len(words) >= n {
				words = words[n:]
			}
			if len(words) > 0 && words[0] == "--" {
				words = words[1:]
			}
		default:
			return words
		}
	}
	return words
}

func isWrapper(w string) bool {
	_, ok := wrappers[path.Base(w)]
	return ok
}

func skipFlags(words []string) []string {
	for len(words) > 0 && strings.HasPrefix(words[0], "-") && words[0] != "--" {
		words = words[1:]
	}
	return words
}

// valueFlags are interpreter flags whose value is a separate token, so
// that token does not end the leading run of flags.
var valueFlags = map[string]bool{"-X": true, "-W": true, "-O": true}

// leadingFlags returns the length of the run of flag tokens at the head
// of words, counting the value of a flag that takes one.
func leadingFlags(words []string) int {
	i := 0
	for i < len(words) {
		switch {
		case strings.HasPrefix(words[i], "-") && words[i] != "--":
		case i > 0 && valueFlags[words[i-1]]:
		default:
			return i
		}
		i++
	}
	return i
}

// signatureOf returns the signature and the remaining tokens.
func signatureOf(words []string) (string, []string) {
	head := path.Base(words[0])
	if a, ok := aliases[head]; ok {
		head = a
	}
	rest := words[1:]
	// An interpreter's own flags never hide the subcommand: `python -W
	// error -m pytest` signs as pytest and `node --experimental-strip-
	// types --test x` as `node --test`, exactly as the unflagged forms
	// do. Two of six runs of the 2026-09-06 smoke invoked the runner
	// this way and were read as "no test run".
	if head == "python" {
		for i, n := 0, leadingFlags(rest); i < n; i++ {
			if rest[i] == "-m" && i+1 < len(rest) {
				mod := rest[i+1]
				if a, ok := aliases[mod]; ok {
					mod = a
				}
				return mod, rest[i+2:]
			}
		}
	}
	if head == "node" {
		for i, n := 0, leadingFlags(rest); i < n; i++ {
			if rest[i] == "--test" {
				return "node --test", append(append([]string{}, rest[:i]...), rest[i+1:]...)
			}
		}
	}
	if multiplexers[head] {
		i := 0
		for i < len(rest) && strings.HasPrefix(rest[i], "-") {
			i++
		}
		if i < len(rest) {
			sub := rest[i]
			flagsBefore := rest[:i]
			after := append(append([]string{}, flagsBefore...), rest[i+1:]...)
			switch head {
			case "npm", "pnpm", "yarn", "bun":
				if (sub == "run" || sub == "run-script") && len(rest) > i+1 {
					sub = rest[i+1]
					after = append(append([]string{}, flagsBefore...), rest[i+2:]...)
				}
			}
			return head + " " + sub, after
		}
	}
	return head, rest
}

// Matches reports whether the claimed command names an executed one:
// equal signatures and every non-flag argument of the claim present in
// the executed argv (quoting, flag case and flag order do not matter).
func Matches(claimed, executed Command) bool {
	if claimed.Sig == "" || claimed.Sig != executed.Sig {
		return false
	}
	have := map[string]bool{}
	for _, a := range executed.Args {
		have[a] = true
		have[strings.TrimPrefix(a, "./")] = true
	}
	for _, a := range claimed.Args {
		if !have[a] && !have[strings.TrimPrefix(a, "./")] {
			return false
		}
	}
	return true
}
