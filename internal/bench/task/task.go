// Package task loads and validates bench task directories (bench-spec
// section 2), stages clean workspaces from them, runs the hidden oracle,
// scans diffs for oracle cheating and scope violations (section 5.7,
// 5.8) and verifies a task's own red proof (section 2.4).
package task

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/ddh4r4m/saga/internal/cli"
)

// Schema is the task metadata schema id.
const Schema = "saga.bench.task/1"

// Task is the decoded task.toml plus its directory.
type Task struct {
	Dir string `toml:"-"`

	Schema          string      `toml:"schema"`
	ID              string      `toml:"id"`
	Language        string      `toml:"language"`
	Size            string      `toml:"size"`
	ExpectedMinutes float64     `toml:"expected_minutes"`
	CostHintUSD     float64     `toml:"cost_hint_usd"`
	Created         time.Time   `toml:"created"`
	Source          Source      `toml:"source"`
	Tags            []string    `toml:"tags"`
	Repo            Repo        `toml:"repo"`
	Env             Env         `toml:"env"`
	Oracle          OracleSpec  `toml:"oracle"`
	Terminal        *Terminal   `toml:"terminal"`
	Harness         Harness     `toml:"harness"`
	Trajectory      *Trajectory `toml:"trajectory"`
}

// Source is where the task came from (section 2.5).
type Source struct {
	Kind string `toml:"kind"`
	Ref  string `toml:"ref"`
	Note string `toml:"note"`
}

// Repo is the workspace source.
type Repo struct {
	Kind     string `toml:"kind"`
	URL      string `toml:"url"`
	Ref      string `toml:"ref"`
	Path     string `toml:"path"`
	Snapshot string `toml:"snapshot"`
}

// Env is the execution environment.
type Env struct {
	Image             string  `toml:"image"`
	Setup             string  `toml:"setup"`
	Network           string  `toml:"network"`
	TimeoutMultiplier float64 `toml:"timeout_multiplier"`
}

// Oracle is the hidden grader.
type OracleSpec struct {
	Run              string   `toml:"run"`
	BaselineMustFail bool     `toml:"baseline_must_fail"`
	Ceiling          float64  `toml:"ceiling"`
	RegressionSet    string   `toml:"regression_set"`
	GoldFiles        []string `toml:"gold_files"`
	TestGlobs        []string `toml:"test_globs"`
}

// Terminal is the expected terminal state for impossible tasks.
type Terminal struct {
	Expected          string   `toml:"expected"`
	ReasonMustMention []string `toml:"reason_must_mention"`
}

// Harness holds the optional per-task harness settings the adapters
// read: permission mode and turn cap (section 3.3 default 200).
type Harness struct {
	PermissionMode string `toml:"permission_mode"`
	MaxTurns       int    `toml:"max_turns"`
}

// Trajectory is the optional canonical tool-category path (section 5.9).
type Trajectory struct {
	CanonicalPath []string `toml:"canonical_path"`
}

// Defaults (section 3.3, section 6.1).
const (
	DefaultMaxTurns       = 200
	DefaultPermissionMode = "acceptEdits"
	CostCapMultiplier     = 3.0
)

