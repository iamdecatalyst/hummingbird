package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iamdecatalyst/hummingbird/cli/client"
	"github.com/iamdecatalyst/hummingbird/cli/tui"
)

const usageText = `Hummingbird — Pump.fun trading agent CLI
by Vylth · VYLTH Strategies

USAGE:
  hummingbird [flags] <command>
  hummingbird                    Launch interactive TUI

COMMANDS:
  login                   Save API URL + token to ~/.config/hummingbird/credentials
  logout                  Remove saved credentials
  status                  One-shot: print trading stats
  positions               One-shot: print open and recent closed positions
  logs                    One-shot: print recent log events

FLAGS:
  -url string    API base URL override (or set HUMMINGBIRD_API_URL env var)
  -token string  JWT token override (or set HUMMINGBIRD_TOKEN env var)
  -h, --help     Show this help message

EXAMPLES:
  hummingbird
  hummingbird login
  hummingbird status
  hummingbird positions
  hummingbird logs

For full documentation visit: https://hummingbird-api.vylth.com
`

func printUsage() {
	fmt.Print(usageText)
}

func main() {
	var (
		apiURL string
		token  string
		help   bool
	)

	flag.StringVar(&apiURL, "url", "", "API base URL (or set HUMMINGBIRD_API_URL)")
	flag.StringVar(&token, "token", "", "JWT token (or set HUMMINGBIRD_TOKEN)")
	flag.BoolVar(&help, "h", false, "Show help")
	flag.Usage = printUsage
	flag.Parse()

	if help {
		printUsage()
		return
	}

	args := flag.Args()

	if len(args) > 0 {
		switch args[0] {
		case "login":
			handleLogin()
			return
		case "logout":
			handleLogout()
			return
		}
	}

	c := client.New(apiURL, token)

	if len(args) == 0 {
		// Set terminal window title
		fmt.Print("\033]0;Hummingbird\007")
		p := tea.NewProgram(tui.NewModel(c), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch args[0] {
	case "status":
		output, err := tui.OverviewOnce(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)

	case "positions":
		output, err := tui.PositionsOnce(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "positions failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)

	case "logs":
		output, err := tui.LogsOnce(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logs failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)

	case "help", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "Run 'hummingbird -h' for usage.\n")
		os.Exit(1)
	}
}

// readHidden reads a line from stdin with echo disabled via stty.
// Falls back to plain reading if stty is unavailable.
func readHidden() string {
	stty := exec.Command("stty", "-echo")
	stty.Stdin = os.Stdin
	_ = stty.Run()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	sttyOn := exec.Command("stty", "echo")
	sttyOn.Stdin = os.Stdin
	_ = sttyOn.Run()
	fmt.Println()

	return input
}

func readLineRaw() string {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func handleLogin() {
	savedURL, savedToken := client.LoadCredentials()

	fmt.Println()
	fmt.Println("  ◈ Hummingbird Login")
	fmt.Println()

	// API URL
	var apiURL string
	if savedURL != "" {
		fmt.Printf("  API URL [%s]: ", savedURL)
		apiURL = readLineRaw()
		if apiURL == "" {
			apiURL = savedURL
		}
	} else {
		fmt.Print("  API URL [http://localhost:8002]: ")
		apiURL = readLineRaw()
		if apiURL == "" {
			apiURL = "http://localhost:8002"
		}
	}
	apiURL = strings.TrimRight(apiURL, "/")

	// Check if multi-tenant (requires login) or single-tenant (no auth)
	c := client.New(apiURL, "")
	mode, err := c.GetMode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  ✗ Could not reach %s: %v\n", apiURL, err)
		os.Exit(1)
	}

	if !mode.MultiTenant {
		// Single-tenant: no auth needed
		if err := client.SaveCredentials(apiURL, ""); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Failed to save: %v\n", err)
			os.Exit(1)
		}
		fmt.Println()
		fmt.Println("  ✓ Single-tenant mode — no authentication required.")
		fmt.Printf("  ✓ Saved to %s\n", client.CredentialsPath())
		fmt.Println("  → Run 'hummingbird' to launch the TUI.")
		return
	}

	// Multi-tenant: open browser to /cli/auth page
	authURL := apiURL + "/cli/auth"
	// Try to derive web dashboard URL from API URL
	// e.g. http://localhost:8002 → show the web URL separately if known
	fmt.Println()
	fmt.Println("  Opening your browser to get a CLI token…")
	fmt.Printf("  %s\n", authURL)
	fmt.Println()

	openBrowser(authURL)

	// Prompt for paste
	if savedToken != "" {
		fmt.Print("  Paste token (Enter to keep existing): ")
	} else {
		fmt.Print("  Paste token: ")
	}

	tokenInput := readHidden()

	if strings.TrimSpace(tokenInput) == "" {
		if savedToken != "" {
			tokenInput = savedToken
			fmt.Println("  Keeping existing token.")
		} else {
			fmt.Fprintln(os.Stderr, "\n  ✗ No token entered. Aborted.")
			os.Exit(1)
		}
	}

	if err := client.SaveCredentials(apiURL, strings.TrimSpace(tokenInput)); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Failed to save: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("  ✓ Saved to %s\n", client.CredentialsPath())
	fmt.Println("  → Run 'hummingbird' to launch the TUI.")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func handleLogout() {
	_, token := client.LoadCredentials()
	if token == "" {
		fmt.Println("No saved credentials.")
		return
	}
	if err := client.RemoveCredentials(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove credentials: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Removed from %s\n", client.CredentialsPath())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
