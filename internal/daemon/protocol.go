package daemon

import (
	"encoding/json"
	"time"
)

// Message types
const (
	// Client → Daemon
	MsgStart      = "start"
	MsgStop       = "stop"
	MsgRestart    = "restart"
	MsgStatus     = "status"
	MsgLogs       = "logs"
	MsgCheckPorts = "check_ports"
	MsgKillPorts  = "kill_ports"

	// Daemon → Client
	MsgStarted        = "started"
	MsgStopped        = "stopped"
	MsgRestarted      = "restarted"
	MsgStatusResponse = "status_response"
	MsgLogsResponse   = "logs_response"
	MsgPortsResponse  = "ports_response"
	MsgKillResponse   = "kill_response"
	MsgLogEntry       = "log_entry" // Broadcast to all clients
	MsgError          = "error"
)

// Message is the wire format for daemon communication
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewMessage creates a message with typed payload
func NewMessage[T any](msgType string, payload T) (Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Message{}, err
	}
	return Message{Type: msgType, Payload: data}, nil
}

// ParsePayload decodes message payload into typed struct
func ParsePayload[T any](msg Message) (T, error) {
	var result T
	if len(msg.Payload) == 0 {
		return result, nil
	}
	err := json.Unmarshal(msg.Payload, &result)
	return result, err
}

// --- Request payloads (Client → Daemon) ---

// StartRequest requests starting services
type StartRequest struct {
	Services  []string `json:"services,omitempty"`
	KillPorts bool     `json:"killPorts,omitempty"`
}

// RestartRequest requests restarting a service
type RestartRequest struct {
	Service string `json:"service"`
}

// LogsRequest requests logs from services
type LogsRequest struct {
	Service string `json:"service,omitempty"`
	Lines   int    `json:"lines,omitempty"`
}

// KillPortsRequest requests killing processes on ports
type KillPortsRequest struct {
	Ports []int `json:"ports"`
}

// --- Response payloads (Daemon → Client) ---

// StartedResponse confirms services started
type StartedResponse struct {
	Services []string `json:"services"`
}

// RestartedResponse confirms service restarted
type RestartedResponse struct {
	Service string `json:"service"`
}

// ServiceStatus represents a service's current state
type ServiceStatus struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Port     int    `json:"port"`
	Color    string `json:"color"`
	Icon     string `json:"icon"`     // custom icon/emoji
	Type     string `json:"type"`     // service, oneshot, interval, http
	Status   string `json:"status"`   // running, completed, failed, waiting, stopped
	Message  string `json:"message"`  // dynamic status message
	LastRun  string `json:"lastRun"`  // ISO timestamp
	NextRun  string `json:"nextRun"`  // ISO timestamp (for interval)
	ExitCode int    `json:"exitCode"` // last exit code
	RunCount int    `json:"runCount"` // number of runs
}

// StatusResponse contains all service statuses
type StatusResponse struct {
	Services []ServiceStatus `json:"services"`
}

// LogEntryData is a single log entry for broadcast
type LogEntryData struct {
	Time    time.Time `json:"time"`
	Service string    `json:"service"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// LogsResponse contains requested logs
type LogsResponse struct {
	Logs []LogEntryData `json:"logs"`
}

// PortInfo represents port status
type PortInfo struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
	InUse   bool   `json:"inUse"`
}

// PortsResponse contains port check results
type PortsResponse struct {
	Ports       []PortInfo `json:"ports"`
	HasConflict bool       `json:"hasConflict"`
}

// KillPortsResponse contains kill results
type KillPortsResponse struct {
	Killed []int `json:"killed"`
	Failed []int `json:"failed"`
}

// ErrorResponse contains error details
type ErrorResponse struct {
	Error string `json:"error"`
}
