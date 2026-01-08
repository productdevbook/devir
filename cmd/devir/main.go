package main

import (
	"flag"
	"fmt"
	"os"

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
)

func init() {
	flag.StringVar(&configFile, "c", "", "Config file path")
	flag.StringVar(&filter, "filter", "", "Filter logs by pattern")
	flag.StringVar(&exclude, "exclude", "", "Exclude logs matching pattern")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&showVersion, "v", false, "Show version")
	flag.BoolVar(&mcpMode, "mcp", false, "Run as MCP server")
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
		defer client.Close()

		mcpServer := mcp.NewWithClient(cfg, client, Version)
		if err := mcpServer.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Start new daemon + MCP
	d := daemon.New(cfg, socketPath)
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
	defer client.Close()

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
		defer client.Close()

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
	d := daemon.New(cfg, socketPath)
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
	defer client.Close()

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

Options:
  -c <file>     Config file path (default: devir.yaml)
  -filter <p>   Show only logs matching pattern
  -exclude <p>  Hide logs matching pattern
  -mcp          Run as MCP server (daemon mode)
  -v            Show version
  -h            Show this help

Examples:
  devir                    # Start all default services
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
  r            Restart current service
  j/k          Scroll up/down
  q            Quit
`, Version)
}
