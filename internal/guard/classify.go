package guard

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// classifier assigns the class and hard-deny rule of each segment on its
// resolved argv (guard-spec 2.4 and 2.4.1).
type classifier struct {
	scope *scope
	pol   *Policy
	shell string
	cwd   string
	root  string
	env   map[string]string
}

// classRank orders classes for merging a verb class with redirect effects.
func classRank(c Class) int {
	switch c {
	case ClassRead:
		return 0
	case ClassNetworkFetch:
		return 1
	case ClassMutateIn:
		return 2
	case ClassInterpreter:
		return 3
	case ClassMutateOut:
		return 4
	case ClassUnresolvable:
		return 5
	case ClassDestructive:
		return 6
	}
	return 0
}

func (c *classifier) raise(s *Segment, cl Class, reason string) {
	if classRank(cl) > classRank(s.Class) {
		s.Class = cl
		if reason != "" {
			s.Reason = reason
		}
	}
}

// deny marks a segment destructive under rule with a reason.
func (c *classifier) deny(s *Segment, rule, reason string, paths ...string) {
	if s.Class == ClassDestructive {
		return
	}
	s.Class, s.Rule = ClassDestructive, rule
	s.Reason = rule + ": " + reason
	if len(s.Raw) > 0 && len(s.Raw) < 120 {
		s.Reason += ". raw: " + s.Raw
	}
	if len(s.Unresolved) > 0 {
		s.Reason += " ; " + strings.Join(s.Unresolved, ", ") + " unset"
	}
	s.Paths = appendUnique(s.Paths, paths)
}

// abs canonicalises a resolved argv path against the segment's cwd.
func (c *classifier) abs(p string) string {
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		p = "/"
	}
	return canonPath(c.cwd, p)
}

func isPlaceholder(a string) bool {
	return a == phSubshell || a == phProcSubst || a == phXargsInput || strings.Contains(a, phFindMatch)
}

// looksLikePath is the heuristic for unknown verbs: absolute, dotted,
// tilde or slash-containing arguments.
func looksLikePath(a string) bool {
	if a == "" || strings.HasPrefix(a, "-") || isPlaceholder(a) || strings.Contains(a, "://") {
		return false
	}
	return filepath.IsAbs(a) || strings.HasPrefix(a, "./") || strings.HasPrefix(a, "../") || a == "." || a == ".." || strings.HasPrefix(a, "~") || strings.Contains(a, "/")
}

// classify sets Class, Rule, Reason and Paths on one segment.
func (c *classifier) classify(s *Segment) {
	if s.Class == ClassDestructive { // pre-marked (fork bomb)
		return
	}
	s.Class = ClassRead
	if len(s.Argv) > 0 {
		c.classifyArgv(s)
	}
	c.redirectEffects(s)
	if s.Class != ClassDestructive && len(s.Argv) > 0 && matchAny(c.pol.Deny.Extra, s.Argv) {
		c.deny(s, "policy", "deny.extra pattern matches "+strings.Join(s.Argv, " "))
	}
	if s.Class != ClassDestructive && len(s.Unresolved) > 0 {
		s.Class = ClassUnresolvable
		if s.Reason == "" {
			s.Reason = "unresolvable: " + strings.Join(s.Unresolved, ", ")
		}
	}
	if s.remote && s.Class != ClassDestructive && s.Class != ClassUnresolvable {
		s.Class = ClassMutateOut
		if s.Reason == "" {
			s.Reason = "remote execution"
		}
	}
}

// pathClass is the class of a write to p.
func (c *classifier) pathClass(s *Segment, p string, verb string) Class {
	q := c.abs(p)
	s.Paths = appendUnique(s.Paths, []string{q})
	switch {
	case isDevice(q):
		c.deny(s, "D9", verb+" writes to device "+q, q)
		return ClassDestructive
	case c.scope.isSagaProtected(q):
		c.deny(s, "D11", verb+" edits the layer's own file "+q, q)
		return ClassDestructive
	case c.scope.inScope(q):
		return ClassMutateIn
	}
	return ClassMutateOut
}

// redirectEffects folds > and >> targets into the class.
func (c *classifier) redirectEffects(s *Segment) {
	for _, w := range s.writes {
		if isPlaceholder(w) {
			s.Unresolved = append(s.Unresolved, "redirect target "+w)
			continue
		}
		q := c.abs(w)
		if q == "/dev/null" || q == "/dev/stdout" || q == "/dev/stderr" || q == "/dev/tty" || strings.HasPrefix(q, "/dev/fd/") {
			continue
		}
		if c.scope.isCredential(q) {
			c.deny(s, "D2", "redirect truncates or appends to credential path "+q+" outside scope", q)
			continue
		}
		cl := c.pathClass(s, w, "redirect")
		if cl == ClassMutateOut {
			c.raise(s, cl, "redirect writes outside scope: "+q)
		} else {
			c.raise(s, cl, "")
		}
	}
	for _, r := range s.reads {
		if isPlaceholder(r) {
			continue
		}
		q := c.abs(r)
		if c.scope.isCredential(q) {
			c.deny(s, "D7", "credential path read via redirect "+q, q)
		}
	}
}

var (
	reDropSQL     = regexp.MustCompile(`(?i)\b(drop\s+(database|schema)|truncate)\b`)
	reDeleteGQL   = regexp.MustCompile(`(?i)mutation[^}]*\bdelete`)
	reFlagHasR    = regexp.MustCompile(`^-[a-zA-Z]*[rR][a-zA-Z]*$`)
	reFlagHasF    = regexp.MustCompile(`^-[a-zA-Z]*f[a-zA-Z]*$`)
	reFlagHasX    = regexp.MustCompile(`^-[a-zA-Z]*x[a-zA-Z]*$`)
	reFlagHasI    = regexp.MustCompile(`^-[a-zA-Z]*i[a-zA-Z]*$`)
	reFlagHasD    = regexp.MustCompile(`^-[a-zA-Z]*d[a-zA-Z]*$`)
	reFlagCaseD   = regexp.MustCompile(`^-[a-zA-Z]*D[a-zA-Z]*$`)
	protectedSaga = []string{"gate approve", "gate attest", "gate check --approve", "trace budget --raise", "trace budget ack", "trace pin --set", "trace prices use", "trace prune", "route policy trust", "route policy validate --write", "route budget --raise", "mem confirm", "mem review", "mem prune", "guard policy trust", "guard policy set", "snapshot gc", "snapshot prune", "install", "uninstall"}
)

func hasFlag(argv []string, re *regexp.Regexp, long ...string) bool {
	for _, a := range argv[1:] {
		if a == "--" {
			return false
		}
		if re != nil && re.MatchString(a) {
			return true
		}
		for _, l := range long {
			if a == l || strings.HasPrefix(a, l+"=") {
				return true
			}
		}
	}
	return false
}

