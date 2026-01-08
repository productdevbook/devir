package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Name        string
	Service     config.Service
	Cmd         *exec.Cmd
	Running     bool
	Logs        []types.LogLine
	Mu          sync.Mutex // Exported for daemon access
	Status      types.ServiceStatus
	LastRun     time.Time
	NextRun     time.Time
	ExitCode    int
	RunCount    int
	ticker      *time.Ticker
	stopChan    chan struct{}
	DynamicIcon string // Icon from .devir-status file
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
				Name:     name,
				Service:  svc,
				Logs:     make([]types.LogLine, 0, 1000),
				Status:   types.StatusStopped,
				stopChan: make(chan struct{}),
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

	// Dispatch based on service type
	switch state.Service.Type {
	case config.ServiceTypeHTTP:
		r.startHTTPService(name, state)
	case config.ServiceTypeInterval:
		r.startIntervalService(name, state)
	case config.ServiceTypeOneshot:
		r.startOneshotService(name, state)
	default:
		r.startLongRunningService(name, state)
	}
}

// startLongRunningService starts a continuously running service
func (r *Runner) startLongRunningService(name string, state *ServiceState) {
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

	state.Mu.Lock()
	state.Cmd = cmd
	state.Running = true
	state.Status = types.StatusRunning
	state.Mu.Unlock()

	if err := cmd.Start(); err != nil {
		state.Mu.Lock()
		state.Status = types.StatusFailed
		state.Mu.Unlock()
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

	state.Mu.Lock()
	state.Running = false
	state.Status = types.StatusStopped
	state.Mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      "Stopped",
		Timestamp: time.Now(),
	}
}

// startOneshotService runs a command once and exits
func (r *Runner) startOneshotService(name string, state *ServiceState) {
	svc := state.Service
	workDir := filepath.Join(r.Config.RootDir, svc.Dir)

	parts := strings.Fields(svc.Cmd)
	if len(parts) == 0 {
		return
	}

	state.Mu.Lock()
	state.Running = true
	state.Status = types.StatusRunning
	state.LastRun = time.Now()
	state.RunCount++
	state.Mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      "[oneshot] Running...",
		Timestamp: time.Now(),
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

	state.Mu.Lock()
	state.Cmd = cmd
	state.Mu.Unlock()

	if err := cmd.Start(); err != nil {
		state.Mu.Lock()
		state.Running = false
		state.Status = types.StatusFailed
		state.ExitCode = -1
		state.Mu.Unlock()
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      "[oneshot] Failed: " + err.Error(),
			Timestamp: time.Now(),
			IsError:   true,
		}
		return
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

	err := cmd.Wait()

	state.Mu.Lock()
	state.Running = false
	if err != nil {
		state.Status = types.StatusFailed
		if exitErr, ok := err.(*exec.ExitError); ok {
			state.ExitCode = exitErr.ExitCode()
		} else {
			state.ExitCode = -1
		}
		state.Mu.Unlock()
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      fmt.Sprintf("[oneshot] Failed (exit %d)", state.ExitCode),
			Timestamp: time.Now(),
			IsError:   true,
		}
	} else {
		state.Status = types.StatusCompleted
		state.ExitCode = 0
		state.Mu.Unlock()
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      "[oneshot] Completed",
			Timestamp: time.Now(),
		}
	}
}

// startIntervalService runs a command at regular intervals
func (r *Runner) startIntervalService(name string, state *ServiceState) {
	svc := state.Service

	state.Mu.Lock()
	state.Running = true
	state.Status = types.StatusWaiting
	state.ticker = time.NewTicker(svc.Interval)
	state.NextRun = time.Now()
	state.Mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      fmt.Sprintf("[interval] Started (every %s)", svc.Interval),
		Timestamp: time.Now(),
	}

	// Run immediately first
	r.runIntervalCommand(name, state)

	for {
		select {
		case <-state.ticker.C:
			r.runIntervalCommand(name, state)
		case <-state.stopChan:
			state.Mu.Lock()
			state.Running = false
			state.Status = types.StatusStopped
			if state.ticker != nil {
				state.ticker.Stop()
			}
			state.Mu.Unlock()
			r.LogChan <- types.LogLine{
				Service:   name,
				Text:      "[interval] Stopped",
				Timestamp: time.Now(),
			}
			return
		}
	}
}

