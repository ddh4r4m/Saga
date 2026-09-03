//go:build !windows

package task

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so a timeout
// kills the whole tree (bench-spec section 3.3).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
}