// positionals returns non-flag arguments (after `--` everything counts).
func positionals(argv []string, withArg ...string) []string {
	var out []string
	after := false
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		if after {
			out = append(out, a)
			continue
		}
		if a == "--" {
			after = true
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			for _, w := range withArg {
				if a == w {
					i++
					break
				}
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func containsAny(argv []string, want ...string) bool {
	for _, a := range argv {
		for _, w := range want {
			if a == w {
				return true
			}
		}
	}
	return false
}

func (c *classifier) classifyArgv(s *Segment) {
	verb := baseName(s.Argv[0])
	argv := s.Argv

	// D11: agent-forbidden saga commands and the layer's own files.
	if verb == "saga" {
		joined := strings.Join(argv[1:], " ")
		for _, p := range protectedSaga {
			if joined == p || strings.HasPrefix(joined, p+" ") || (strings.Contains(p, "--") && strings.HasPrefix(joined, strings.Fields(p)[0]+" "+strings.Fields(p)[1]) && strings.Contains(joined, strings.Fields(p)[len(strings.Fields(p))-1])) {
				c.deny(s, "D11", "agent-forbidden command saga "+joined)
				return
			}
		}
		s.Class = ClassMutateIn
		return
	}

	// D10: piped or substituted remote scripts.
	if s.fetchSubst && (c.shellConsumer(verb, argv) || isShellVerb(verb) || isInterpreter(verb) || verb == "eval") {
		c.deny(s, "D10", "remote script substituted into "+verb)
		return
	}
	if c.shellConsumer(verb, argv) {
		if s.stdinFrom != nil && isFetchArgv(s.stdinFrom.Argv) {
			c.deny(s, "D10", "remote script piped into "+verb+" from "+strings.Join(s.stdinFrom.Argv, " "))
			return
		}
		if s.fetchSubst {
			c.deny(s, "D10", "remote script substituted into "+verb)
			return
		}
	}
	if verb == "eval" {
		if s.fetchSubst || (len(s.argvSubst) > 1 && s.argvSubst[1] == "fetch") {
			c.deny(s, "D10", "remote script substituted into eval")
			return
		}
		s.Unresolved = appendUnique(s.Unresolved, []string{"eval"})
		s.Class = ClassUnresolvable
		return
	}

	// Injected values: an argument built from a variable that itself
	// carries a compound command with a hard-deny pattern (I-15).
	if c.injectedDeny(s) {
		return
	}

	switch verb {
	case "rm", "unlink", "rmdir", "trash":
		c.classifyRm(s, verb)
	case "find":
		c.classifyFind(s)
	case "mv", "rename":
		c.classifyMv(s)
	case "cp", "install", "rsync", "scp", "sftp":
		c.classifyCopy(s, verb)
	case "git":
		c.classifyGit(s)
	case "cat", "less", "more", "head", "tail", "bat", "strings", "xxd", "od", "hexdump", "view", "vi", "vim", "nvim", "nano", "emacs", "source", ".", "base64", "grep", "egrep", "fgrep", "rg", "ag", "awk", "gawk", "sed", "cut", "sort", "jq", "yq", "wc", "diff", "cmp", "tee", "open", "code":
		c.classifyReader(s, verb)
	case "env", "printenv", "set", "export", "declare", "typeset":
		if verb == "set" || verb == "export" || verb == "declare" || verb == "typeset" {
			if len(argv) == 1 || containsAny(argv, "-p", "-x") && len(argv) <= 2 {
				c.deny(s, "D7", verb+" prints the environment (secrets) without a mask wrapper")
				return
			}
			s.Class = ClassRead
			return
		}
		c.deny(s, "D7", verb+" prints the environment (secrets) without a mask wrapper")
	case "security":
		if containsAny(argv, "find-generic-password", "find-internet-password", "dump-keychain", "export", "find-certificate") {
			c.deny(s, "D7", "keychain read: "+strings.Join(argv[:min(len(argv), 3)], " "))
			return
		}
		s.Class = ClassMutateOut
	case "gh":
		c.classifyGh(s)
	case "aws", "gcloud", "az", "gsutil", "doctl", "linode-cli", "wrangler":
		c.classifyCloud(s, verb)
	case "terraform", "tofu", "terragrunt", "pulumi", "cdk", "sam", "sls", "serverless", "eb", "firebase", "heroku", "fly", "flyctl", "vercel", "netlify":
		c.classifyIaC(s, verb)
	case "kubectl", "oc", "helm":
		c.classifyKube(s, verb)
	case "docker", "podman", "nerdctl", "docker-compose":
		c.classifyDocker(s)
	case "npm", "pnpm", "yarn", "bun":
		c.classifyNpm(s, verb)
	case "pip", "pip3", "uv", "poetry", "pipenv", "conda", "twine", "flit", "hatch":
		c.classifyPip(s, verb)
	case "cargo":
		c.classifyCargo(s)
	case "go":
		c.classifyGo(s)
	case "gem", "bundle", "bundler", "pod", "dart", "flutter", "mvn", "gradle", "gradlew", "dotnet", "nuget", "goreleaser", "swift", "xcodebuild", "mix", "composer", "stack", "cabal", "zig":
		c.classifyBuildTool(s, verb)
	case "brew", "apt", "apt-get", "yum", "dnf", "pacman", "snap", "port", "choco", "winget", "rustup", "nvm", "pyenv", "asdf", "mise", "volta", "sdk":
		if containsAny(argv, "list", "ls", "info", "search", "--version", "-v", "outdated", "doctor", "which", "current", "show", "config") {
			s.Class = ClassRead
			return
		}
		s.Class = ClassMutateOut
		s.Reason = verb + " changes the toolchain outside scope"
	case "dd":
		for _, a := range argv[1:] {
			if strings.HasPrefix(a, "of=") {
				q := c.abs(a[3:])
				if isDevice(q) {
					c.deny(s, "D9", "dd writes to device "+q, q)
					return
				}
				c.raise(s, c.pathClass(s, a[3:], "dd"), "")
			}
		}
		if s.Class == ClassRead {
			s.Class = ClassRead
		}
	case "mkfs", "mkswap", "wipefs", "shred", "fdisk", "sfdisk", "parted", "sgdisk", "format", "hdparm", "blkdiscard", "zpool", "newfs":
		c.deny(s, "D9", verb+" destroys a filesystem or device")
	case "diskutil":
		if len(argv) > 1 {
			sub := strings.ToLower(argv[1])
			if strings.HasPrefix(sub, "erase") || sub == "partitiondisk" || sub == "zerodisk" || sub == "randomdisk" || sub == "secureerase" || sub == "reformat" || (sub == "apfs" && len(argv) > 2 && strings.HasPrefix(strings.ToLower(argv[2]), "delete")) {
				c.deny(s, "D9", "diskutil "+argv[1]+" destroys a volume")
				return
			}
		}
		s.Class = ClassRead
	case "chmod", "chown", "chgrp":
		c.classifyChmod(s, verb)
	case "psql", "mysql", "mysqladmin", "sqlite3", "redis-cli", "mongo", "mongosh", "dropdb", "clickhouse-client", "cqlsh":
		c.classifyDB(s, verb)
	case "prisma", "supabase", "rails", "rake", "artisan", "sequelize", "sequelize-cli", "knex", "typeorm", "drizzle-kit", "alembic", "flask", "manage.py", "django-admin", "atlas", "dbmate", "goose", "migrate":
		c.classifyMigrate(s, verb)
	case "curl", "wget", "http", "https", "xh", "aria2c":
		c.classifyFetch(s, verb)
	case "ssh", "mosh", "telnet", "nc", "ncat", "socat":
		s.Class = ClassMutateOut
		s.Reason = verb + ": remote or network session"
	case "python", "python2", "python3", "node", "nodejs", "ruby", "perl", "php", "lua", "deno", "osascript", "pwsh", "powershell", "ts-node", "tsx", "julia", "Rscript", "elixir", "racket", "irb":
		c.classifyInterpreter(s, verb)
	case "sh", "bash", "zsh", "dash", "ksh", "mksh", "fish":
		if s.shellBody {
			s.Class = ClassRead
			return
		}
		if s.scriptFile != "" {
			c.scriptExec(s, s.scriptFile)
			return
		}
		if len(argv) == 1 {
			s.Class = ClassMutateOut
			s.Reason = "interactive shell"
			return
		}
		s.Class = ClassRead
	case "xargs":
		s.Class = ClassRead
	case "make", "just", "cmake", "ninja", "meson", "bazel", "buck", "gradlew.bat", "ant", "scons", "rake.bat", "task", "mage", "earthly":
		s.Class = ClassMutateIn
	case "kill", "pkill", "killall", "launchctl", "systemctl", "service", "crontab", "at", "defaults", "scutil", "networksetup", "pmset", "nvram", "csrutil", "spctl", "tmutil", "softwareupdate", "xcode-select", "reboot", "shutdown", "halt", "poweroff", "mount", "umount", "diskimage", "hdiutil", "sysctl", "iptables", "pfctl", "ufw", "useradd", "userdel", "usermod", "passwd", "dscl", "visudo", "chsh", "loginctl", "timedatectl", "hostnamectl", "modprobe", "insmod", "rmmod", "ip", "ifconfig", "route", "nmcli", "xdg-open", "start", "explorer":
		if verb == "crontab" && containsAny(argv, "-l") || (verb == "defaults" && containsAny(argv, "read")) || (verb == "systemctl" && containsAny(argv, "status", "list-units", "is-active", "show", "cat")) || (verb == "launchctl" && containsAny(argv, "list", "print")) || (verb == "ip" && (len(argv) < 3 || containsAny(argv, "show", "addr", "a", "link", "route") && !containsAny(argv, "add", "del", "set", "flush"))) || ((verb == "ifconfig" || verb == "route" || verb == "sysctl") && len(argv) <= 2) || (verb == "pmset" && containsAny(argv, "-g")) || (verb == "mount" && len(argv) == 1) || (verb == "tmutil" && containsAny(argv, "listlocalsnapshots", "status", "listbackups")) {
			s.Class = ClassRead
			return
		}
		s.Class = ClassMutateOut
		s.Reason = verb + " changes process, service or system state"
	case "sudo", "doas", "su":
		s.Class = ClassMutateOut
		s.Reason = "privilege escalation"
	case "cd", "pushd", "popd", "dirs", "ls", "dir", "pwd", "echo", "printf", "true", "false", "test", "[", "[[", "which", "whereis", "type", "command", "file", "stat", "date", "whoami", "id", "uname", "hostname", "tree", "basename", "dirname", "realpath", "readlink", "md5", "md5sum", "sha1sum", "sha256sum", "shasum", "cksum", "column", "fold", "nl", "rev", "tac", "seq", "expr", "bc", "sleep", "wait", "unset", "alias", "unalias", "shopt", "ulimit", "umask", "history", "help", "man", "info", "tldr", "yes", "tput", "clear", "tty", "stty", "uptime", "w", "who", "last", "lsof", "netstat", "ss", "dig", "nslookup", "host", "nproc", "arch", "sw_vers", "lscpu", "free", "vmstat", "iostat", "du", "df", "ps", "top", "htop", "uniq", "tr", "comm", "paste", "join", "split", "xargs.bat", "return", "exit", "break", "continue", "shift", "getopts", "read", "let", "local", "readonly", "hash", "times", "trap", "fc", "bind", "compgen", "complete", "enable", "logout", "mapfile", "printenv.bat", "ping", "traceroute", "mtr", "openssl", "gpg", "envsubst", "fzf", "xclip", "pbcopy", "pbpaste", "say", "afplay", "pandoc", "fmt", "pr", "look", "locate", "mdfind", "mdls", "plutil", "sqlformat", "glow", "delta", "difft", "batcat", "exa", "eza", "lsd", "fd", "fdfind", "tokei", "cloc", "scc", "hyperfine", "watch", "entr", "tail.bat", "ctags", "etags", "gofmt", "goimports", "golangci-lint", "staticcheck", "govulncheck", "eslint", "prettier", "biome", "black", "ruff", "isort", "mypy", "pyright", "flake8", "pylint", "rubocop", "clippy", "rustfmt", "shellcheck", "shfmt", "hadolint", "yamllint", "markdownlint", "tsc", "swiftlint", "swiftformat", "ktlint", "detekt", "checkstyle", "clang-format", "clang-tidy", "cppcheck", "stylua", "luacheck", "vale", "codespell", "typos", "actionlint":
		s.Class = ClassRead
		if verb == "openssl" && containsAny(argv, "req", "genrsa", "genpkey", "x509", "pkcs12", "enc", "dgst", "rand") {
			c.writersDefault(s, verb)
		}
		if verb == "gpg" && containsAny(argv, "--export-secret-keys", "--export-secret-subkeys") {
			c.deny(s, "D7", "gpg exports secret keys")
		}
		if verb == "pbcopy" || verb == "xclip" {
			s.Class = ClassMutateOut
			s.Reason = "clipboard write leaves the process"
		}
		if verb == "gofmt" && containsAny(argv, "-w") || verb == "prettier" && containsAny(argv, "--write", "-w") || verb == "black" && !containsAny(argv, "--check", "--diff") || verb == "ruff" && containsAny(argv, "--fix", "format") || verb == "isort" && !containsAny(argv, "--check", "--check-only", "--diff") || verb == "eslint" && containsAny(argv, "--fix") || verb == "rustfmt" && !containsAny(argv, "--check") || verb == "shfmt" && containsAny(argv, "-w") || verb == "clang-format" && containsAny(argv, "-i") || verb == "swiftformat" && !containsAny(argv, "--lint") || verb == "goimports" && containsAny(argv, "-w") || verb == "biome" && containsAny(argv, "--write") || verb == "stylua" && !containsAny(argv, "--check") {
			c.writersDefault(s, verb)
		}
		if verb == "tsc" && !containsAny(argv, "--noEmit") {
			c.writersDefault(s, verb)
		}
	case "touch", "mkdir", "ln", "truncate", "tar", "unzip", "zip", "gzip", "gunzip", "bzip2", "xz", "zstd", "7z", "patch", "mktemp", "sqlite3.bat", "split.bat", "cpio", "pax", "ditto", "xattr", "setfacl", "mkfifo", "mknod":
		c.writersDefault(s, verb)
	case "pytest", "py.test", "jest", "vitest", "mocha", "ava", "tap", "tox", "nox", "rspec", "minitest", "phpunit", "pest", "cypress", "playwright", "karma", "nyc", "c8", "coverage", "unittest", "behave", "robot", "ctest", "gotestsum", "ginkgo", "richgo", "bats", "shunit2", "hurl", "k6", "artillery", "ab", "wrk", "locust", "vegeta", "storybook", "webpack", "vite", "rollup", "esbuild", "parcel", "turbo", "nx", "lerna", "next", "nuxt", "astro", "remix", "gatsby", "svelte-kit", "ng", "expo", "react-native", "eas", "fastlane", "pod.bat", "xcrun", "swiftc", "gcc", "g++", "clang", "clang++", "cc", "c++", "ld", "as", "javac", "java", "kotlinc", "kotlin", "scalac", "scala", "sbt", "lein", "clj", "clojure", "ghc", "ghci", "runghc", "ocaml", "opam", "dune", "nim", "nimble", "crystal", "shards", "v", "gleam", "rebar3", "mix.bat", "iex", "erl", "haxe", "dotnet.bat", "msbuild", "nuget.bat", "mono", "fsharpc", "rustc", "wasm-pack", "trunk", "emcc", "tsc.bat", "babel", "swc", "coffee", "elm", "purs", "spago", "reason", "bsb", "rescript", "protoc", "buf", "grpcurl", "swagger", "openapi-generator", "graphql-codegen", "prisma.bat", "sqlc", "ent", "wire", "mockgen", "mockery", "stringer", "gopls", "dlv", "gdb", "lldb", "valgrind", "strace", "ltrace", "dtrace", "dtruss", "perf", "instruments", "leaks", "heap", "sample", "spindump", "fs_usage", "opensnoop", "execsnoop":
		s.Class = ClassMutateIn
		if (verb == "java" || verb == "dotnet" || verb == "mono") && len(argv) > 1 {
			s.Class = ClassInterpreter
		}
	default:
		c.unknownVerb(s, verb)
	}
}

// shellConsumer reports a shell reading a script from stdin or a
// substitution: `sh`, `bash -s`, `sudo bash`, `python -`.
func (c *classifier) shellConsumer(verb string, argv []string) bool {
	switch verb {
	case "sh", "bash", "zsh", "dash", "ksh", "mksh", "fish":
		for _, a := range argv[1:] {
			if a == "-" || a == "-s" || a == phProcSubst {
				return true
			}
			if !strings.HasPrefix(a, "-") {
				return false
			}
			if strings.Contains(a, "c") && !strings.HasPrefix(a, "--") {
				return false
			}
		}
		return true
	case "python", "python3", "python2", "node", "perl", "ruby", "php", "pwsh", "powershell", "iex":
		for _, a := range argv[1:] {
			if a == "-" {
				return true
			}
			if !strings.HasPrefix(a, "-") {
				return false
			}
		}
		return len(argv) == 1 || verb == "iex"
	}
	return false
}

// injectedDeny re-parses argument values that came from the environment
// and carry shell control operators (the Codex branch-name injection):
// a hard-deny pattern inside them denies the outer segment.
func (c *classifier) injectedDeny(s *Segment) bool {
	for i, a := range s.Argv {
		if i == 0 || (len(s.argvClean) > i && !s.argvClean[i]) {
			continue
		}
		if !strings.ContainsAny(a, ";|&\n") || !strings.Contains(a, " ") {
			continue
		}
		// Was this value expanded from a variable? Only values that are
		// not literally present in the raw text count.
		if strings.Contains(s.Raw, a) {
			continue
		}
		sub := newExpander(c.shell, c.cwd, c.env["HOME"], c.root, c.env, c.pol)
		sub.cwd = c.cwd
		segs, err := sub.run(a)
		if err != nil {
			continue
		}
		inner := &classifier{scope: c.scope, pol: c.pol, shell: c.shell, cwd: c.cwd, root: c.root, env: c.env}
		for _, is := range segs {
			inner.classify(is)
			if is.Class == ClassDestructive {
				c.deny(s, is.Rule, "injected value expands to a denied command ("+is.Reason+")")
				return true
			}
		}
	}
	return false
}

// unknownVerb classifies commands guard has no table for: in-scope
// mutation unless an argument resolves outside scope.
func (c *classifier) unknownVerb(s *Segment, verb string) {
	if s.interpreterBody {
		s.Class = ClassInterpreter
		s.Reason = "heredoc fed to " + verb
		return
	}
	if strings.Contains(s.Argv[0], "/") || strings.HasPrefix(s.Argv[0], ".") {
		c.scriptExec(s, s.Argv[0])
		return
	}
	s.Class = ClassMutateIn
	for _, a := range s.Argv[1:] {
		if !looksLikePath(a) {
			continue
		}
		q := c.abs(a)
		if c.scope.isCredential(q) {
			c.deny(s, "D7", verb+" reads credential path "+q, q)
			return
		}
		if !c.scope.inScope(q) {
			s.Paths = appendUnique(s.Paths, []string{q})
			s.Class = ClassMutateOut
			s.Reason = verb + " touches " + q + " outside scope"
		}
	}
}

// writersDefault classifies verbs that write to every path argument.
func (c *classifier) writersDefault(s *Segment, verb string) {
	s.Class = ClassMutateIn
	for _, a := range s.Argv[1:] {
		if strings.HasPrefix(a, "-") || isPlaceholder(a) {
			continue
		}
		if verb == "tar" && !strings.Contains(a, "/") && !strings.HasSuffix(a, ".tar") && !strings.Contains(a, ".tar.") && !strings.HasSuffix(a, ".tgz") && !strings.HasSuffix(a, ".tbz") {
			continue
		}
		if !looksLikePath(a) && (verb == "ln" || verb == "mkdir" || verb == "touch" || verb == "truncate") {
			// bare names are relative to cwd
			a = "./" + a
		} else if !looksLikePath(a) {
			continue
		}
		if cl := c.pathClass(s, a, verb); cl == ClassMutateOut {
			c.raise(s, cl, verb+" writes outside scope: "+c.abs(a))
		}
	}
}

// scriptExec is interpreter_exec of a script path: tracked and in scope
// counts as mutate_in_scope (guard-spec 2.4).
func (c *classifier) scriptExec(s *Segment, script string) {
	if isPlaceholder(script) {
		s.Class = ClassUnresolvable
		s.Unresolved = appendUnique(s.Unresolved, []string{"script " + script})
		return
	}
	q := c.abs(script)
	s.Paths = appendUnique(s.Paths, []string{q})
	if c.scope.isCredential(q) {
		c.deny(s, "D7", "executes credential path "+q, q)
		return
	}
	if c.scope.inScope(q) && c.tracked(q) {
		s.Class = ClassMutateIn
		return
	}
	s.Class = ClassInterpreter
	s.Reason = "interpreter_exec: " + q + " is not a tracked in-scope script"
}

// tracked asks git whether q is tracked (read-only, 300 ms cap).
func (c *classifier) tracked(q string) bool {
	if c.scope.root == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := exec.CommandContext(ctx, "git", "-C", c.scope.root, "ls-files", "--error-unmatch", "--", q).Run()
	return err == nil
}

func (c *classifier) classifyInterpreter(s *Segment, verb string) {
	argv := s.Argv
	if len(argv) == 1 {
		if s.interpreterBody || s.heredoc != "" {
			s.Class = ClassInterpreter
			s.Reason = "heredoc fed to " + verb
			return
		}
		s.Class = ClassMutateOut
		s.Reason = "interactive " + verb
		return
	}
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--version" || a == "-V" || a == "-v" || a == "--help" || a == "-h":
			s.Class = ClassRead
			return
		case a == "-c" || a == "-e" || a == "-p" || a == "--eval" || a == "-r" || a == "--print":
			if i+1 < len(argv) && len(s.argvClean) > i+1 && !s.argvClean[i+1] && s.argvSubst[i+1] == "fetch" {
				c.deny(s, "D10", "remote script substituted into "+verb+" "+a)
				return
			}
			s.Class = ClassInterpreter
			s.Reason = verb + " " + a + ": inline program, effects opaque to the parser"
			return
		case a == "-m" && i+1 < len(argv):
			c.pythonModule(s, argv[i+1], argv[i+2:])
			return
		case a == "-":
			if s.stdinFrom != nil && isFetchArgv(s.stdinFrom.Argv) {
				c.deny(s, "D10", "remote script piped into "+verb)
				return
			}
			s.Class = ClassInterpreter
			s.Reason = verb + " reads the program from stdin"
			return
		case strings.HasPrefix(a, "-"):
			continue
		}
		c.scriptExec(s, a)
		return
	}
	s.Class = ClassInterpreter
}

func (c *classifier) pythonModule(s *Segment, mod string, rest []string) {
	switch mod {
	case "pytest", "unittest", "venv", "build", "black", "ruff", "mypy", "flake8", "isort", "compileall", "py_compile", "pydoc", "coverage", "tox", "nox", "pre_commit", "sphinx", "mkdocs", "json.tool", "timeit", "cProfile", "pstats", "this", "site":
		s.Class = ClassMutateIn
		if mod == "json.tool" || mod == "pydoc" || mod == "site" || mod == "this" {
			s.Class = ClassRead
		}
	case "pip", "ensurepip", "twine", "pipx":
		argv := append([]string{mod}, rest...)
		c.classifyPip(s, mod)
		_ = argv
	case "http.server", "SimpleHTTPServer", "smtpd", "webbrowser":
		s.Class = ClassMutateOut
		s.Reason = "python -m " + mod + " opens network state"
	default:
		s.Class = ClassInterpreter
		s.Reason = "python -m " + mod + ": effects opaque to the parser"
	}
}

// rmTargets resolves the target list of a delete verb, expanding the
// xargs and find placeholders.
func (c *classifier) rmTargets(s *Segment, pos []string) (targets []string, unresolvedInput bool) {
	for _, a := range pos {
		switch {
		case a == phXargsInput:
			ts, ok := c.xargsInputs(s)
			if !ok {
				unresolvedInput = true
			}
			targets = append(targets, ts...)
		case a == phSubshell || a == phProcSubst:
			unresolvedInput = true
		default:
			targets = append(targets, a)
		}
	}
	return targets, unresolvedInput
}

// xargsInputs returns the pseudo paths an xargs input produces when the
// producer is a find, and ok = false when the input is opaque.
func (c *classifier) xargsInputs(s *Segment) ([]string, bool) {
	p := s.xargsFrom
	if p == nil || len(p.Argv) == 0 || baseName(p.Argv[0]) != "find" {
		return nil, false
	}
	f := parseFind(p.Argv)
	var out []string
	for _, start := range f.starts {
		if len(f.names) > 0 {
			for _, n := range f.names {
				out = append(out, filepath.Join(start, filepath.Base(n)))
			}
		} else if f.filtered {
			out = append(out, filepath.Join(start, phFindMatch))
		} else {
			out = append(out, start)
		}
	}
	return out, true
}

// credentialHit reports whether a resolved target names a credential
// file, including <start>/<pattern> pseudo paths from find.
func (c *classifier) credentialHit(q string) bool {
	if c.scope.isCredential(q) {
		return true
	}
	base := path.Base(filepath.ToSlash(q))
	return hasMeta(base) && c.scope.credentialPattern(base)
}

func (c *classifier) classifyRm(s *Segment, verb string) {
	argv := s.Argv
	recursive := verb == "rmdir" && false
	if verb == "rm" || verb == "trash" {
		recursive = hasFlag(argv, reFlagHasR, "--recursive")
	}
	pos := positionals(argv)
	targets, opaque := c.rmTargets(s, pos)
	if opaque && recursive {
		c.deny(s, "D2", "recursive delete over an input guard cannot resolve (xargs or substitution)")
		return
	}
	s.Class = ClassMutateIn
	for _, gr := range s.globRoots {
		if recursive && c.scope.isProtectedRoot(gr) {
			c.deny(s, "D1", "recursive delete of the contents of "+gr+" ("+c.scope.protectedName(gr)+") via glob", gr)
			return
		}
	}
	for _, t := range targets {
		q := c.abs(t)
		s.Paths = appendUnique(s.Paths, []string{q})
		if recursive && c.scope.isProtectedRoot(q) {
			c.deny(s, "D1", "recursive delete resolves to "+q+" ("+c.scope.protectedName(q)+")", q)
			return
		}
		if c.scope.isSagaProtected(q) {
			c.deny(s, "D11", verb+" removes the layer's own file "+q, q)
			return
		}
		if isDevice(q) {
			c.deny(s, "D9", verb+" on device "+q, q)
			return
		}
		if recursive && !c.scope.inScope(q) {
			c.deny(s, "D2", "recursive delete of "+q+" outside scope", q)
			return
		}
		if !recursive && !c.scope.inScope(q) {
			c.raise(s, ClassMutateOut, verb+" removes "+q+" outside scope")
		}
	}
}

func (c *classifier) classifyFind(s *Segment) {
	f := s.find
	if f == nil {
		ff := parseFind(s.Argv)
		f = &ff
	}
	s.Class = ClassRead
	if len(f.execs) > 0 {
		s.Class = ClassRead // the payload is its own segment
	}
	if !f.delete {
		return
	}
	s.Class = ClassMutateIn
	for _, start := range f.starts {
		if isPlaceholder(start) {
			s.Unresolved = appendUnique(s.Unresolved, []string{"find start " + start})
			continue
		}
		q := c.abs(start)
		s.Paths = appendUnique(s.Paths, []string{q})
		if !f.filtered && c.scope.isProtectedRoot(q) {
			c.deny(s, "D1", "find -delete rooted at "+q+" ("+c.scope.protectedName(q)+")", q)
			return
		}
		if !c.scope.inScope(q) {
			c.deny(s, "D2", "find -delete rooted at "+q+" outside scope", q)
			return
		}
	}
}

func (c *classifier) classifyMv(s *Segment) {
	argv := s.Argv
	var dest string
	pos := positionals(argv)
	for i, a := range argv {
		if (a == "-t" || a == "--target-directory") && i+1 < len(argv) {
			dest = argv[i+1]
		}
	}
	if dest == "" {
		if len(pos) < 2 {
			s.Class = ClassMutateIn
			return
		}
		dest = pos[len(pos)-1]
		pos = pos[:len(pos)-1]
	}
	s.Class = ClassMutateIn
	if isPlaceholder(dest) {
		s.Unresolved = appendUnique(s.Unresolved, []string{"mv destination " + dest})
		return
	}
	dq := c.abs(dest)
	s.Paths = appendUnique(s.Paths, []string{dq})
	if c.scope.isSagaProtected(dq) {
		c.deny(s, "D11", "mv onto the layer's own file "+dq, dq)
		return
	}
	if isDevice(dq) {
		c.deny(s, "D9", "mv onto device "+dq, dq)
		return
	}
	if !c.scope.inScope(dq) {
		c.deny(s, "D2", "mv moves files to "+dq+" outside scope", dq)
		return
	}
	for _, src := range pos {
		if isPlaceholder(src) {
			continue
		}
		q := c.abs(src)
		s.Paths = appendUnique(s.Paths, []string{q})
		if c.scope.isProtectedRoot(q) {
			c.deny(s, "D1", "mv of "+q+" ("+c.scope.protectedName(q)+")", q)
			return
		}
		if c.scope.isSagaProtected(q) {
			c.deny(s, "D11", "mv removes the layer's own file "+q, q)
			return
		}
		if c.credentialHit(q) {
			c.deny(s, "D7", "mv reads credential path "+q, q)
			return
		}
		if !c.scope.inScope(q) {
			c.raise(s, ClassMutateOut, "mv takes "+q+" from outside scope")
		}
	}
}

func (c *classifier) classifyCopy(s *Segment, verb string) {
	argv := s.Argv
	pos := positionals(argv, "-e", "--rsh", "-i", "-F", "-o", "-P", "-l", "-c", "-J", "-S", "--exclude", "--include", "--exclude-from", "--files-from", "--rsync-path", "--log-file", "--chmod", "--chown", "-t", "-m", "-g", "-o")
	s.Class = ClassMutateIn
	if len(pos) == 0 {
		return
	}
	dest := pos[len(pos)-1]
	srcs := pos[:len(pos)-1]
	if verb == "install" {
		// install [-d] dirs...  or install src dest
		if containsAny(argv, "-d") {
			srcs, dest = nil, ""
			for _, p := range pos {
				c.raise(s, c.pathClass(s, p, verb), "")
			}
			return
		}
	}
	for _, src := range srcs {
		if isPlaceholder(src) || strings.Contains(src, ":") && (verb == "scp" || verb == "rsync") {
			if strings.Contains(src, ":") {
				c.raise(s, ClassMutateOut, verb+" pulls from a remote host")
			}
			continue
		}
		q := c.abs(src)
		if c.credentialHit(q) {
			c.deny(s, "D7", verb+" reads credential path "+q, q)
			return
		}
	}
	if dest == "" || isPlaceholder(dest) {
		return
	}
	if strings.Contains(dest, ":") && (verb == "scp" || verb == "rsync" || verb == "sftp") && !filepath.IsAbs(dest) {
		c.raise(s, ClassMutateOut, verb+" writes to a remote host: "+dest)
		return
	}
	dq := c.abs(dest)
	if verb == "rsync" && containsAny(argv, "--delete", "--delete-after", "--delete-before", "--delete-during", "--delete-excluded") {
		if c.scope.isProtectedRoot(dq) {
			c.deny(s, "D1", "rsync --delete onto "+dq+" ("+c.scope.protectedName(dq)+")", dq)
			return
		}
		if !c.scope.inScope(dq) {
			c.deny(s, "D2", "rsync --delete onto "+dq+" outside scope", dq)
			return
		}
	}
	if cl := c.pathClass(s, dest, verb); cl == ClassMutateOut {
		c.raise(s, cl, verb+" writes outside scope: "+dq)
	}
}

func (c *classifier) classifyReader(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassRead
	if verb == "tee" {
		for _, a := range positionals(argv) {
			if isPlaceholder(a) {
				continue
			}
			q := c.abs(a)
			if q == "/dev/null" {
				continue
			}
			if cl := c.pathClass(s, a, "tee"); cl == ClassMutateOut {
				c.raise(s, cl, "tee writes outside scope: "+q)
			}
		}
		if s.stdinFrom != nil {
			for _, p := range s.stdinFrom.Paths {
				_ = p
			}
		}
		return
	}
	inPlace := (verb == "sed" && hasFlag(argv, reFlagHasI, "--in-place")) || ((verb == "vi" || verb == "vim" || verb == "nvim" || verb == "nano" || verb == "emacs" || verb == "code" || verb == "open") && len(argv) > 1)
	if verb == "grep" || verb == "egrep" || verb == "fgrep" || verb == "rg" || verb == "ag" {
		if hasFlag(argv, reFlagHasR, "--recursive") || verb == "rg" || verb == "ag" {
			// recursive searches over $HOME or / read credential files too
			for _, a := range positionals(argv, "-e", "--regexp", "-f", "--file", "-g", "--glob", "-t", "--type", "-m", "--max-count", "-A", "-B", "-C", "--include", "--exclude", "--exclude-dir") {
				if looksLikePath(a) {
					q := c.abs(a)
					if c.scope.isProtectedRoot(q) && (c.scope.eq(q, c.scope.home) || isDriveOrFSRoot(q)) {
						c.deny(s, "D7", verb+" recurses over "+q+" (credential paths included)", q)
						return
					}
				}
			}
		}
	}
	pos := positionals(argv, "-e", "--regexp", "-f", "--file", "-n", "-c", "--lines", "--bytes", "-g", "--glob", "-t", "--type", "--expression")
	switch verb {
	case "sed", "awk", "gawk", "grep", "egrep", "fgrep", "rg", "ag", "jq", "yq":
		// the first positional is the script or pattern unless -e/-f gave it
		if len(pos) > 0 && !containsAny(argv, "-e", "--expression", "-f", "--file", "--regexp") {
			pos = pos[1:]
		}
	}
	for _, a := range pos {
		if isPlaceholder(a) && a != phXargsInput {
			continue
		}
		var cands []string
		if a == phXargsInput {
			ts, ok := c.xargsInputs(s)
			if !ok {
				continue
			}
			cands = ts
		} else {
			cands = []string{a}
		}
		for _, cand := range cands {
			q := c.abs(cand)
			if c.credentialHit(q) {
				c.deny(s, "D7", verb+" reads credential path "+q, q)
				return
			}
			if inPlace {
				if cl := c.pathClass(s, cand, verb); cl == ClassMutateOut {
					c.raise(s, cl, verb+" edits outside scope: "+q)
				}
			}
		}
	}
	if verb == "open" || verb == "code" {
		c.raise(s, ClassMutateOut, verb+" launches an application")
	}
}

func (c *classifier) classifyChmod(s *Segment, verb string) {
	argv := s.Argv
	recursive := hasFlag(argv, reFlagHasR, "--recursive")
	pos := positionals(argv)
	if len(pos) == 0 {
		s.Class = ClassMutateIn
		return
	}
	mode, targets := pos[0], pos[1:]
	s.Class = ClassMutateIn
	if verb == "chmod" && recursive && (mode == "000" || mode == "0000" || mode == "a-rwx" || mode == "-rwx" || mode == "ugo-rwx") {
		c.deny(s, "D9", "chmod -R "+mode+" makes files unreadable")
		return
	}
	for _, t := range targets {
		if isPlaceholder(t) {
			continue
		}
		q := c.abs(t)
		s.Paths = appendUnique(s.Paths, []string{q})
		if recursive && c.scope.isProtectedRoot(q) {
			c.deny(s, "D9", verb+" -R on "+q+" ("+c.scope.protectedName(q)+")", q)
			return
		}
		if recursive && !c.scope.inScope(q) && verb != "chmod" {
			c.deny(s, "D9", verb+" -R outside scope: "+q, q)
			return
		}
		if c.scope.isSagaProtected(q) {
			c.deny(s, "D11", verb+" on the layer's own file "+q, q)
			return
		}
		if !c.scope.inScope(q) {
			c.raise(s, ClassMutateOut, verb+" outside scope: "+q)
		}
	}
}

func (c *classifier) classifyDB(s *Segment, verb string) {
	argv := s.Argv
	joined := strings.Join(argv[1:], " ")
	switch verb {
	case "dropdb":
		c.deny(s, "D5", "dropdb removes a database")
		return
	case "redis-cli":
		for _, a := range argv[1:] {
			if l := strings.ToLower(a); l == "flushall" || l == "flushdb" {
				c.deny(s, "D5", "redis-cli "+a+" wipes the store")
				return
			}
		}
	case "mongo", "mongosh":
		if strings.Contains(joined, "dropDatabase") || strings.Contains(joined, ".drop(") {
			c.deny(s, "D5", verb+" drops a database or collection")
			return
		}
	case "mysqladmin":
		if containsAny(argv, "drop") {
			c.deny(s, "D5", "mysqladmin drop removes a database")
			return
		}
	}
	if reDropSQL.MatchString(joined) {
		c.deny(s, "D5", verb+" runs "+reDropSQL.FindString(joined))
		return
	}
	for _, a := range argv[1:] {
		if looksLikePath(a) && !strings.HasPrefix(a, "postgres") {
			c.raise(s, c.pathClass(s, a, verb), "")
		}
	}
	s.Class = maxClass(s.Class, ClassMutateOut)
	if s.Reason == "" {
		s.Reason = verb + ": database session (effects opaque)"
	}
}

func maxClass(a, b Class) Class {
	if classRank(b) > classRank(a) {
		return b
	}
	return a
}

var wipeTokens = map[string]bool{"migrate:fresh": true, "migrate:reset": true, "db:drop": true, "db:reset": true, "db:purge": true, "db:migrate:reset": true, "db:schema:load": true, "db:drop:all": true, "db:reset:all": true, "db:seed:replant": true, "migrate:fresh:all": true, "db:wipe": true}

func (c *classifier) classifyMigrate(s *Segment, verb string) {
	argv := s.Argv
	for _, a := range argv[1:] {
		if wipeTokens[strings.ToLower(a)] {
			c.deny(s, "D5", verb+" "+a+" wipes the database")
			return
		}
	}
	switch verb {
	case "prisma":
		if containsAny(argv, "--shadow-database-url") || hasFlag(argv, nil, "--shadow-database-url") {
			c.deny(s, "D5", "prisma with --shadow-database-url resets the shadow database")
			return
		}
		if len(argv) > 2 && argv[1] == "migrate" && argv[2] == "reset" {
			c.deny(s, "D5", "prisma migrate reset drops the database")
			return
		}
		if containsAny(argv, "--force-reset") {
			c.deny(s, "D5", "prisma db push --force-reset drops the database")
			return
		}
		if len(argv) > 2 && argv[1] == "db" && argv[2] == "execute" && reDropSQL.MatchString(strings.Join(argv, " ")) {
			c.deny(s, "D5", "prisma db execute runs "+reDropSQL.FindString(strings.Join(argv, " ")))
			return
		}
	case "supabase":
		if len(argv) > 2 && argv[1] == "db" && argv[2] == "reset" && containsAny(argv, "--linked") {
			c.deny(s, "D5", "supabase db reset --linked resets the linked project")
			return
		}
		if len(argv) > 2 && argv[1] == "projects" && argv[2] == "delete" {
			c.deny(s, "D6", "supabase projects delete")
			return
		}
	case "manage.py", "django-admin":
		if containsAny(argv, "flush", "sqlflush", "reset_db") {
			c.deny(s, "D5", verb+" flush wipes the database")
			return
		}
	case "alembic":
		if containsAny(argv, "downgrade") && containsAny(argv, "base") {
			c.deny(s, "D5", "alembic downgrade base drops every migration")
			return
		}
	case "knex", "sequelize", "sequelize-cli", "typeorm", "goose", "migrate", "dbmate", "atlas":
		if containsAny(argv, "rollback") && containsAny(argv, "--all") || containsAny(argv, "reset", "drop", "schema:drop", "db:drop", "down-to", "clean") && (containsAny(argv, "0", "--all", "schema:drop", "db:drop", "clean", "drop") || verb == "goose" || verb == "migrate") {
			if !(verb == "goose" && !containsAny(argv, "reset", "down-to")) && !(verb == "migrate" && !containsAny(argv, "drop", "down")) {
				c.deny(s, "D5", verb+" resets the schema")
				return
			}
		}
	}
	s.Class = ClassMutateOut
	s.Reason = verb + ": database or migration state"
	if containsAny(argv, "status", "--version", "generate", "validate", "format", "studio", "--help", "introspect", "diff") {
		s.Class = ClassMutateIn
		s.Reason = ""
	}
}

func (c *classifier) classifyFetch(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassNetworkFetch
	write := false
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "-X" || a == "--request" || a == "--method":
			if i+1 < len(argv) && strings.ToUpper(argv[i+1]) != "GET" && strings.ToUpper(argv[i+1]) != "HEAD" {
				write = true
			}
			i++
		case strings.HasPrefix(a, "-X") && len(a) > 2 && strings.ToUpper(a[2:]) != "GET":
			write = true
		case a == "-d" || a == "--data" || a == "--data-binary" || a == "--data-raw" || a == "--data-urlencode" || a == "--data-ascii" || a == "--json" || a == "-F" || a == "--form" || a == "--form-string" || a == "-T" || a == "--upload-file" || a == "--post-data" || a == "--post-file" || a == "--body-file" || a == "--body-data":
			write = true
			if i+1 < len(argv) {
				v := argv[i+1]
				if j := strings.IndexByte(v, '@'); j >= 0 && (j == 0 || a == "-F" || a == "--form") {
					file := v[j+1:]
					if k := strings.IndexByte(file, ';'); k > 0 {
						file = file[:k]
					}
					if file != "" && file != "-" {
						q := c.abs(file)
						if c.credentialHit(q) {
							c.deny(s, "D7", verb+" uploads credential path "+q, q)
							return
						}
						s.Paths = appendUnique(s.Paths, []string{q})
					}
				} else if a == "-T" || a == "--upload-file" || a == "--post-file" || a == "--body-file" {
					q := c.abs(v)
					if c.credentialHit(q) {
						c.deny(s, "D7", verb+" uploads credential path "+q, q)
						return
					}
				}
				i++
			}
		case strings.HasPrefix(a, "--post-data=") || strings.HasPrefix(a, "--method=") && !strings.EqualFold(a, "--method=GET"):
			write = true
		case strings.HasPrefix(a, "--post-file=") || strings.HasPrefix(a, "--body-file="):
			write = true
			q := c.abs(a[strings.IndexByte(a, '=')+1:])
			if c.credentialHit(q) {
				c.deny(s, "D7", verb+" uploads credential path "+q, q)
				return
			}
		case a == "-o" || a == "--output" || a == "-O" && verb == "wget" || a == "--output-document":
			if i+1 < len(argv) {
				out := argv[i+1]
				i++
				if out != "-" && out != "/dev/stdout" && out != "/dev/null" {
					if cl := c.pathClass(s, out, verb); cl == ClassMutateOut {
						c.raise(s, cl, verb+" writes outside scope: "+c.abs(out))
					} else {
						c.raise(s, cl, "")
					}
				}
			}
		case strings.HasPrefix(a, "-O") && verb == "wget" && len(a) > 2:
			out := a[2:]
			if out != "-" {
				c.raise(s, c.pathClass(s, out, verb), "")
			}
		case a == "-O" && verb == "curl" || a == "--remote-name" || a == "-J" || a == "--remote-header-name":
			c.raise(s, c.pathClass(s, ".", verb), "")
		case a == "-K" || a == "--config" || a == "--netrc-file" || a == "-E" || a == "--cert" || a == "--key" || a == "--cacert":
			if i+1 < len(argv) {
				q := c.abs(argv[i+1])
				if (a == "-E" || a == "--cert" || a == "--key") && c.credentialHit(q) {
					// using a client cert is not a leak of it
					_ = q
				}
				i++
			}
		}
	}
	if verb == "wget" && !containsAny(argv, "-O", "--output-document", "-O-", "-qO-", "--spider") {
		hasDash := false
		for _, a := range argv {
			if strings.HasPrefix(a, "-O") || strings.HasPrefix(a, "-qO") || strings.HasPrefix(a, "--output-document") {
				hasDash = true
			}
		}
		if !hasDash {
			c.raise(s, c.pathClass(s, ".", verb), "")
		}
	}
	if write {
		c.raise(s, ClassMutateOut, verb+" sends data to the network")
	}
}

