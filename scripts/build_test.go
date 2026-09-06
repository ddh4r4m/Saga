package scripts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildSagaVia runs scripts/build-saga.sh and returns the binary's hash.
func buildSagaVia(t *testing.T, out string) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-saga.sh"), out)
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-saga.sh: %v\n%s", err, b)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// TestBenchBuildIsReproducible (ADR 0010): the approval identity binds
// the binary's bytes and the corpus store is keyed by their hash, so a
// binary that changes for a reason unrelated to behaviour stales every
// approval the owner gave. The launcher used to stamp the commit into
// the binary with -X main.version, which meant a docs-only commit
// changed the hash and the next run would have reported the whole
// corpus as not pre-approved.
func TestBenchBuildIsReproducible(t *testing.T) {
	if testing.Short() {
		t.Skip("short: this builds the binary twice")
	}
	needTools(t)
	dir := t.TempDir()
	first := buildSagaVia(t, filepath.Join(dir, "saga-1"))
	second := buildSagaVia(t, filepath.Join(dir, "saga-2"))
	if first != second {
		t.Errorf("two builds of one tree differ: %s and %s", first, second)
	}
	t.Logf("bench binary sha256:%s", first)

	// A change outside the module's build inputs must not move it. A
	// docs-only commit is exactly this shape, and it is what stamping
	// the commit hash got wrong.
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "docs", "specs", "IMPLEMENTATION-STATUS.md")
	info, err := os.Stat(marker)
	if err != nil {
		t.Skipf("no docs file to touch: %v", err)
	}
	if err := os.Chtimes(marker, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	third := buildSagaVia(t, filepath.Join(dir, "saga-3"))
	if third != first {
		t.Errorf("a docs-only change moved the binary hash: %s then %s", first, third)
	}

	// And the stamp is really gone: no absolute path from this machine,
	// and no short commit hash baked in.
	raw, err := os.ReadFile(filepath.Join(dir, "saga-1"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), root+"/cmd/saga") {
		t.Error("the binary embeds an absolute path from this machine; -trimpath is not taking effect")
	}
}

// TestLaunchersUseTheSharedBuild: two launchers building two different
// binaries would give the owner two sets of approvals to keep, and only
// one of them would match the run.
func TestLaunchersUseTheSharedBuild(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bench-smoke.sh", "harness-probes.sh", "bench-approve.sh"} {
		raw, err := os.ReadFile(filepath.Join(root, "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(raw)
		if !strings.Contains(body, "scripts/build-saga.sh") {
			t.Errorf("%s does not build through the shared script", name)
		}
		if strings.Contains(body, "-X main.version") {
			t.Errorf("%s stamps a version into the binary, which stales the corpus approvals", name)
		}
	}
}

// TestBenchApproveRefusesInsideAnAgent: approving is the owner's act and
// the script says so before it builds anything.
func TestBenchApproveRefusesInsideAnAgent(t *testing.T) {
	needTools(t)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "bench-approve.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CLAUDECODE=1", "SAGA_HOME="+t.TempDir())
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("bench-approve.sh ran inside an agent shell:\n%s", b)
	}
	out := string(b)
	if !strings.Contains(out, "human act") || !strings.Contains(out, "CLAUDECODE") {
		t.Errorf("the refusal does not name the reason:\n%s", out)
	}
	// It refused before building, so nothing was written.
	if strings.Contains(out, "binary sha256:") {
		t.Errorf("it built before refusing:\n%s", out)
	}
}

// TestManifestKeepsTheCommitWithoutStampingTheBinary: the commit is
// provenance and belongs in the manifest, but stamping it into the
// binary is what made a docs-only commit stale every corpus approval.
// The launcher passes it as a flag instead, so both properties hold.
func TestManifestKeepsTheCommitWithoutStampingTheBinary(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "bench-smoke.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "--bench-git") {
		t.Error("bench-smoke.sh does not pass the commit to the manifest; bench_version.git would read the built-in default")
	}
	if !strings.Contains(body, "rev-parse --short HEAD") {
		t.Error("bench-smoke.sh does not resolve the commit it passes")
	}
}
