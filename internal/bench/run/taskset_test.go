package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/cli"
)

// corpus loads the frozen corpus, or skips.
func corpus(t *testing.T) []*task.Task {
	t.Helper()
	dirs, err := task.Find(filepath.Join("..", "..", "..", "bench", "tasks", "*"))
	if err != nil || len(dirs) == 0 {
		t.Skip("no corpus")
	}
	var out []*task.Task
	for _, d := range dirs {
		tk, err := task.Load(d)
		if err != nil {
			t.Fatalf("load %s: %v", d, err)
		}
		out = append(out, tk)
	}
	return out
}

// TestTasksetMatchesTheFrozenFile (docs/12 row 15): the committed
// TASKSET.sha256 is the current corpus. When this fails, a task changed
// and the freeze has to be rewritten deliberately, which is the point of
// having it.
func TestTasksetMatchesTheFrozenFile(t *testing.T) {
	tasks := corpus(t)
	path := filepath.Join("..", "..", "..", "bench", "tasks", FrozenName)
	frozen, err := ReadFrozen(path)
	if err != nil {
		t.Fatalf("%s: %v", FrozenName, err)
	}
	if err := frozen.Check(tasks); err != nil {
		t.Fatalf("the corpus no longer matches %s:\n%v", FrozenName, err)
	}
	if len(frozen.Tasks) != len(tasks) {
		t.Errorf("%s names %d tasks, the corpus has %d", FrozenName, len(frozen.Tasks), len(tasks))
	}
	// The set hash in the file is the one a manifest would carry.
	_, set, err := TasksetLines(tasks)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Set != set {
		t.Errorf("set hash %s, computed %s", frozen.Set, set)
	}
	t.Logf("%d tasks frozen at %s", len(tasks), set)
}

// TestFrozenCheckRefusesAChangedTask: the check names every difference
// at once and points at the two ways out. Fixing them one run at a time
// is how a corpus drifts under a pilot.
func TestFrozenCheckRefusesAChangedTask(t *testing.T) {
	tasks := corpus(t)[:2]
	lines, _, err := TasksetLines(tasks)
	if err != nil {
		t.Fatal(err)
	}
	// Freeze the first task at a hash it does not have, and drop the
	// second from the set entirely.
	dir := t.TempDir()
	path := filepath.Join(dir, FrozenName)
	bogus := tasks[0].ID + " sha256:" + strings.Repeat("0", 64) + "\n" + lines[len(lines)-1] + "\n"
	if err := os.WriteFile(path, []byte(bogus), 0o644); err != nil {
		t.Fatal(err)
	}
	frozen, err := ReadFrozen(path)
	if err != nil {
		t.Fatal(err)
	}
	err = frozen.Check(tasks)
	if err == nil {
		t.Fatal("a changed task was accepted")
	}
	if cli.CodeOf(err) != cli.ExitIntegrity {
		t.Errorf("exit %v, want %v", cli.CodeOf(err), cli.ExitIntegrity)
	}
	msg := err.Error()
	if !strings.Contains(msg, tasks[0].ID+" changed") {
		t.Errorf("the changed task is not named: %s", msg)
	}
	if !strings.Contains(msg, tasks[1].ID+" is not in the frozen set") {
		t.Errorf("the unknown task is not named: %s", msg)
	}
	for _, way := range []string{"saga bench taskset", "--unfrozen"} {
		if !strings.Contains(msg, way) {
			t.Errorf("the refusal does not mention %q: %s", way, msg)
		}
	}
	// A subset of a frozen set is legitimate: a smoke takes three tasks.
	if err := frozen.Check(tasks[:1]); err == nil {
		t.Error("the first task is still frozen at the wrong hash")
	}
	good, _, err := TasksetText(tasks)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	frozen, err = ReadFrozen(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := frozen.Check(tasks[:1]); err != nil {
		t.Errorf("a subset of the frozen set was refused: %v", err)
	}
}

// TestTasksetTextIsStable: the listing is sorted by id, so the frozen
// file does not depend on a glob's filesystem order.
func TestTasksetTextIsStable(t *testing.T) {
	tasks := corpus(t)
	forward, setA, err := TasksetText(tasks)
	if err != nil {
		t.Fatal(err)
	}
	reversed := make([]*task.Task, len(tasks))
	for i := range tasks {
		reversed[len(tasks)-1-i] = tasks[i]
	}
	backward, setB, err := TasksetText(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if forward != backward || setA != setB {
		t.Errorf("the listing depends on input order:\n%s\n---\n%s", forward, backward)
	}
	if !strings.HasSuffix(forward, "set "+setA+"\n") {
		t.Errorf("no trailing set line:\n%s", forward[len(forward)-120:])
	}
}