func (c *classifier) classifyGit(s *Segment) {
	argv := s.Argv
	i := 1
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		a := argv[i]
		if a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "--namespace" {
			i += 2
			continue
		}
		if a == "--version" || a == "--help" || a == "-v" || a == "-h" {
			s.Class = ClassRead
			return
		}
		i++
	}
	if i >= len(argv) {
		s.Class = ClassRead
		return
	}
	sub := argv[i]
	rest := argv[i+1:]
	full := append([]string{"git"}, rest...) // for positionals()
	pos := positionals(full, "-m", "-F", "-C", "--message", "--file", "-o", "--onto", "-x", "--exec", "-S", "--gpg-sign", "-u", "--set-upstream", "--author", "--date", "-U", "--unified", "-n", "--max-count", "--since", "--until", "--author", "--grep", "--format", "--pretty", "-L", "-e", "--expire", "--expire-unreachable", "--repo", "--receive-pack", "--push-option", "--upstream", "--exclude", "--include", "--recurse-submodules", "--depth", "--branch", "--origin", "--template", "--separate-git-dir", "--reference", "--jobs", "-j", "--pathspec-from-file", "--staged", "-c", "-b", "-B", "--orphan", "--conflict", "-t", "--track", "-d", "--detach")
	s.Class = ClassMutateIn
	switch sub {
	case "status", "diff", "log", "show", "blame", "rev-parse", "ls-files", "ls-tree", "cat-file", "describe", "shortlog", "whatchanged", "grep", "count-objects", "help", "version", "check-ignore", "merge-base", "for-each-ref", "rev-list", "name-rev", "show-ref", "show-branch", "range-diff", "diff-tree", "diff-index", "diff-files", "var", "verify-commit", "verify-tag", "cherry", "annotate", "fsck", "bugreport", "diagnose", "format-patch", "request-pull", "mailinfo", "stripspace", "column", "interpret-trailers", "hash-object", "write-tree", "mktree":
		s.Class = ClassRead
		if sub == "format-patch" && !containsAny(rest, "--stdout") {
			s.Class = ClassMutateIn
		}
	case "fetch", "ls-remote":
		s.Class = ClassNetworkFetch
	case "pull", "clone":
		s.Class = ClassMutateIn
		if sub == "clone" && len(pos) >= 2 {
			c.raise(s, c.pathClass(s, pos[1], "git clone"), "")
		} else if sub == "clone" && len(pos) == 1 {
			c.raise(s, c.pathClass(s, ".", "git clone"), "")
		}
	case "config":
		if containsAny(rest, "--get", "--get-all", "--get-regexp", "-l", "--list", "--show-origin", "--get-urlmatch") || len(pos) == 1 && !containsAny(rest, "--unset", "--unset-all", "--add", "--replace-all", "--remove-section", "--rename-section", "-e", "--edit") {
			s.Class = ClassRead
			if containsAny(rest, "-e", "--edit") {
				s.Class = ClassMutateIn
			}
		} else if containsAny(rest, "--global", "--system") {
			s.Class = ClassMutateOut
			s.Reason = "git config --global writes ~/.gitconfig"
		}
		if containsAny(rest, "--global", "--system") && containsAny(rest, "--get", "--get-all", "-l", "--list") {
			for _, p := range pos {
				if strings.Contains(strings.ToLower(p), "credential") || strings.Contains(strings.ToLower(p), "token") || strings.Contains(strings.ToLower(p), "password") {
					c.deny(s, "D7", "git config reads a credential setting: "+p)
					return
				}
			}
		}
	case "branch":
		if hasFlag(full, reFlagCaseD) || (containsAny(rest, "--delete") && containsAny(rest, "--force", "-f")) {
			c.deny(s, "D3", "git branch -D discards unmerged history")
			return
		}
		if len(pos) == 0 || containsAny(rest, "-a", "-r", "-l", "--list", "--show-current", "-v", "-vv", "--contains", "--merged", "--no-merged") {
			s.Class = ClassRead
		}
	case "tag":
		if len(pos) == 0 || containsAny(rest, "-l", "--list", "-n", "--contains", "--points-at") {
			s.Class = ClassRead
		}
	case "remote":
		if len(rest) == 0 || containsAny(rest, "-v", "show", "get-url") {
			s.Class = ClassRead
		}
	case "stash":
		if containsAny(rest, "drop", "clear") {
			c.deny(s, "D3", "git stash "+firstOf(rest, "drop", "clear")+" discards stashed work")
			return
		}
		if containsAny(rest, "list", "show") {
			s.Class = ClassRead
		}
	case "worktree":
		if containsAny(rest, "list") {
			s.Class = ClassRead
		}
		if containsAny(rest, "remove", "prune") && containsAny(rest, "--force", "-f") {
			c.deny(s, "D3", "git worktree remove --force discards changes")
			return
		}
		if containsAny(rest, "add") && len(pos) >= 2 {
			c.raise(s, c.pathClass(s, pos[1], "git worktree add"), "")
		}
	case "reflog":
		if containsAny(rest, "expire", "delete") && (hasFlag(full, nil, "--expire=now", "--expire-unreachable=now", "--expire=all", "--expire-unreachable=all") || containsAny(rest, "delete", "--all")) {
			c.deny(s, "D3", "git reflog "+rest[0]+" destroys the recovery log")
			return
		}
		s.Class = ClassRead
		if containsAny(rest, "expire", "delete") {
			s.Class = ClassMutateIn
		}
	case "gc", "prune", "repack", "prune-packed":
		if hasFlag(full, nil, "--prune=now", "--prune=all") || sub == "prune" && (containsAny(rest, "--expire") && (containsAny(rest, "now", "all") || hasFlag(full, nil, "--expire=now", "--expire=all"))) {
			c.deny(s, "D3", "git "+sub+" with --prune=now removes unreachable objects immediately")
			return
		}
	case "filter-branch", "filter-repo", "replace":
		c.deny(s, "D3", "git "+sub+" rewrites history")
		return
	case "reset":
		if containsAny(rest, "--hard", "--merge") {
			c.deny(s, "D3", "git reset "+firstOf(rest, "--hard", "--merge")+" discards the working tree")
			return
		}
	case "checkout", "switch":
		if sub == "checkout" {
			dashdash := -1
			for j, a := range rest {
				if a == "--" {
					dashdash = j
				}
			}
			var paths []string
			if dashdash >= 0 {
				paths = rest[dashdash+1:]
			} else if len(pos) >= 1 && (pos[0] == "." || pos[0] == "./" || pos[0] == ":/") && !containsAny(rest, "-b", "-B", "--orphan") {
				paths = pos
			}
			for _, p := range paths {
				q := c.abs(p)
				if p == ":/" || c.scope.eq(q, c.scope.rawRoot) || c.scope.under(c.scope.rawRoot, q) {
					c.deny(s, "D3", "git checkout -- "+p+" at the repo root discards every uncommitted change", q)
					return
				}
			}
		}
		if containsAny(rest, "--force", "-f") && len(pos) > 0 {
			s.Reason = "git " + sub + " --force discards local changes"
			s.Class = ClassMutateOut
		}
	case "restore":
		if containsAny(rest, "--staged", "-S") && !containsAny(rest, "--worktree", "-W") {
			break
		}
		for _, p := range pos {
			q := c.abs(p)
			if p == ":/" || c.scope.eq(q, c.scope.rawRoot) || c.scope.under(c.scope.rawRoot, q) {
				c.deny(s, "D3", "git restore "+p+" at the repo root discards every uncommitted change", q)
				return
			}
		}
	case "clean":
		force := hasFlag(full, reFlagHasF, "--force")
		if !force {
			s.Class = ClassRead // dry run
			return
		}
		x := hasFlag(full, reFlagHasX)
		if len(pos) == 0 {
			q := c.abs(".")
			if x {
				c.deny(s, "D1", "git clean -fdx at "+q+" removes every untracked and ignored file (repo root)", q)
				return
			}
			if !c.scope.inScope(q) {
				c.deny(s, "D3", "git clean -f at "+q+" outside scope", q)
				return
			}
			break
		}
		for _, p := range pos {
			q := c.abs(p)
			if x && c.scope.isProtectedRoot(q) {
				c.deny(s, "D1", "git clean -fdx over "+q, q)
				return
			}
			if !c.scope.inScope(q) {
				c.deny(s, "D3", "git clean -f over "+q+" outside scope", q)
				return
			}
		}
	case "rebase":
		if len(pos) >= 2 && c.protectedBranch(pos[1]) {
			c.deny(s, "D3", "git rebase rewrites protected branch "+pos[1])
			return
		}
	case "push":
		c.classifyGitPush(s, full, rest, pos)
	case "update-ref", "symbolic-ref":
		for _, p := range pos {
			if strings.HasPrefix(p, "refs/saga/") {
				c.deny(s, "D11", "git "+sub+" touches refs/saga/")
				return
			}
			if containsAny(rest, "-d", "--delete") && (strings.HasPrefix(p, "refs/heads/") && c.protectedBranch(strings.TrimPrefix(p, "refs/heads/"))) {
				c.deny(s, "D3", "git update-ref -d deletes protected "+p)
				return
			}
		}
	case "rm", "mv":
		for _, p := range pos {
			q := c.abs(p)
			if c.scope.isSagaProtected(q) {
				c.deny(s, "D11", "git "+sub+" on the layer's own file "+q, q)
				return
			}
		}
	case "submodule":
		if containsAny(rest, "deinit") && containsAny(rest, "--all", "-f", "--force") {
			c.deny(s, "D3", "git submodule deinit --force discards submodule work")
			return
		}
	case "credential", "credential-store", "credential-cache":
		if containsAny(rest, "fill", "get") {
			c.deny(s, "D7", "git credential "+rest[0]+" prints a stored credential")
			return
		}
	case "lfs":
		if containsAny(rest, "prune") && containsAny(rest, "--force", "-f") {
			c.deny(s, "D3", "git lfs prune --force removes objects")
			return
		}
	}
}

