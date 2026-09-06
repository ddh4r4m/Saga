package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/run"
	"github.com/ddh4r4m/saga/internal/cli"
)

func needCorpusTools(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short")
	}
	for _, bin := range []string{"git", "bash", "node"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

func readManifest(t *testing.T, dir string) run.Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m run.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestPreregFlagRoundTrip (docs/12 row 15): --prereg copies the file
// verbatim into the archive, records its hash in the manifest, names it
// in the archive's SHA256SUMS, and the report header prints it. Without
// the flag the manifest stays null and the header says so, which is what
// keeps an unregistered run from reading as a registered one.
func TestPreregFlagRoundTrip(t *testing.T) {
	needCorpusTools(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	tasks := filepath.Join(repo, "bench", "tasks", "ts-0001-*")
	root := t.TempDir()

	prereg := filepath.Join(root, "prereg.md")
	body := "# Pre-registration\n\nPrimary: false-done rate.\n"
	if err := os.WriteFile(prereg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "with")
	if o, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--k", "1",
		"--out", out, "--arm", "A", "--tier", "smoke", "--no-verify", "--prereg", prereg); code != cli.ExitOK {
		t.Fatalf("run: %v\n%s\n%s", code, o, errs)
	}
	// Copied verbatim.
	got, err := os.ReadFile(filepath.Join(out, run.PreregName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("preregistration.md is not the file that was given:\n%q", got)
	}
	m := readManifest(t, out)
	if m.PreregistrationSHA256 == nil {
		t.Fatal("manifest carries no preregistration_sha256")
	}
	hash := *m.PreregistrationSHA256
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash %q", hash)
	}
	// The archive-root SHA256SUMS covers it, and the report header names it.
	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{run.PreregName, "manifest.json", "rows.jsonl", "report.json", "report.md"} {
		if !strings.Contains(string(sums), want) {
			t.Errorf("SHA256SUMS does not cover %s:\n%s", want, sums)
		}
	}
	md, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "pre-registration: "+hash) {
		t.Errorf("the header does not print the hash:\n%s", firstLines(string(md), 12))
	}

	// Without the flag: null, and the header says so.
	bare := filepath.Join(root, "without")
	if o, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--k", "1",
		"--out", bare, "--arm", "B", "--tier", "smoke", "--no-verify"); code != cli.ExitOK {
		t.Fatalf("run: %v\n%s\n%s", code, o, errs)
	}
	if m := readManifest(t, bare); m.PreregistrationSHA256 != nil {
		t.Errorf("an unregistered run claims a pre-registration: %v", *m.PreregistrationSHA256)
	}
	if _, err := os.Stat(filepath.Join(bare, run.PreregName)); err == nil {
		t.Error("an unregistered run wrote a preregistration.md")
	}

	// Pairing a registered arm with an unregistered one is refused: the
	// pre-registration is what makes the primary a test and not a search.
	_, errs, code := runIn(t, repo, "", "bench", "compare", out, bare)
	if code != cli.ExitUsage {
		t.Errorf("compare across the registration boundary exited %v, want %v", code, cli.ExitUsage)
	}
	if !strings.Contains(errs, "pre-registered") {
		t.Errorf("refusal does not say why: %s", errs)
	}
}

// TestPreregMismatchWarns: two arms may cite different pre-registrations,
// but the report says so, because the primary each declared may differ.
func TestPreregMismatchWarns(t *testing.T) {
	needCorpusTools(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	tasks := filepath.Join(repo, "bench", "tasks", "ts-0001-*")
	root := t.TempDir()
	var dirs []string
	for i, body := range []string{"# One\n", "# Two\n"} {
		p := filepath.Join(root, "prereg"+string(rune('a'+i))+".md")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		d := filepath.Join(root, "arm"+string(rune('a'+i)))
		arm := "A"
		if i == 1 {
			arm = "B"
		}
		if o, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--k", "1",
			"--out", d, "--arm", arm, "--tier", "smoke", "--no-verify", "--prereg", p); code != cli.ExitOK {
			t.Fatalf("run: %v\n%s\n%s", code, o, errs)
		}
		dirs = append(dirs, d)
	}
	md, errs, code := runIn(t, repo, "", "bench", "compare", dirs[0], dirs[1])
	if code != cli.ExitOK {
		t.Fatalf("compare: %v %s", code, errs)
	}
	if !strings.Contains(md, "**warning**") || !strings.Contains(md, "different pre-registrations") {
		t.Errorf("no warning about the mismatch:\n%s", firstLines(md, 14))
	}
}

