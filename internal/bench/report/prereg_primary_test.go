package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/run"
)

// TestPrimaryFromPreregistration (2026-09-06 smoke 3, finding 3): the
// fourth smoke archived and hashed its pre-registration and the report
// still called pass@1 the default primary. docs/12 section 2.1 carries a
// machine-readable PRIMARY line for the report to read.
func TestPrimaryFromPreregistration(t *testing.T) {
	dir := t.TempDir()
	hash := "sha256:1af94af4"
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, run.PreregName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A file that names one.
	write("# Protocol\n\nH1: the gate reduces false-done.\n\nPRIMARY: false_done\n\nMore prose.\n")
	metric, src := PrimaryFromPrereg(dir, &hash)
	if metric != "false_done" || src != run.PreregName+" "+hash {
		t.Errorf("metric %q source %q", metric, src)
	}
	// A file that names none: the default, and no source to cite.
	write("# Protocol\n\nNo machine-readable line here.\n")
	if metric, src = PrimaryFromPrereg(dir, &hash); metric != DefaultPrimary || src != "" {
		t.Errorf("no PRIMARY line: metric %q source %q", metric, src)
	}
	// No file at all: the same default, and the report says which.
	if metric, src = PrimaryFromPrereg(t.TempDir(), nil); metric != DefaultPrimary || src != "" {
		t.Errorf("no file: metric %q source %q", metric, src)
	}
	// Only a whole line counts, so prose that mentions PRIMARY does not.
	write("Discussion of PRIMARY: pass_at_1 in a sentence.\n")
	if metric, _ = PrimaryFromPrereg(dir, &hash); metric != DefaultPrimary {
		t.Errorf("a mention inside a line was read as the declaration: %q", metric)
	}
}