func firstOf(list []string, want ...string) string {
	for _, a := range list {
		for _, w := range want {
			if a == w {
				return a
			}
		}
	}
	return ""
}

// protectedBranch matches a branch name against git.protected globs.
func (c *classifier) protectedBranch(name string) bool {
	name = strings.TrimPrefix(name, "refs/heads/")
	for _, p := range c.pol.Git.Protected {
		if ok, _ := globMatch(p, name); ok {
			return true
		}
	}
	return false
}

// currentBranch reads HEAD (read-only, 300 ms cap); "" when detached or
// unknown, which callers treat as protected.
func (c *classifier) currentBranch() string {
	root := c.scope.rawRoot
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c *classifier) classifyGitPush(s *Segment, full, rest, pos []string) {
	s.Class = ClassMutateOut
	s.Reason = "git push writes to the remote"
	force := hasFlag(full, reFlagHasF, "--force") || (hasFlag(full, nil, "--force-with-lease") && !containsAny(rest, "--force-if-includes"))
	del := containsAny(rest, "--delete", "-d")
	mirror := containsAny(rest, "--mirror", "--prune")
	var refspecs []string
	if len(pos) >= 2 {
		refspecs = pos[1:]
	} else if len(pos) == 1 && strings.Contains(pos[0], ":") && !strings.Contains(pos[0], "://") && !strings.Contains(pos[0], "@") {
		refspecs = pos
	}
	if mirror {
		c.deny(s, "D4", "git push "+firstOf(rest, "--mirror", "--prune")+" can delete protected branches on the remote")
		return
	}
	var targets []string
	for _, r := range refspecs {
		if strings.HasPrefix(r, "+") {
			force = true
			r = r[1:]
		}
		if strings.HasPrefix(r, ":") {
			del = true
			targets = append(targets, r[1:])
			continue
		}
		if i := strings.IndexByte(r, ':'); i >= 0 {
			targets = append(targets, r[i+1:])
			continue
		}
		if del {
			targets = append(targets, r)
			continue
		}
		if r == "HEAD" {
			targets = append(targets, c.currentBranch())
			continue
		}
		targets = append(targets, r)
	}
	if !force && !del {
		if containsAny(rest, "--all", "--tags") {
			return
		}
		return
	}
	if len(targets) == 0 {
		targets = []string{c.currentBranch()}
	}
	for _, t := range targets {
		if t == "" || c.protectedBranch(t) {
			what := "force push"
			if del {
				what = "branch deletion"
			}
			name := t
			if name == "" {
				name = "the current branch (unknown, treated as protected)"
			}
			c.deny(s, "D4", what+" to protected branch "+name)
			return
		}
	}
	s.Reason = "git push --force to an unprotected branch"
}

