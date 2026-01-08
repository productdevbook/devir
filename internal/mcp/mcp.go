package mcp

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"devir/internal/config"
	"devir/internal/runner"
)

// Server holds the MCP server and runner
type Server struct {
	server  *mcp.Server
	runner  *runner.Runner
	cfg     *config.Config
	version string
}

// New creates a new MCP server
func New(cfg *config.Config, version string) *Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "devir",
			Version: version,
		},
		nil,
	)

	// Create runner with all services from config
	var services []string
	for name := range cfg.Services {
		services = append(services, name)
	}
	r := runner.New(cfg, services, "", "")

	mcpServer := &Server{
		server:  server,
		runner:  r,
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
		m.runner.Stop()
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
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Port    int    `json:"port"`
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
	var ports []PortInfo
	hasConflict := false

	for name, svc := range m.cfg.Services {
		port := svc.Port
		if port > 0 {
			inUse := runner.IsPortInUse(port)
			if inUse {
				hasConflict = true
			}
			ports = append(ports, PortInfo{
				Service: name,
				Port:    port,
				InUse:   inUse,
			})
		}
	}

	return nil, CheckPortsOutput{
		Ports:       ports,
		HasConflict: hasConflict,
	}, nil
}

func (m *Server) handleKillPorts(ctx context.Context, req *mcp.CallToolRequest, input KillPortsInput) (*mcp.CallToolResult, KillPortsOutput, error) {
	var killed, failed []int

	for _, port := range input.Ports {
		if err := killPort(port); err != nil {
			failed = append(failed, port)
		} else {
			killed = append(killed, port)
		}
	}

	return nil, KillPortsOutput{
		Killed: killed,
		Failed: failed,
	}, nil
}

func killPort(port int) error {
	pid, err := runner.GetPortPID(port)
	if err != nil {
		return err
	}
	if pid > 0 {
		return runner.KillProcess(pid)
	}
	return nil
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

	if input.KillPorts {
		for _, name := range services {
			if svc, ok := m.cfg.Services[name]; ok && svc.Port > 0 {
				if runner.IsPortInUse(svc.Port) {
					_ = killPort(svc.Port)
				}
			}
		}
	}

	m.runner = runner.New(m.cfg, services, "", "")
	m.runner.Start()

	return nil, StartOutput{
		Status:   "started",
		Services: services,
	}, nil
}

func (m *Server) handleStop(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StopOutput, error) {
	m.runner.Stop()
	return nil, StopOutput{Status: "stopped"}, nil
}

func (m *Server) handleStatus(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StatusOutput, error) {
	var statuses []ServiceStatus

	for name, state := range m.runner.Services {
		statuses = append(statuses, ServiceStatus{
			Name:    name,
			Running: state.Running,
			Port:    state.Service.Port,
		})
	}

	return nil, StatusOutput{Services: statuses}, nil
}

func (m *Server) handleLogs(ctx context.Context, req *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, LogsOutput, error) {
	lines := input.Lines
	if lines <= 0 {
		lines = 100
	}

	var logs []LogEntry

	for name, state := range m.runner.Services {
		if input.Service != "" && name != input.Service {
			continue
		}

		startIdx := 0
		if len(state.Logs) > lines {
			startIdx = len(state.Logs) - lines
		}

		for _, log := range state.Logs[startIdx:] {
			level := "info"
			if log.IsError {
				level = "error"
			}
			logs = append(logs, LogEntry{
				Service: name,
				Level:   level,
				Message: log.Text,
			})
		}
	}

	return nil, LogsOutput{Logs: logs}, nil
}

func (m *Server) handleRestart(ctx context.Context, req *mcp.CallToolRequest, input RestartInput) (*mcp.CallToolResult, RestartOutput, error) {
	if input.Service == "" {
		return nil, RestartOutput{}, fmt.Errorf("service name is required")
	}

	if _, ok := m.runner.Services[input.Service]; !ok {
		return nil, RestartOutput{}, fmt.Errorf("unknown service: %s", input.Service)
	}

	m.runner.RestartService(input.Service)

	return nil, RestartOutput{
		Status:  "restarted",
		Service: input.Service,
	}, nil
}
