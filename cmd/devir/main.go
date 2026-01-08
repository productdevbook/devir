package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"devir/internal/config"
	"devir/internal/daemon"
	"devir/internal/mcp"
	"devir/internal/runner"
	"devir/internal/tui"
)

// Version is set by -ldflags at build time
var Version = "dev"

var (
	configFile  string
	filter      string
	exclude     string
	showHelp    bool
	showVersion bool
	mcpMode     bool
	wsPort      int
)

func init() {
	flag.StringVar(&configFile, "c", "", "Config file path")
	flag.StringVar(&filter, "filter", "", "Filter logs by pattern")
	flag.StringVar(&exclude, "exclude", "", "Exclude logs matching pattern")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&showVersion, "v", false, "Show version")
	flag.BoolVar(&mcpMode, "mcp", false, "Run as MCP server")
	flag.IntVar(&wsPort, "ws-port", daemon.DefaultWSPort, "WebSocket server port (0 to disable)")
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Printf("devir %s\n", Version)
		return
	}

	if showHelp {
		printHelp()
		return
	}

	// Check for init subcommand
	args := flag.Args()
	if len(args) > 0 && args[0] == "init" {
		runInit()
		return
	}

	// Load config
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// Get socket path based on config directory
	socketPath := daemon.SocketPath(cfg.RootDir)

	// MCP mode
	if mcpMode {
		runMCPMode(cfg, socketPath)
		return
	}

	// TUI mode
	runTUIMode(cfg, socketPath)
}

func runMCPMode(cfg *config.Config, socketPath string) {
	// Check if daemon already exists
	if daemon.Exists(socketPath) {
		// Connect to existing daemon
		client, err := daemon.Connect(socketPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to daemon: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		mcpServer := mcp.NewWithClient(cfg, client, Version)
		if err := mcpServer.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Start new daemon + MCP
	d := daemon.NewWithWSPort(cfg, socketPath, wsPort)
	if err := d.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start daemon: %v\n", err)
		os.Exit(1)
	}
	defer d.Stop()

	// Connect as client
	client, err := daemon.Connect(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to daemon: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = client.Close() }()

	mcpServer := mcp.NewWithClient(cfg, client, Version)
	if err := mcpServer.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
		os.Exit(1)
	}
}

