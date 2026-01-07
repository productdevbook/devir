package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

type LogLine struct {
	Service   string
	Text      string
	Timestamp time.Time
	IsError   bool
}

type ServiceState struct {
	Name    string
	Service Service
	Cmd     *exec.Cmd
	Running bool
	Logs    []LogLine
	mu      sync.Mutex
}

type Runner struct {
	Config        *Config
	Services      map[string]*ServiceState
	ServiceOrder  []string // Ordered list of service names
	LogChan       chan LogLine
	LogEntryChan  chan LogEntry // For TUI mode
	filter        *regexp.Regexp
	exclude       *regexp.Regexp
	activeService string // Empty = all, or specific service name
	tuiMode       bool
	mu            sync.RWMutex
}

func NewRunner(cfg *Config, serviceNames []string, filterPattern, excludePattern string) *Runner {
	r := &Runner{
		Config:       cfg,
		Services:     make(map[string]*ServiceState),
		ServiceOrder: serviceNames,
		LogChan:      make(chan LogLine, 1000),
		LogEntryChan: make(chan LogEntry, 1000),
	}

	// Compile filter patterns
	if filterPattern != "" {
		r.filter, _ = regexp.Compile("(?i)" + filterPattern)
	}
	if excludePattern != "" {
		r.exclude, _ = regexp.Compile("(?i)" + excludePattern)
	}

	// Initialize service states
	for _, name := range serviceNames {
		if svc, ok := cfg.Services[name]; ok {
			r.Services[name] = &ServiceState{
				Name:    name,
				Service: svc,
				Logs:    make([]LogLine, 0, 1000),
			}
		}
	}

	return r
}

// SetActiveService sets which service logs to show (empty = all)
func (r *Runner) SetActiveService(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeService = name
}

// GetActiveService returns current active service filter
func (r *Runner) GetActiveService() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeService
}

// CycleService cycles to next service (or all)
func (r *Runner) CycleService() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.activeService == "" {
		// Currently showing all, switch to first service
		if len(r.ServiceOrder) > 0 {
			r.activeService = r.ServiceOrder[0]
		}
	} else {
		// Find current index and go to next
		for i, name := range r.ServiceOrder {
			if name == r.activeService {
				if i+1 < len(r.ServiceOrder) {
					r.activeService = r.ServiceOrder[i+1]
				} else {
					r.activeService = "" // Back to all
				}
				break
			}
		}
	}
	return r.activeService
}

func (r *Runner) Start() {
	// Start log printer for simple mode
	go r.printLogs()

	// Start all services
	for name := range r.Services {
		go r.startService(name)
	}
}

// StartWithChannel starts services in TUI mode (no console printing)
func (r *Runner) StartWithChannel() {
	r.tuiMode = true

	// Start all services
	for name := range r.Services {
		go r.startService(name)
	}
}

// CheckPorts checks if any service ports are in use and returns them
func (r *Runner) CheckPorts() map[string]int {
	inUse := make(map[string]int)
	for name, state := range r.Services {
		port := state.Service.Port
		if port > 0 && isPortInUse(port) {
			inUse[name] = port
		}
	}
	return inUse
}

// KillPort kills the process using the given port
func (r *Runner) KillPort(port int) error {
	pid, err := getPortPID(port)
	if err != nil {
		return err
	}
	if pid > 0 {
		return syscall.Kill(pid, syscall.SIGTERM)
	}
	return nil
}

func isPortInUse(port int) bool {
	pid, _ := getPortPID(port)
	return pid > 0
}

func getPortPID(port int) (int, error) {
	// Use lsof to find process using port
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil {
		return 0, nil // No process using port
	}

	// Parse PID from output
	pidStr := strings.TrimSpace(string(output))
	if pidStr == "" {
		return 0, nil
	}

	// May have multiple PIDs, take first
	lines := strings.Split(pidStr, "\n")
	if len(lines) > 0 {
		var pid int
		fmt.Sscanf(lines[0], "%d", &pid)
		return pid, nil
	}
	return 0, nil
}

func (r *Runner) Stop() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, state := range r.Services {
		r.stopService(state)
	}
}