var (
	languages   = set("typescript", "python", "go", "rust", "jvm", "swift", "dart")
	sizes       = set("S", "M", "L", "XL")
	primaryTags = set("hack-bait", "impossible", "absence", "regression", "localization", "plain")
	sourceKinds = set("real-failure", "issue", "synthetic")
	networks    = set("offline", "registry-only", "open")
	idPattern   = regexp.MustCompile(`^[a-z]+-[0-9]{4}-[a-z0-9-]+$`)
	canaryLine  = regexp.MustCompile(`^[0-9a-fA-F-]{8,}$`)
	shaPattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	permModes   = set("default", "acceptEdits", "plan", "bypassPermissions", "dontAsk")
)

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// Load reads and validates <dir>/task.toml. Every failure is a usage
// error (exit 2, contracts section 4): the task cannot be run.
func Load(dir string) (*Task, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "task", err)
	}
	t := &Task{Dir: abs, Oracle: OracleSpec{Ceiling: 1.0}}
	raw, err := os.ReadFile(filepath.Join(abs, "task.toml"))
	if err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "task", err)
	}
	md, err := toml.Decode(string(raw), t)
	if err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "task.toml", err)
	}
	if un := md.Undecoded(); len(un) > 0 {
		keys := make([]string, 0, len(un))
		for _, k := range un {
			keys = append(keys, k.String())
		}
		return nil, cli.Errorf(cli.ExitUsage, "task.toml: unknown fields %s", strings.Join(keys, ", "))
	}
	if problems := t.validate(); len(problems) > 0 {
		return t, cli.Errorf(cli.ExitUsage, "task %s: %s", filepath.Base(abs), strings.Join(problems, "; "))
	}
	return t, nil
}

func (t *Task) validate() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	if t.Schema != Schema {
		add("schema %q, want %s", t.Schema, Schema)
	}
	if !idPattern.MatchString(t.ID) {
		add("id %q does not match <lang>-<nnnn>-<slug>", t.ID)
	} else if t.ID != filepath.Base(t.Dir) {
		add("id %q differs from directory %q", t.ID, filepath.Base(t.Dir))
	}
	if !languages[t.Language] {
		add("language %q not in typescript|python|go|rust|jvm|swift|dart", t.Language)
	}
	if !sizes[t.Size] {
		add("size %q not in S|M|L|XL", t.Size)
	}
	if t.ExpectedMinutes <= 0 {
		add("expected_minutes must be positive")
	}
	if t.CostHintUSD <= 0 {
		add("cost_hint_usd must be positive")
	}
	if t.Created.IsZero() {
		add("created is required")
	}
	if !sourceKinds[t.Source.Kind] {
		add("source.kind %q not in real-failure|issue|synthetic", t.Source.Kind)
	}
	if len(t.Tags) == 0 {
		add("tags is empty")
	} else {
		primary := false
		for _, tag := range t.Tags {
			if primaryTags[tag] {
				primary = true
			}
		}
		if !primary {
			add("tags %v carry none of hack-bait|impossible|absence|regression|localization|plain", t.Tags)
		}
	}
	switch t.Repo.Kind {
	case "snapshot":
		if t.Repo.Path == "" {
			t.Repo.Path = "repo"
		}
		if st, err := os.Stat(t.RepoDir()); err != nil || !st.IsDir() {
			add("repo.path %q is not a directory", t.Repo.Path)
		}
		if !shaPattern.MatchString(t.Repo.Snapshot) {
			add("repo.snapshot must be sha256:<64 hex>")
		}
	case "git":
		if t.Repo.URL == "" || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(t.Repo.Ref) {
			add("repo.kind git needs url and a full 40-hex ref")
		}
		add("repo.kind git is not supported by this runner yet (snapshot only)")
	default:
		add("repo.kind %q not in git|snapshot", t.Repo.Kind)
	}
	if t.Env.Image == "" {
		add("env.image is required")
	}
	if t.Env.Setup == "" {
		t.Env.Setup = "setup.sh"
	}
	if _, err := os.Stat(t.SetupPath()); err != nil {
		add("env.setup %q not found", t.Env.Setup)
	}
	if !networks[t.Env.Network] {
		add("env.network %q not in offline|registry-only|open", t.Env.Network)
	}
	if t.Env.TimeoutMultiplier <= 0 {
		add("env.timeout_multiplier must be positive")
	}
	if t.Oracle.Run == "" {
		t.Oracle.Run = "oracle/run.sh"
	}
	if _, err := os.Stat(t.OraclePath()); err != nil {
		add("oracle.run %q not found", t.Oracle.Run)
	}
	if t.Oracle.Ceiling <= 0 || t.Oracle.Ceiling > 1 {
		add("oracle.ceiling must be in (0, 1]")
	}
	if t.Oracle.RegressionSet != "" {
		if _, err := os.Stat(filepath.Join(t.Dir, t.Oracle.RegressionSet)); err != nil {
			add("oracle.regression_set %q not found", t.Oracle.RegressionSet)
		}
	}
	for _, name := range []string{"prompt.md", "contract.md", "CANARY"} {
		if st, err := os.Stat(filepath.Join(t.Dir, name)); err != nil || st.Size() == 0 {
			add("%s missing or empty", name)
		}
	}
	if c := t.Canary(); c == "" || !canaryLine.MatchString(c) {
		add("CANARY must hold one GUID line")
	}
	gold, broken, cheat := t.Controls()
	if len(broken) == 0 {
		add("controls/ has no broken-*.patch")
	}
	if t.Impossible() {
		if gold != "" {
			add("impossible task must not ship a gold.patch")
		}
		if t.Terminal == nil || t.Terminal.Expected != "ABANDON" {
			add("impossible task needs [terminal] expected = \"ABANDON\"")
		}
	} else {
		if gold == "" {
			add("controls/gold.patch is required")
		}
		if t.Terminal != nil && t.Terminal.Expected != "" && t.Terminal.Expected != "DONE" {
			add("[terminal] expected %q only applies to impossible tasks", t.Terminal.Expected)
		}
	}
	_ = cheat
	if t.Harness.MaxTurns < 0 {
		add("harness.max_turns must be non-negative")
	}
	if t.Harness.PermissionMode != "" && !permModes[t.Harness.PermissionMode] {
		add("harness.permission_mode %q unknown", t.Harness.PermissionMode)
	}
	return p
}

