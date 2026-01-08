package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"devir/internal/config"
	"devir/internal/types"
)

// ServiceState holds the state of a running service
type ServiceState struct {
	Name    string
	Service config.Service
	Cmd     *exec.Cmd
	Running bool
	Logs    []types.LogLine
	mu      sync.Mutex
}

// Runner manages multiple services
type Runner struct {
	Config        *config.Config
	Services      map[string]*ServiceState
	ServiceOrder  []string // Ordered list of service names
	LogChan       chan types.LogLine
	LogEntryChan  chan types.LogEntry // For TUI mode
	filter        *regexp.Regexp
	exclude       *regexp.Regexp
	activeService string // Empty = all, or specific service name
	tuiMode       bool
	mu            sync.RWMutex
}

// New creates a new Runner
func New(cfg *config.Config, serviceNames []string, filterPattern, excludePattern string) *Runner {
	r := &Runner{
		Config:       cfg,
		Services:     make(map[string]*ServiceState),
		ServiceOrder: serviceNames,
		LogChan:      make(chan types.LogLine, 1000),
		LogEntryChan: make(chan types.LogEntry, 1000),
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
				Logs:    make([]types.LogLine, 0, 1000),
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
		if len(r.ServiceOrder) > 0 {
			r.activeService = r.ServiceOrder[0]
		}
	} else {
		for i, name := range r.ServiceOrder {
			if name == r.activeService {
				if i+1 < len(r.ServiceOrder) {
					r.activeService = r.ServiceOrder[i+1]
				} else {
					r.activeService = ""
				}
				break
			}
		}
	}
	return r.activeService
}

// Start starts all services in simple mode
func (r *Runner) Start() {
	go r.printLogs()
	for name := range r.Services {
		go r.startService(name)
	}
}

// StartWithChannel starts services in TUI mode
func (r *Runner) StartWithChannel() {
	r.tuiMode = true
	for name := range r.Services {
		go r.startService(name)
	}
}

// CheckPorts checks if any service ports are in use
func (r *Runner) CheckPorts() map[string]int {
	inUse := make(map[string]int)
	for name, state := range r.Services {
		port := state.Service.Port
		if port > 0 && IsPortInUse(port) {
			inUse[name] = port
		}
	}
	return inUse
}

// KillPort kills the process using the given port
func (r *Runner) KillPort(port int) error {
	pid, err := GetPortPID(port)
	if err != nil {
		return err
	}
	if pid > 0 {
		return KillProcess(pid)
	}
	return nil
}

// Stop stops all services
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

	parts := strings.Fields(svc.Cmd)
	if len(parts) == 0 {
		return
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"CI=true",
		"TERM=dumb",
		"NO_COLOR=1",
		"FORCE_COLOR=0",
	)

	SetSysProcAttr(cmd)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	state.mu.Lock()
	state.Cmd = cmd
	state.Running = true
	state.mu.Unlock()

	if err := cmd.Start(); err != nil {
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      "Failed to start: " + err.Error(),
			Timestamp: time.Now(),
			IsError:   true,
		}
		return
	}

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      "Started (port " + formatPort(svc.Port) + ")",
		Timestamp: time.Now(),
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			r.processLine(name, scanner.Text(), false)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			r.processLine(name, scanner.Text(), true)
		}
	}()

	_ = cmd.Wait()

	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      "Stopped",
		Timestamp: time.Now(),
	}
}

func (r *Runner) stopService(state *ServiceState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Cmd != nil && state.Cmd.Process != nil {
		KillProcessGroup(state.Cmd.Process.Pid)
		time.Sleep(100 * time.Millisecond)
		ForceKillProcessGroup(state.Cmd.Process.Pid)
	}
}

// RestartService restarts a specific service
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

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func (r *Runner) processLine(service, text string, isError bool) {
	text = ansiPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)

	if text == "" {
		return
	}

	if r.exclude != nil && r.exclude.MatchString(text) {
		return
	}
	if r.filter != nil && !r.filter.MatchString(text) {
		return
	}

	level := "info"
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "error") || strings.Contains(lowerText, "fail") || isError {
		level = "error"
	} else if strings.Contains(lowerText, "warn") {
		level = "warn"
	} else if strings.Contains(lowerText, "debug") {
		level = "debug"
	}

	line := types.LogLine{
		Service:   service,
		Text:      text,
		Timestamp: time.Now(),
		IsError:   isError,
	}

	r.mu.RLock()
	state := r.Services[service]
	r.mu.RUnlock()

	if state != nil {
		state.mu.Lock()
		state.Logs = append(state.Logs, line)
		if len(state.Logs) > 1000 {
			state.Logs = state.Logs[len(state.Logs)-1000:]
		}
		state.mu.Unlock()
	}

	if r.tuiMode {
		entry := types.LogEntry{
			Time:    time.Now(),
			Level:   level,
			Service: service,
			Message: text,
		}
		select {
		case r.LogEntryChan <- entry:
		default:
		}
	} else {
		select {
		case r.LogChan <- line:
		default:
		}
	}
}

func (r *Runner) printLogs() {
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

// GetServices returns service info for TUI
func (r *Runner) GetServices() map[string]types.ServiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]types.ServiceInfo)
	for name, state := range r.Services {
		state.mu.Lock()
		logs := make([]types.LogLine, len(state.Logs))
		copy(logs, state.Logs)
		result[name] = types.ServiceInfo{
			Name:    name,
			Color:   state.Service.Color,
			Running: state.Running,
			Logs:    logs,
		}
		state.mu.Unlock()
	}
	return result
}

// GetServiceNames returns list of service names
func (r *Runner) GetServiceNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.Services {
		names = append(names, name)
	}
	return names
}
