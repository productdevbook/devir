package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer holds the MCP server and runner
type MCPServer struct {
	server *mcp.Server
	runner *Runner
	cfg    *Config
}

// NewMCPServer creates a new MCP server
func NewMCPServer(cfg *Config) *MCPServer {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "devir",
			Version: "1.0.0",
		},
		nil,
	)

	// Create runner with all services from config
	var services []string
	for name := range cfg.Services {
		services = append(services, name)
	}
	runner := NewRunner(cfg, services, "", "")

	mcpServer := &MCPServer{
		server: server,
		runner: runner,
		cfg:    cfg,
	}

	// Register tools
	mcpServer.registerTools()

	return mcpServer
}

// registerTools registers all MCP tools
func (m *MCPServer) registerTools() {
	// devir_start
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_start",
		Description: "Start dev services. If no services specified, starts all default services.",
	}, m.handleStart)

	// devir_stop
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_stop",
		Description: "Stop all running services",
	}, m.handleStop)

	// devir_status
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_status",
		Description: "Get status of all services including running state and ports",
	}, m.handleStatus)

	// devir_logs
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_logs",
		Description: "Get recent logs from services",
	}, m.handleLogs)

	// devir_restart
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "devir_restart",
		Description: "Restart a specific service",
	}, m.handleRestart)
}

// Run starts the MCP server
func (m *MCPServer) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		m.runner.Stop()
		cancel()
	}()

	// Run server over stdio
	return m.server.Run(ctx, &mcp.StdioTransport{})
}

// Tool input/output types

type StartInput struct {
	Services []string `json:"services,omitempty" jsonschema:"description=List of services to start. If empty starts all defaults."`
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
	Service string `json:"service,omitempty" jsonschema:"description=Service name to get logs from. If empty returns all logs."`
	Lines   int    `json:"lines,omitempty" jsonschema:"description=Number of log lines to return. Default 100."`
}

type MCPLogEntry struct {
	Service string `json:"service"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type LogsOutput struct {
	Logs []MCPLogEntry `json:"logs"`
}

type RestartInput struct {
	Service string `json:"service" jsonschema:"description=Service name to restart,required"`
}

type RestartOutput struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Tool handlers

func (m *MCPServer) handleStart(ctx context.Context, req *mcp.CallToolRequest, input StartInput) (*mcp.CallToolResult, StartOutput, error) {
	services := input.Services
	if len(services) == 0 {
		services = m.cfg.Defaults
	}

	// Validate services
	for _, name := range services {
		if _, ok := m.cfg.Services[name]; !ok {
			return nil, StartOutput{}, fmt.Errorf("unknown service: %s", name)
		}
	}

	// Create new runner with selected services
	m.runner = NewRunner(m.cfg, services, "", "")
	m.runner.Start()

	return nil, StartOutput{
		Status:   "started",
		Services: services,
	}, nil
}

func (m *MCPServer) handleStop(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StopOutput, error) {
	m.runner.Stop()
	return nil, StopOutput{Status: "stopped"}, nil
}

func (m *MCPServer) handleStatus(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StatusOutput, error) {
	var statuses []ServiceStatus

	for name, state := range m.runner.Services {
		state.mu.Lock()
		running := state.Running
		state.mu.Unlock()

		statuses = append(statuses, ServiceStatus{
			Name:    name,
			Running: running,
			Port:    state.Service.Port,
		})
	}

	return nil, StatusOutput{Services: statuses}, nil
}

func (m *MCPServer) handleLogs(ctx context.Context, req *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, LogsOutput, error) {
	lines := input.Lines
	if lines <= 0 {
		lines = 100
	}

	var logs []MCPLogEntry

	for name, state := range m.runner.Services {
		if input.Service != "" && name != input.Service {
			continue
		}

		state.mu.Lock()
		startIdx := 0
		if len(state.Logs) > lines {
			startIdx = len(state.Logs) - lines
		}

		for _, log := range state.Logs[startIdx:] {
			level := "info"
			if log.IsError {
				level = "error"
			}
			logs = append(logs, MCPLogEntry{
				Service: name,
				Level:   level,
				Message: log.Text,
			})
		}
		state.mu.Unlock()
	}

	return nil, LogsOutput{Logs: logs}, nil
}

func (m *MCPServer) handleRestart(ctx context.Context, req *mcp.CallToolRequest, input RestartInput) (*mcp.CallToolResult, RestartOutput, error) {
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
