package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/gate"
)

// fakeTool writes an executable named tool into a fresh directory and
// returns the directory.
func fakeTool(t *testing.T, tool string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestBenchPathIsCanonical: the composed PATH depends on the binary and
// on where the toolchain is, never on what else the caller's shell
// happens to carry. The approval identity hashes the whole of PATH, so
// an inherited one bound the corpus approvals to the shell that gave
// them: 101 records approved by the owner, and `--check` from another
// shell found none of them.
func TestBenchPathIsCanonical(t *testing.T) {
	t.Setenv("SAGA_HOME", t.TempDir())
	saga := filepath.Join(t.TempDir(), "saga")
	if err := os.WriteFile(saga, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := fakeTool(t, "node")
	if err := os.WriteFile(filepath.Join(tools, "python3"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &ClaudeCode{SagaBinary: saga}

	// Two shells with quite different PATHs, both finding the same tools.
	t.Setenv("PATH", strings.Join([]string{"/opt/one/bin", tools, "/usr/bin", "/usr/local/sbin"}, string(os.PathListSeparator)))
	first := c.BenchPath("")
	t.Setenv("PATH", strings.Join([]string{"/entirely/other", "/somewhere/else/bin", tools, "/bin"}, string(os.PathListSeparator)))
	second := c.BenchPath("")

	if strings.Join(first, ":") != strings.Join(second, ":") {
		t.Errorf("the composed PATH depends on the caller's shell:\n  %v\n  %v", first, second)
	}
	// It is the documented list, in the documented order.
	want := []string{}
	if d, err := BenchBinDir(saga); err == nil {
		want = append(want, d)
	}
	want = append(want, tools)
	want = append(want, SystemPathDirs...)
	if strings.Join(first, ":") != strings.Join(want, ":") {
		t.Errorf("composed %v, want %v", first, want)
	}
	// No entry the caller carried survives.
	for _, junk := range []string{"/opt/one/bin", "/entirely/other", "/usr/local/sbin"} {
		for _, got := range first {
			if got == junk {
				t.Errorf("a caller's own PATH entry %q reached the composition", junk)
			}
		}
	}
	// The control arm's shim leads when the config dir has one; it never
	// enters an identity, because a bare arm is never approved.
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, shimDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, shimDir, "saga"), []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if bare := c.BenchPath(cfg); bare[0] != filepath.Join(cfg, shimDir) {
		t.Errorf("the shim does not lead the bare arm's PATH: %v", bare)
	}
}

// TestApprovalIdentityIgnoresTheCallersShell: the same task, binary and
// toolchain give the same identity from two different shells, which is
// the whole point. Nothing in gate.ApprovalIdentity changes; it is
// handed a composed PATH instead of an inherited one.
func TestApprovalIdentityIgnoresTheCallersShell(t *testing.T) {
	t.Setenv("SAGA_HOME", t.TempDir())
	saga := filepath.Join(t.TempDir(), "saga")
	if err := os.WriteFile(saga, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := fakeTool(t, "node")
	if err := os.WriteFile(filepath.Join(tools, "python3"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &ClaudeCode{SagaBinary: saga}
	identity := func() string {
		return gate.ApprovalIdentity(".saga/contract.md", "G1", "sha256:oracle", "darwin/arm64",
			strings.Join(c.BenchPath(""), string(os.PathListSeparator)), "sha256:witness")
	}
	t.Setenv("PATH", strings.Join([]string{"/the/owners/bin", tools, "/usr/bin"}, string(os.PathListSeparator)))
	owner := identity()
	t.Setenv("PATH", strings.Join([]string{"/another/session/bin", "/yet/more", tools}, string(os.PathListSeparator)))
	other := identity()
	if owner != other {
		t.Errorf("the identity still depends on the caller's shell:\n  %s\n  %s", owner, other)
	}
	// And it is not the empty-PATH identity: the composition is real.
	if owner == gate.ApprovalIdentity(".saga/contract.md", "G1", "sha256:oracle", "darwin/arm64", "", "sha256:witness") {
		t.Error("the identity was taken over an empty PATH")
	}
}

// TestToolchainMoveInvalidatesApproval: a task graded with a different
// node is a different cell, so a toolchain that moves must invalidate
// the approvals rather than quietly reuse them. The failure is visible:
// the composed PATH is in harness.json and printed by approve-corpus.
func TestToolchainMoveInvalidatesApproval(t *testing.T) {
	t.Setenv("SAGA_HOME", t.TempDir())
	saga := filepath.Join(t.TempDir(), "saga")
	if err := os.WriteFile(saga, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &ClaudeCode{SagaBinary: saga}
	identity := func() string {
		return gate.ApprovalIdentity(".saga/contract.md", "G1", "sha256:oracle", "darwin/arm64",
			strings.Join(c.BenchPath(""), string(os.PathListSeparator)), "sha256:witness")
	}
	first := fakeTool(t, "node")
	if err := os.WriteFile(filepath.Join(first, "python3"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", first+string(os.PathListSeparator)+"/usr/bin")
	atApproval := identity()
	pathAtApproval := c.BenchPath("")

	// The toolchain moves: node is somewhere else now.
	moved := fakeTool(t, "node")
	t.Setenv("PATH", strings.Join([]string{moved, first, "/usr/bin"}, string(os.PathListSeparator)))
	atRun := identity()
	pathAtRun := c.BenchPath("")

	if atApproval == atRun {
		t.Error("a moved toolchain kept the same identity; the approval would be reused for a different cell")
	}
	if strings.Join(pathAtApproval, ":") == strings.Join(pathAtRun, ":") {
		t.Errorf("the composed PATH did not change: %v", pathAtRun)
	}
	// The difference is nameable, which is what the failure message needs.
	if !strings.Contains(strings.Join(pathAtRun, ":"), moved) {
		t.Errorf("the new toolchain directory is not in the composition: %v", pathAtRun)
	}
}