func (c *classifier) classifyGh(s *Segment) {
	argv := s.Argv
	s.Class = ClassMutateOut
	if len(argv) < 2 {
		s.Class = ClassRead
		return
	}
	sub := argv[1]
	rest := argv[2:]
	switch sub {
	case "auth":
		if containsAny(rest, "token") {
			c.deny(s, "D7", "gh auth token prints the OAuth token")
			return
		}
		if containsAny(rest, "status") {
			s.Class = ClassRead
		}
	case "api":
		joined := strings.Join(rest, " ")
		for i, a := range rest {
			if (a == "-X" || a == "--method") && i+1 < len(rest) && strings.EqualFold(rest[i+1], "DELETE") || strings.EqualFold(a, "-XDELETE") || strings.EqualFold(a, "--method=DELETE") {
				c.deny(s, "D6", "gh api -X DELETE")
				return
			}
		}
		if containsAny(rest, "graphql") && reDeleteGQL.MatchString(joined) {
			c.deny(s, "D6", "gh api graphql with a delete mutation")
			return
		}
		if containsAny(rest, "graphql") && strings.Contains(strings.ToLower(joined), "mutation") || containsAny(rest, "-X", "--method", "-f", "-F", "--field", "--raw-field", "--input") {
			s.Reason = "gh api mutation"
			return
		}
		s.Class = ClassNetworkFetch
	case "repo", "release", "project", "secret", "variable", "gist", "codespace", "cache", "label", "ssh-key", "gpg-key", "ruleset", "workflow", "run", "pr", "issue":
		if containsAny(rest, "delete", "delete-asset", "item-delete", "purge") || sub == "repo" && containsAny(rest, "archive") {
			c.deny(s, "D6", "gh "+sub+" "+firstOf(rest, "delete", "delete-asset", "item-delete", "purge", "archive")+" removes a remote resource")
			return
		}
		if sub == "release" && containsAny(rest, "create", "upload") {
			c.deny(s, "D8", "gh release "+firstOf(rest, "create", "upload")+" publishes")
			return
		}
		if containsAny(rest, "list", "view", "ls", "status", "diff", "checks", "download", "watch", "clone") {
			s.Class = ClassNetworkFetch
			if containsAny(rest, "clone") || containsAny(rest, "download") {
				s.Class = ClassMutateIn
			}
		}
	case "browse", "search", "status", "extension", "alias", "config", "completion", "help", "--version", "version":
		s.Class = ClassRead
		if sub == "extension" && containsAny(rest, "install", "remove", "upgrade") || sub == "alias" && containsAny(rest, "set", "delete") || sub == "config" && containsAny(rest, "set") {
			s.Class = ClassMutateOut
		}
	}
}

