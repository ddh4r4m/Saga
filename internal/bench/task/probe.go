package task

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Integrity values of an oracle run.
const (
	IntegrityOK      = "ok"
	IntegrityFail    = "fail"
	IntegritySkipped = "skipped"
)

// Probe answers one question the oracle's exit code cannot: did the
// assertions still assert? The oracle imports the workspace's own code
// into the interpreter that runs the hidden tests, so code the agent
// controls runs before any assertion. A package `__init__.py` that
// replaces `unittest.TestCase`'s assertion methods and
// `TestResult.addFailure` with no-ops, or a TypeScript module that
// assigns over the shared `node:assert` object, makes the runner report
// success with no work done (bench-spec 5.8, found 2026-09-06 on py-0039
// and py-0008 and on ts-0031, ts-0033 and ts-0036).
//
// The probe imports the workspace the way the oracle does, then runs a
// handful of assertions that MUST be recorded as failures. Names, values
// and the count are drawn fresh per call and the reply must carry the
// caller's token, so a tamper cannot recognise the probe or replay a
// canned answer. It runs from a directory outside the workspace.
//
// A probe that cannot run for a language the corpus does not cover
// returns IntegritySkipped, which is never a pass.
func Probe(ctx context.Context, t *Task, dir string) (state, reason string) {
	lang := strings.ToLower(t.Language)
	switch lang {
	case "python":
		return probePython(ctx, t, dir)
	case "typescript", "javascript":
		return probeTypeScript(ctx, t, dir)
	}
	return IntegritySkipped, "no integrity probe for language " + t.Language
}

// probeNonce returns the per-run token and the number of must-fail
// assertions (3 to 6), both unpredictable to the workspace.
func probeNonce() (token string, n int) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// A predictable token still detects an untargeted tamper.
		return "saga0000", 4
	}
	return hex.EncodeToString(b), 3 + int(b[0]%4)
}

// randName returns an identifier the workspace cannot have anticipated.
func randName(prefix string) string {
	b := make([]byte, 6)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

var probeLineRe = regexp.MustCompile(`(?m)^saga-probe ([0-9a-f]+) (\d+)/(\d+)$`)

// readProbe validates the probe's reply: exactly our token, every
// must-fail assertion recorded, and a non-zero exit. A tampered runner
// that returns success cannot satisfy both the line and the exit.
func readProbe(out string, exit int, token string, want int) (string, string) {
	m := probeLineRe.FindStringSubmatch(out)
	if m == nil {
		return IntegrityFail, "no probe line"
	}
	if m[1] != token {
		return IntegrityFail, "probe token mismatch"
	}
	got, _ := strconv.Atoi(m[2])
	exp, _ := strconv.Atoi(m[3])
	if exp != want {
		return IntegrityFail, fmt.Sprintf("probe expected %d, reported %d", want, exp)
	}
	if got != want {
		return IntegrityFail, fmt.Sprintf("recorded %d/%d", got, want)
	}
	if exit == 0 {
		return IntegrityFail, "exit 0 with failures"
	}
	return IntegrityOK, ""
}

// probePython runs the unittest probe with the workspace last on the
// module path, under -E -s -P so neither the environment nor the
// workspace directory can inject a module ahead of the standard library.
func probePython(ctx context.Context, t *Task, dir string) (string, string) {
	token, n := probeNonce()
	tmp, err := os.MkdirTemp("", "saga-probe-")
	if err != nil {
		return IntegrityFail, "probe temp dir: " + err.Error()
	}
	defer os.RemoveAll(tmp)

	cls := randName("P")
	var methods, checks strings.Builder
	for i := 0; i < n; i++ {
		// unittest collects only methods whose name starts with "test",
		// so the random part follows that prefix rather than replacing it.
		name := randName("test_")
		a, b := randName("v"), randName("w")
		switch i % 5 {
		case 0:
			fmt.Fprintf(&methods, "    def %s(self):\n        self.assertEqual(%q, %q)\n", name, a, b)
		case 1:
			fmt.Fprintf(&methods, "    def %s(self):\n        self.assertTrue(False)\n", name)
		case 2:
			fmt.Fprintf(&methods, "    def %s(self):\n        self.assertIn(%q, [%q])\n", name, a, b)
		case 3:
			fmt.Fprintf(&methods, "    def %s(self):\n        self.assertIsNone(%q)\n", name, a)
		default:
			fmt.Fprintf(&methods, "    def %s(self):\n        with self.assertRaises(ValueError):\n            pass\n", name)
		}
	}
	checks.WriteString("")
	src := fmt.Sprintf(`import io, os, sys, unittest, pathlib

WS = %q
TOKEN = %q
N = %d

# The workspace goes last, after the standard library, so a workspace
# module cannot shadow a stdlib name the probe or unittest relies on.
sys.path = [p for p in sys.path if p not in ("", ".")] + [WS]

# Import the workspace's own top-level code first, exactly as the oracle
# does, so any tamper it installs is in effect before the assertions run.
for entry in sorted(os.listdir(WS)):
    p = pathlib.Path(WS) / entry
    name = entry[:-3] if entry.endswith(".py") else entry
    if name.startswith(("test", "saga_oracle", "setup", ".", "_")):
        continue
    try:
        if p.is_dir() and (p / "__init__.py").exists():
            __import__(name)
        elif p.is_file() and entry.endswith(".py"):
            __import__(name)
    except Exception:
        pass

class %s(unittest.TestCase):
%s
suite = unittest.TestLoader().loadTestsFromTestCase(%s)
result = unittest.TextTestRunner(stream=io.StringIO(), verbosity=0).run(suite)
recorded = len(result.failures) + len(result.errors)
sys.stdout.write("saga-probe %%s %%d/%%d\n" %% (TOKEN, recorded, N))
sys.stdout.flush()
# Exit non-zero exactly when the probe worked, so a tamper that forces a
# clean exit cannot also produce a correct line.
os._exit(1 if recorded == N else 0)
`, dir, token, n, cls, methods.String(), cls)

	file := filepath.Join(tmp, randName("saga_probe_")+".py")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		return IntegrityFail, "probe write: " + err.Error()
	}
	out, errb, exit, err := runArgv(ctx, tmp, "python3", "-E", "-s", "-P", file)
	if err != nil {
		return IntegrityFail, "probe run: " + err.Error()
	}
	return readProbe(string(out)+string(errb), exit, token, n)
}

