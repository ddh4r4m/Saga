//go:build !unix

package gate

import (
	"io/fs"
	"os/exec"
)

// ownedByCaller has no portable answer off unix; the mode check stands.
func ownedByCaller(fi fs.FileInfo) bool { return true }

func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
