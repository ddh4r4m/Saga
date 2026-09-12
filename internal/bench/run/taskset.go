package run

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
)

// FrozenName is the task-set freeze artefact, kept beside the tasks
// (bench-spec section 2). It is the record of which corpus a
// pre-registered run was declared against: `saga bench run` refuses when
// a task's content hash differs from the frozen one, so a corpus edit
// during a pilot has to be a deliberate act (docs/12 row 15).
const FrozenName = "TASKSET.sha256"

// TasksetLines renders one `<id> <content-hash>` line per task plus the
// final `set <sha256>` line. The set hash is computed exactly the way
// the manifest's task_set.sha256 is, so the frozen file and a manifest
// from the same corpus agree by construction rather than by convention.
func TasksetLines(tasks []*task.Task) ([]string, string, error) {
	rows := make([]TaskHash, 0, len(tasks))
	for _, t := range tasks {
		h, err := t.ContentHash()
		if err != nil {
			return nil, "", cli.Wrap(cli.ExitEnvironment, "task hash "+t.ID, err)
		}
		rows = append(rows, TaskHash{ID: t.ID, SHA256: h})
	}
	// Sorted, because a glob's order is the filesystem's and the frozen
	// file has to be stable across machines. The manifest keeps the
	// order the run used; the set hash is over this canonical order.
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	lines := make([]string, 0, len(rows)+1)
	h := sha256.New()
	for _, r := range rows {
		fmt.Fprintf(h, "%s %s\n", r.ID, r.SHA256)
		lines = append(lines, r.ID+" "+r.SHA256)
	}
	set := "sha256:" + hex.EncodeToString(h.Sum(nil))
	return append(lines, "set "+set), set, nil
}

// TasksetText is TasksetLines as a file.
func TasksetText(tasks []*task.Task) (string, string, error) {
	lines, set, err := TasksetLines(tasks)
	if err != nil {
		return "", "", err
	}
	return strings.Join(lines, "\n") + "\n", set, nil
}

// Frozen is a parsed TASKSET.sha256.
type Frozen struct {
	Path string
	// Tasks maps task id to the frozen content hash.
	Tasks map[string]string
	// Set is the frozen set hash over every task in the file.
	Set string
}

// FindFrozen looks for TASKSET.sha256 beside the tasks: in the parent of
// each task directory, which is bench/tasks/ for the corpus. It returns
// nil and no error when there is none, because a task set that has not
// been frozen yet is not an error.
func FindFrozen(tasks []*task.Task) (*Frozen, error) {
	seen := map[string]bool{}
	for _, t := range tasks {
		dir := filepath.Dir(t.Dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		p := filepath.Join(dir, FrozenName)
		if _, err := os.Stat(p); err == nil {
			return ReadFrozen(p)
		}
	}
	return nil, nil
}

// ReadFrozen parses a freeze artefact.
func ReadFrozen(path string) (*Frozen, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, cli.Wrap(cli.ExitUsage, FrozenName, err)
	}
	f := &Frozen{Path: path, Tasks: map[string]string{}}
	for n, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, hash, ok := strings.Cut(line, " ")
		if !ok {
			return nil, cli.Errorf(cli.ExitUsage, "%s line %d: %q is not `<id> <hash>`", path, n+1, line)
		}
		if id == "set" {
			f.Set = hash
			continue
		}
		f.Tasks[id] = hash
	}
	if len(f.Tasks) == 0 {
		return nil, cli.Errorf(cli.ExitUsage, "%s names no task", path)
	}
	return f, nil
}

// Check compares the tasks a run is about to use with the frozen set. It
// reports every difference at once, because fixing them one run at a
// time is how a corpus drifts. A task the frozen file does not name is a
// difference too: the freeze is the whole set, not a floor.
func (f *Frozen) Check(tasks []*task.Task) error {
	if f == nil {
		return nil
	}
	var problems []string
	for _, t := range tasks {
		want, ok := f.Tasks[t.ID]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is not in the frozen set", t.ID))
			continue
		}
		got, err := t.ContentHash()
		if err != nil {
			return cli.Wrap(cli.ExitEnvironment, "task hash "+t.ID, err)
		}
		if got != want {
			problems = append(problems, fmt.Sprintf("%s changed: frozen %s, now %s", t.ID, short(want), short(got)))
		}
	}
	// A run over a subset is legitimate (a smoke takes three tasks), so a
	// frozen task the run does not use is not a problem. A changed or
	// unknown task is.
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &cli.Error{Code: cli.ExitIntegrity, Msg: fmt.Sprintf(
		"run: the task set does not match %s:\n  %s\nrewrite it with `saga bench taskset <glob> --write %s` if the change is deliberate, or pass --unfrozen to record an unfrozen run",
		f.Path, strings.Join(problems, "\n  "), f.Path)}
}

func short(hash string) string {
	h := strings.TrimPrefix(hash, "sha256:")
	if len(h) > 12 {
		h = h[:12]
	}
	return "sha256:" + h
}

// PreregName is the pre-registration copied into the archive root by
// `saga bench run --prereg` (docs/12 row 15). The file is copied
// verbatim: the hash in the manifest is over the bytes an archive can be
// read against later, not over a path that may have moved since.
const PreregName = "preregistration.md"

// archiveSums are the archive-root files SHA256SUMS covers, in a fixed
// order. A file that a run did not produce is skipped rather than
// listed as absent, because the set differs by invocation
// (--no-report writes no report, a run without --prereg no
// pre-registration).
var archiveSums = []string{
	"manifest.json", PreregName, "claims.txt", "abstain.txt",
	"rows.jsonl", "exclusions.jsonl", "status.json", "report.json", "report.md",
}

// WriteArchiveSums writes the archive-root SHA256SUMS. Per-run
// directories carry their own; this one covers what sits above them, so
// a published archive's pre-registration and rows are as checkable as
// its artifacts (bench-spec 3.4).
func WriteArchiveSums(dir string) error {
	var b strings.Builder
	for _, name := range archiveSums {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(raw)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	if b.Len() == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(b.String()), 0o644)
}

// CorpusKey is the one place a corpus approval store's name is decided
// (ADR 0010 decision 2): the `set` line of the freeze artefact beside
// the tasks, which is the hash over the whole frozen corpus and not
// over the tasks one invocation happens to select.
//
// Both producers call it. They did not, and that was the defect of the
// 2026-09-13 dev run: `approve-corpus` keyed the store on the freeze
// file's set line (40 tasks, fc22a4d4...) while `Run` keyed it on the
// manifest's hash over the 20 tasks the batch selected (ec52d09a...),
// so the owner's 202 records were written into one directory and every
// arm B run read an empty other one and ended infra with
// `not pre-approved`. A subset run is legitimate (Check says so); it is
// still the same frozen corpus, and the approvals are the corpus's.
func CorpusKey(tasks []*task.Task) (string, error) {
	frozen, err := FindFrozen(tasks)
	if err != nil {
		return "", err
	}
	if frozen == nil {
		return "", cli.Errorf(cli.ExitUsage, "no %s beside the tasks; freeze the set first with `saga bench taskset --write`", FrozenName)
	}
	if err := frozen.Check(tasks); err != nil {
		return "", err
	}
	if frozen.Set == "" {
		return "", cli.Errorf(cli.ExitUsage, "%s carries no `set` line; re-freeze with `saga bench taskset --write`", frozen.Path)
	}
	return frozen.Set, nil
}