func (r *Runner) startService(name string) {
	r.mu.RLock()
	state := r.Services[name]
	r.mu.RUnlock()

	if state == nil {
		return
	}

	svc := state.Service
	workDir := filepath.Join(r.Config.RootDir, svc.Dir)

	// Parse command
	parts := strings.Fields(svc.Cmd)
	if len(parts) == 0 {
		return
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workDir
	// CI mode to disable terminal control characters from Nuxt/Vite
	cmd.Env = append(os.Environ(),
		"CI=true",
		"TERM=dumb",
		"NO_COLOR=1",
		"FORCE_COLOR=0",
	)

	// Set process group for proper cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Create pipes
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	state.mu.Lock()
	state.Cmd = cmd
	state.Running = true
	state.mu.Unlock()

	// Start command
	if err := cmd.Start(); err != nil {
		r.LogChan <- LogLine{
			Service:   name,
			Text:      "Failed to start: " + err.Error(),
			Timestamp: time.Now(),
			IsError:   true,
		}
		return
	}

	r.LogChan <- LogLine{
		Service:   name,
		Text:      "Started (port " + formatPort(svc.Port) + ")",
		Timestamp: time.Now(),
	}

	// Read stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer
		for scanner.Scan() {
			r.processLine(name, scanner.Text(), false)
		}
	}()

	// Read stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer
		for scanner.Scan() {
			r.processLine(name, scanner.Text(), true)
		}
	}()

	// Wait for exit
	cmd.Wait()

	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()

	r.LogChan <- LogLine{
		Service:   name,
		Text:      "Stopped",
		Timestamp: time.Now(),
	}
}

func (r *Runner) stopService(state *ServiceState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Cmd != nil && state.Cmd.Process != nil {
		// Kill process group
		syscall.Kill(-state.Cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(-state.Cmd.Process.Pid, syscall.SIGKILL)
	}
}

func (r *Runner) RestartService(name string) {
	r.mu.RLock()
	state := r.Services[name]
	r.mu.RUnlock()

	if state == nil {
		return
	}

	r.stopService(state)
	time.Sleep(500 * time.Millisecond)
	go r.startService(name)
}

// ANSI escape sequence pattern
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func (r *Runner) processLine(service, text string, isError bool) {
	// Clean ANSI codes and control characters
	text = ansiPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)

	// Skip empty lines
	if text == "" {
		return
	}

	// Apply filters
	if r.exclude != nil && r.exclude.MatchString(text) {
		return
	}
	if r.filter != nil && !r.filter.MatchString(text) {
		return
	}

	// Determine log level from text
	level := "info"
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "error") || strings.Contains(lowerText, "fail") || isError {
		level = "error"
	} else if strings.Contains(lowerText, "warn") {
		level = "warn"
	} else if strings.Contains(lowerText, "debug") {
		level = "debug"
	}

	line := LogLine{
		Service:   service,
		Text:      text,
		Timestamp: time.Now(),
		IsError:   isError,
	}

	// Store in service logs
	r.mu.RLock()
	state := r.Services[service]
	r.mu.RUnlock()

	if state != nil {
		state.mu.Lock()
		state.Logs = append(state.Logs, line)
		// Keep only last 1000 lines
		if len(state.Logs) > 1000 {
			state.Logs = state.Logs[len(state.Logs)-1000:]
		}
		state.mu.Unlock()
	}

	// Send to appropriate channel
	if r.tuiMode {
		// TUI mode - send LogEntry
		entry := LogEntry{
			Time:    time.Now(),
			Level:   level,
			Service: service,
			Message: text,
		}
		select {
		case r.LogEntryChan <- entry:
		default:
			// Channel full, drop
		}
	} else {
		// Simple mode - send LogLine
		select {
		case r.LogChan <- line:
		default:
			// Channel full, drop oldest
		}
	}
}

func (r *Runner) printLogs() {
	// Color codes
	colors := map[string]string{
		"blue":    "\033[1;34m",
		"green":   "\033[1;32m",
		"yellow":  "\033[1;33m",
		"magenta": "\033[1;35m",
		"cyan":    "\033[1;36m",
		"red":     "\033[1;31m",
		"white":   "\033[1;37m",
	}
	reset := "\033[0m"
	errorColor := "\033[31m"

	for line := range r.LogChan {
		r.mu.RLock()
		state := r.Services[line.Service]
		active := r.activeService
		r.mu.RUnlock()

		// Filter by active service
		if active != "" && line.Service != active {
			continue
		}

		color := "white"
		if state != nil {
			color = state.Service.Color
		}

		c := colors[color]
		if c == "" {
			c = colors["white"]
		}

		prefix := fmt.Sprintf("%s[%s]%s", c, line.Service, reset)
		text := line.Text
		if line.IsError {
			text = errorColor + text + reset
		}

		fmt.Printf("%s %s\n", prefix, text)
	}
}

func formatPort(port int) string {
	if port == 0 {
		return "?"
	}
	return fmt.Sprintf("%d", port)
}

// ServiceInfo for TUI
type ServiceInfo struct {
	Name    string
	Color   string
	Running bool
	Logs    []LogLine
}

func (r *Runner) GetServices() map[string]ServiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]ServiceInfo)
	for name, state := range r.Services {
		state.mu.Lock()
		logs := make([]LogLine, len(state.Logs))
		copy(logs, state.Logs)
		result[name] = ServiceInfo{
			Name:    name,
			Color:   state.Service.Color,
			Running: state.Running,
			Logs:    logs,
		}
		state.mu.Unlock()
	}
	return result
}

func (r *Runner) GetServiceNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.Services {
		names = append(names, name)
	}
	return names
}