// TestComparePrimaryIsTheRegisteredMetric drives the whole path: two
// registered arms whose pre-registration names false_done, and the
// report's primary line must name it with its provenance.
func TestComparePrimaryIsTheRegisteredMetric(t *testing.T) {
	a := fixture("A", map[string][]bool{"x": {true, false}, "y": {true, true}})
	b := fixture("B", map[string][]bool{"x": {true, true}, "y": {true, true}})
	dirA, dirB := t.TempDir(), t.TempDir()
	body := "# Protocol\n\nPRIMARY: false_done\n"
	for _, d := range []string{dirA, dirB} {
		if err := os.WriteFile(filepath.Join(d, run.PreregName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hash := "sha256:1af94af4"
	ma, mb := manifest(2, "A"), manifest(2, "B")
	ma.PreregistrationSHA256, mb.PreregistrationSHA256 = &hash, &hash

	rep, err := Compare(
		&Archive{Dir: dirA, Manifest: ma, Hash: "sha256:" + strings.Repeat("1", 64), Rows: a},
		&Archive{Dir: dirB, Manifest: mb, Hash: "sha256:" + strings.Repeat("2", 64), Rows: b}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("report schema: %v", err)
	}
	if rep.Primary == nil || rep.Primary.Metric != "false_done" {
		t.Fatalf("primary %+v, want false_done", rep.Primary)
	}
	md := rep.Markdown()
	want := "Metric `false_done` (primary from preregistration.md " + hash + ")"
	if !strings.Contains(md, want) {
		t.Errorf("the primary line does not cite the pre-registration:\n%s", primaryLine(md))
	}
	if strings.Contains(md, "default primary") {
		t.Errorf("a registered report still called its primary the default:\n%s", primaryLine(md))
	}

	// Without the file: today's default and today's wording.
	rep, err = Compare(
		&Archive{Dir: t.TempDir(), Manifest: manifest(2, "A"), Hash: "sha256:" + strings.Repeat("1", 64), Rows: a},
		&Archive{Dir: t.TempDir(), Manifest: manifest(2, "B"), Hash: "sha256:" + strings.Repeat("2", 64), Rows: b}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Primary == nil || rep.Primary.Metric != DefaultPrimary {
		t.Fatalf("unregistered primary %+v", rep.Primary)
	}
	if !strings.Contains(rep.Markdown(), "default primary; no pre-registration file") {
		t.Errorf("unregistered wording changed:\n%s", primaryLine(rep.Markdown()))
	}

	// Registered, but the file names no PRIMARY line: the default, said
	// differently, so the reader knows a file was there and was silent.
	silent := t.TempDir()
	if err := os.WriteFile(filepath.Join(silent, run.PreregName), []byte("# Protocol\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err = Compare(
		&Archive{Dir: silent, Manifest: ma, Hash: "sha256:" + strings.Repeat("1", 64), Rows: a},
		&Archive{Dir: silent, Manifest: mb, Hash: "sha256:" + strings.Repeat("2", 64), Rows: b}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rep.Markdown(), "preregistration.md names no PRIMARY line") {
		t.Errorf("a silent pre-registration is not distinguished from an absent one:\n%s", primaryLine(rep.Markdown()))
	}
}

// TestPrimaryNamesAnUncomputableMetric: the report must not silently
// substitute a metric nobody declared.
func TestPrimaryNamesAnUncomputableMetric(t *testing.T) {
	a := fixture("A", map[string][]bool{"x": {true, false}})
	b := fixture("B", map[string][]bool{"x": {true, true}})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, run.PreregName), []byte("PRIMARY: happiness\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:deadbeef"
	ma, mb := manifest(1, "A"), manifest(1, "B")
	ma.PreregistrationSHA256, mb.PreregistrationSHA256 = &hash, &hash
	rep, err := Compare(
		&Archive{Dir: dir, Manifest: ma, Hash: "sha256:" + strings.Repeat("1", 64), Rows: a},
		&Archive{Dir: dir, Manifest: mb, Hash: "sha256:" + strings.Repeat("2", 64), Rows: b}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Primary == nil || rep.Primary.Metric != DefaultPrimary {
		t.Fatalf("primary %+v, want the default fallback", rep.Primary)
	}
	md := rep.Markdown()
	if !strings.Contains(md, `names PRIMARY "happiness"`) || !strings.Contains(md, "**warning**") {
		t.Errorf("the substitution is not stated:\n%s", md[:800])
	}
	if err := rep.Validate(); err != nil {
		t.Errorf("report schema: %v", err)
	}
}

// primaryLine pulls the report's primary sentence out for an error.
func primaryLine(md string) string {
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "Metric `") {
			return l
		}
	}
	return "(no primary line)"
}

// TestPrimaryOnTheFourthSmoke is brief 13's own check, over the archived
// rows of the fourth smoke. The archived preregistration.md predates the
// PRIMARY line (it was copied from docs/12 at be95ecc), and it is
// evidence: its hash is in both manifests, so it is not edited. The
// rows are read as archived and paired against the current docs/12,
// which is what a run made today would freeze.
func TestPrimaryOnTheFourthSmoke(t *testing.T) {
	root := filepath.Join("..", "..", "..", "bench", "results", "smoke-2026-09-06-3")
	prereg, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "12-experiment-protocol.md"))
	if err != nil {
		t.Skipf("docs/12 not present: %v", err)
	}
	if !strings.Contains(string(prereg), "\nPRIMARY: false_done\n") {
		t.Fatal("docs/12 carries no PRIMARY line; the report has nothing to read")
	}
	load := func(arm string) *Archive {
		t.Helper()
		dir := filepath.Join(root, arm)
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
			t.Skipf("archive not present: %v", err)
		}
		arch, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		// A scratch copy of the pre-registration, so the archived
		// evidence is read and never written.
		scratch := t.TempDir()
		if err := os.WriteFile(filepath.Join(scratch, run.PreregName), prereg, 0o644); err != nil {
			t.Fatal(err)
		}
		arch.Dir = scratch
		return arch
	}
	rep, err := Compare(load("A"), load("B"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Primary == nil || rep.Primary.Metric != "false_done" {
		t.Fatalf("primary %+v, want false_done", rep.Primary)
	}
	if rep.Primary.A == nil || *rep.Primary.A != 0 || rep.Primary.B == nil || *rep.Primary.B != 0 {
		t.Errorf("false_done A=%v B=%v, want 0.000 and 0.000", rep.Primary.A, rep.Primary.B)
	}
	t.Logf("fourth smoke primary: %s", primaryLine(rep.Markdown()))
}
