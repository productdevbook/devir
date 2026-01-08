package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"devir/internal/config"
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

	// MCP mode
	if mcpMode {
		mcpServer := mcp.New(cfg, Version)
		if err := mcpServer.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// TUI mode
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

	// Create runner
	r := runner.New(cfg, services, filter, exclude)

	// Check for ports in use
	portsInUse := r.CheckPorts()
	if len(portsInUse) > 0 {
		fmt.Println("\n⚠️  Aşağıdaki portlar zaten kullanımda:")
		for name, port := range portsInUse {
			fmt.Printf("   • %s: port %d\n", name, port)
		}
		fmt.Print("\nBu portları kapatıp devam edilsin mi? [y/N] ")

		var answer string
		_, _ = fmt.Scanln(&answer)

		if answer == "y" || answer == "Y" {
			for name, port := range portsInUse {
				if err := r.KillPort(port); err != nil {
					fmt.Printf("   ✗ %s (port %d) kapatılamadı: %v\n", name, port, err)
				} else {
					fmt.Printf("   ✓ %s (port %d) kapatıldı\n", name, port)
				}
			}
			fmt.Println()
		}
	}

	// Start TUI
	p := tea.NewProgram(
		tui.New(r),
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
  -v            Show version
  -h            Show this help

Examples:
  devir                    # Start all default services
  devir admin server       # Start only admin and server
  devir --filter "error"   # Show only errors
  devir --exclude "hmr"    # Hide HMR logs

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
