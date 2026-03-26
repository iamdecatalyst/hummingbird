package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

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
	solanaRPC  string
	exitCh     chan<- ExitSignal
	httpClient *http.Client
	cfg        MonitorConfig
}

func New(pos *models.Position, solanaRPC string, exitCh chan<- ExitSignal, cfg MonitorConfig) *Monitor {
	return &Monitor{
		pos:        pos,
		solanaRPC:  solanaRPC,
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

// fetchPrice uses Jupiter Price API — simplest way to get SOL-denominated token price.
func (m *Monitor) fetchPrice(mint string) (float64, error) {
	url := fmt.Sprintf("https://price.jup.ag/v4/price?ids=%s&vsToken=SOL", mint)
	resp, err := m.httpClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data map[string]struct {
			Price float64 `json:"price"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if d, ok := result.Data[mint]; ok {
		return d.Price, nil
	}
	return 0, fmt.Errorf("mint %s not found in Jupiter response", mint[:8])
}

// isDevSelling checks recent transactions on the dev wallet for large token transfers.
func (m *Monitor) isDevSelling() bool {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSignaturesForAddress",
		"params":  []any{m.pos.Mint, map[string]any{"limit": 5}},
	})

	req, _ := http.NewRequest("POST", m.solanaRPC, nil)
	req.Header.Set("Content-Type", "application/json")

	req.Body = io.NopCloser(bytes.NewReader(body))

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// If there are fresh transactions on the mint account itself,
	// that's a signal — full analysis would require parsing tx details.
	// For now: if we see >2 new txs since we entered, flag for review.
	var result struct {
		Result []struct {
			BlockTime int64 `json:"blockTime"`
		} `json:"result"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false
	}

	entryTime := m.pos.OpenedAt.Unix()
	postEntryTxs := 0
	for _, sig := range result.Result {
		if sig.BlockTime > entryTime {
			postEntryTxs++
		}
	}

	// >3 new transactions on the mint after we entered = investigate
	return postEntryTxs > 3
}

func (m *Monitor) exit(reason models.ExitReason, partial float64) {
	select {
	case m.exitCh <- ExitSignal{Mint: m.pos.Mint, Reason: reason, Partial: partial}:
	default:
		log.Printf("[monitor] exit channel full for %s", m.pos.Mint[:8])
	}
}
