package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"devir/internal/config"
	"devir/internal/runner"
	"devir/internal/types"
)

// DefaultSocketPath returns the default socket path
func DefaultSocketPath() string {
	// Use XDG_RUNTIME_DIR if available, otherwise /tmp
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "devir.sock")
	}
	return "/tmp/devir.sock"
}

// SocketPath returns the socket path for a specific config directory
func SocketPath(configDir string) string {
	// Hash config dir to create unique socket per project
	hash := simpleHash(configDir)
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, fmt.Sprintf("devir-%s.sock", hash))
	}
	return fmt.Sprintf("/tmp/devir-%s.sock", hash)
}

func simpleHash(s string) string {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("%08x", h)
}

// Exists checks if a daemon is running
func Exists(socketPath string) bool {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		// Socket file might exist but daemon not running
		_ = os.Remove(socketPath)
		return false
	}
	_ = conn.Close()
	return true
}

// Daemon manages services and client connections
type Daemon struct {
	config     *config.Config
	runner     *runner.Runner
	listener   net.Listener
	clients    map[*clientConn]bool
	clientsMu  sync.RWMutex
	socketPath string
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

type clientConn struct {
	conn   net.Conn
	sendCh chan Message
	daemon *Daemon
}

// New creates a new daemon
func New(cfg *config.Config, socketPath string) *Daemon {
	return &Daemon{
		config:     cfg,
		clients:    make(map[*clientConn]bool),
		socketPath: socketPath,
		stopCh:     make(chan struct{}),
	}
}

// Start starts the daemon
func (d *Daemon) Start() error {
	// Remove stale socket
	_ = os.Remove(d.socketPath)

	listener, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	d.listener = listener

	// Accept connections
	d.wg.Add(1)
	go d.acceptLoop()

	return nil
}

// RunEmbedded runs daemon in embedded mode (same process as TUI/MCP)
// Returns a local client connected to this daemon
func (d *Daemon) RunEmbedded() (*Client, error) {
	if err := d.Start(); err != nil {
		return nil, err
	}

	// Create local client via socket
	return Connect(d.socketPath)
}

// Stop stops the daemon
func (d *Daemon) Stop() {
	close(d.stopCh)

	if d.runner != nil {
		d.runner.Stop()
	}

	if d.listener != nil {
		_ = d.listener.Close()
	}

	d.clientsMu.Lock()
	for c := range d.clients {
		_ = c.conn.Close()
	}
	d.clientsMu.Unlock()

	d.wg.Wait()
	_ = os.Remove(d.socketPath)
}

func (d *Daemon) acceptLoop() {
	defer d.wg.Done()

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.stopCh:
				return
			default:
				continue
			}
		}

		client := &clientConn{
			conn:   conn,
			sendCh: make(chan Message, 100),
			daemon: d,
		}

		d.clientsMu.Lock()
		d.clients[client] = true
		d.clientsMu.Unlock()

		d.wg.Add(2)
		go client.readLoop()
		go client.writeLoop()
	}
}

func (c *clientConn) readLoop() {
	defer c.daemon.wg.Done()
	defer c.cleanup()

	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		c.daemon.handleMessage(c, msg)
	}
}

func (c *clientConn) writeLoop() {
	defer c.daemon.wg.Done()

	encoder := json.NewEncoder(c.conn)
	for msg := range c.sendCh {
		if err := encoder.Encode(msg); err != nil {
			return
		}
	}
}

func (c *clientConn) cleanup() {
	c.daemon.clientsMu.Lock()
	delete(c.daemon.clients, c)
	c.daemon.clientsMu.Unlock()
	close(c.sendCh)
	_ = c.conn.Close()
}

func (c *clientConn) send(msg Message) {
	select {
	case c.sendCh <- msg:
	default:
		// Drop if buffer full
	}
}

func (d *Daemon) broadcast(msg Message) {
	d.clientsMu.RLock()
	defer d.clientsMu.RUnlock()

	for c := range d.clients {
		c.send(msg)
	}
}

func (d *Daemon) handleMessage(c *clientConn, msg Message) {
	switch msg.Type {
	case MsgStart:
		d.handleStart(c, msg)
	case MsgStop:
		d.handleStop(c)
	case MsgRestart:
		d.handleRestart(c, msg)
	case MsgStatus:
		d.handleStatus(c)
	case MsgLogs:
		d.handleLogs(c, msg)
	case MsgCheckPorts:
		d.handleCheckPorts(c)
	case MsgKillPorts:
		d.handleKillPorts(c, msg)
	}
}

