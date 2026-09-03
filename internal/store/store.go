// Package store owns the .saga/ directory layout of
// docs/specs/00-cross-spec-contracts.md section 2: discovery of the
// repository root, `saga init`, the config skeleton, the observed
// per-session counters and the hostile-shape check every path goes
// through before it is opened.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Dir is the name of the per-repository Saga directory.
const Dir = ".saga"

// GitignoreBody is written to .saga/.gitignore by `saga init`; the
// repository's own .gitignore is never edited (contracts section 2).
const GitignoreBody = "trace/\nindex/\nshape/\nsnap/\nobserved/\naudit.jsonl\n"

// ConfigSkeleton is the .saga/config.toml written by `saga init`. One
// table per layer; absent keys take the defaults in Config.
const ConfigSkeleton = `# Saga configuration. One table per layer (docs/specs/00-cross-spec-contracts.md section 2).
# Absent keys take the documented defaults.

[hook]
# The composed entry runs its own deadline because a timed-out PreToolUse
# allows on Claude Code (contracts section 1.1). Milliseconds.
deadline_ms = 5000

[trace.budget]
# session_usd = 25.00
# task_usd = null
turn_context_tokens = 400000
soft_pct = 80
compact_pct = 95
hard_pct = 100
hard_action = "stop"

[trace.watchdog]

[trace.claims]

[gate]

[index]

[shape]

[mem]
`

// ManifestSchema is the schema id of .saga/manifest.json.
const ManifestSchema = "saga.manifest/1"

// Store is a located .saga directory.
type Store struct {
	// Root is the repository root that contains .saga.
	Root string
}

// Config is the parsed .saga/config.toml. Only the tables the M0 code
// reads are typed; unknown tables are ignored so later layers can add
// theirs without a schema bump.
type Config struct {
	Hook  HookConfig  `toml:"hook"`
	Trace TraceConfig `toml:"trace"`
}

// HookConfig is the [hook] table.
type HookConfig struct {
	// DeadlineMS is the entry-side deadline of contracts section 1.1.
	DeadlineMS int `toml:"deadline_ms"`
}

// TraceConfig is the [trace] table family.
type TraceConfig struct {
	Budget BudgetConfig `toml:"budget"`
}

// BudgetConfig is [trace.budget] (trace-spec section 3.6).
type BudgetConfig struct {
	SessionUSD        *float64 `toml:"session_usd"`
	TaskUSD           *float64 `toml:"task_usd"`
	TurnContextTokens int      `toml:"turn_context_tokens"`
	SoftPct           int      `toml:"soft_pct"`
	CompactPct        int      `toml:"compact_pct"`
	HardPct           int      `toml:"hard_pct"`
	HardAction        string   `toml:"hard_action"`
}

// DefaultConfig returns the documented defaults.
func DefaultConfig() Config {
	return Config{
		Hook: HookConfig{DeadlineMS: 5000},
		Trace: TraceConfig{Budget: BudgetConfig{
			TurnContextTokens: 400000, SoftPct: 80, CompactPct: 95, HardPct: 100, HardAction: "stop",
		}},
	}
}

// Manifest is .saga/manifest.json: the installed layers and every
// auto-executing entry the installer wrote.
type Manifest struct {
	Schema      string          `json:"schema"`
	SagaVersion string          `json:"saga_version"`
	Layers      []string        `json:"layers"`
	Entries     []ManifestEntry `json:"entries"`
}

// ManifestEntry is one hook binding written by `saga install`.
type ManifestEntry struct {
	Harness string `json:"harness"`
	Event   string `json:"event"`
	File    string `json:"file"`
	Command string `json:"command"`
}

// ErrNotFound is returned by Find when no .saga directory exists at or
// above the start directory.
var ErrNotFound = errors.New("no .saga directory found")

// RepoRoot walks up from start to the first directory containing .saga
// or .git. When neither exists it returns start itself.
func RepoRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, Dir)); err == nil {
			return d
		}
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

// Find locates the store for the repository containing start. It returns
// ErrNotFound when .saga does not exist there.
func Find(start string) (*Store, error) {
	root := RepoRoot(start)
	s := &Store{Root: root}
	if !s.Exists() {
		return s, ErrNotFound
	}
	return s, nil
}

// Open returns the store rooted at root without checking existence.
func Open(root string) *Store { return &Store{Root: root} }

