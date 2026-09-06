package task

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Fixed commit identity and date so a staged workspace's base commit is
// the same everywhere (section 8.4: task checkout is deterministic).
var gitEnv = []string{
	"GIT_AUTHOR_NAME=saga-bench", "GIT_AUTHOR_EMAIL=bench@saga.invalid",
	"GIT_COMMITTER_NAME=saga-bench", "GIT_COMMITTER_EMAIL=bench@saga.invalid",
	"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "HOME=/nonexistent-saga-bench-home",
}

// SetupTimeout bounds setup.sh and each oracle run.
const SetupTimeout = 10 * time.Minute

// Git runs git in dir with the fixed identity.
func Git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false", "-c", "core.safecrlf=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitEnv...)
	setProcessGroup(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// Stage copies the snapshot into dst, initialises a repository and
// commits it as the base state. dst must not exist or be empty.
func Stage(ctx context.Context, t *Task, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := copyTree(t.RepoDir(), dst); err != nil {
		return fmt.Errorf("stage %s: %w", t.ID, err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A", "-f", "."}, {"commit", "-q", "--allow-empty", "-m", "base"}} {
		if _, err := Git(ctx, dst, args...); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" && filepath.Dir(rel) == "." {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
		return nil
	})
}

// Apply applies a control patch or a workspace diff with git apply. An
// empty diff is a no-op.
func Apply(ctx context.Context, dir, patch string) error {
	raw, err := os.ReadFile(patch)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	_, err = Git(ctx, dir, "apply", "--binary", "--whitespace=nowarn", patch)
	return err
}

// Diff is the binary-safe diff of the workspace against its base commit,
// including untracked files and excluding the bench's own directories
// (.saga, .claude) so harness state never grades as agent work.
func Diff(ctx context.Context, dir string) ([]byte, error) {
	if _, err := Git(ctx, dir, "add", "-A", "-f", "--", ".", ":(exclude).saga", ":(exclude).claude"); err != nil {
		return nil, err
	}
	out, err := Git(ctx, dir, "diff", "--cached", "--binary", "--no-color", "--no-ext-diff", "HEAD", "--", ".", ":(exclude).saga", ":(exclude).claude")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Run executes a bash script with cwd dir and the given environment
// additions, capturing stdout and stderr, under the context's deadline.
func Run(ctx context.Context, dir, script string, env []string) (stdout, stderr []byte, exit int, err error) {
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	setProcessGroup(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	exit = 0
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		exit = ee.ExitCode()
		if exit < 0 {
			exit = 128
		}
		err = nil
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.Bytes(), errb.Bytes(), exit, err
}

// Setup runs setup.sh in the workspace.
func Setup(ctx context.Context, t *Task, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, SetupTimeout)
	defer cancel()
	out, errb, exit, err := Run(ctx, dir, t.SetupPath(), []string{"WORKSPACE=" + dir})
	if err != nil {
		return fmt.Errorf("setup.sh: %w", err)
	}
	if exit != 0 {
		return fmt.Errorf("setup.sh exited %d: %s", exit, strings.TrimSpace(string(append(out, errb...))))
	}
	return nil
}

// OracleResult is one oracle run.
type OracleResult struct {
	Exit      int               `json:"exit"`
	Tests     map[string]string `json:"tests"`
	Order     []string          `json:"-"`
	Pass      bool              `json:"pass"`
	Regressed []string          `json:"regressed"`
	// Integrity is the section 5.8 probe verdict: ok, fail or skipped.
	// A run passes only when the oracle exits 0 AND the assertions were
	// still asserting, because the oracle shares its interpreter with
	// code the agent controls.
	Integrity       string `json:"integrity"`
	IntegrityReason string `json:"integrity_reason,omitempty"`
	Stdout          []byte `json:"-"`
	Stderr          []byte `json:"-"`
	TimedOut        bool   `json:"-"`
}

var testLineRe = regexp.MustCompile(`^(\S+) (PASS|FAIL)$`)

// Counts returns the PASS and FAIL line counts.
func (o *OracleResult) Counts() (pass, fail int) {
	for _, id := range o.Order {
		if o.Tests[id] == "PASS" {
			pass++
		} else {
			fail++
		}
	}
	return
}

// Oracle runs the hidden oracle against the workspace (section 2.1: exit
// 0 = pass, one "<id> PASS|FAIL" line per hidden test) and marks
// regressions from the task's regression set.
func Oracle(ctx context.Context, t *Task, dir string) (*OracleResult, error) {
	ctx, cancel := context.WithTimeout(ctx, SetupTimeout)
	defer cancel()
	out, errb, exit, err := Run(ctx, dir, t.OraclePath(), []string{"WORKSPACE=" + dir})
	res := &OracleResult{Exit: exit, Tests: map[string]string{}, Regressed: []string{}, Stdout: out, Stderr: errb}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			res.TimedOut = true
			res.Exit = 124
			return res, nil
		}
		return res, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if m := testLineRe.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			if _, dup := res.Tests[m[1]]; !dup {
				res.Order = append(res.Order, m[1])
			}
			res.Tests[m[1]] = m[2]
		}
	}
	res.Integrity, res.IntegrityReason = Probe(ctx, t, dir)
	res.Pass = exit == 0 && res.Integrity == IntegrityOK
	for _, id := range t.RegressionIDs() {
		if res.Tests[id] == "FAIL" {
			res.Regressed = append(res.Regressed, id)
		}
	}
	return res, nil
}

// Text renders the archived oracle.txt: the per-test lines, then the
// exit code and the captured streams.
func (o *OracleResult) Text() []byte {
	var b bytes.Buffer
	b.Write(o.Stdout)
	if len(o.Stdout) > 0 && !bytes.HasSuffix(o.Stdout, []byte("\n")) {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "--- exit %d\n", o.Exit)
	if o.Integrity != "" {
		fmt.Fprintf(&b, "--- integrity %s", o.Integrity)
		if o.IntegrityReason != "" {
			fmt.Fprintf(&b, " %s", o.IntegrityReason)
		}
		b.WriteByte('\n')
	}
	if len(o.Stderr) > 0 {
		b.WriteString("--- stderr\n")
		b.Write(o.Stderr)
		if !bytes.HasSuffix(o.Stderr, []byte("\n")) {
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}
