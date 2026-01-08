//go:build !windows

package runner

import (
	"fmt"
	"os/exec"
	"strconv"
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

// ProcessMetrics holds CPU and memory metrics for a process
type ProcessMetrics struct {
	CPU    float64 // CPU percentage
	Memory uint64  // Memory in bytes (RSS)
}

// GetProcessMetrics gets CPU and memory usage for a process and its children
// Uses ps command which works on both macOS and Linux
func GetProcessMetrics(pid int) (ProcessMetrics, error) {
	if pid <= 0 {
		return ProcessMetrics{}, nil
	}

	// Get all child PIDs using pgrep
	pids := []int{pid}
	pgrepCmd := exec.Command("pgrep", "-P", strconv.Itoa(pid))
	if output, err := pgrepCmd.Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if line != "" {
				if childPid, err := strconv.Atoi(line); err == nil {
					pids = append(pids, childPid)
				}
			}
		}
	}

	var totalCPU float64
	var totalMemory uint64

	for _, p := range pids {
		// ps -o %cpu=,rss= -p <pid>
		// %cpu = CPU percentage, rss = resident set size in KB
		cmd := exec.Command("ps", "-o", "%cpu=,rss=", "-p", strconv.Itoa(p))
		output, err := cmd.Output()
		if err != nil {
			continue // Process might have exited
		}

		fields := strings.Fields(strings.TrimSpace(string(output)))
		if len(fields) >= 2 {
			if cpu, err := strconv.ParseFloat(fields[0], 64); err == nil {
				totalCPU += cpu
			}
			if rssKB, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				totalMemory += rssKB * 1024 // Convert KB to bytes
			}
		}
	}

	return ProcessMetrics{
		CPU:    totalCPU,
		Memory: totalMemory,
	}, nil
}
