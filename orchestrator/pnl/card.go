// Package pnl generates shareable PnL card images for closed positions.
// Uses wkhtmltoimage to render an HTML template → PNG.
// Install on server: sudo apt-get install -y wkhtmltopdf
package pnl

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

//go:embed assets/logo.png
var logoPNG []byte

//go:embed assets/solana.svg
var solanaSVG []byte

var logoDataURI string
var solanaDataURI string

func init() {
	logoDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(logoPNG)
	solanaDataURI = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(solanaSVG)
}

// GenerateCard renders a PnL share card as a PNG (600×400px, Chimera-style layout).
func GenerateCard(c *models.ClosedPosition) ([]byte, error) {
	html := renderCardHTML(c)

	tmpHTML := filepath.Join(os.TempDir(), fmt.Sprintf("hb-card-%d.html", time.Now().UnixNano()))
	tmpPNG := filepath.Join(os.TempDir(), fmt.Sprintf("hb-card-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpHTML)
	defer os.Remove(tmpPNG)

	if err := os.WriteFile(tmpHTML, []byte(html), 0600); err != nil {
		return nil, fmt.Errorf("write temp HTML: %w", err)
	}

	cmd := exec.Command("wkhtmltoimage",
		"--width", "800",
		"--height", "500",
		"--format", "png",
		"--quality", "95",
		"--enable-local-file-access",
		"--quiet",
		tmpHTML, tmpPNG,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("wkhtmltoimage: %w — output: %s", err, strings.TrimSpace(string(out)))
	}

	return os.ReadFile(tmpPNG)
}

func renderCardHTML(c *models.ClosedPosition) string {
	held := c.ClosedAt.Sub(c.OpenedAt).Round(time.Second)
	pnlSign := "+"
	if c.PnLSOL < 0 {
		pnlSign = ""
	}

	// Accent = profit/loss color — matches Chimera's mascot color role
	accent := "#00D26A"
	if c.PnLSOL < 0 {
		accent = "#FF3B3B"
	}

	shortMint := c.Mint
	if len(shortMint) > 10 {
		shortMint = shortMint[:10]
	}

	platform := c.Platform
	switch platform {
	case "pump_fun", "":
		platform = "pump.fun"
	case "raydium_launchlab":
		platform = "Raydium"
	case "moonshot":
		platform = "Moonshot"
	case "boop":
		platform = "boop.fun"
	}

	exitLabel := strings.ReplaceAll(strings.ToUpper(string(c.Reason)), "_", " ")
	if exitLabel == "" {
		exitLabel = "CLOSED"
	}

	closedDate := c.ClosedAt.UTC().Format("Jan 2, 15:04 UTC")

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body, html {
  width: 800px;
  height: 500px;
  background: #000000;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Inter', sans-serif;
  overflow: hidden;
}
.card {
  position: relative;
  width: 800px;
  height: 500px;
  background: #000000;
  border-radius: 16px;
  overflow: hidden;
}
/* Grid pattern */
.grid-bg {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(%s18 1px, transparent 1px),
    linear-gradient(90deg, %s18 1px, transparent 1px);
  background-size: 32px 32px;
}
/* Top accent bar */
.top-bar {
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 3px;
  background: %s;
}
/* Top gradient glow */
.top-glow {
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 96px;
  background: linear-gradient(to bottom, %s26, transparent);
}
/* Left accent bar */
.left-bar {
  position: absolute;
  top: 0; left: 0; bottom: 0;
  width: 3px;
  background: %s;
  opacity: 0.5;
}
/* Bird logo — bottom right, like mascot slot */
.mascot {
  position: absolute;
  right: -20px;
  bottom: 0;
  width: 48%%;
  height: 95%%;
  display: flex;
  align-items: flex-end;
  justify-content: flex-end;
  overflow: hidden;
  opacity: 0.22;
}
.mascot img {
  height: 100%%;
  width: auto;
  object-fit: contain;
  object-position: bottom right;
  mix-blend-mode: screen;
}
/* Content */
.content {
  position: relative;
  z-index: 10;
  padding: 44px 48px 36px;
  height: 100%%;
  display: flex;
  flex-direction: column;
}
/* Module row */
.module-name {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 3px;
  color: %s;
  margin-bottom: 4px;
}
.platform-text {
  font-size: 13px;
  color: rgba(255,255,255,0.4);
  margin-bottom: 32px;
}
/* Token + badge */
.token-row {
  display: flex;
  align-items: center;
  margin-bottom: 20px;
}
.token-name {
  font-size: 28px;
  font-weight: 900;
  color: #ffffff;
  letter-spacing: -0.5px;
  margin-right: 12px;
}
.exit-badge {
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 700;
  color: #ffffff;
  background: %s;
  letter-spacing: 0.5px;
}
/* P&L */
.pnl-label {
  font-size: 12px;
  color: rgba(255,255,255,0.4);
  letter-spacing: 3px;
  margin-bottom: 6px;
  text-transform: uppercase;
}
.pnl-sol {
  font-size: 56px;
  font-weight: 900;
  color: %s;
  letter-spacing: -2px;
  line-height: 1;
  margin-bottom: 8px;
}
.pnl-pct {
  font-size: 24px;
  font-weight: 700;
  color: %s;
  opacity: 0.75;
  margin-bottom: 32px;
}
/* Entry/Exit row */
.stats-row {
  display: flex;
  margin-bottom: auto;
}
.stat {
  margin-right: 48px;
}
.stat-label {
  font-size: 11px;
  color: rgba(255,255,255,0.4);
  letter-spacing: 2px;
  text-transform: uppercase;
  display: block;
  margin-bottom: 4px;
}
.stat-value {
  font-size: 18px;
  font-weight: 700;
  font-family: 'Courier New', monospace;
  color: #ffffff;
}
/* Footer */
.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 12px;
}
.footer-date {
  font-size: 11px;
  color: rgba(255,255,255,0.35);
}
.footer-url {
  font-size: 11px;
  color: rgba(255,255,255,0.35);
  letter-spacing: 1px;
}
</style>
</head>
<body>
<div class="card">
  <div class="grid-bg"></div>
  <div class="top-bar"></div>
  <div class="top-glow"></div>
  <div class="left-bar"></div>

  <!-- Bird logo as mascot (bottom-right, faint) -->
  <div class="mascot">
    <img src="%s" />
  </div>

  <div class="content">
    <!-- Module = HUMMINGBIRD, platform below -->
    <div class="module-name">HUMMINGBIRD</div>
    <div class="platform-text">%s · Solana</div>

    <!-- Token + exit reason badge -->
    <div class="token-row">
      <span class="token-name">%s</span>
      <span class="exit-badge">%s</span>
    </div>

    <!-- P&L -->
    <div class="pnl-label">P &amp; L</div>
    <div class="pnl-sol">%s%.4f SOL</div>
    <div class="pnl-pct">%s%.1f%%</div>

    <!-- Entry / Exit / Duration -->
    <div class="stats-row">
      <div class="stat">
        <span class="stat-label">Entry</span>
        <span class="stat-value">%.4f SOL</span>
      </div>
      <div class="stat">
        <span class="stat-label">Exit</span>
        <span class="stat-value">%.4f SOL</span>
      </div>
      <div class="stat">
        <span class="stat-label">Duration</span>
        <span class="stat-value">%s</span>
      </div>
      <div class="stat">
        <span class="stat-label">Score</span>
        <span class="stat-value">%d / 100</span>
      </div>
    </div>

    <!-- Footer -->
    <div class="footer">
      <span class="footer-date">%s</span>
      <span class="footer-url">hummingbird.vylth.com</span>
    </div>
  </div>
</div>
</body>
</html>`,
		// grid colors (accent used twice for both axes)
		accent, accent,
		// top-bar
		accent,
		// top-glow
		accent,
		// left-bar
		accent,
		// module-name color (accent = green/red)
		accent,
		// exit-badge background
		accent,
		// pnl-sol color
		accent,
		// pnl-pct color
		accent,
		// mascot logo
		logoDataURI,
		// platform
		platform,
		// token, exit label
		shortMint, exitLabel,
		// pnl sol
		pnlSign, c.PnLSOL,
		// pnl pct
		pnlSign, c.PnLPercent,
		// stats
		c.EntryAmountSOL,
		c.ExitAmountSOL,
		held.String(),
		c.Score,
		// footer date
		closedDate,
	)
}
