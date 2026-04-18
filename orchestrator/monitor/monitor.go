package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/cricket"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

const (
	priceInterval     = 2 * time.Second
	walletInterval    = 5 * time.Second
	emergencyDumpDrop = -0.15 // -15% in <10s = emergency (always hard limit)
)

// MonitorConfig holds per-user exit parameters.
type MonitorConfig struct {
	StopLossPercent float64 // e.g. 0.25 (stored positive; applied as negative)
	TakeProfit1x    float64 // price multiple for first partial exit, e.g. 2.0
	TakeProfit2x    float64
	TakeProfit3x    float64
	TimeoutMinutes  int
}

// DefaultMonitorConfig returns the original hardcoded defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		StopLossPercent: 0.25,
		TakeProfit1x:    2.0,
		TakeProfit2x:    5.0,
		TakeProfit3x:    10.0,
		TimeoutMinutes:  8,
	}
}

// ExitSignal is sent back to the trader when an exit condition is triggered.
type ExitSignal struct {
	Mint    string
	Reason  models.ExitReason
	Partial float64 // 0 = full exit, 0.4 = sell 40%, etc.
}

// Monitor watches a single open position and sends exit signals.
type Monitor struct {
	pos        *models.Position
	cricket    *cricket.Client // for Firefly exodus signal detection
	exitCh     chan<- ExitSignal
	httpClient *http.Client
	cfg        MonitorConfig
}

func New(pos *models.Position, cc *cricket.Client, exitCh chan<- ExitSignal, cfg MonitorConfig) *Monitor {
	return &Monitor{
		pos:        pos,
		cricket:    cc,
		exitCh:     exitCh,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		cfg:        cfg,
	}
}

// Watch runs until the position should be exited, then sends an ExitSignal.
// Call in a goroutine.
func (m *Monitor) Watch(ctx context.Context) {
	priceTicker := time.NewTicker(priceInterval)
	walletTicker := time.NewTicker(walletInterval)
	timeoutMins := m.cfg.TimeoutMinutes
	if timeoutMins <= 0 {
		timeoutMins = 8
	}
	timeout := time.NewTimer(time.Duration(timeoutMins) * time.Minute)
	defer priceTicker.Stop()
	defer walletTicker.Stop()
	defer timeout.Stop()

	var lastPrice float64
	var lastPriceTime time.Time

	for {
		select {
		case <-ctx.Done():
			return

		case <-timeout.C:
			m.exit(models.ExitTimeout, 0)
			return

		case <-priceTicker.C:
			price, err := m.fetchPrice(m.pos.Mint)
			if err != nil {
				log.Printf("[monitor] price fetch error for %s: %v", m.pos.Mint[:8], err)
				continue
			}

			// Emergency: big drop very fast
			if lastPrice > 0 && !lastPriceTime.IsZero() {
				elapsed := time.Since(lastPriceTime).Seconds()
				if elapsed <= 10 {
					change := (price - lastPrice) / lastPrice
					if change <= emergencyDumpDrop {
						log.Printf("[monitor] 🚨 emergency dump detected on %s", m.pos.Mint[:8])
						m.exit(models.ExitRugDetected, 0)
						return
					}
				}
			}
			lastPrice = price
			lastPriceTime = time.Now()

			// Fallback baseline: trader couldn't fetch token balance after swap, so it
			// recorded EntryPriceSOL=0. Treat the first valid DexScreener tick as our
			// reference price. Skip ratio checks this round so we don't compare against 0.
			if m.pos.EntryPriceSOL <= 0 {
				m.pos.EntryPriceSOL = price
				m.pos.PeakPriceSOL = price
				log.Printf("[monitor] %s baseline price set from first tick: %.10f SOL", m.pos.Mint[:8], price)
				continue
			}

			// Update peak
			if price > m.pos.PeakPriceSOL {
				m.pos.PeakPriceSOL = price
			}

			ratio := price / m.pos.EntryPriceSOL

			// Stop loss
			slThreshold := 1.0 - m.cfg.StopLossPercent
			if ratio <= slThreshold {
				m.exit(models.ExitStopLoss, 0)
				return
			}

			// Take profit — staged exits
			switch m.pos.TakeProfitLevel {
			case 0:
				if ratio >= m.cfg.TakeProfit1x {
					m.pos.TakeProfitLevel = 1
					m.exit(models.ExitTakeProfit, 0.40) // sell 40%, keep watching
				}
			case 1:
				if ratio >= m.cfg.TakeProfit2x {
					m.pos.TakeProfitLevel = 2
					m.exit(models.ExitTakeProfit, 0.40) // sell another 40%
				}
			case 2:
				if ratio >= m.cfg.TakeProfit3x {
					m.exit(models.ExitTakeProfit, 0) // sell everything
					return
				}
			}

		case <-walletTicker.C:
			// Check if dev wallet is selling aggressively
			if m.isDevSelling() {
				log.Printf("[monitor] 🚨 dev wallet selling on %s", m.pos.Mint[:8])
				m.exit(models.ExitRugDetected, 0)
				return
			}
		}
	}
}

