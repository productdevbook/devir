package mcp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"devir/internal/config"
	"devir/internal/daemon"
)

// Server holds the MCP server and daemon client
type Server struct {
	server  *mcp.Server
	client  *daemon.Client
	cfg     *config.Config
	version string
}

// NewWithClient creates a new MCP server with daemon client
func NewWithClient(cfg *config.Config, client *daemon.Client, version string) *Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "devir",
			Version: version,
		},
		nil,
	)

	mcpServer := &Server{
		server:  server,
		client:  client,
		cfg:     cfg,
		version: version,
	}

	mcpServer.registerTools()

	return mcpServer
}

func (m *Server) registerTools() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_check_ports",
		Description: "Check if any service ports are already in use. Call this before starting services.",
	}, m.handleCheckPorts)

	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_kill_ports",
		Description: "Kill processes using the specified ports",
	}, m.handleKillPorts)

	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_start",
		Description: "Start dev services. If no services specified, starts all default services. Use killPorts:true to auto-kill processes on conflicting ports.",
	}, m.handleStart)

	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_stop",
		Description: "Stop all running services",
	}, m.handleStop)

	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_status",
		Description: "Get status of all services including running state and ports",
	}, m.handleStatus)

	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_logs",
		Description: "Get recent logs from services",
	}, m.handleLogs)

	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_restart",
		Description: "Restart a specific service",
	}, m.handleRestart)
}

// Run starts the MCP server
func (m *Server) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		_ = m.client.Stop()
		cancel()
	}()

	return m.server.Run(ctx, &mcp.StdioTransport{})
}

// Input/Output types

type StartInput struct {
	Services  []string `json:"services,omitempty" jsonschema:"List of services to start. If empty starts all defaults."`
	KillPorts bool     `json:"killPorts,omitempty" jsonschema:"If true, automatically kill processes using conflicting ports before starting."`
}

type PortInfo struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
	InUse   bool   `json:"inUse"`
}

type CheckPortsOutput struct {
	Ports       []PortInfo `json:"ports"`
	HasConflict bool       `json:"hasConflict"`
}

type KillPortsInput struct {
	Ports []int `json:"ports" jsonschema:"List of ports to kill processes on,required"`
}

type KillPortsOutput struct {
	Killed []int `json:"killed"`
	Failed []int `json:"failed"`
}

type StartOutput struct {
	Status   string   `json:"status"`
	Services []string `json:"services"`
}

type StopOutput struct {
	Status string `json:"status"`
}

type ServiceStatus struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Port     int    `json:"port"`
	Type     string `json:"type"`     // service, oneshot, interval, http
	Status   string `json:"status"`   // running, completed, failed, waiting, stopped
	LastRun  string `json:"lastRun"`  // ISO timestamp
	NextRun  string `json:"nextRun"`  // ISO timestamp (for interval)
	ExitCode int    `json:"exitCode"` // last exit code
	RunCount int    `json:"runCount"` // number of runs
}

type StatusOutput struct {
	Services []ServiceStatus `json:"services"`
}

type LogsInput struct {
	Service string `json:"service,omitempty" jsonschema:"Service name to get logs from. If empty returns all logs."`
	Lines   int    `json:"lines,omitempty" jsonschema:"Number of log lines to return. Default 100."`
}

type LogEntry struct {
	Service string `json:"service"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type LogsOutput struct {
	Logs []LogEntry `json:"logs"`
}

type RestartInput struct {
	Service string `json:"service" jsonschema:"Service name to restart,required"`
}

type RestartOutput struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Handlers

func (m *Server) handleCheckPorts(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, CheckPortsOutput, error) {
	resp, err := m.client.CheckPortsSync(5 * time.Second)
	if err != nil {
		return nil, CheckPortsOutput{}, err
	}

	var ports []PortInfo
	for _, p := range resp.Ports {
		ports = append(ports, PortInfo{
			Service: p.Service,
			Port:    p.Port,
			InUse:   p.InUse,
		})
	}

	return nil, CheckPortsOutput{
		Ports:       ports,
		HasConflict: resp.HasConflict,
	}, nil
}

func (m *Server) handleKillPorts(ctx context.Context, req *mcp.CallToolRequest, input KillPortsInput) (*mcp.CallToolResult, KillPortsOutput, error) {
	resp, err := m.client.KillPortsSync(input.Ports, 5*time.Second)
	if err != nil {
		return nil, KillPortsOutput{}, err
	}

	return nil, KillPortsOutput{
		Killed: resp.Killed,
		Failed: resp.Failed,
	}, nil
}

func (m *Server) handleStart(ctx context.Context, req *mcp.CallToolRequest, input StartInput) (*mcp.CallToolResult, StartOutput, error) {
	services := input.Services
	if len(services) == 0 {
		services = m.cfg.Defaults
	}

	for _, name := range services {
		if _, ok := m.cfg.Services[name]; !ok {
			return nil, StartOutput{}, fmt.Errorf("unknown service: %s", name)
		}
	}

	started, err := m.client.StartAndWait(services, input.KillPorts, 10*time.Second)
	if err != nil {
		return nil, StartOutput{}, err
	}

	return nil, StartOutput{
		Status:   "started",
		Services: started,
	}, nil
}

func (m *Server) handleStop(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StopOutput, error) {
	if err := m.client.Stop(); err != nil {
		return nil, StopOutput{}, err
	}
	return nil, StopOutput{Status: "stopped"}, nil
}

func (m *Server) handleStatus(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StatusOutput, error) {
	statuses, err := m.client.StatusSync(5 * time.Second)
	if err != nil {
		return nil, StatusOutput{Services: []ServiceStatus{}}, err
	}

	result := make([]ServiceStatus, 0, len(statuses))
	for _, s := range statuses {
		result = append(result, ServiceStatus{
			Name:     s.Name,
			Running:  s.Running,
			Port:     s.Port,
			Type:     s.Type,
			Status:   s.Status,
			LastRun:  s.LastRun,
			NextRun:  s.NextRun,
			ExitCode: s.ExitCode,
			RunCount: s.RunCount,
		})
	}

	return nil, StatusOutput{Services: result}, nil
}

func (m *Server) handleLogs(ctx context.Context, req *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, LogsOutput, error) {
	lines := input.Lines
	if lines <= 0 {
		lines = 100
	}

	logs, err := m.client.LogsSync(input.Service, lines, 5*time.Second)
	if err != nil {
		return nil, LogsOutput{}, err
	}

	var result []LogEntry
	for _, l := range logs {
		result = append(result, LogEntry{
			Service: l.Service,
			Level:   l.Level,
			Message: l.Message,
		})
	}

	return nil, LogsOutput{Logs: result}, nil
}

func (m *Server) handleRestart(ctx context.Context, req *mcp.CallToolRequest, input RestartInput) (*mcp.CallToolResult, RestartOutput, error) {
	if input.Service == "" {
		return nil, RestartOutput{}, fmt.Errorf("service name is required")
	}

	if err := m.client.Restart(input.Service); err != nil {
		return nil, RestartOutput{}, err
	}

	return nil, RestartOutput{
		Status:  "restarted",
		Service: input.Service,
	}, nil
}
