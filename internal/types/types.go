package types

import "time"

// ServiceStatus represents the current status of a service
type ServiceStatus string

const (
	StatusStopped   ServiceStatus = "stopped"   // Not running
	StatusRunning   ServiceStatus = "running"   // Long-running service active
	StatusCompleted ServiceStatus = "completed" // Oneshot completed successfully
	StatusFailed    ServiceStatus = "failed"    // Oneshot or interval failed
	StatusWaiting   ServiceStatus = "waiting"   // Interval waiting for next run
)

// DynamicStatus is written by services to .devir-status file
type DynamicStatus struct {
	Icon    string `json:"icon,omitempty"`    // Custom icon/emoji
	Color   string `json:"color,omitempty"`   // Override color
	Message string `json:"message,omitempty"` // Status message
	Status  string `json:"status,omitempty"`  // Override status (running, completed, failed, waiting)
}

// LogLine represents a single log line from a service
type LogLine struct {
	Service   string
	Text      string
	Timestamp time.Time
	IsError   bool
}

// LogEntry represents a structured log entry for TUI
type LogEntry struct {
	Time    time.Time
	Level   string // info, warn, error, debug
	Service string
	Message string
}

// ServiceInfo provides service status for TUI
type ServiceInfo struct {
	Name     string
	Color    string
	Icon     string // custom icon/emoji
	Running  bool
	Logs     []LogLine
	Type     string        // service, oneshot, interval, http
	Status   ServiceStatus // detailed status
	LastRun  time.Time     // last execution time
	NextRun  time.Time     // next scheduled run (for interval)
	ExitCode int           // last exit code
	RunCount int           // number of runs (for interval)
	Message  string        // dynamic status message from .devir-status
}
