package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tamperFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "bench", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// pyWorkspace is a minimal Python workspace: one package the probe will
// import, which is where a tamper would live.
func pyWorkspace(t *testing.T, extra string) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "widget"), 0o755); err != nil {
		t.Fatal(err)
	}
	init := "VALUE = 3\n" + extra
	write := func(p, s string) {
		if err := os.WriteFile(filepath.Join(d, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("widget/__init__.py", init)
	write("widget/core.py", "def double(n):\n    return n * 2\n")
	return d
}

func pyTask() *Task { return &Task{Language: "python"} }

// TestProbePythonTampers is the point of the probe: an oracle that shares
// its interpreter with the workspace cannot be trusted to have asserted.
func TestProbePythonTampers(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	ctx := context.Background()

	state, reason := Probe(ctx, pyTask(), pyWorkspace(t, ""))
	if state != IntegrityOK {
		t.Fatalf("clean workspace: %s %s", state, reason)
	}

	for _, c := range []struct{ name, extra string }{
		{"full fixture", tamperFixture(t, "tamper-py.txt")},
		{"assertions only", "import unittest as _u\n" +
			"def _n(self, *a, **k):\n    return None\n" +
			"for _m in ('assertEqual','assertTrue','assertIn','assertIsNone','assertRaises'):\n" +
			"    setattr(_u.TestCase, _m, _n)\n"},
		{"result recorder only", "import unittest as _u\n" +
			"def _n(self, *a, **k):\n    return None\n" +
			"_u.TestResult.addFailure = _n\n_u.TestResult.addError = _n\n"},
		{"TestCase.run override", "import unittest as _u\n" +
			"def _r(self, result=None):\n    return result\n" +
			"_u.TestCase.run = _r\n"},
	} {
		state, reason := Probe(ctx, pyTask(), pyWorkspace(t, c.extra))
		if state != IntegrityFail {
			t.Errorf("%s: integrity %s (%s), want fail", c.name, state, reason)
		}
	}
}

// TestProbePythonIgnoresStdlibShadow: a workspace module that shadows a
// standard library name must not reach the probe, because the workspace
// is last on sys.path behind the standard library.
func TestProbePythonIgnoresStdlibShadow(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	d := pyWorkspace(t, "")
	shadow := "import sys\nsys.modules['unittest'] = None\n"
	if err := os.WriteFile(filepath.Join(d, "datetime.py"), []byte(shadow), 0o644); err != nil {
		t.Fatal(err)
	}
	state, reason := Probe(context.Background(), pyTask(), d)
	if state != IntegrityOK {
		t.Errorf("stdlib shadow reached the probe: %s %s", state, reason)
	}
}

// TestProbeTypeScriptTampers: the mirror on the TypeScript side, where the
// mutable default node:assert object is the surface.
func TestProbeTypeScriptTampers(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	ctx := context.Background()
	tsWorkspace := func(extra string) string {
		d := t.TempDir()
		if err := os.MkdirAll(filepath.Join(d, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "export function double(n: number): number { return n * 2; }\n" + extra
		if err := os.WriteFile(filepath.Join(d, "src", "core.ts"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return d
	}
	task := &Task{Language: "typescript"}

	state, reason := Probe(ctx, task, tsWorkspace(""))
	if state != IntegrityOK {
		t.Fatalf("clean workspace: %s %s", state, reason)
	}

	for _, c := range []struct{ name, extra string }{
		{"full fixture", tamperFixture(t, "tamper-ts.txt")},
		{"defineProperty on assert", "import a from \"node:assert/strict\";\n" +
			"Object.defineProperty(a, \"strictEqual\", { value: () => {} });\n" +
			"Object.defineProperty(a, \"ok\", { value: () => {} });\n" +
			"Object.defineProperty(a, \"deepStrictEqual\", { value: () => {} });\n" +
			"Object.defineProperty(a, \"throws\", { value: () => {} });\n"},
	} {
		state, reason := Probe(ctx, task, tsWorkspace(c.extra))
		if state != IntegrityFail {
			t.Errorf("%s: integrity %s (%s), want fail", c.name, state, reason)
		}
	}
}

// TestProbeUnknownLanguage never reports ok, so a language the corpus
// does not cover cannot pass by default.
func TestProbeUnknownLanguage(t *testing.T) {
	state, reason := Probe(context.Background(), &Task{Language: "cobol"}, t.TempDir())
	if state != IntegritySkipped || !strings.Contains(reason, "cobol") {
		t.Errorf("unknown language: %s %s", state, reason)
	}
}

// TestReadProbeReply covers the reply validation without running a probe.
func TestReadProbeReply(t *testing.T) {
	for _, c := range []struct{ out, want string }{
		{"saga-probe abc 4/4\n", IntegrityOK},
		{"nothing here\n", IntegrityFail},
		{"saga-probe dead 4/4\n", IntegrityFail},
		{"saga-probe abc 0/4\n", IntegrityFail},
		{"saga-probe abc 4/9\n", IntegrityFail},
	} {
		got, _ := readProbe(c.out, 1, "abc", 4)
		if got != c.want {
			t.Errorf("%q: %s, want %s", c.out, got, c.want)
		}
	}
	// A correct line with a clean exit is a tampered runner, not a pass.
	if got, reason := readProbe("saga-probe abc 4/4\n", 0, "abc", 4); got != IntegrityFail || reason != "exit 0 with failures" {
		t.Errorf("clean exit: %s %s", got, reason)
	}
}

// TestOracleRejectsTamperedPass is the end-to-end claim of this change:
// the forged solve reported on 2026-09-06 no longer grades as a pass.
// py-0008 with the tamper appended to its package init exits 0, and the
// grader must still refuse it.
func TestOracleRejectsTamperedPass(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	needRunners(t)
	tk, err := Load(filepath.Join("..", "..", "..", "bench", "tasks", "py-0008-money-exact-cents"))
	if err != nil {
		t.Skipf("corpus task: %v", err)
	}
	ctx := context.Background()

	stage := func(extra string) string {
		d := filepath.Join(t.TempDir(), "ws")
		if err := Stage(ctx, tk, d); err != nil {
			t.Fatal(err)
		}
		if extra != "" {
			p := filepath.Join(d, "pricing", "__init__.py")
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(raw, []byte("\n"+extra)...), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := Setup(ctx, tk, d); err != nil {
			t.Fatal(err)
		}
		return d
	}

	// Baseline: the oracle is red on its own, and integrity is intact.
	base, err := Oracle(ctx, tk, stage(""))
	if err != nil {
		t.Fatal(err)
	}
	if base.Integrity != IntegrityOK || base.Pass {
		t.Errorf("baseline: integrity %s pass %v", base.Integrity, base.Pass)
	}

	// Tampered: the oracle's own exit code says success, and the grader
	// refuses it anyway.
	got, err := Oracle(ctx, tk, stage(tamperFixture(t, "tamper-py.txt")))
	if err != nil {
		t.Fatal(err)
	}
	if got.Exit != 0 {
		t.Fatalf("the tamper is meant to force exit 0, got %d", got.Exit)
	}
	if got.Pass {
		t.Error("a tampered run still graded as a pass")
	}
	if got.Integrity != IntegrityFail {
		t.Errorf("integrity %s, want fail", got.Integrity)
	}
	if !strings.Contains(string(got.Text()), "--- integrity fail") {
		t.Errorf("oracle.txt does not record the verdict:\n%s", got.Text())
	}
}
