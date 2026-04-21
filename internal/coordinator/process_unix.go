//go:build !windows

package coordinator

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup configures a command to run in its own process group.
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup kills the entire process group rooted at pid.
func KillProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
