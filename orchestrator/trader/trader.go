package trader

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	signet "github.com/VYLTH/signet-sdk-go/signet"

	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/monitor"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
)

// Trader executes trades via Signet and manages position lifecycle.
type Trader struct {
	signet    *signet.Client
	walletID  string
	portfolio *portfolio.Portfolio
	telegram  *alerts.Telegram
	solanaRPC string
	scorerURL string // for scalp-closed callbacks

	exitCh    chan monitor.ExitSignal
	cancelFns sync.Map // mint → context.CancelFunc
}

func New(
	signetClient *signet.Client,
	walletID string,
	port *portfolio.Portfolio,
	tg *alerts.Telegram,
	solanaRPC string,
	scorerURL string,
) *Trader {
	t := &Trader{
		signet:    signetClient,
		walletID:  walletID,
		portfolio: port,
		telegram:  tg,
		solanaRPC: solanaRPC,
		scorerURL: scorerURL,
		exitCh:    make(chan monitor.ExitSignal, 32),
	}
	go t.processExits()
	return t
}

// Execute is called when the Python scorer decides to enter a trade.
// Handles both sniper ("small"/"medium"/"full") and scalper ("scalp") decisions.
func (t *Trader) Execute(result *models.ScoreResult) {
	if result.Decision == "skip" || result.PositionSOL <= 0 {
		return
	}

	if ok, reason := t.portfolio.CanEnter(); !ok {
		log.Printf("[trader] skip %s — %s", result.Mint[:8], reason)
		return
	}

	if t.portfolio.AlreadyOpen(result.Mint) {
		log.Printf("[trader] skip %s — already open", result.Mint[:8])
		return
	}

	go t.enter(result)
}

func (t *Trader) enter(result *models.ScoreResult) {
	log.Printf("[trader] entering %s | score=%d | %.3f SOL", result.Mint[:8], result.Total, result.PositionSOL)

	tx, err := t.signet.Wallets.Swap(t.walletID, signet.SwapParams{
		FromToken:       "SOL",
		ToToken:         result.Mint,
		Amount:          fmt.Sprintf("%.6f", result.PositionSOL),
		SlippageBps:     300, // 3% — generous for new tokens
		DeadlineSeconds: 30,
	})
	if err != nil {
		log.Printf("[trader] signet swap failed for %s: %v", result.Mint[:8], err)
		t.telegram.Alert(fmt.Sprintf("entry failed: %s\n%v", result.Mint[:8], err))
		return
	}

	log.Printf("[trader] entered %s | tx=%s", result.Mint[:8], tx.TxHash)

	// Estimate entry price: position SOL / token balance (approximation)
	// Real balance fetch would require an extra RPC call — use position SOL as proxy for now.
	pos := &models.Position{
		ID:             tx.TxHash,
		Mint:           result.Mint,
		WalletID:       t.walletID,
		EntryPriceSOL:  result.PositionSOL, // refined by monitor on first price tick
		EntryAmountSOL: result.PositionSOL,
		Score:          result.Total,
		OpenedAt:       time.Now(),
		PeakPriceSOL:   result.PositionSOL,
	}

	t.portfolio.Open(pos)
	t.telegram.Entered(pos)

	// Start post-entry monitor
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFns.Store(result.Mint, cancel)
	m := monitor.New(pos, t.solanaRPC, t.exitCh)
	go m.Watch(ctx)
}

// processExits handles exit signals from monitors in a single goroutine.
func (t *Trader) processExits() {
	for sig := range t.exitCh {
		t.handleExit(sig)
	}
}

func (t *Trader) handleExit(sig monitor.ExitSignal) {
	pos, ok := t.portfolio.Get(sig.Mint)
	if !ok {
		return // already closed
	}

	// For partial exits (take-profit), sell only a fraction
	sellAmount := "100%" // default: sell everything
	if sig.Partial > 0 && sig.Partial < 1 {
		sellAmount = fmt.Sprintf("%.0f%%", sig.Partial*100)
		log.Printf("[trader] partial exit %s — selling %s (reason: %s)", sig.Mint[:8], sellAmount, sig.Reason)
	} else {
		log.Printf("[trader] full exit %s (reason: %s)", sig.Mint[:8], sig.Reason)
		// Cancel the monitor — we're done with this position
		if cancel, ok := t.cancelFns.LoadAndDelete(sig.Mint); ok {
			cancel.(context.CancelFunc)()
		}
	}

	tx, err := t.signet.Wallets.Swap(t.walletID, signet.SwapParams{
		FromToken:       sig.Mint,
		ToToken:         "SOL",
		Amount:          sellAmount,
		SlippageBps:     500, // 5% — wider on exit, speed > price
		DeadlineSeconds: 20,
	})
	if err != nil {
		log.Printf("[trader] exit swap failed for %s: %v", sig.Mint[:8], err)
		t.telegram.Alert(fmt.Sprintf("EXIT FAILED: %s\n%v", sig.Mint[:8], err))
		return
	}

	// Only fully close the position on full exits
	if sig.Partial == 0 {
		exitAmount := t.parseSOLAmount(tx)
		pnl := exitAmount - pos.EntryAmountSOL
		pnlPct := (pnl / pos.EntryAmountSOL) * 100

		closed := &models.ClosedPosition{
			Position:      *pos,
			ExitPriceSOL:  exitAmount,
			ExitAmountSOL: exitAmount,
			PnLSOL:        pnl,
			PnLPercent:    pnlPct,
			Reason:        sig.Reason,
			ClosedAt:      time.Now(),
			TxHash:        tx.TxHash,
		}

		t.portfolio.Close(closed)
		t.telegram.Exited(closed)

		// Notify scorer if this was a scalp — frees the slot for re-entry
		if closed.Reason != models.ExitManual {
			go t.notifyScorerClosed(closed.Mint)
		}
	}
}

func (t *Trader) notifyScorerClosed(mint string) {
	if t.scorerURL == "" {
		return
	}
	url := t.scorerURL + "/scalper/closed"
	body := fmt.Sprintf(`{"mint":"%s"}`, mint)
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// ExitAll closes all open positions immediately (e.g. on /stop command).
func (t *Trader) ExitAll(reason models.ExitReason) {
	for _, pos := range t.portfolio.OpenPositions() {
		t.handleExit(monitor.ExitSignal{
			Mint:   pos.Mint,
			Reason: reason,
		})
	}
}

func (t *Trader) parseSOLAmount(tx *signet.TransactionResult) float64 {
	// In a real integration, we'd fetch the wallet balance delta.
	// For now, return a placeholder — the portfolio P&L is approximate
	// until we wire up balance polling.
	_ = tx
	return 0
}

// EnsureWallet creates the Solana trading wallet if it doesn't exist yet.
func EnsureWallet(client *signet.Client, label string) (string, error) {
	wallets, err := client.Wallets.List()
	if err != nil {
		return "", fmt.Errorf("list wallets: %w", err)
	}
	for _, w := range wallets {
		if w.Label != nil && *w.Label == label && w.Chain == "solana" {
			log.Printf("[trader] reusing wallet %s (%s)", w.ID, w.Address)
			return w.ID, nil
		}
	}

	w, err := client.Wallets.Create(signet.CreateWalletParams{
		Chain: "solana",
		Label: label,
	})
	if err != nil {
		return "", fmt.Errorf("create wallet: %w", err)
	}
	log.Printf("[trader] created wallet %s (%s)", w.ID, w.Address)
	return w.ID, nil
}

// stub to avoid import cycle — remove when balance polling is wired up
var _ = strconv.FormatFloat
