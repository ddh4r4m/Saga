package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitIdempotent(t *testing.T) {
	root := t.TempDir()
	created, err := Init(root)
	if err != nil || !created {
		t.Fatalf("first init: created=%v err=%v", created, err)
	}
	s := Open(root)
	if !s.Exists() {
		t.Fatal("store does not exist after init")
	}
	gi, err := os.ReadFile(s.Path(".gitignore"))
	if err != nil || string(gi) != GitignoreBody {
		t.Fatalf("gitignore: %q %v", gi, err)
	}
	// Mutate config, then re-init: nothing is overwritten and nothing is created.
	if err := os.WriteFile(s.Path("config.toml"), []byte("[hook]\ndeadline_ms = 1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err = Init(root)
	if err != nil || created {
		t.Fatalf("second init: created=%v err=%v", created, err)
	}
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hook.DeadlineMS != 1234 {
		t.Errorf("deadline_ms = %d", cfg.Hook.DeadlineMS)
	}
	if cfg.Trace.Budget.SoftPct != 80 || cfg.Trace.Budget.SessionUSD != nil {
		t.Errorf("defaults not applied: %+v", cfg.Trace.Budget)
	}
	for _, d := range []string{"trace/sessions", "trace/pins", "trace/prices", "observed"} {
		if fi, err := os.Stat(s.Path(d)); err != nil || !fi.IsDir() {
			t.Errorf("missing dir %s", d)
		}
	}
}

func TestConfigSkeletonParses(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	cfg, err := Open(root).Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hook.DeadlineMS != 5000 || cfg.Trace.Budget.HardAction != "stop" || cfg.Trace.Budget.TurnContextTokens != 400000 {
		t.Errorf("skeleton values: %+v", cfg)
	}
}

func TestRepoRootAndFind(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(sub); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	s, err := Find(sub)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	got, _ := filepath.EvalSymlinks(s.Root)
	if got != want {
		t.Errorf("root %s want %s", got, want)
	}
}

func TestCheckShapeRefusesSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := CheckShape(link); err == nil {
		t.Error("symlink accepted")
	}
	if err := CheckShape(target); err != nil {
		t.Errorf("regular file refused: %v", err)
	}
	if err := CheckShape(filepath.Join(root, "missing")); err != nil {
		t.Errorf("missing path refused: %v", err)
	}
}

func TestObservedRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	s := Open(root)
	o, err := s.ReadObserved("s1")
	if err != nil || o.Turn != 0 {
		t.Fatalf("fresh: %+v %v", o, err)
	}
	o.Turn = 3
	o.Pending["tu1"] = 7
	if err := s.WriteObserved(o); err != nil {
		t.Fatal(err)
	}
	o2, err := s.ReadObserved("s1")
	if err != nil || o2.Turn != 3 || o2.Pending["tu1"] != 7 {
		t.Fatalf("reread: %+v %v", o2, err)
	}
}
