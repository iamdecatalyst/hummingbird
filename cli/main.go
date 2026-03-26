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
  hummingbird              Launch interactive TUI
  hummingbird <command>    Run a one-shot command

COMMANDS:
  login       Authenticate — opens browser, saves token
  logout      Remove saved credentials
  status      One-shot: print trading stats
  positions   One-shot: print open and recent closed positions
  logs        One-shot: print recent log events

Get started:
  hummingbird login        → authenticate with Nexus
  hummingbird              → launch TUI

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

	c, notLoggedIn := client.New(apiURL, token)

	if notLoggedIn {
		fmt.Fprintln(os.Stderr, "\n  "+cp(cRed, "✗  not logged in"))
		fmt.Fprintln(os.Stderr, "  "+cp(cMuted, "run ")+cp(cBlue, cBold+"hummingbird login"+cReset)+cp(cMuted, " to authenticate"))
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

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

// readHidden reads a token from stdin, printing * for each character typed/pasted.
// Falls back to silent read if stty raw mode is unavailable.
func readHidden() string {
	// Try interactive mode: raw + no echo so we can print * ourselves
	rawOn := exec.Command("stty", "raw", "-echo")
	rawOn.Stdin = os.Stdin
	if err := rawOn.Run(); err != nil {
		return readHiddenSilent()
	}
	defer func() {
		restore := exec.Command("stty", "-raw", "echo")
		restore.Stdin = os.Stdin
		_ = restore.Run()
	}()

	var buf []byte
	b := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(b)
		if err != nil {
			break
		}
		switch b[0] {
		case '\r', '\n':
			fmt.Print("\r\n")
			return strings.TrimSpace(string(buf))
		case 127, 8: // backspace / delete
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Print("\b \b")
			}
		case 3: // Ctrl+C
			fmt.Print("\r\n")
			os.Exit(1)
		default:
			if b[0] >= 32 && b[0] < 127 {
				buf = append(buf, b[0])
				fmt.Print("*")
			}
		}
	}
	return strings.TrimSpace(string(buf))
}

// readHiddenSilent is the fallback when stty raw mode is unavailable.
func readHiddenSilent() string {
	noEcho := exec.Command("stty", "-echo")
	noEcho.Stdin = os.Stdin
	_ = noEcho.Run()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())
	echo := exec.Command("stty", "echo")
	echo.Stdin = os.Stdin
	_ = echo.Run()
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

const (
	hostedAPIURL  = "https://hummingbird-api.vylth.com"
	hostedAuthURL = "https://hummingbird.vylth.com/cli/auth"
)

func handleLogin() {
	_, savedToken := client.LoadCredentials()

	fmt.Println()
	fmt.Println(cp(cBlue, cBold+"  ◈ "+cReset+cBlue+cBold+"HUMMINGBIRD"+cReset+cp(cMuted, "  ·  cli login")))
	fmt.Println(cp(cDim, "  ────────────────────────────────────────"))
	fmt.Println()
	fmt.Println("  " + cp(cGreen, "→") + cp(cMuted, "  opening browser"))
	fmt.Println("  " + cp(cBlue, hostedAuthURL))
	fmt.Println()
	fmt.Println(cp(cMuted, "  log in with Nexus, copy your token, paste it below"))
	fmt.Println()
	openBrowser(hostedAuthURL)

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
	} else {
		dots := strings.Repeat("●", min(len(strings.TrimSpace(tokenInput))/8, 12))
		if dots == "" {
			dots = "●"
		}
		fmt.Println("  " + cp(cGreen, "✓") + "  " + cp(cDim, dots) + cp(cMuted, fmt.Sprintf("  %d chars", len(strings.TrimSpace(tokenInput)))))
	}

	if err := client.SaveCredentials(hostedAPIURL, strings.TrimSpace(tokenInput)); err != nil {
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