func (d *Daemon) handleStart(c *clientConn, msg Message) {
	req, err := ParsePayload[StartRequest](msg)
	if err != nil {
		d.sendError(c, err.Error())
		return
	}

	services := req.Services
	if len(services) == 0 {
		services = d.config.Defaults
	}

	// Validate services
	for _, name := range services {
		if _, ok := d.config.Services[name]; !ok {
			d.sendError(c, fmt.Sprintf("unknown service: %s", name))
			return
		}
	}

	// Kill ports if requested
	if req.KillPorts {
		for _, name := range services {
			if svc, ok := d.config.Services[name]; ok && svc.Port > 0 {
				if runner.IsPortInUse(svc.Port) {
					pid, _ := runner.GetPortPID(svc.Port)
					if pid > 0 {
						_ = runner.KillProcess(pid)
					}
				}
			}
		}
	}

	// Create runner and start services
	d.runner = runner.New(d.config, services, "", "")
	d.runner.StartWithChannel()

	// Forward logs to all clients
	go d.forwardLogs()

	resp, _ := NewMessage(MsgStarted, StartedResponse{Services: services})
	c.send(resp)
}

func (d *Daemon) forwardLogs() {
	if d.runner == nil {
		return
	}

	for {
		select {
		case <-d.stopCh:
			return
		case entry := <-d.runner.LogEntryChan:
			logData := LogEntryData{
				Time:    entry.Time,
				Service: entry.Service,
				Level:   entry.Level,
				Message: entry.Message,
			}
			msg, _ := NewMessage(MsgLogEntry, logData)
			d.broadcast(msg)
		}
	}
}

func (d *Daemon) handleStop(c *clientConn) {
	if d.runner != nil {
		d.runner.Stop()
	}

	resp, _ := NewMessage(MsgStopped, struct{}{})
	c.send(resp)
}

func (d *Daemon) handleRestart(c *clientConn, msg Message) {
	req, err := ParsePayload[RestartRequest](msg)
	if err != nil {
		d.sendError(c, err.Error())
		return
	}

	if d.runner == nil {
		d.sendError(c, "no services running")
		return
	}

	if _, ok := d.runner.Services[req.Service]; !ok {
		d.sendError(c, fmt.Sprintf("unknown service: %s", req.Service))
		return
	}

	d.runner.RestartService(req.Service)

	resp, _ := NewMessage(MsgRestarted, RestartedResponse(req))
	c.send(resp)
}

func (d *Daemon) handleStatus(c *clientConn) {
	var statuses []ServiceStatus

	if d.runner != nil {
		for name, state := range d.runner.Services {
			state.Mu.Lock()

			// Check for dynamic status from .devir-status file
			icon := state.Service.Icon
			color := state.Service.Color
			status := string(state.Status)
			message := ""

			if ds := d.readDynamicStatus(state); ds != nil {
				if ds.Icon != "" {
					icon = ds.Icon
				}
				if ds.Color != "" {
					color = ds.Color
				}
				if ds.Status != "" {
					status = ds.Status
				}
				message = ds.Message
			}

			s := ServiceStatus{
				Name:     name,
				Running:  state.Running,
				Port:     state.Service.Port,
				Color:    color,
				Icon:     icon,
				Type:     string(state.Service.GetEffectiveType()),
				Status:   status,
				Message:  message,
				ExitCode: state.ExitCode,
				RunCount: state.RunCount,
			}
			if !state.LastRun.IsZero() {
				s.LastRun = state.LastRun.Format(time.RFC3339)
			}
			if !state.NextRun.IsZero() {
				s.NextRun = state.NextRun.Format(time.RFC3339)
			}
			state.Mu.Unlock()
			statuses = append(statuses, s)
		}
	}

	resp, _ := NewMessage(MsgStatusResponse, StatusResponse{Services: statuses})
	c.send(resp)
}

