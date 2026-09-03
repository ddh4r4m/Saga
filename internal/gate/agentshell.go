package gate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/store"
)

// AgentShellMarkers are environment variables a harness sets in the
// agent's shell. They are the cheapest signal and the easiest to strip
// (`env -u CLAUDECODE ...`), so AgentShell also walks the parent process
// chain and HumanAct consults the hook's own record of a tool call in
// flight (contracts section 8: the refusal must not rest on one signal).
var AgentShellMarkers = []string{"SAGA_AGENT_SHELL", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CODEX_SANDBOX", "CODEX_CI", "GEMINI_CLI", "CURSOR_AGENT"}

// harnessProcessNames are executable basenames of the harnesses whose
// shells the human-only commands refuse to run under.
var harnessProcessNames = map[string]bool{"claude": true, "claude-code": true, "codex": true, "gemini": true, "cursor-agent": true, "opencode": true, "copilot": true}

// harnessPathHints are substrings of a parent's command line that name a
// harness even when the basename does not (Claude Code's versioned
// binary lives under `.../claude/versions/<v>`; npm installs run
// `node .../@anthropic-ai/claude-code/cli.js`).
var harnessPathHints = []string{"/claude/versions/", "@anthropic-ai/claude-code", "@openai/codex", "@google/gemini-cli", "/.codex/", "/.gemini/"}

// AgentShell returns a short description of why this process looks like
// an agent's shell (an environment marker or a harness in the parent
// process chain), or "" when it does not.
func AgentShell() string {
	for _, m := range AgentShellMarkers {
		if os.Getenv(m) != "" {
			return m + " is set"
		}
	}
	if p := ParentHarness(); p != "" {
		return "parent process " + p
	}
	return ""
}

// ParentHarness is the parent-chain detector. It is a variable so that a
// test suite running under a harness (which is exactly what the detector
// is for) can stand it down; nothing outside Go code can.
var ParentHarness = parentHarness

// parentHarness walks the parent chain and returns the first harness
// executable found, or "". It is best effort: a process that detaches
// from its parent (double fork) hides from it, which is why the hook's
// in-flight record is consulted as well.
func parentHarness() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	pid := os.Getppid()
	for depth := 0; depth < 32 && pid > 1; depth++ {
		out, err := exec.Command("ps", "-o", "ppid=,command=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return ""
		}
		f := strings.Fields(string(out))
		if len(f) < 2 {
			return ""
		}
		ppid, err := strconv.Atoi(f[0])
		if err != nil {
			return ""
		}
		cmd := strings.Join(f[1:], " ")
		if name := harnessOf(cmd); name != "" {
			return name
		}
		pid = ppid
	}
	return ""
}

// harnessOf classifies one process command line.
func harnessOf(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	base := filepath.Base(f[0])
	if harnessProcessNames[base] {
		return base
	}
	for _, seg := range strings.Split(filepath.ToSlash(f[0]), "/") {
		if harnessProcessNames[seg] {
			return seg
		}
	}
	for _, h := range harnessPathHints {
		if strings.Contains(cmd, h) {
			return strings.Trim(h, "/@")
		}
	}
	return ""
}

// InFlightMaxAge bounds how long a hook-recorded tool call counts as in
// flight without a further hook event; a harness that died mid-tool
// stops blocking the human after it.
const InFlightMaxAge = 10 * time.Minute

// HumanActError is the refusal for approve, attest, check --approve and
// reverify --ci: they are a human act (gate-spec sections 4.2, 6.4 and
// 8) and refuse to run from an agent's shell, detected by an environment
// marker, a harness in the parent process chain, or a tool call the hook
// recorded as in flight for a session of this repository.
func HumanActError(what string, s *store.Store) error {
	if m := AgentShell(); m != "" {
		return cli.Errorf(cli.ExitRefusal, "saga gate %s is a human act; refused inside an agent shell (%s)", what, m)
	}
	if s != nil {
		if session, tool, ok := ToolInFlight(s, InFlightMaxAge); ok {
			return cli.Errorf(cli.ExitRefusal, "saga gate %s is a human act; refused while the hook reports a %s call in flight for session %s (retry when the agent is idle)", what, tool, session)
		}
	}
	return nil
}