func (r *Runner) runIntervalCommand(name string, state *ServiceState) {
	svc := state.Service
	workDir := filepath.Join(r.Config.RootDir, svc.Dir)

	parts := strings.Fields(svc.Cmd)
	if len(parts) == 0 {
		return
	}

	state.Mu.Lock()
	state.LastRun = time.Now()
	state.NextRun = time.Now().Add(svc.Interval)
	state.RunCount++
	runCount := state.RunCount
	state.Mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      fmt.Sprintf("[interval #%d] Running...", runCount),
		Timestamp: time.Now(),
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"CI=true",
		"TERM=dumb",
		"NO_COLOR=1",
		"FORCE_COLOR=0",
	)

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if line != "" {
				r.processLine(name, line, err != nil)
			}
		}
	}

	state.Mu.Lock()
	if err != nil {
		state.Status = types.StatusFailed
		if exitErr, ok := err.(*exec.ExitError); ok {
			state.ExitCode = exitErr.ExitCode()
		}
	} else {
		state.Status = types.StatusWaiting
		state.ExitCode = 0
	}
	state.Mu.Unlock()
}

// startHTTPService makes HTTP requests
func (r *Runner) startHTTPService(name string, state *ServiceState) {
	svc := state.Service

	state.Mu.Lock()
	state.Running = true
	state.Status = types.StatusRunning
	state.LastRun = time.Now()
	state.RunCount++
	state.Mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      fmt.Sprintf("[http] %s %s", svc.Method, svc.URL),
		Timestamp: time.Now(),
	}

	var bodyReader io.Reader
	if svc.Body != "" {
		bodyReader = bytes.NewBufferString(svc.Body)
	}

	req, err := http.NewRequest(svc.Method, svc.URL, bodyReader)
	if err != nil {
		state.Mu.Lock()
		state.Running = false
		state.Status = types.StatusFailed
		state.Mu.Unlock()
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      "[http] Request error: " + err.Error(),
			Timestamp: time.Now(),
			IsError:   true,
		}
		return
	}

	// Set default headers
	if svc.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set custom headers
	for _, h := range svc.Headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		state.Mu.Lock()
		state.Running = false
		state.Status = types.StatusFailed
		state.Mu.Unlock()
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      "[http] Failed: " + err.Error(),
			Timestamp: time.Now(),
			IsError:   true,
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := strings.TrimSpace(string(body))
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}

	isError := resp.StatusCode >= 400
	state.Mu.Lock()
	state.Running = false
	state.ExitCode = resp.StatusCode
	if isError {
		state.Status = types.StatusFailed
	} else {
		state.Status = types.StatusCompleted
	}
	state.Mu.Unlock()

	r.LogChan <- types.LogLine{
		Service:   name,
		Text:      fmt.Sprintf("[http] %d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
		Timestamp: time.Now(),
		IsError:   isError,
	}

	if bodyStr != "" {
		r.LogChan <- types.LogLine{
			Service:   name,
			Text:      bodyStr,
			Timestamp: time.Now(),
			IsError:   isError,
		}
	}
}

func (r *Runner) stopService(state *ServiceState) {
	// Handle interval services with stopChan
	if state.Service.Type == config.ServiceTypeInterval {
		select {
		case state.stopChan <- struct{}{}:
		default:
		}
		return
	}

	state.Mu.Lock()
	defer state.Mu.Unlock()

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

// ClearLogs clears logs for a specific service or all services
func (r *Runner) ClearLogs(service string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, state := range r.Services {
		if service != "" && name != service {
			continue
		}
		state.Mu.Lock()
		state.Logs = nil
		state.Mu.Unlock()
	}
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
		state.Mu.Lock()
		state.Logs = append(state.Logs, line)
		if len(state.Logs) > 1000 {
			state.Logs = state.Logs[len(state.Logs)-1000:]
		}
		state.Mu.Unlock()
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
		state.Mu.Lock()
		logs := make([]types.LogLine, len(state.Logs))
		copy(logs, state.Logs)

		// Check for dynamic status from .devir-status file
		icon := state.Service.Icon
		color := state.Service.Color
		status := state.Status
		message := ""

		if ds := r.readDynamicStatus(state); ds != nil {
			if ds.Icon != "" {
				icon = ds.Icon
			}
			if ds.Color != "" {
				color = ds.Color
			}
			if ds.Status != "" {
				status = types.ServiceStatus(ds.Status)
			}
			message = ds.Message
		}

		result[name] = types.ServiceInfo{
			Name:     name,
			Color:    color,
			Icon:     icon,
			Running:  state.Running,
			Logs:     logs,
			Type:     string(state.Service.GetEffectiveType()),
			Status:   status,
			LastRun:  state.LastRun,
			NextRun:  state.NextRun,
			ExitCode: state.ExitCode,
			RunCount: state.RunCount,
			Message:  message,
		}
		state.Mu.Unlock()
	}
	return result
}

// readDynamicStatus reads status from .devir-status file in service directory
// Supports both plain text (just icon) and JSON format
func (r *Runner) readDynamicStatus(state *ServiceState) *types.DynamicStatus {
	statusFile := filepath.Join(r.Config.RootDir, state.Service.Dir, ".devir-status")
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