// fetchPrice returns the SOL-denominated price of a token via DexScreener.
// Jupiter's price.jup.ag was deprecated; DexScreener covers pump.fun + Raydium + all DEXes.
func (m *Monitor) fetchPrice(mint string) (float64, error) {
	url := fmt.Sprintf("https://api.dexscreener.com/latest/dex/tokens/%s", mint)
	resp, err := m.httpClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Pairs []struct {
			BaseToken struct {
				Address string `json:"address"`
			} `json:"baseToken"`
			QuoteToken struct {
				Symbol string `json:"symbol"`
			} `json:"quoteToken"`
			PriceNative string `json:"priceNative"`
		} `json:"pairs"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("dexscreener decode: %w", err)
	}

	// Find the SOL-quoted pair — most relevant for our P&L tracking.
	for _, pair := range result.Pairs {
		if pair.QuoteToken.Symbol == "SOL" {
			var price float64
			fmt.Sscanf(pair.PriceNative, "%f", &price)
			if price > 0 {
				return price, nil
			}
		}
	}
	// Fallback: use first pair if no SOL pair found.
	if len(result.Pairs) > 0 {
		var price float64
		fmt.Sscanf(result.Pairs[0].PriceNative, "%f", &price)
		if price > 0 {
			return price, nil
		}
	}
	return 0, fmt.Errorf("no price data for %s on DexScreener", mint[:8])
}

// isDevSelling checks Cricket Firefly for an exodus signal on our token.
// An exodus signal means smart-money wallets are exiting — strong rug warning.
func (m *Monitor) isDevSelling() bool {
	if m.cricket == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := m.cricket.FireflySignals(ctx)
	if err != nil {
		return false
	}

	for _, sig := range resp.Data {
		if sig.TokenAddress != m.pos.Mint {
			continue
		}
		if sig.SignalType == "exodus" && sig.Strength != "weak" {
			log.Printf("[monitor] 🚨 Cricket exodus signal on %s (strength=%s)", m.pos.Mint[:8], sig.Strength)
			return true
		}
	}
	return false
}

// exit sends an exit signal to the trader. Blocks if the trader's exit channel
// is full — losing a stop-loss or rug signal here means the position becomes
// unwatched (the monitor goroutine returns immediately after exit() at every
// critical-path call site). 5s timeout guards against a wedged trader.
func (m *Monitor) exit(reason models.ExitReason, partial float64) {
	sig := ExitSignal{Mint: m.pos.Mint, Reason: reason, Partial: partial}
	select {
	case m.exitCh <- sig:
	case <-time.After(5 * time.Second):
		// Trader is wedged. Last-ditch blocking send: better to stall the monitor
		// goroutine than to lose a stop-loss / rug signal.
		log.Printf("[monitor] 🚨 exit channel full for %s — blocking until accepted (reason=%s)", m.pos.Mint[:8], reason)
		m.exitCh <- sig
	}
}
