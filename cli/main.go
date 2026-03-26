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
by VYLTH Strategies · @iamdecatalyst

USAGE:
  hummingbird [flags] <command>
  hummingbird                    Launch interactive TUI (multi-tenant)
  hummingbird -url <url>         Launch TUI against a self-hosted instance

COMMANDS:
  login                   Authenticate via Nexus — opens browser, saves token
  logout                  Remove saved credentials
  status                  One-shot: print trading stats
  positions               One-shot: print open and recent closed positions
  logs                    One-shot: print recent log events

FLAGS:
  -url string    API base URL override (or set HUMMINGBIRD_API_URL env var)
  -token string  JWT token override (or set HUMMINGBIRD_TOKEN env var)
  -h, --help     Show this help message

SELF-HOSTED (single-tenant):
  hummingbird -url http://localhost:8002     no login needed

For full documentation visit: https://github.com/iamdecatalyst/hummingbird
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

// ANSI color helpers for the login prompt
const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cBlue   = "\033[38;2;0;168;255m"   // electric blue #00A8FF
	cNavy   = "\033[38;2;0;80;180m"    // navy blue
	cGreen  = "\033[38;2;74;222;128m"  // green #4ADE80
	cMuted  = "\033[38;2;85;85;85m"    // muted #555
	cDim    = "\033[38;2;42;42;42m"    // dim #2a2a2a
	cYellow = "\033[38;2;245;158;11m"  // yellow
	cRed    = "\033[38;2;239;68;68m"   // red
)

func cp(color, s string) string { return color + s + cReset }

func handleLogin() {
	savedURL, savedToken := client.LoadCredentials()

	fmt.Println()
	fmt.Println(cp(cBlue, cBold+"  ◈ "+cReset+cBlue+cBold+"HUMMINGBIRD"+cReset+cp(cMuted, "  ·  cli login")))
	fmt.Println(cp(cDim, "  ────────────────────────────────────────"))
	fmt.Println()

	defaultURL := "https://hummingbird.vylth.com"
	if savedURL != "" {
		defaultURL = savedURL
	}
	fmt.Printf("  %s %s[%s]%s ", cp(cMuted, "api url"), cNavy, defaultURL, cReset)
	apiURL := readLineRaw()
	if apiURL == "" {
		apiURL = defaultURL
	}
	apiURL = strings.TrimRight(apiURL, "/")

	authURL := apiURL + "/cli/auth"
	fmt.Println()
	fmt.Println("  " + cp(cGreen, "→") + cp(cMuted, "  opening browser"))
	fmt.Println("  " + cp(cBlue, authURL))
	fmt.Println()
	fmt.Println(cp(cMuted, "  log in with Nexus, copy your token, paste it below"))
	fmt.Println()
	openBrowser(authURL)

	if savedToken != "" {
		fmt.Print("  " + cp(cMuted, "token") + cp(cDim, " (enter to keep existing)") + "  ")
	} else {
		fmt.Print("  " + cp(cMuted, "token") + "  " + cp(cBlue, "▸ "))
	}
	tokenInput := readHidden()

	if strings.TrimSpace(tokenInput) == "" {
		if savedToken != "" {
			tokenInput = savedToken
			fmt.Println(cp(cMuted, "  keeping existing token"))
		} else {
			fmt.Fprintln(os.Stderr, "\n  "+cp(cRed, "✗  no token entered — aborted"))
			os.Exit(1)
		}
	}

	if err := client.SaveCredentials(apiURL, strings.TrimSpace(tokenInput)); err != nil {
		fmt.Fprintln(os.Stderr, "  "+cp(cRed, "✗  failed to save: ")+err.Error())
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("  " + cp(cGreen, "✓") + cp(cBold, "  authenticated"))
	fmt.Println(cp(cMuted, "  saved to "+client.CredentialsPath()))
	fmt.Println()
	fmt.Println("  " + cp(cBlue, "→") + "  run " + cp(cBlue, cBold+"hummingbird"+cReset) + " to launch the TUI")
	fmt.Println()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		if isWSL() {
			// wslview (from wslu) respects the Windows default browser.
			// Fall back to explorer.exe if wslview isn't installed.
			if _, err := exec.LookPath("wslview"); err == nil {
				cmd = exec.Command("wslview", url)
			} else {
				cmd = exec.Command("explorer.exe", url)
			}
		} else {
			cmd = exec.Command("xdg-open", url)
		}
	}
	_ = cmd.Start()
}

func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSLENV") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(data))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
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
