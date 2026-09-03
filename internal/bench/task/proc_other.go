//go:build windows

package task

import "os/exec"

func setProcessGroup(cmd *exec.Cmd) {}
