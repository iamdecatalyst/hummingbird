// Package pnl generates shareable PnL card images for closed positions.
// Uses wkhtmltoimage (subprocess) to render an HTML template → PNG.
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

// GenerateCard renders a PnL share card as a PNG (1200×630px).
// Returns the PNG bytes or an error if wkhtmltoimage isn't available.
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
		"--width", "1200",
		"--height", "630",
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

	pnlColor := "#00ff88"
	glowColor := "rgba(0,255,136,0.15)"
	if c.PnLSOL < 0 {
		pnlColor = "#ff4444"
		glowColor = "rgba(255,68,68,0.15)"
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

	exitReason := strings.ReplaceAll(strings.ToUpper(string(c.Reason)), "_", " ")

	heldStr := held.String()
	// Clean up duration: "6m30s" stays, "1h2m3s" stays
	_ = heldStr

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }

@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;900&display=swap');

body, html {
  width: 1200px;
  height: 630px;
  background: #080808;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  color: #ffffff;
  overflow: hidden;
}

.card {
  width: 1200px;
  height: 630px;
  background: #080808;
  position: relative;
  display: flex;
  flex-direction: column;
  padding: 48px 56px 40px;
  overflow: hidden;
}

/* Subtle grid */
.card::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255,255,255,0.025) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,0.025) 1px, transparent 1px);
  background-size: 40px 40px;
  pointer-events: none;
}

/* Glow blob behind PnL number */
.glow {
  position: absolute;
  top: 50%%;
  left: 50%%;
  transform: translate(-50%%, -50%%);
  width: 700px;
  height: 400px;
  background: radial-gradient(ellipse at center, %s 0%%, transparent 70%%);
  pointer-events: none;
  z-index: 0;
}

/* Header */
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  position: relative;
  z-index: 1;
}

.logo-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-img {
  width: 36px;
  height: 36px;
  object-fit: contain;
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 4px;
  color: #ffffff;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
  opacity: 0.45;
}

.solana-img {
  width: 20px;
  height: 20px;
  object-fit: contain;
}

.solana-text {
  font-size: 13px;
  font-weight: 600;
  color: #aaa;
  letter-spacing: 2px;
}

/* Center content */
.center {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  position: relative;
  z-index: 1;
  gap: 0;
}

.token-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.token-name {
  font-size: 28px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 1px;
}

.platform-badge {
  font-size: 11px;
  font-weight: 600;
  color: #555;
  padding: 3px 10px;
  border: 1px solid #222;
  border-radius: 20px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.reason-badge {
  font-size: 11px;
  font-weight: 600;
  color: %s;
  padding: 3px 10px;
  border: 1px solid currentColor;
  border-radius: 20px;
  letter-spacing: 1px;
  opacity: 0.7;
}

.pnl-sol {
  font-size: 108px;
  font-weight: 900;
  color: %s;
  line-height: 1;
  letter-spacing: -4px;
  margin: 4px 0;
}

.pnl-pct {
  font-size: 44px;
  font-weight: 700;
  color: %s;
  opacity: 0.75;
  letter-spacing: -1px;
}

/* Stats row */
.stats-row {
  display: flex;
  justify-content: center;
  gap: 48px;
  position: relative;
  z-index: 1;
  padding-top: 24px;
  border-top: 1px solid #141414;
}

.stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.stat-label {
  font-size: 10px;
  font-weight: 600;
  color: #333;
  letter-spacing: 2px;
  text-transform: uppercase;
}

.stat-value {
  font-size: 16px;
  font-weight: 600;
  color: #888;
}

/* Footer */
.footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 20px;
  position: relative;
  z-index: 1;
}

.footer-text {
  font-size: 11px;
  color: #222;
  letter-spacing: 0.5px;
}

.footer-url {
  font-size: 11px;
  color: #222;
  letter-spacing: 1px;
}
</style>
</head>
<body>
<div class="card">
  <div class="glow"></div>

  <div class="header">
    <div class="logo-group">
      <img class="logo-img" src="%s" />
      <span class="logo-text">HUMMINGBIRD</span>
    </div>
    <div class="header-right">
      <img class="solana-img" src="%s" />
      <span class="solana-text">SOLANA</span>
    </div>
  </div>

  <div class="center">
    <div class="token-row">
      <span class="token-name">%s</span>
      <span class="platform-badge">%s</span>
      <span class="reason-badge">%s</span>
    </div>
    <div class="pnl-sol">%s%.4f SOL</div>
    <div class="pnl-pct">%s%.1f%%</div>
  </div>

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

  <div class="footer">
    <span class="footer-text">Traded autonomously by Hummingbird</span>
    <span class="footer-url">hummingbird.vylth.com</span>
  </div>
</div>
</body>
</html>`,
		glowColor,
		pnlColor,
		pnlColor, pnlColor,
		logoDataURI,
		solanaDataURI,
		shortMint, platform, exitReason,
		pnlSign, c.PnLSOL,
		pnlSign, c.PnLPercent,
		c.EntryAmountSOL,
		c.ExitAmountSOL,
		held.String(),
		c.Score,
	)
}