// RepoDir is the snapshot directory the workspace is copied from.
func (t *Task) RepoDir() string { return filepath.Join(t.Dir, t.Repo.Path) }

// SetupPath is the absolute path of setup.sh.
func (t *Task) SetupPath() string { return filepath.Join(t.Dir, t.Env.Setup) }

// OraclePath is the absolute path of oracle/run.sh.
func (t *Task) OraclePath() string { return filepath.Join(t.Dir, t.Oracle.Run) }

// Impossible reports whether the task expects ABANDON.
func (t *Task) Impossible() bool { return t.HasTag("impossible") }

// HasTag reports whether tag is set.
func (t *Task) HasTag(tag string) bool {
	for _, x := range t.Tags {
		if x == tag {
			return true
		}
	}
	return false
}

// Canary is the GUID line from CANARY.
func (t *Task) Canary() string {
	raw, err := os.ReadFile(filepath.Join(t.Dir, "CANARY"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
}

// Prompt is prompt.md verbatim.
func (t *Task) Prompt() string {
	raw, _ := os.ReadFile(filepath.Join(t.Dir, "prompt.md"))
	return string(raw)
}

// Controls lists gold.patch (empty when absent), the broken-*.patch and
// the cheat-*.patch files, sorted.
func (t *Task) Controls() (gold string, broken, cheat []string) {
	dir := filepath.Join(t.Dir, "controls")
	if _, err := os.Stat(filepath.Join(dir, "gold.patch")); err == nil {
		gold = filepath.Join(dir, "gold.patch")
	}
	broken, _ = filepath.Glob(filepath.Join(dir, "broken-*.patch"))
	cheat, _ = filepath.Glob(filepath.Join(dir, "cheat-*.patch"))
	sort.Strings(broken)
	sort.Strings(cheat)
	return
}

// RegressionIDs are the hidden test ids that pass at baseline.
func (t *Task) RegressionIDs() []string {
	if t.Oracle.RegressionSet == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(t.Dir, t.Oracle.RegressionSet))
	if err != nil {
		return nil
	}
	var ids []string
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		ids = append(ids, l)
	}
	return ids
}

// WallLimit is expected_minutes times timeout_multiplier (section 3.3).
func (t *Task) WallLimit() time.Duration {
	return time.Duration(t.ExpectedMinutes * t.Env.TimeoutMultiplier * float64(time.Minute))
}

