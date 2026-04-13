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

// GenerateCard renders a PnL share card as a PNG (800×420px).
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
		"--width", "800",
		"--height", "420",
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
	if c.PnLSOL < 0 {
		pnlColor = "#ff4444"
	}

	shortMint := c.Mint
	if len(shortMint) > 8 {
		shortMint = shortMint[:8]
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

	mode := strings.ToUpper(c.Decision)
	if mode == "" {
		mode = "SNIPER"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }

body, html {
  width: 800px;
  height: 420px;
  background: #0a0a0a;
  font-family: 'Courier New', 'Lucida Console', monospace;
  color: #ffffff;
  overflow: hidden;
}

.card {
  width: 800px;
  height: 420px;
  background: #0a0a0a;
  display: flex;
  flex-direction: column;
  padding: 32px 36px 28px 36px;
  position: relative;
}

.card::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(0,212,255,0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(0,212,255,0.03) 1px, transparent 1px);
  background-size: 32px 32px;
  pointer-events: none;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  position: relative;
  z-index: 1;
}

.logo-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.logo-img {
  width: 32px;
  height: 32px;
  object-fit: contain;
  filter: drop-shadow(0 0 8px rgba(0,212,255,0.4));
}

.logo-text {
  font-size: 15px;
  font-weight: bold;
  letter-spacing: 3px;
  color: #00d4ff;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.solana-group {
  display: flex;
  align-items: center;
  gap: 6px;
  opacity: 0.4;
}

.solana-img {
  width: 18px;
  height: 18px;
  object-fit: contain;
}

.solana-text {
  font-size: 11px;
  color: #888;
  letter-spacing: 2px;
}

.brand {
  font-size: 12px;
  color: #222;
  font-weight: bold;
  letter-spacing: 4px;
}

.divider {
  height: 1px;
  background: linear-gradient(to right, transparent, #1e1e1e 20%%, #1e1e1e 80%%, transparent);
  margin: 0 0 24px 0;
  position: relative;
  z-index: 1;
}

.body {
  flex: 1;
  display: flex;
  gap: 0;
  position: relative;
  z-index: 1;
}

.left {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.token-label {
  font-size: 10px;
  color: #2a2a2a;
  letter-spacing: 3px;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.token {
  font-size: 30px;
  font-weight: bold;
  color: #fff;
  letter-spacing: 2px;
  margin-bottom: 4px;
}

.platform {
  font-size: 11px;
  color: #3a3a3a;
  letter-spacing: 1px;
  margin-bottom: 28px;
}

.pnl-sol {
  font-size: 54px;
  font-weight: bold;
  color: %s;
  line-height: 1;
  margin-bottom: 5px;
  letter-spacing: -1px;
}

.pnl-pct {
  font-size: 28px;
  font-weight: bold;
  color: %s;
  opacity: 0.8;
}

.right {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 18px;
  min-width: 200px;
  padding-left: 36px;
  border-left: 1px solid #111;
}

.stat { display: flex; flex-direction: column; gap: 3px; }
.stat-label { font-size: 10px; color: #2a2a2a; letter-spacing: 2px; text-transform: uppercase; }
.stat-value { font-size: 14px; color: #777; }

.footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid #0f0f0f;
  position: relative;
  z-index: 1;
}

.footer-text { font-size: 10px; color: #222; letter-spacing: 1px; }
.footer-url  { font-size: 10px; color: #222; letter-spacing: 1px; }
</style>
</head>
<body>
<div class="card">
  <div class="header">
    <div class="logo-group">
      <img class="logo-img" src="%s" />
      <span class="logo-text">HUMMINGBIRD</span>
    </div>
    <div class="header-right">
      <div class="solana-group">
        <img class="solana-img" src="%s" />
        <span class="solana-text">SOLANA</span>
      </div>
      <span class="brand">VYLTH</span>
    </div>
  </div>
  <div class="divider"></div>
  <div class="body">
    <div class="left">
      <div class="token-label">Token · %s</div>
      <div class="token">%s</div>
      <div class="platform">%s</div>
      <div class="pnl-sol">%s%.4f SOL</div>
      <div class="pnl-pct">%s%.1f%%</div>
    </div>
    <div class="right">
      <div class="stat">
        <div class="stat-label">Entry</div>
        <div class="stat-value">%.4f SOL</div>
      </div>
      <div class="stat">
        <div class="stat-label">Exit</div>
        <div class="stat-value">%.4f SOL</div>
      </div>
      <div class="stat">
        <div class="stat-label">Duration</div>
        <div class="stat-value">%s</div>
      </div>
      <div class="stat">
        <div class="stat-label">Score</div>
        <div class="stat-value">%d / 100</div>
      </div>
    </div>
  </div>
  <div class="footer">
    <div class="footer-text">Traded autonomously by Hummingbird</div>
    <div class="footer-url">hummingbird.vylth.com</div>
  </div>
</div>
</body>
</html>`,
		pnlColor, pnlColor,
		logoDataURI,
		solanaDataURI,
		mode,
		shortMint,
		platform,
		pnlSign, c.PnLSOL,
		pnlSign, c.PnLPercent,
		c.EntryAmountSOL,
		c.ExitAmountSOL,
		held.String(),
		c.Score,
	)
}