func (c *classifier) classifyCloud(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassMutateOut
	s.Reason = verb + ": cloud mutation"
	low := make([]string, len(argv))
	for i, a := range argv {
		low[i] = strings.ToLower(a)
	}
	switch verb {
	case "aws":
		if len(low) > 2 && low[1] == "configure" && (low[2] == "get" || low[2] == "export-credentials" || low[2] == "list") {
			c.deny(s, "D7", "aws configure "+low[2]+" prints credentials")
			return
		}
		for i := 2; i < len(low); i++ {
			a := low[i]
			if strings.HasPrefix(a, "delete-") || a == "terminate-instances" || a == "purge-queue" || a == "deregister-image" || a == "remove-permission" || a == "empty-bucket" {
				c.deny(s, "D6", "aws "+low[1]+" "+argv[i]+" destroys a cloud resource")
				return
			}
		}
		if len(low) > 2 && low[1] == "s3" {
			if low[2] == "rb" && containsAny(low, "--force") {
				c.deny(s, "D6", "aws s3 rb --force removes a bucket and its objects")
				return
			}
			if low[2] == "rm" && containsAny(low, "--recursive") {
				c.deny(s, "D6", "aws s3 rm --recursive removes objects in bulk")
				return
			}
			if low[2] == "sync" && containsAny(low, "--delete") {
				c.deny(s, "D6", "aws s3 sync --delete removes objects in bulk")
				return
			}
		}
		if len(low) > 2 && (strings.HasPrefix(low[2], "describe-") || strings.HasPrefix(low[2], "list-") || strings.HasPrefix(low[2], "get-") || low[2] == "ls" || low[2] == "head-object" || low[2] == "head-bucket") && !(low[1] == "secretsmanager" || low[1] == "ssm" && low[2] == "get-parameter" && containsAny(low, "--with-decryption") || low[1] == "sts" && low[2] == "get-session-token" || low[1] == "sts" && low[2] == "assume-role" || low[1] == "iam" && low[2] == "create-access-key") {
			s.Class = ClassNetworkFetch
			s.Reason = ""
			return
		}
		if len(low) > 2 && (low[1] == "secretsmanager" && low[2] == "get-secret-value" || low[1] == "ssm" && low[2] == "get-parameter" && containsAny(low, "--with-decryption") || low[1] == "sts" && (low[2] == "get-session-token" || low[2] == "assume-role") || low[1] == "iam" && low[2] == "create-access-key" || low[1] == "ecr" && low[2] == "get-login-password") {
			c.deny(s, "D7", "aws "+low[1]+" "+low[2]+" returns a secret")
			return
		}
	case "gcloud":
		if containsAny(low, "delete", "destroy", "purge") {
			c.deny(s, "D6", "gcloud "+firstOf(low, "delete", "destroy", "purge")+" destroys a cloud resource")
			return
		}
		if len(low) > 2 && low[1] == "auth" && (strings.HasPrefix(low[2], "print-") || low[2] == "application-default" && containsAny(low, "print-access-token")) {
			c.deny(s, "D7", "gcloud auth "+low[2]+" prints a token")
			return
		}
		if len(low) > 2 && low[1] == "secrets" && containsAny(low, "access") {
			c.deny(s, "D7", "gcloud secrets access prints a secret")
			return
		}
		if containsAny(low, "list", "describe", "get", "config", "info", "version", "get-iam-policy") {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
	case "az":
		if containsAny(low, "delete", "purge") {
			c.deny(s, "D6", "az "+firstOf(low, "delete", "purge")+" destroys a cloud resource")
			return
		}
		if len(low) > 2 && low[1] == "account" && low[2] == "get-access-token" || len(low) > 2 && low[1] == "keyvault" && low[2] == "secret" && containsAny(low, "show", "download") || containsAny(low, "list-keys", "list-credentials", "renew-key", "regenerate-key") {
			c.deny(s, "D7", "az "+strings.Join(argv[1:min(len(argv), 4)], " ")+" returns a secret")
			return
		}
		if containsAny(low, "list", "show", "get", "version") {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
	case "gsutil":
		if (containsAny(low, "rm") && (containsAny(low, "-r", "-R") || hasFlag(low, reFlagHasR))) || containsAny(low, "rb") || (containsAny(low, "rsync") && containsAny(low, "-d")) {
			c.deny(s, "D6", "gsutil bulk removal")
			return
		}
		if containsAny(low, "ls", "cat", "stat", "du", "version", "hash") {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
	case "doctl", "linode-cli":
		if containsAny(low, "delete", "d", "rm", "destroy") {
			c.deny(s, "D6", verb+" deletes a cloud resource")
			return
		}
		if containsAny(low, "list", "get", "ls") {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
	case "wrangler":
		if containsAny(low, "delete") {
			c.deny(s, "D6", "wrangler delete removes a deployed resource")
			return
		}
		if containsAny(low, "dev", "whoami", "tail", "list", "--version") {
			s.Class = ClassMutateIn
			s.Reason = ""
		}
	}
}

func (c *classifier) classifyIaC(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassMutateOut
	s.Reason = verb + ": infrastructure mutation"
	sub := ""
	if len(argv) > 1 {
		sub = argv[1]
	}
	switch verb {
	case "terraform", "tofu", "terragrunt":
		switch sub {
		case "destroy":
			c.deny(s, "D6", verb+" destroy removes every managed resource")
			return
		case "apply":
			if containsAny(argv, "-auto-approve", "--auto-approve", "-destroy", "--destroy") {
				c.deny(s, "D6", verb+" apply -auto-approve against a backend guard cannot verify as local")
				return
			}
		case "state":
			if containsAny(argv, "rm", "push", "replace-provider") {
				c.deny(s, "D6", verb+" state "+firstOf(argv, "rm", "push", "replace-provider")+" rewrites remote state")
				return
			}
			s.Class = ClassNetworkFetch
			s.Reason = ""
		case "workspace":
			if containsAny(argv, "delete") {
				c.deny(s, "D6", verb+" workspace delete")
				return
			}
			s.Class = ClassNetworkFetch
			s.Reason = ""
		case "init", "get", "providers", "fmt", "validate", "graph", "version", "console", "login":
			s.Class = ClassMutateIn
			s.Reason = ""
			if sub == "login" {
				s.Class = ClassMutateOut
			}
		case "plan", "show", "output", "refresh", "test":
			s.Class = ClassNetworkFetch
			s.Reason = ""
			if sub == "output" && (containsAny(argv, "-json") || containsAny(argv, "-raw")) {
				s.Class = ClassMutateOut
				s.Reason = "terraform output may print sensitive values"
			}
		}
	case "pulumi":
		if sub == "destroy" || sub == "stack" && containsAny(argv, "rm") || sub == "state" && containsAny(argv, "delete") {
			c.deny(s, "D6", "pulumi "+sub+" destroys stack resources or state")
			return
		}
		if sub == "up" && containsAny(argv, "--yes", "-y") {
			s.Reason = "pulumi up --yes applies without a prompt"
		}
		if sub == "preview" || sub == "stack" && containsAny(argv, "ls", "output") && !containsAny(argv, "--show-secrets") || sub == "whoami" || sub == "version" || sub == "about" {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
		if containsAny(argv, "--show-secrets") || sub == "config" && containsAny(argv, "get") {
			c.deny(s, "D7", "pulumi prints secret configuration")
			return
		}
	case "cdk", "sam", "sls", "serverless", "eb":
		if sub == "destroy" || sub == "delete" || sub == "remove" || sub == "terminate" {
			c.deny(s, "D6", verb+" "+sub+" removes the deployed stack")
			return
		}
		if sub == "synth" || sub == "diff" || sub == "ls" || sub == "list" || sub == "validate" || sub == "build" || sub == "package" || sub == "local" || sub == "status" || sub == "info" || sub == "print" || sub == "doctor" || sub == "bootstrap" && false {
			s.Class = ClassMutateIn
			s.Reason = ""
		}
	case "firebase":
		if strings.HasSuffix(sub, ":delete") || sub == "hosting:disable" || strings.HasSuffix(sub, ":remove") || sub == "firestore:delete" || sub == "database:remove" || sub == "projects:delete" {
			c.deny(s, "D6", "firebase "+sub+" deletes remote data")
			return
		}
		if sub == "emulators:start" || sub == "serve" || sub == "init" || strings.HasSuffix(sub, ":get") || strings.HasSuffix(sub, ":list") || sub == "projects:list" || sub == "use" || sub == "--version" {
			s.Class = ClassMutateIn
			s.Reason = ""
		}
		if sub == "login:ci" || sub == "functions:config:get" || sub == "functions:secrets:access" {
			c.deny(s, "D7", "firebase "+sub+" prints a credential")
			return
		}
	case "heroku":
		if sub == "apps:destroy" || sub == "pg:reset" || sub == "addons:destroy" || sub == "domains:remove" || sub == "pipelines:destroy" {
			c.deny(s, "D6", "heroku "+sub+" destroys a remote resource")
			return
		}
		if sub == "config" || sub == "config:get" || sub == "auth:token" {
			c.deny(s, "D7", "heroku "+sub+" prints config vars or a token")
			return
		}
		if strings.HasSuffix(sub, ":info") || sub == "apps" || sub == "logs" || sub == "ps" || sub == "releases" {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
	case "fly", "flyctl":
		if sub == "apps" && containsAny(argv, "destroy") || sub == "destroy" || sub == "volumes" && containsAny(argv, "destroy", "delete") || sub == "postgres" && containsAny(argv, "destroy") || sub == "machine" && containsAny(argv, "destroy") {
			c.deny(s, "D6", "fly "+sub+" destroy")
			return
		}
		if sub == "secrets" && containsAny(argv, "list") || sub == "status" || sub == "logs" || sub == "apps" && containsAny(argv, "list") || sub == "version" {
			s.Class = ClassNetworkFetch
			s.Reason = ""
		}
		if sub == "auth" && containsAny(argv, "token") {
			c.deny(s, "D7", "fly auth token prints the token")
			return
		}
	case "vercel", "netlify":
		if sub == "remove" || sub == "rm" || sub == "sites:delete" || sub == "env" && containsAny(argv, "rm", "remove") || sub == "domains" && containsAny(argv, "rm") || sub == "project" && containsAny(argv, "rm") || sub == "teams" && containsAny(argv, "rm") {
			c.deny(s, "D6", verb+" "+sub+" removes a deployment or project")
			return
		}
		if sub == "env" && containsAny(argv, "pull", "ls", "list") || sub == "--version" || sub == "whoami" || sub == "ls" || sub == "list" || sub == "inspect" || sub == "logs" || sub == "status" || sub == "dev" || sub == "build" {
			s.Class = ClassMutateIn
			s.Reason = ""
			if sub == "env" && containsAny(argv, "pull") {
				c.raise(s, c.pathClass(s, ".env.local", verb), "")
			}
		}
	}
}

func (c *classifier) classifyKube(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassMutateOut
	s.Reason = verb + ": cluster mutation"
	if len(argv) < 2 {
		return
	}
	sub := argv[1]
	rest := argv[2:]
	switch verb {
	case "helm":
		switch sub {
		case "uninstall", "delete", "del", "un":
			s.Reason = "helm uninstall removes a release"
		case "list", "ls", "status", "get", "show", "search", "repo", "version", "env", "history", "template", "lint", "dependency", "dep", "inspect", "verify", "package", "create":
			s.Class = ClassNetworkFetch
			s.Reason = ""
			if sub == "get" && containsAny(rest, "values") && containsAny(rest, "--all", "-a") {
				s.Class = ClassMutateOut
				s.Reason = "helm get values --all may print secrets"
			}
		}
		return
	}
	switch sub {
	case "delete", "del":
		if containsAny(rest, "namespace", "ns", "namespaces", "--all", "-A", "--all-namespaces", "pv", "persistentvolume", "persistentvolumes", "pvc", "persistentvolumeclaim", "persistentvolumeclaims", "crd", "customresourcedefinition", "customresourcedefinitions", "node", "nodes") {
			c.deny(s, "D6", "kubectl delete of a namespace, volume, node or --all")
			return
		}
		for _, a := range rest {
			if strings.HasPrefix(a, "ns/") || strings.HasPrefix(a, "namespace/") || strings.HasPrefix(a, "pv/") || strings.HasPrefix(a, "pvc/") || strings.HasPrefix(a, "node/") {
				c.deny(s, "D6", "kubectl delete "+a)
				return
			}
		}
		s.Reason = "kubectl delete removes cluster objects"
	case "drain", "cordon", "taint":
		c.deny(s, "D6", "kubectl "+sub+" takes a node out of service")
		return
	case "get", "describe", "logs", "top", "version", "api-resources", "api-versions", "explain", "cluster-info", "diff", "auth", "events", "wait", "completion", "options", "kustomize":
		s.Class = ClassNetworkFetch
		s.Reason = ""
		if sub == "get" && containsAny(rest, "secret", "secrets") && (containsAny(rest, "-o", "--output") || hasFlag(argv, nil, "-o", "--output")) {
			c.deny(s, "D7", "kubectl get secret -o prints secret data")
			return
		}
	case "config":
		if containsAny(rest, "view") && containsAny(rest, "--raw") {
			c.deny(s, "D7", "kubectl config view --raw prints tokens")
			return
		}
		if containsAny(rest, "view", "current-context", "get-contexts", "get-clusters", "get-users") {
			s.Class = ClassRead
			s.Reason = ""
		}
	}
}

func (c *classifier) classifyDocker(s *Segment) {
	argv := s.Argv
	s.Class = ClassMutateOut
	s.Reason = "docker: daemon state outside scope"
	if len(argv) < 2 {
		s.Class = ClassRead
		return
	}
	sub := argv[1]
	rest := argv[2:]
	if sub == "compose" && len(rest) > 0 {
		sub, rest = "compose "+rest[0], rest[1:]
	}
	switch sub {
	case "login":
		c.deny(s, "D7", "docker login output exposes registry credentials")
		return
	case "push", "compose push", "buildx":
		if sub == "buildx" && !containsAny(rest, "--push") {
			break
		}
		c.deny(s, "D8", "docker push publishes an image")
		return
	case "ps", "images", "image", "logs", "inspect", "version", "info", "stats", "top", "port", "diff", "history", "events", "search", "manifest", "context", "compose ps", "compose logs", "compose config", "compose images", "compose ls", "compose version", "network", "volume", "system":
		s.Class = ClassRead
		s.Reason = ""
		if containsAny(rest, "rm", "prune", "remove", "create", "connect", "disconnect", "import", "load", "tag", "build", "push", "pull", "use", "update") {
			s.Class = ClassMutateOut
			s.Reason = "docker " + sub + " " + firstOf(rest, "rm", "prune", "remove", "create", "connect", "disconnect", "import", "load", "tag", "build", "push", "pull", "use", "update")
			if containsAny(rest, "push") {
				c.deny(s, "D8", "docker image push publishes an image")
				return
			}
		}
	case "build", "compose build", "pull", "compose pull", "save", "load", "tag":
		s.Class = ClassMutateIn
		s.Reason = ""
		if sub == "build" || sub == "compose build" {
			if containsAny(rest, "--push") {
				c.deny(s, "D8", "docker build --push publishes an image")
				return
			}
		}
		if sub == "save" {
			for i, a := range rest {
				if (a == "-o" || a == "--output") && i+1 < len(rest) {
					c.raise(s, c.pathClass(s, rest[i+1], "docker save"), "")
				}
			}
		}
	case "exec", "run", "compose run", "compose exec":
		if sub == "run" || sub == "compose run" {
			for i, a := range rest {
				if (a == "-v" || a == "--volume" || a == "--mount") && i+1 < len(rest) {
					spec := rest[i+1]
					host := spec
					if strings.HasPrefix(spec, "type=") {
						for _, kv := range strings.Split(spec, ",") {
							if strings.HasPrefix(kv, "src=") || strings.HasPrefix(kv, "source=") {
								host = kv[strings.IndexByte(kv, '=')+1:]
							}
						}
					} else if j := strings.IndexByte(spec, ':'); j > 0 {
						host = spec[:j]
					}
					if looksLikePath(host) {
						q := c.abs(host)
						if c.scope.isProtectedRoot(q) && !containsAny(strings.Split(spec, ":"), "ro") {
							c.deny(s, "D1", "docker run mounts "+q+" ("+c.scope.protectedName(q)+") read-write", q)
							return
						}
						if c.credentialHit(q) {
							c.deny(s, "D7", "docker run mounts credential path "+q, q)
							return
						}
					}
				}
			}
		}
		s.Reason = "docker " + sub + ": container process"
	case "rm", "rmi", "compose down", "compose rm", "kill", "stop", "restart", "compose stop", "compose kill", "compose restart", "start", "compose start", "compose up", "up", "create", "compose create", "cp", "commit", "compose pause", "compose unpause", "pause", "unpause", "update", "rename", "attach", "wait", "export", "import":
		s.Reason = "docker " + sub
		if sub == "cp" {
			for _, a := range rest {
				if !strings.Contains(a, ":") && looksLikePath(a) {
					q := c.abs(a)
					if c.credentialHit(q) {
						c.deny(s, "D7", "docker cp reads credential path "+q, q)
						return
					}
				}
			}
		}
		if (sub == "compose down") && containsAny(rest, "-v", "--volumes") {
			s.Reason = "docker compose down -v removes volumes"
		}
	}
}

func (c *classifier) classifyNpm(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassMutateIn
	if len(argv) < 2 {
		return
	}
	sub := argv[1]
	rest := argv[2:]
	if sub == "npm" && verb == "yarn" && len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}
	switch sub {
	case "publish", "unpublish", "deprecate", "owner", "dist-tag", "access", "star", "unstar":
		if sub == "publish" && containsAny(rest, "--dry-run") {
			return
		}
		c.deny(s, "D8", verb+" "+sub+" changes the public registry")
		return
	case "login", "logout", "adduser", "token", "whoami", "profile":
		if sub == "token" && (len(rest) == 0 || containsAny(rest, "list", "create")) || sub == "profile" && containsAny(rest, "get") {
			c.deny(s, "D7", verb+" "+sub+" prints registry credentials")
			return
		}
		s.Class = ClassMutateOut
		s.Reason = verb + " " + sub + " changes ~/.npmrc"
	case "config", "c", "set", "get":
		if sub == "get" || sub == "config" && (len(rest) > 0 && (rest[0] == "get" || rest[0] == "list" || rest[0] == "ls")) {
			joined := strings.ToLower(strings.Join(rest, " "))
			if strings.Contains(joined, "authtoken") || strings.Contains(joined, "_auth") || strings.Contains(joined, "password") || containsAny(rest, "-l", "--json") {
				c.deny(s, "D7", verb+" config prints auth settings")
				return
			}
			s.Class = ClassRead
			return
		}
		s.Class = ClassMutateOut
		s.Reason = verb + " config set writes ~/.npmrc"
		if containsAny(rest, "--location=project") || containsAny(rest, "-L", "project") {
			s.Class = ClassMutateIn
			s.Reason = ""
		}
	case "install", "i", "add", "ci", "remove", "rm", "uninstall", "un", "update", "up", "upgrade", "dedupe", "prune", "rebuild", "link", "unlink", "audit", "outdated", "cache", "install-test", "it", "install-ci-test", "cit", "import", "patch", "patch-commit", "set-script", "pkg", "init", "create", "version", "workspaces", "workspace", "w":
		if containsAny(argv, "-g", "--global", "global") {
			s.Class = ClassMutateOut
			s.Reason = verb + " -g installs outside the repository"
		}
		if sub == "audit" && !containsAny(rest, "fix") || sub == "outdated" {
			s.Class = ClassNetworkFetch
		}
		if sub == "cache" && containsAny(rest, "clean", "clear", "rm") {
			s.Class = ClassMutateOut
			s.Reason = verb + " cache clean removes the shared cache"
		}
	case "ls", "list", "la", "ll", "view", "v", "info", "show", "search", "s", "se", "find", "why", "explain", "help", "-v", "--version", "-h", "--help", "bin", "root", "prefix", "docs", "repo", "bugs", "fund", "ping", "doctor", "diff", "licenses", "query", "explore", "licenses.bat", "org", "team", "hook", "sbom":
		s.Class = ClassRead
		if sub == "view" || sub == "v" || sub == "info" || sub == "show" || sub == "search" || sub == "s" || sub == "se" || sub == "ping" || sub == "docs" || sub == "repo" || sub == "bugs" || sub == "fund" {
			s.Class = ClassNetworkFetch
		}
		if sub == "org" || sub == "team" || sub == "hook" {
			s.Class = ClassMutateOut
			s.Reason = verb + " " + sub + " changes registry organisation state"
		}
	case "test", "t", "tst", "run", "run-script", "start", "stop", "restart", "build", "exec", "x", "dlx", "pack", "dev", "lint", "format", "fmt", "check", "typecheck", "coverage", "bench", "clean", "prepare", "prepublishOnly", "postinstall", "preinstall", "compile", "watch", "serve", "storybook", "e2e", "docs:build", "generate", "codegen", "migrate", "seed", "release", "deploy", "changeset", "turbo", "nx", "lerna", "husky", "lint-staged", "prettier", "eslint", "tsc", "jest", "vitest", "next", "vite", "webpack", "node", "env", "info.bat", "why.bat", "run.bat":
		s.Class = ClassMutateIn
		if sub == "release" || sub == "deploy" || sub == "changeset" && containsAny(rest, "publish") {
			s.Class = ClassMutateOut
			s.Reason = verb + " " + sub + ": script that may publish or deploy (body classified separately)"
			if sub == "changeset" && containsAny(rest, "publish") {
				c.deny(s, "D8", "changeset publish publishes packages")
				return
			}
		}
	default:
		s.Class = ClassMutateIn // scripts run through package.json are classified as children
	}
}

func (c *classifier) classifyPip(s *Segment, verb string) {
	argv := s.Argv
	sub := ""
	if len(argv) > 1 {
		sub = argv[1]
	}
	venv := c.env["VIRTUAL_ENV"]
	inVenv := venv != "" && c.scope.inScope(canonPath(c.cwd, venv))
	if !inVenv && (verb == "uv" || verb == "poetry" || verb == "pipenv") {
		inVenv = isDir(filepath.Join(c.cwd, ".venv")) || isDir(filepath.Join(c.scope.rawRoot, ".venv"))
	}
	switch verb {
	case "twine", "flit", "hatch":
		if sub == "upload" || sub == "publish" {
			c.deny(s, "D8", verb+" "+sub+" publishes to PyPI")
			return
		}
		s.Class = ClassMutateIn
		return
	case "poetry", "uv":
		if sub == "publish" {
			c.deny(s, "D8", verb+" publish publishes to PyPI")
			return
		}
		if verb == "uv" && sub == "pip" && len(argv) > 2 {
			sub = argv[2]
		}
	}
	switch sub {
	case "install", "uninstall", "sync", "add", "remove", "update", "upgrade", "lock", "download", "wheel", "compile", "tool", "python", "self", "cache", "env", "shell", "run", "build", "init", "new", "check", "config", "source", "search", "show", "list", "freeze", "inspect", "index", "debug", "hash", "help", "tree", "version", "--version", "export", "venv":
		switch sub {
		case "download", "search", "index", "show", "list", "freeze", "check", "inspect", "debug", "hash", "help", "tree", "--version", "version", "export":
			s.Class = ClassRead
			if sub == "download" || sub == "search" || sub == "index" || sub == "show" && verb == "pip" && false {
				s.Class = ClassNetworkFetch
			}
			if sub == "config" && containsAny(argv, "list", "get") {
				s.Class = ClassRead
			}
			return
		case "config", "self", "tool", "python":
			s.Class = ClassMutateOut
			s.Reason = verb + " " + sub + " changes the user toolchain"
			if verb == "uv" && sub == "tool" && containsAny(argv, "run", "list", "dir") {
				s.Class = ClassMutateIn
				s.Reason = ""
			}
			return
		case "install", "uninstall", "sync", "add", "remove", "update", "upgrade", "wheel", "compile":
			if containsAny(argv, "--user") || containsAny(argv, "-g") || verb == "pipx" {
				s.Class = ClassMutateOut
				s.Reason = verb + " " + sub + " --user writes the user site"
				return
			}
			if inVenv || containsAny(argv, "--target", "-t", "--prefix", "--root") && false || verb == "poetry" || verb == "pipenv" || verb == "uv" && sub != "pip" {
				s.Class = ClassMutateIn
				for i, a := range argv {
					if (a == "--target" || a == "-t" || a == "--prefix" || a == "--root") && i+1 < len(argv) {
						c.raise(s, c.pathClass(s, argv[i+1], verb), "")
					}
				}
				return
			}
			s.Class = ClassMutateOut
			s.Reason = verb + " " + sub + " writes site-packages outside scope"
			return
		}
		s.Class = ClassMutateIn
	default:
		s.Class = ClassMutateIn
	}
}

func (c *classifier) classifyCargo(s *Segment) {
	argv := s.Argv
	s.Class = ClassMutateIn
	if len(argv) < 2 {
		return
	}
	switch argv[1] {
	case "publish":
		if containsAny(argv, "--dry-run") {
			return
		}
		c.deny(s, "D8", "cargo publish publishes to crates.io")
	case "owner", "yank":
		c.deny(s, "D8", "cargo "+argv[1]+" changes the registry")
	case "login", "logout":
		s.Class = ClassMutateOut
		s.Reason = "cargo login writes ~/.cargo/credentials"
	case "install", "uninstall":
		if containsAny(argv, "--path", "--root") && false {
			return
		}
		s.Class = ClassMutateOut
		s.Reason = "cargo install writes ~/.cargo/bin"
	case "metadata", "tree", "search", "--version", "-V", "version", "--list", "help", "pkgid", "read-manifest", "locate-project", "verify-project", "info":
		s.Class = ClassRead
		if argv[1] == "search" || argv[1] == "info" {
			s.Class = ClassNetworkFetch
		}
	case "fetch", "update", "generate-lockfile", "vendor", "add", "remove":
		s.Class = ClassMutateIn
	}
}

func (c *classifier) classifyGo(s *Segment) {
	argv := s.Argv
	s.Class = ClassMutateIn
	if len(argv) < 2 {
		return
	}
	switch argv[1] {
	case "version", "doc", "list", "env", "help", "bug", "vet", "fix", "fmt":
		s.Class = ClassRead
		if argv[1] == "env" && containsAny(argv, "-w", "-u") {
			s.Class = ClassMutateOut
			s.Reason = "go env -w writes the user go env"
		}
		if argv[1] == "fmt" || argv[1] == "fix" {
			s.Class = ClassMutateIn
		}
	case "install":
		for _, a := range argv[2:] {
			if strings.Contains(a, "@") {
				s.Class = ClassMutateOut
				s.Reason = "go install pkg@version writes GOPATH/bin"
				return
			}
		}
		s.Class = ClassMutateOut
		s.Reason = "go install writes GOPATH/bin"
	case "clean":
		if containsAny(argv, "-cache", "-modcache", "-testcache", "-fuzzcache") {
			s.Class = ClassMutateOut
			s.Reason = "go clean removes shared caches outside scope"
		}
	case "run":
		s.Class = ClassMutateIn
	case "get", "mod", "work":
		s.Class = ClassMutateIn
	case "tool", "generate", "build", "test", "telemetry":
		if argv[1] == "telemetry" && containsAny(argv, "on", "off", "local") {
			s.Class = ClassMutateOut
		}
	}
}

func (c *classifier) classifyBuildTool(s *Segment, verb string) {
	argv := s.Argv
	s.Class = ClassMutateIn
	low := make([]string, len(argv))
	for i, a := range argv {
		low[i] = strings.ToLower(a)
	}
	switch verb {
	case "gem":
		if containsAny(low, "push", "yank", "owner") {
			c.deny(s, "D8", "gem "+firstOf(low, "push", "yank", "owner")+" changes rubygems.org")
			return
		}
		if containsAny(low, "install", "uninstall", "update", "cleanup") && !containsAny(low, "--install-dir") {
			s.Class = ClassMutateOut
			s.Reason = "gem install writes the gem home outside scope"
		}
		if containsAny(low, "list", "search", "info", "spec", "which", "env", "-v", "--version", "contents", "dependency") {
			s.Class = ClassRead
		}
	case "bundle", "bundler":
		if containsAny(low, "exec") || containsAny(low, "install") || containsAny(low, "update") || containsAny(low, "add") {
			s.Class = ClassMutateIn
		}
		if containsAny(low, "list", "show", "info", "check", "outdated", "-v", "--version", "platform", "env") {
			s.Class = ClassRead
		}
		if containsAny(low, "exec") {
			for i, a := range low {
				if a == "exec" && i+1 < len(argv) {
					child := &Segment{Raw: s.Raw, Argv: argv[i+1:], depth: s.depth + 1}
					child.argvClean = boolsOf(len(child.Argv), true)
					child.argvSubst = make([]string, len(child.Argv))
					c.classify(child)
					if child.Class == ClassDestructive {
						s.Class, s.Rule, s.Reason, s.Paths = child.Class, child.Rule, child.Reason, child.Paths
					} else if classRank(child.Class) > classRank(s.Class) {
						s.Class, s.Reason = child.Class, child.Reason
					}
					return
				}
			}
		}
	case "pod":
		if containsAny(low, "trunk") && containsAny(low, "push") || containsAny(low, "repo") && containsAny(low, "push") {
			c.deny(s, "D8", "pod trunk push publishes a podspec")
			return
		}
		if containsAny(low, "trunk") && containsAny(low, "register", "me") {
			s.Class = ClassMutateOut
			s.Reason = "pod trunk session"
		}
		if containsAny(low, "search", "list", "spec", "outdated", "--version", "env", "ipc") {
			s.Class = ClassRead
		}
	case "dart", "flutter":
		if containsAny(low, "pub") && containsAny(low, "publish") && !containsAny(low, "--dry-run") || containsAny(low, "pub") && containsAny(low, "uploader") {
			c.deny(s, "D8", verb+" pub publish publishes to pub.dev")
			return
		}
		if containsAny(low, "doctor", "--version", "devices", "emulators", "analyze", "format", "channel", "config") && !containsAny(low, "--enable-", "--disable-") {
			s.Class = ClassRead
			if containsAny(low, "format", "channel") {
				s.Class = ClassMutateIn
				if containsAny(low, "channel") && len(low) > 2 {
					s.Class = ClassMutateOut
				}
			}
		}
		if containsAny(low, "pub") && containsAny(low, "global") && containsAny(low, "activate", "deactivate") || containsAny(low, "upgrade", "downgrade") && verb == "flutter" && !containsAny(low, "pub") || containsAny(low, "precache") {
			s.Class = ClassMutateOut
			s.Reason = verb + " changes the SDK outside scope"
		}
	case "mvn", "gradle", "gradlew":
		for _, a := range low {
			if a == "deploy" || strings.HasPrefix(a, "deploy:") || a == "publish" || strings.HasPrefix(a, "publish") && strings.Contains(a, "publish") && !strings.Contains(a, "tomavenlocal") || a == "release:perform" || a == "nexus-staging:release" || a == "jreleaser:deploy" || a == "uploadarchives" || a == "bintrayupload" || a == "closeandreleaserepository" {
				c.deny(s, "D8", verb+" "+a+" publishes artifacts")
				return
			}
		}
		if containsAny(low, "install", "publishtomavenlocal") {
			s.Class = ClassMutateOut
			s.Reason = verb + " install writes ~/.m2"
		}
		if containsAny(low, "help", "--version", "-v", "dependency:tree", "dependencies", "tasks", "projects", "properties", "components", "model", "buildenvironment", "javatoolchains", "--status", "--stop", "wrapper") {
			s.Class = ClassRead
			if containsAny(low, "wrapper", "--stop") {
				s.Class = ClassMutateIn
			}
		}
	case "dotnet", "nuget":
		if containsAny(low, "nuget") && containsAny(low, "push", "delete") || verb == "nuget" && containsAny(low, "push", "delete", "setapikey") {
			c.deny(s, "D8", verb+" nuget push publishes a package")
			return
		}
		if containsAny(low, "tool") && containsAny(low, "install", "update", "uninstall") && containsAny(low, "-g", "--global") || containsAny(low, "workload") || containsAny(low, "dev-certs") || containsAny(low, "user-secrets") && containsAny(low, "list") {
			s.Class = ClassMutateOut
			s.Reason = verb + " changes the user toolchain"
			if containsAny(low, "user-secrets") && containsAny(low, "list") {
				c.deny(s, "D7", "dotnet user-secrets list prints secrets")
				return
			}
		}
		if containsAny(low, "--version", "--info", "--list-sdks", "--list-runtimes", "sln", "list", "help") {
			s.Class = ClassRead
			if containsAny(low, "sln") && containsAny(low, "add", "remove") {
				s.Class = ClassMutateIn
			}
		}
	case "goreleaser":
		if containsAny(low, "release") && !containsAny(low, "--snapshot", "--skip-publish", "--skip=publish", "--skip", "--clean") || containsAny(low, "publish") {
			if !containsAny(low, "--snapshot", "--skip-publish", "--skip=publish") {
				c.deny(s, "D8", "goreleaser release publishes artifacts")
				return
			}
		}
		if containsAny(low, "check", "healthcheck", "--version", "jsonschema", "schema") {
			s.Class = ClassRead
		}
	case "swift", "xcodebuild":
		if verb == "swift" && len(argv) > 1 && (argv[1] == "package" && containsAny(low, "clean", "reset", "purge-cache") || argv[1] == "run" || argv[1] == "build" || argv[1] == "test") {
			if containsAny(low, "purge-cache") {
				s.Class = ClassMutateOut
				s.Reason = "swift package purge-cache removes the shared cache"
			}
			return
		}
		if verb == "swift" && len(argv) > 1 && !strings.HasPrefix(argv[1], "-") && (strings.HasSuffix(argv[1], ".swift") || looksLikePath(argv[1])) {
			c.scriptExec(s, argv[1])
			return
		}
		if verb == "swift" && len(argv) == 1 {
			s.Class = ClassMutateOut
			s.Reason = "interactive swift REPL"
			return
		}
		if verb == "xcodebuild" && (containsAny(low, "-list", "-showsdks", "-version", "-showbuildsettings", "-showdestinations", "-usage", "-help")) {
			s.Class = ClassRead
		}
		if verb == "xcodebuild" && containsAny(low, "-exportarchive", "-allowprovisioningupdates") {
			s.Class = ClassMutateOut
			s.Reason = "xcodebuild signing or export touches the keychain and developer portal"
		}
	case "mix":
		if containsAny(low, "hex.publish") {
			c.deny(s, "D8", "mix hex.publish publishes to hex.pm")
			return
		}
		if containsAny(low, "ecto.drop", "ecto.reset") {
			c.deny(s, "D5", "mix "+firstOf(low, "ecto.drop", "ecto.reset")+" drops the database")
			return
		}
		if containsAny(low, "archive.install", "escript.install", "local.hex", "local.rebar", "hex.config") {
			s.Class = ClassMutateOut
			s.Reason = "mix changes the user toolchain"
		}
	case "composer":
		if containsAny(low, "global") || containsAny(low, "self-update", "selfupdate") {
			s.Class = ClassMutateOut
			s.Reason = "composer global or self-update writes outside scope"
		}
		if containsAny(low, "config") && containsAny(low, "-g", "--global") && !containsAny(low, "--unset") || containsAny(low, "config") && containsAny(low, "http-basic", "github-oauth", "gitlab-token", "bearer") && !containsAny(low, "--unset") && len(low) <= 4 {
			c.deny(s, "D7", "composer config prints or sets auth tokens")
			return
		}
		if containsAny(low, "show", "info", "outdated", "licenses", "depends", "why", "prohibits", "why-not", "validate", "check-platform-reqs", "diagnose", "--version", "-v", "status", "suggests", "fund", "search", "browse", "home", "list", "help", "about", "audit") {
			s.Class = ClassRead
		}
	case "stack", "cabal":
		if containsAny(low, "upload", "publish", "sdist") && containsAny(low, "upload", "publish") {
			c.deny(s, "D8", verb+" upload publishes to Hackage")
			return
		}
		if containsAny(low, "install") && !containsAny(low, "--local-bin-path") || containsAny(low, "setup", "update", "upgrade") {
			s.Class = ClassMutateOut
			s.Reason = verb + " install writes ~/.local/bin or the global package db"
		}
		if containsAny(low, "--version", "list", "ls", "path", "info", "query", "haddock", "check", "freeze", "gen-bounds", "outdated") && !containsAny(low, "install") {
			s.Class = ClassRead
			if containsAny(low, "freeze", "haddock", "gen-bounds") {
				s.Class = ClassMutateIn
			}
		}
	case "zig":
		if containsAny(low, "fetch") && containsAny(low, "--save", "--save-exact") || containsAny(low, "build") && containsAny(low, "--prefix", "-p") && false {
			return
		}
		if containsAny(low, "version", "env", "zen", "targets", "libc", "fmt") && containsAny(low, "--check") || containsAny(low, "version", "env", "zen", "targets", "libc") {
			s.Class = ClassRead
		}
	}
}

func isShellVerb(v string) bool {
	switch v {
	case "sh", "bash", "zsh", "dash", "ksh", "mksh", "fish":
		return true
	}
	return false
}

// globMatch is path.Match tolerant of `**`.
func globMatch(pattern, s string) (bool, error) {
	if pattern == "*" || pattern == "**" {
		return true, nil
	}
	if strings.Contains(pattern, "**") {
		pattern = strings.ReplaceAll(pattern, "**", "*")
	}
	return path.Match(pattern, s)
}

// ensure fmt is linked for reason formatting helpers
var _ = fmt.Sprint
