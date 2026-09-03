package gate

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the [gate] table of .saga/config.toml (gate-spec section
// 7.3). It is read from BASE: with `git show`, never from the working
// tree (section 5.4), so an agent cannot loosen it for the run it is in.
type Config struct {
	RequireRed        *bool    `toml:"require_red"`
	TimeoutS          int      `toml:"timeout_s"`
	Shell             string   `toml:"shell"`
	TestGlobs         []string `toml:"test_globs"`
	ScopeExempt       []string `toml:"scope_exempt"`
	MaxBlocks         int      `toml:"max_blocks"`
	WaiverPolicy      string   `toml:"waiver_policy"`
	AgentIdentity     string   `toml:"agent_identity"`
	RedTTLDays        int      `toml:"red_ttl_days"`
	RedTTLCommits     int      `toml:"red_ttl_commits"`
	RedRejectPatterns []string `toml:"red_reject_patterns"`
	WitnessImports    bool     `toml:"witness_imports"`
	Languages         []string `toml:"languages"`

	// Present reports whether a config file existed at BASE: (full mode).
	Present bool `toml:"-"`
}

// DefaultTestGlobs are the per-language discovery conventions of section
// 5.2 for the languages the guards know.
var DefaultTestGlobs = []string{
	"**/*.test.*", "**/*.spec.*", "**/__tests__/**",
	"**/test_*.py", "**/*_test.py",
	"**/*_test.go",
	"**/*Test.java", "**/*Spec.kt", "**/src/test/**",
	"**/*_test.dart", "**/test/**",
	"**/tests/**",
}

// DefaultRedRejectPatterns are the wrong-reason signals of section 3.3.
var DefaultRedRejectPatterns = []string{"command not found", "MODULE_NOT_FOUND", "Cannot find module", "ModuleNotFoundError", "no such file or directory", "No such file or directory"}

// DefaultConfig returns the documented defaults.
func DefaultConfig() Config {
	t := true
	return Config{
		RequireRed: &t, TimeoutS: DefaultTimeout, Shell: DefaultShell, TestGlobs: DefaultTestGlobs,
		MaxBlocks: 6, WaiverPolicy: "logged", RedTTLDays: 30, RedTTLCommits: 200,
		RedRejectPatterns: DefaultRedRejectPatterns,
	}
}

// RequireRedOn reports the effective require_red.
func (c Config) RequireRedOn() bool { return c.RequireRed == nil || *c.RequireRed }

// LoadConfig reads [gate] from .saga/config.toml at rev. A missing file
// yields the defaults with Present = false; a malformed one is an error
// the caller maps to exit 2.
func LoadConfig(root, rev string) (Config, error) {
	cfg := DefaultConfig()
	raw, ok := ShowAt(root, rev, ".saga/config.toml")
	if !ok {
		return cfg, nil
	}
	cfg.Present = true
	var doc struct {
		Gate Config `toml:"gate"`
	}
	doc.Gate = cfg
	if _, err := toml.Decode(string(raw), &doc); err != nil {
		return cfg, fmt.Errorf("config.toml at %s: %w", ShortRev(rev), err)
	}
	out := doc.Gate
	out.Present = true
	if out.TimeoutS < 1 || out.TimeoutS > 86400 {
		out.TimeoutS = DefaultTimeout
	}
	if out.Shell == "" {
		out.Shell = DefaultShell
	}
	if len(out.TestGlobs) == 0 {
		out.TestGlobs = DefaultTestGlobs
	}
	if out.MaxBlocks <= 0 || out.MaxBlocks > 8 {
		out.MaxBlocks = 6
	}
	if out.RedTTLDays <= 0 {
		out.RedTTLDays = 30
	}
	if out.RedTTLCommits <= 0 {
		out.RedTTLCommits = 200
	}
	if len(out.RedRejectPatterns) == 0 {
		out.RedRejectPatterns = DefaultRedRejectPatterns
	}
	if out.WaiverPolicy == "" {
		out.WaiverPolicy = "logged"
	}
	return out, nil
}

// FoldCase reports whether paths are compared case-folded (G-SCOPE on a
// case-insensitive filesystem).
func FoldCase() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

// join builds an absolute path under root from a slash-separated
// repo-relative path.
func join(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// rel converts an absolute or relative path into slash-separated form
// relative to root, and ok = false when it lies outside.
func rel(root, p string) (string, bool) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}