func runTUIMode(cfg *config.Config, socketPath string) {
	services := flag.Args()
	if len(services) == 0 {
		services = cfg.Defaults
	}

	// Validate services
	for _, name := range services {
		if _, ok := cfg.Services[name]; !ok {
			fmt.Fprintf(os.Stderr, "Unknown service: %s\n", name)
			fmt.Fprintf(os.Stderr, "Available: ")
			for k := range cfg.Services {
				fmt.Fprintf(os.Stderr, "%s ", k)
			}
			fmt.Fprintln(os.Stderr)
			os.Exit(1)
		}
	}

	// Check if daemon already exists
	if daemon.Exists(socketPath) {
		// Connect to existing daemon (services already running)
		client, err := daemon.Connect(socketPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to daemon: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		// Start TUI with client
		p := tea.NewProgram(
			tui.NewWithClient(client, services, cfg),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// No existing daemon - start new daemon + TUI
	d := daemon.NewWithWSPort(cfg, socketPath, wsPort)
	if err := d.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start daemon: %v\n", err)
		os.Exit(1)
	}
	defer d.Stop()

	// Check for ports in use
	r := runner.New(cfg, services, filter, exclude)
	portsInUse := r.CheckPorts()
	killPorts := false

	if len(portsInUse) > 0 {
		fmt.Println("\n⚠️  Aşağıdaki portlar zaten kullanımda:")
		for name, port := range portsInUse {
			fmt.Printf("   • %s: port %d\n", name, port)
		}
		fmt.Print("\nBu portları kapatıp devam edilsin mi? [y/N] ")

		var answer string
		_, _ = fmt.Scanln(&answer)

		if answer == "y" || answer == "Y" {
			killPorts = true
		}
	}

	// Connect as client
	client, err := daemon.Connect(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to daemon: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = client.Close() }()

	// Start services via daemon
	_, err = client.StartAndWait(services, killPorts, 10*1e9) // 10 seconds timeout
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start services: %v\n", err)
		os.Exit(1)
	}

	// Start TUI
	p := tea.NewProgram(
		tui.NewWithClient(client, services, cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`devir %s - Dev Runner CLI

Usage:
  devir [options] [services...]
  devir init               # Create devir.yaml

Commands:
  init          Create devir.yaml in current directory

Options:
  -c <file>     Config file path (default: devir.yaml)
  -filter <p>   Show only logs matching pattern
  -exclude <p>  Hide logs matching pattern
  -mcp          Run as MCP server (daemon mode)
  -ws-port <n>  WebSocket server port (default: 9222, 0 to disable)
  -v            Show version
  -h            Show this help

Examples:
  devir                    # Start all default services
  devir init               # Create devir.yaml
  devir admin server       # Start only admin and server
  devir --filter "error"   # Show only errors
  devir --exclude "hmr"    # Hide HMR logs

Daemon Mode:
  Multiple TUI/MCP clients can connect to same daemon.
  First instance starts daemon, others connect automatically.

Keyboard Shortcuts:
  Tab          Cycle through services
  1-9          Select specific service
  a            Show all services
  /            Search
  c            Copy logs to clipboard
  r            Restart current service
  j/k          Scroll up/down
  q            Quit
`, Version)
}

// runInit creates a devir.yaml file in the current directory
func runInit() {
	configPath := "devir.yaml"

	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(os.Stderr, "Error: %s already exists\n", configPath)
		os.Exit(1)
	}

	// Detect project structure
	services := detectServices()

	// Generate YAML content
	var sb strings.Builder
	sb.WriteString("services:\n")

	colors := []string{"blue", "green", "magenta", "cyan", "yellow", "red"}
	icons := map[string]string{
		"web":      "🌐",
		"server":   "🚀",
		"api":      "📡",
		"admin":    "👤",
		"worker":   "⚙️",
		"frontend": "🎨",
		"backend":  "🔧",
		"db":       "💾",
		"redis":    "📦",
		"queue":    "📬",
	}

	defaults := []string{}

	for i, svc := range services {
		color := colors[i%len(colors)]
		icon := icons[svc.name]
		if icon == "" {
			icon = "📦"
		}

		sb.WriteString(fmt.Sprintf("  %s:\n", svc.name))
		sb.WriteString(fmt.Sprintf("    icon: \"%s\"\n", icon))
		sb.WriteString(fmt.Sprintf("    dir: %s\n", svc.dir))
		sb.WriteString(fmt.Sprintf("    cmd: %s\n", svc.cmd))
		if svc.port > 0 {
			sb.WriteString(fmt.Sprintf("    port: %d\n", svc.port))
		}
		sb.WriteString(fmt.Sprintf("    color: %s\n", color))
		sb.WriteString("\n")

		defaults = append(defaults, svc.name)
	}

	// Add defaults
	sb.WriteString("defaults:\n")
	for _, name := range defaults {
		sb.WriteString(fmt.Sprintf("  - %s\n", name))
	}

	// Write file
	if err := os.WriteFile(configPath, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", configPath, err)
		os.Exit(1)
	}

	fmt.Printf("✓ Created %s with %d service(s)\n", configPath, len(services))
	if len(services) > 0 {
		fmt.Println("\nDetected services:")
		for _, svc := range services {
			fmt.Printf("  • %s (%s)\n", svc.name, svc.dir)
		}
	}
	fmt.Println("\nRun 'devir' to start your services!")
}

type detectedService struct {
	name string
	dir  string
	cmd  string
	port int
}

// detectServices scans the current directory for common project structures
func detectServices() []detectedService {
	var services []detectedService

	// Check current directory
	if svc := detectServiceInDir("."); svc != nil {
		svc.name = "app"
		services = append(services, *svc)
		return services
	}

	// Check common monorepo patterns
	patterns := []string{
		"apps/*",
		"packages/*",
		"services/*",
		"src/*",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.IsDir() {
				continue
			}

			if svc := detectServiceInDir(match); svc != nil {
				svc.name = filepath.Base(match)
				svc.dir = match
				services = append(services, *svc)
			}
		}
	}

	// If nothing found, create a sample
	if len(services) == 0 {
		services = append(services, detectedService{
			name: "app",
			dir:  ".",
			cmd:  "npm run dev",
			port: 3000,
		})
	}

	return services
}

// detectServiceInDir checks if a directory contains a runnable project
func detectServiceInDir(dir string) *detectedService {
	// Check package.json
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal(data, &pkg); err == nil {
			if _, ok := pkg.Scripts["dev"]; ok {
				return &detectedService{
					dir:  dir,
					cmd:  "npm run dev",
					port: 3000,
				}
			}
		}
	}

	// Check go.mod
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return &detectedService{
			dir:  dir,
			cmd:  "go run .",
			port: 8080,
		}
	}

	// Check Cargo.toml (Rust)
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return &detectedService{
			dir:  dir,
			cmd:  "cargo run",
			port: 8080,
		}
	}

	// Check requirements.txt or pyproject.toml (Python)
	if _, err := os.Stat(filepath.Join(dir, "requirements.txt")); err == nil {
		return &detectedService{
			dir:  dir,
			cmd:  "python main.py",
			port: 8000,
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return &detectedService{
			dir:  dir,
			cmd:  "python -m uvicorn main:app --reload",
			port: 8000,
		}
	}

	return nil
}
