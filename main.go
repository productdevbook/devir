package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	configFile string
	filter     string
	exclude    string
	showHelp   bool
	mcpMode    bool
)

func init() {
	flag.StringVar(&configFile, "c", "", "Config file path")
	flag.StringVar(&filter, "filter", "", "Filter logs by pattern")
	flag.StringVar(&exclude, "exclude", "", "Exclude logs matching pattern")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&mcpMode, "mcp", false, "Run as MCP server")
}

func main() {
	flag.Parse()

	if showHelp {
		printHelp()
		return
	}

	// Find config file
	cfg, err := loadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// MCP mode
	if mcpMode {
		mcpServer := NewMCPServer(cfg)
		if err := mcpServer.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// TUI mode continues below...

	// Get services to run from args or defaults
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
	runner := NewRunner(cfg, services, filter, exclude)

	// Check for ports in use
	portsInUse := runner.CheckPorts()
	if len(portsInUse) > 0 {
		fmt.Println("\n⚠️  Aşağıdaki portlar zaten kullanımda:")
		for name, port := range portsInUse {
			fmt.Printf("   • %s: port %d\n", name, port)
		}
		fmt.Print("\nBu portları kapatıp devam edilsin mi? [y/N] ")

		var answer string
		fmt.Scanln(&answer)

		if answer == "y" || answer == "Y" {
			for name, port := range portsInUse {
				if err := runner.KillPort(port); err != nil {
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
		NewModel(runner),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`devir - Dev Runner CLI

Usage:
  devir [options] [services...]

Options:
  -c <file>     Config file path (default: devir.yaml)
  -filter <p>   Show only logs matching pattern
  -exclude <p>  Hide logs matching pattern
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
  q            Quit`)
}

func findConfigFile() string {
	// Look for config in current dir and parents
	dir, _ := os.Getwd()
	for {
		path := filepath.Join(dir, "devir.yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
