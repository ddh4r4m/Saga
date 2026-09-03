package gate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
)

// OutputCap is the combined stdout+stderr cap of gate-spec section 4.1.
const OutputCap = 1 << 20

// DefaultTimeout is the per-gate timeout default.
const DefaultTimeout = 120

// DefaultShell runs CHECK: lines. It is the resolved shell recorded in
// every record; `[gate] shell` overrides it.
const DefaultShell = "/bin/sh"

// Oracle is a resolved CHECK:/EXPECT: pair with everything that binds it
// (section 3.4): the exact texts, CWD:, the resolved shell, the timeout
// and the output cap.
type Oracle struct {
	Check     string `json:"check"`
	Expect    string `json:"expect"`
	CWD       string `json:"cwd"`
	Shell     string `json:"shell"`
	TimeoutS  int    `json:"timeout_s"`
	OutputCap int    `json:"output_cap_bytes"`
}

// Hash is the oracle_hash.
func (o Oracle) Hash() string {
	h, _ := canon.SHA256JSON(o)
	return h
}

// Toolchain is the platform and PATH fingerprint bound into records; the
// PATH itself is never persisted (section 4.4).
type Toolchain struct {
	Shell           string `json:"shell"`
	Platform        string `json:"platform"`
	PathFingerprint string `json:"path_fingerprint"`
	PathEntries     int    `json:"path_entries"`
}

// CurrentToolchain fingerprints this process's environment.
func CurrentToolchain(shell string) Toolchain {
	p := os.Getenv("PATH")
	n := 0
	if p != "" {
		n = len(strings.Split(p, string(os.PathListSeparator)))
	}
	return Toolchain{Shell: shell, Platform: runtime.GOOS + "-" + runtime.GOARCH, PathFingerprint: canon.SHA256([]byte(p)), PathEntries: n}
}

// Result is one oracle execution.
type Result struct {
	Exit       int
	Output     []byte
	Bytes      int
	Truncated  bool
	TimedOut   bool
	StartFail  bool
	DurationMS int
	StartedAt  time.Time
}

// OutputSHA256 fingerprints the output.
func (r *Result) OutputSHA256() string { return canon.SHA256(r.Output) }

// capWriter stops the process once the cap is breached.
type capWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
	kill      func()
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	room := w.limit - w.buf.Len()
	if room <= 0 {
		if !w.truncated {
			w.truncated = true
			w.kill()
		}
		return len(p), nil
	}
	if len(p) > room {
		w.buf.Write(p[:room])
		w.truncated = true
		w.kill()
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

// Run executes command under shell -c in dir with the timeout and cap,
// combining stdout and stderr. The process group is killed on timeout
// or cap breach.
func Run(ctx context.Context, shell, command, dir string, timeout time.Duration, capBytes int) *Result {
	r := &Result{StartedAt: time.Now()}
	if capBytes <= 0 {
		capBytes = OutputCap
	}
	if timeout <= 0 {
		timeout = DefaultTimeout * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = dir
	setProcessGroup(cmd)
	var once sync.Once
	kill := func() { once.Do(func() { killProcessGroup(cmd) }) }
	w := &capWriter{limit: capBytes, kill: kill}
	cmd.Stdout, cmd.Stderr = w, w
	cmd.Stdin = nil
	cmd.Cancel = func() error { killProcessGroup(cmd); return nil }
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Start()
	if err != nil {
		r.StartFail = true
		r.Exit = 127
		r.Output = []byte("shell start failed: " + err.Error())
		r.Bytes = len(r.Output)
		r.DurationMS = int(time.Since(r.StartedAt) / time.Millisecond)
		return r
	}
	err = cmd.Wait()
	r.DurationMS = int(time.Since(r.StartedAt) / time.Millisecond)
	w.mu.Lock()
	r.Output = append([]byte(nil), w.buf.Bytes()...)
	r.Truncated = w.truncated
	w.mu.Unlock()
	r.Bytes = len(r.Output)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		r.TimedOut = true
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			r.Exit = ee.ExitCode()
			if r.Exit < 0 {
				r.Exit = 137
			}
		} else {
			r.Exit = 1
		}
	}
	return r
}

// FailureTail returns the error-aware tail of a failing output for the
// terminal: at most limit bytes, never cut mid-line, control- and
// bidi-stripped; the caller masks it (section 4.4).
func FailureTail(out []byte, limit int) string {
	s := string(bytes.ToValidUTF8(out, []byte("�")))
	if len(s) > limit {
		s = s[len(s)-limit:]
		if i := strings.IndexByte(s, '\n'); i >= 0 && i < len(s)-1 {
			s = s[i+1:]
		}
	}
	return canon.CleanText(s)
}

// short12 is the first 12 hex of sha256 over b (waiver hunk hashes).
func short12(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:12]
}