// TestBadgeRefusedBelowPublish (docs/12 commitment 4): a number from a
// smoke, user or dev archive may appear in its own report and nowhere
// else.
func TestBadgeRefusedBelowPublish(t *testing.T) {
	needCorpusTools(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	tasks := filepath.Join(repo, "bench", "tasks", "ts-0001-*")
	root := t.TempDir()
	for _, tier := range []string{"smoke", "user", "dev"} {
		dir := filepath.Join(root, tier)
		if o, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", tasks, "--adapter", "replay", "--k", "1",
			"--out", dir, "--arm", "A", "--tier", tier, "--no-verify"); code != cli.ExitOK {
			t.Fatalf("run %s: %v\n%s\n%s", tier, code, o, errs)
		}
		_, errs, code := runIn(t, repo, "", "bench", "badge", dir, "--metric", "pass_at_1")
		if code != cli.ExitUsage {
			t.Errorf("badge at tier %s exited %v, want %v", tier, code, cli.ExitUsage)
		}
		if !strings.Contains(errs, "cannot be cited") {
			t.Errorf("badge at tier %s: %s", tier, errs)
		}
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestFrozenTaskSetRefusesAndUnfrozenRecords (docs/12 row 15): a run
// over a task that no longer matches TASKSET.sha256 is refused with exit
// 5, and --unfrozen runs it while recording task_set.frozen = false, so
// a report from it can never be read as pre-registered against the
// frozen set.
func TestFrozenTaskSetRefusesAndUnfrozenRecords(t *testing.T) {
	needCorpusTools(t)
	repo, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	root := t.TempDir()

	// A private copy of one task, with its own freeze artefact, so the
	// committed corpus is never touched.
	corpus := filepath.Join(root, "tasks")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(repo, "bench", "tasks", "ts-0001-slug-collapse")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("task not present: %v", err)
	}
	if out, err := exec.Command("cp", "-R", src, corpus+"/").CombinedOutput(); err != nil {
		t.Fatalf("copy: %v %s", err, out)
	}
	glob := filepath.Join(corpus, "*")
	frozenFile := filepath.Join(corpus, run.FrozenName)
	if o, errs, code := runIn(t, repo, "", "bench", "taskset", glob, "--write", frozenFile); code != cli.ExitOK {
		t.Fatalf("taskset: %v\n%s\n%s", code, o, errs)
	}

	// Frozen and unchanged: the run proceeds and records it.
	ok := filepath.Join(root, "ok")
	if o, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", glob, "--adapter", "replay", "--k", "1",
		"--out", ok, "--arm", "A", "--tier", "smoke", "--no-verify"); code != cli.ExitOK {
		t.Fatalf("frozen run: %v\n%s\n%s", code, o, errs)
	}
	m := readManifest(t, ok)
	if m.TaskSet.Frozen == nil || !*m.TaskSet.Frozen {
		t.Errorf("a matching run did not record task_set.frozen = true: %v", m.TaskSet.Frozen)
	}

	// Change the task: the run is refused with exit 5 and names it.
	prompt := filepath.Join(corpus, "ts-0001-slug-collapse", "prompt.md")
	body, err := os.ReadFile(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, append(body, []byte("\nAn edit made during a pilot.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", glob, "--adapter", "replay", "--k", "1",
		"--out", filepath.Join(root, "refused"), "--arm", "A", "--tier", "smoke", "--no-verify")
	if code != cli.ExitIntegrity {
		t.Fatalf("a changed task exited %v, want %v: %s", code, cli.ExitIntegrity, errs)
	}
	for _, want := range []string{"ts-0001-slug-collapse changed", "--unfrozen"} {
		if !strings.Contains(errs, want) {
			t.Errorf("refusal lacks %q: %s", want, errs)
		}
	}

	// --unfrozen runs it and says so in the manifest and the report.
	un := filepath.Join(root, "unfrozen")
	if o, errs, code := runIn(t, repo, "", "bench", "run", "--tasks", glob, "--adapter", "replay", "--k", "1",
		"--out", un, "--arm", "A", "--tier", "smoke", "--no-verify", "--unfrozen"); code != cli.ExitOK {
		t.Fatalf("unfrozen run: %v\n%s\n%s", code, o, errs)
	}
	m = readManifest(t, un)
	if m.TaskSet.Frozen == nil || *m.TaskSet.Frozen {
		t.Errorf("an unfrozen run did not record task_set.frozen = false: %v", m.TaskSet.Frozen)
	}
	md, err := os.ReadFile(filepath.Join(un, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "unfrozen") {
		t.Errorf("the report does not say the run was unfrozen:\n%s", firstLines(string(md), 14))
	}
}