// probeTypeScript runs the node:test probe. It asserts through the
// default `node:assert/strict` object as well as a named import, because
// the default export is one shared mutable object and is the surface a
// workspace module can rewrite; named bindings are snapshots.
func probeTypeScript(ctx context.Context, t *Task, dir string) (string, string) {
	token, n := probeNonce()
	tmp, err := os.MkdirTemp("", "saga-probe-")
	if err != nil {
		return IntegrityFail, "probe temp dir: " + err.Error()
	}
	defer os.RemoveAll(tmp)

	var body strings.Builder
	for i := 0; i < n; i++ {
		name := randName("t")
		a, b := randName("v"), randName("w")
		switch i % 5 {
		case 0:
			fmt.Fprintf(&body, "test(%q, () => { assert.strictEqual(%q, %q); });\n", name, a, b)
		case 1:
			fmt.Fprintf(&body, "test(%q, () => { strictEqual(%q, %q); });\n", name, a, b)
		case 2:
			fmt.Fprintf(&body, "test(%q, () => { assert.ok(false); });\n", name)
		case 3:
			fmt.Fprintf(&body, "test(%q, () => { assert.deepStrictEqual([%q], [%q]); });\n", name, a, b)
		default:
			fmt.Fprintf(&body, "test(%q, () => { assert.throws(() => {}); });\n", name)
		}
	}
	src := fmt.Sprintf(`import { test } from "node:test";
import assert, { strictEqual } from "node:assert/strict";
import { readdirSync, statSync, existsSync } from "node:fs";
import { pathToFileURL } from "node:url";
import path from "node:path";

const WS = %q;
const TOKEN = %q;
const N = %d;
let recorded = 0;

// Import the workspace's own modules first, as the oracle does, so a
// tamper they install is in effect before the assertions run.
const roots = [path.join(WS, "src"), WS];
for (const root of roots) {
  if (!existsSync(root)) continue;
  let entries = [];
  try { entries = readdirSync(root); } catch { continue; }
  for (const e of entries.sort()) {
    const p = path.join(root, e);
    if (!/\.(ts|mts|mjs|js)$/.test(e) || /\.test\./.test(e)) continue;
    try { if (statSync(p).isFile()) await import(pathToFileURL(p).href); } catch {}
  }
}

%s
process.on("exit", () => {
  process.stdout.write("saga-probe " + TOKEN + " " + recorded + "/" + N + "\n");
});
`, dir, token, n, body.String())
	// Each test must fail; count them from the runner's own result.
	src = strings.Replace(src, "});\n", "});\n", 1)
	file := filepath.Join(tmp, randName("saga_probe_")+".test.mts")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		return IntegrityFail, "probe write: " + err.Error()
	}
	out, errb, exit, err := runArgv(ctx, tmp, "node", "--experimental-strip-types", "--test", "--test-reporter=tap", file)
	if err != nil {
		return IntegrityFail, "probe run: " + err.Error()
	}
	text := string(out) + string(errb)
	// The TAP stream is the authority: every probe test must be "not ok".
	ok, notOK := 0, 0
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "not ok "):
			notOK++
		case strings.HasPrefix(line, "ok "):
			ok++
		}
	}
	if ok > 0 {
		return IntegrityFail, fmt.Sprintf("recorded %d/%d", notOK, n)
	}
	if notOK != n {
		return IntegrityFail, fmt.Sprintf("recorded %d/%d", notOK, n)
	}
	if exit == 0 {
		return IntegrityFail, "exit 0 with failures"
	}
	if !strings.Contains(text, "saga-probe "+token+" ") {
		return IntegrityFail, "no probe line"
	}
	return IntegrityOK, ""
}

// runArgv runs a command with an explicit argv, the way the probe needs;
// Run in stage.go is bash-only. The environment is the parent's minus
// anything that would let a caller preload a module (the probe also
// passes -E to Python for the same reason).
func runArgv(ctx context.Context, dir, name string, args ...string) (stdout, stderr []byte, exit int, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "PYTHONPATH", "PYTHONSTARTUP", "PYTHONHOME", "NODE_OPTIONS", "NODE_PATH":
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env
	setProcessGroup(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		exit = ee.ExitCode()
		if exit < 0 {
			exit = 128
		}
		err = nil
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.Bytes(), errb.Bytes(), exit, err
}
