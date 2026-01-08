//go:build !windows

package runner

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// KillProcess kills a process by PID
func KillProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// KillProcessGroup kills a process group
func KillProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

// ForceKillProcessGroup force kills a process group
func ForceKillProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// SetSysProcAttr sets platform-specific process attributes
func SetSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// GetPortPID gets the PID of process using a port
func GetPortPID(port int) (int, error) {
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil {
		return 0, nil
	}

	pidStr := strings.TrimSpace(string(output))
	if pidStr == "" {
		return 0, nil
	}

	lines := strings.Split(pidStr, "\n")
	if len(lines) > 0 {
		var pid int
		_, _ = fmt.Sscanf(lines[0], "%d", &pid)
		return pid, nil
	}
	return 0, nil
}

// IsPortInUse checks if a port is in use
func IsPortInUse(port int) bool {
	pid, _ := GetPortPID(port)
	return pid > 0
}
