//go:build windows

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// KillProcess kills a process by PID
func KillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// KillProcessGroup kills a process (no process groups on Windows)
func KillProcessGroup(pid int) {
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Kill()
	}
}

// ForceKillProcessGroup force kills a process (no process groups on Windows)
func ForceKillProcessGroup(pid int) {
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Kill()
	}
}

// SetSysProcAttr sets platform-specific process attributes (no-op on Windows)
func SetSysProcAttr(cmd *exec.Cmd) {
	// No process group support on Windows
}

// GetPortPID gets the PID of process using a port
func GetPortPID(port int) (int, error) {
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return 0, nil
	}

	lines := strings.Split(string(output), "\n")
	portStr := fmt.Sprintf(":%d", port)

	for _, line := range lines {
		if strings.Contains(line, portStr) && strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				var pid int
				_, _ = fmt.Sscanf(fields[4], "%d", &pid)
				return pid, nil
			}
		}
	}
	return 0, nil
}

// IsPortInUse checks if a port is in use
func IsPortInUse(port int) bool {
	pid, _ := GetPortPID(port)
	return pid > 0
}