// readDynamicStatus reads status from .devir-status file in service directory
func (d *Daemon) readDynamicStatus(state *runner.ServiceState) *types.DynamicStatus {
	statusFile := filepath.Join(d.config.RootDir, state.Service.Dir, ".devir-status")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}

	// Try JSON first
	if strings.HasPrefix(content, "{") {
		var ds types.DynamicStatus
		if err := json.Unmarshal([]byte(content), &ds); err == nil {
			return &ds
		}
	}

	// Fallback: plain text is just the icon
	if len(content) > 20 {
		content = content[:20]
	}
	return &types.DynamicStatus{Icon: content}
}

func (d *Daemon) handleLogs(c *clientConn, msg Message) {
	req, err := ParsePayload[LogsRequest](msg)
	if err != nil {
		d.sendError(c, err.Error())
		return
	}

	lines := req.Lines
	if lines <= 0 {
		lines = 100
	}

	var logs []LogEntryData

	if d.runner != nil {
		for name, state := range d.runner.Services {
			if req.Service != "" && name != req.Service {
				continue
			}

			state.Mu.Lock()
			startIdx := 0
			if len(state.Logs) > lines {
				startIdx = len(state.Logs) - lines
			}

			for _, log := range state.Logs[startIdx:] {
				level := "info"
				if log.IsError {
					level = "error"
				}
				logs = append(logs, LogEntryData{
					Time:    log.Timestamp,
					Service: name,
					Level:   level,
					Message: log.Text,
				})
			}
			state.Mu.Unlock()
		}
	}

	resp, _ := NewMessage(MsgLogsResponse, LogsResponse{Logs: logs})
	c.send(resp)
}

func (d *Daemon) handleCheckPorts(c *clientConn) {
	var ports []PortInfo
	hasConflict := false

	for name, svc := range d.config.Services {
		if svc.Port > 0 {
			inUse := runner.IsPortInUse(svc.Port)
			if inUse {
				hasConflict = true
			}
			ports = append(ports, PortInfo{
				Service: name,
				Port:    svc.Port,
				InUse:   inUse,
			})
		}
	}

	resp, _ := NewMessage(MsgPortsResponse, PortsResponse{Ports: ports, HasConflict: hasConflict})
	c.send(resp)
}

func (d *Daemon) handleKillPorts(c *clientConn, msg Message) {
	req, err := ParsePayload[KillPortsRequest](msg)
	if err != nil {
		d.sendError(c, err.Error())
		return
	}

	var killed, failed []int

	for _, port := range req.Ports {
		pid, err := runner.GetPortPID(port)
		if err != nil {
			failed = append(failed, port)
			continue
		}
		if pid > 0 {
			if err := runner.KillProcess(pid); err != nil {
				failed = append(failed, port)
			} else {
				killed = append(killed, port)
			}
		}
	}

	resp, _ := NewMessage(MsgKillResponse, KillPortsResponse{Killed: killed, Failed: failed})
	c.send(resp)
}

func (d *Daemon) sendError(c *clientConn, errMsg string) {
	resp, _ := NewMessage(MsgError, ErrorResponse{Error: errMsg})
	c.send(resp)
}

// GetRunner returns the runner (for embedded mode)
func (d *Daemon) GetRunner() *runner.Runner {
	return d.runner
}

// GetConfig returns the config
func (d *Daemon) GetConfig() *config.Config {
	return d.config
}

// StartServices starts services directly (for embedded mode without client)
func (d *Daemon) StartServices(services []string, killPorts bool) error {
	if len(services) == 0 {
		services = d.config.Defaults
	}

	for _, name := range services {
		if _, ok := d.config.Services[name]; !ok {
			return fmt.Errorf("unknown service: %s", name)
		}
	}

	if killPorts {
		for _, name := range services {
			if svc, ok := d.config.Services[name]; ok && svc.Port > 0 {
				if runner.IsPortInUse(svc.Port) {
					pid, _ := runner.GetPortPID(svc.Port)
					if pid > 0 {
						_ = runner.KillProcess(pid)
					}
				}
			}
		}
	}

	d.runner = runner.New(d.config, services, "", "")
	d.runner.StartWithChannel()
	go d.forwardLogs()

	return nil
}

// LogEntryChan returns the log entry channel for embedded mode
func (d *Daemon) LogEntryChan() <-chan types.LogEntry {
	if d.runner != nil {
		return d.runner.LogEntryChan
	}
	return nil
}