// CostCap is 3 times cost_hint_usd (section 3.3).
func (t *Task) CostCap() float64 { return CostCapMultiplier * t.CostHintUSD }

// MaxTurns is the harness turn cap.
func (t *Task) MaxTurns() int {
	if t.Harness.MaxTurns > 0 {
		return t.Harness.MaxTurns
	}
	return DefaultMaxTurns
}

// PermissionMode is the harness permission mode.
func (t *Task) PermissionMode() string {
	if t.Harness.PermissionMode != "" {
		return t.Harness.PermissionMode
	}
	return DefaultPermissionMode
}

// Scope parses the IN: and OUT: lines of contract.md into glob lists.
func (t *Task) Scope() (in, out []string) {
	raw, err := os.ReadFile(filepath.Join(t.Dir, "contract.md"))
	if err != nil {
		return nil, nil
	}
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		var dst *[]string
		switch {
		case strings.HasPrefix(l, "IN:"):
			dst, l = &in, l[3:]
		case strings.HasPrefix(l, "OUT:"):
			dst, l = &out, l[4:]
		default:
			continue
		}
		for _, g := range strings.Split(l, ",") {
			if g = strings.TrimSpace(g); g != "" {
				*dst = append(*dst, g)
			}
		}
	}
	return in, out
}

// IsTest reports whether a repo-relative path is a visible test file:
// it matches oracle.test_globs when declared, or the built-in patterns.
func (t *Task) IsTest(path string) bool {
	for _, g := range t.Oracle.TestGlobs {
		if GlobMatch(g, path) {
			return true
		}
	}
	return IsTestPath(path)
}

// ScanOptions builds the scan options for this task.
func (t *Task) ScanOptions() ScanOptions {
	in, out := t.Scope()
	return ScanOptions{HackBait: t.HasTag("hack-bait"), TestLiterals: t.TestLiterals(), ScopeIn: in, ScopeOut: out, TestGlobs: t.Oracle.TestGlobs}
}

// TestLiterals collects the string literals of the visible test files in
// the snapshot, the values a hard-coded shortcut would echo (section
// 5.8 hard-coded detector, offline form).
func (t *Task) TestLiterals() map[string]bool {
	lits := map[string]bool{}
	filepath.WalkDir(t.RepoDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(t.RepoDir(), p)
		if !t.IsTest(filepath.ToSlash(rel)) {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, lit := range Literals(string(raw)) {
			lits[lit] = true
		}
		return nil
	})
	return lits
}

// ContentHash hashes every file under the task directory as sorted
// "./<path> <sha256>\n" lines; it is the task's identity in a manifest.
func (t *Task) ContentHash() (string, error) { return TreeHash(t.Dir) }

// SnapshotHash hashes the snapshot directory the same way; it must equal
// repo.snapshot.
func (t *Task) SnapshotHash() (string, error) { return TreeHash(t.RepoDir()) }

// TreeHash is sha256 over sorted "./<relative path> <sha256 of file>\n"
// lines for every regular file under dir (the corpus convention).
func TreeHash(dir string) (string, error) {
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		sum := sha256.Sum256(raw)
		lines = append(lines, "./"+filepath.ToSlash(rel)+" "+hex.EncodeToString(sum[:])+"\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// Find expands a glob (or a directory) into task directories: every
// match that holds a task.toml, sorted.
func Find(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		if st, err := os.Stat(pattern); err == nil && st.IsDir() {
			matches = []string{pattern}
		}
	}
	var dirs []string
	for _, m := range matches {
		if _, err := os.Stat(filepath.Join(m, "task.toml")); err == nil {
			dirs = append(dirs, m)
			continue
		}
		// A directory of tasks.
		sub, _ := filepath.Glob(filepath.Join(m, "*", "task.toml"))
		for _, s := range sub {
			dirs = append(dirs, filepath.Dir(s))
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}
