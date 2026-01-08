package types

import "time"

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
	Name    string
	Color   string
	Running bool
	Logs    []LogLine
}
