//go:build windows

package coordinator

import (
	"os"
	"os/exec"
)

// SetProcessGroup is a no-op on Windows.
func SetProcessGroup(cmd *exec.Cmd) {}

// KillProcessGroup falls back to killing a single process on Windows.
func KillProcessGroup(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