// Dir returns the absolute path of .saga.
func (s *Store) Dir() string { return filepath.Join(s.Root, Dir) }

// Path joins parts under .saga.
func (s *Store) Path(parts ...string) string {
	return filepath.Join(append([]string{s.Dir()}, parts...)...)
}

// Exists reports whether .saga/.gitignore is present, the contracts
// section 9 step 1 probe.
func (s *Store) Exists() bool {
	_, err := os.Stat(s.Path(".gitignore"))
	return err == nil
}

// Init writes the .saga layout for root: .gitignore, config.toml
// skeleton and the trace and observed directories. It is idempotent and
// never overwrites an existing file. It reports whether anything was
// created.
func Init(root string) (bool, error) {
	s := Open(root)
	created := false
	for _, d := range []string{s.Dir(), s.Path("trace"), s.Path("trace", "sessions"), s.Path("trace", "pins"), s.Path("trace", "prices"), s.Path("observed")} {
		if err := CheckShape(d); err != nil {
			return created, err
		}
		if _, err := os.Stat(d); errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return created, fmt.Errorf("init: %w", err)
			}
			created = true
		}
	}
	for name, body := range map[string]string{".gitignore": GitignoreBody, "config.toml": ConfigSkeleton} {
		p := s.Path(name)
		if err := CheckShape(p); err != nil {
			return created, err
		}
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				return created, fmt.Errorf("init: %w", err)
			}
			created = true
		}
	}
	return created, nil
}

// Config parses .saga/config.toml. A missing file yields the defaults;
// a malformed file is an error the caller maps to exit 2.
func (s *Store) Config() (Config, error) {
	cfg := DefaultConfig()
	p := s.Path("config.toml")
	if err := CheckShape(p); err != nil {
		return cfg, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if _, err := toml.Decode(string(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("config.toml: %w", err)
	}
	if cfg.Hook.DeadlineMS <= 0 {
		cfg.Hook.DeadlineMS = 5000
	}
	return cfg, nil
}

// HasConfig reports whether .saga/config.toml exists, which is the
// minimal versus full mode switch of gate-spec section 1.2.
func (s *Store) HasConfig() bool {
	_, err := os.Stat(s.Path("config.toml"))
	return err == nil
}

// ReadManifest reads .saga/manifest.json. A missing manifest returns a
// manifest with the default layer set (trace only) and ok = false.
func (s *Store) ReadManifest() (Manifest, bool, error) {
	m := Manifest{Schema: ManifestSchema, Layers: []string{"trace"}}
	p := s.Path("manifest.json")
	if err := CheckShape(p); err != nil {
		return m, false, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, false, fmt.Errorf("manifest.json: %w", err)
	}
	if m.Schema != ManifestSchema {
		return m, false, fmt.Errorf("manifest.json: unknown schema %q", m.Schema)
	}
	return m, true, nil
}

// WriteManifest writes .saga/manifest.json atomically.
func (s *Store) WriteManifest(m Manifest) error {
	m.Schema = ManifestSchema
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.Path("manifest.json"), append(raw, '\n'), 0o600)
}

// CheckShape applies the hostile-shape rules of gate-spec section 8 to
// p: a symlink, a FIFO, a device or a hard-linked regular file is
// refused. A missing path passes.
func CheckShape(p string) error {
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	mode := fi.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		return &ShapeError{Path: p, Reason: "symlink"}
	case mode&fs.ModeNamedPipe != 0:
		return &ShapeError{Path: p, Reason: "fifo"}
	case mode&(fs.ModeDevice|fs.ModeCharDevice|fs.ModeSocket) != 0:
		return &ShapeError{Path: p, Reason: "device"}
	}
	if mode.IsRegular() && nlink(fi) > 1 {
		return &ShapeError{Path: p, Reason: "hard link"}
	}
	return nil
}

// ShapeError reports a hostile file shape; callers map it to exit 6.
type ShapeError struct {
	Path   string
	Reason string
}

// Error implements error.
func (e *ShapeError) Error() string {
	return fmt.Sprintf("hostile file shape: %s is a %s", e.Path, e.Reason)
}

// WriteFileAtomic writes data to a temporary file beside p and renames it
// into place.
func WriteFileAtomic(p string, data []byte, perm fs.FileMode) error {
	if err := CheckShape(p); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, p)
}
