package trader

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	signet "github.com/VYLTH/signet-sdk-go/signet"

	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
	"github.com/iamdecatalyst/hummingbird/orchestrator/cricket"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/monitor"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
)

// compile-time check that alerts.Telegram still satisfies Notifier
var _ alerts.Notifier = (*alerts.Telegram)(nil)

// ScalperCloser is notified when a position closes so the scalper can free the slot.
type ScalperCloser interface {
	OnPositionClosed(mint string)
}

// Trader executes trades via Signet and manages position lifecycle.
type Trader struct {
	signet        *signet.Client
	walletID      string
	portfolio     *portfolio.Portfolio
	telegram      alerts.Notifier
	cricket       *cricket.Client // passed to per-position monitors
	scalper       ScalperCloser   // notified on close to free scalp slots
	monitorCfg    monitor.MonitorConfig
	minBalanceSOL float64

	exitCh    chan monitor.ExitSignal
	cancelFns sync.Map // mint → context.CancelFunc

	lastTradeMu   sync.Mutex
	lastTradeAt   time.Time
	lastEntryAt   time.Time // tracks last entry ATTEMPT (including failures) for cooldown
	walletAddress string    // Solana public key — used for Helius balance lookups
	rpcURL        string    // Helius HTTP RPC URL
}

func New(
	signetClient *signet.Client,
	walletID string,
	port *portfolio.Portfolio,
	tg alerts.Notifier,
	cc *cricket.Client,
	sc ScalperCloser,
	monitorCfg monitor.MonitorConfig,
	minBalanceSOL float64,
	rpcURL string,
) *Trader {
	t := &Trader{
		signet:        signetClient,
		walletID:      walletID,
		portfolio:     port,
		telegram:      tg,
		cricket:       cc,
		scalper:       sc,
		monitorCfg:    monitorCfg,
		minBalanceSOL: minBalanceSOL,
		rpcURL:        rpcURL,
		exitCh:        make(chan monitor.ExitSignal, 32),
	}

	// Fetch and cache the wallet's Solana public address for RPC balance lookups.
	if w, err := signetClient.Wallets.Get(walletID); err == nil {
		t.walletAddress = w.Address
	}

	go t.processExits()
	return t
}

const entryCooldown = 45 * time.Second // min gap between entry attempts

// Execute is called when the scorer decides to enter a trade.
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

	// Cooldown guard — prevents piling into multiple tokens simultaneously.
	// Lock and stamp first so concurrent calls all see the updated time.
	t.lastTradeMu.Lock()
	since := time.Since(t.lastEntryAt)
	if since < entryCooldown && !t.lastEntryAt.IsZero() {
		t.lastTradeMu.Unlock()
		log.Printf("[trader] skip %s — cooldown (%s remaining)", result.Mint[:8], (entryCooldown - since).Round(time.Second))
		return
	}
	t.lastEntryAt = time.Now()
	t.lastTradeMu.Unlock()

	go t.enter(result)
}

func (t *Trader) enter(result *models.ScoreResult) {
	log.Printf("[trader] entering %s | score=%d | %.3f SOL", result.Mint[:8], result.Total, result.PositionSOL)

	// Min balance guard — check before swap so users' floor is respected
	if t.minBalanceSOL > 0 {
		balance := t.Balance()
		if balance-result.PositionSOL < t.minBalanceSOL {
			log.Printf("[trader] skip %s — would drop below min balance (balance=%.3f pos=%.3f min=%.3f)",
				result.Mint[:8], balance, result.PositionSOL, t.minBalanceSOL)
			return
		}
	}

	params := signet.SwapParams{
		FromToken:       "SOL",
		ToToken:         result.Mint,
		Amount:          fmt.Sprintf("%d", int64(result.PositionSOL*1e9)), // lamports
		SlippageBps:     300, // 3% — generous for new tokens
		DeadlineSeconds: 30,
	}

	// Wait briefly before first swap — new tokens take ~10s to be indexed
	time.Sleep(10 * time.Second)

	var tx *signet.TransactionResult
	var err error
	const maxRetries = 3
	for attempt := range maxRetries {
		tx, err = t.signet.Wallets.Swap(t.walletID, params)
		if err == nil {
			break
		}
		var sigErr *signet.SignetError
		if errors.As(err, &sigErr) {
			// 422 token_not_routable — Signet new API (explicit signal)
			if sigErr.StatusCode == 422 && strings.Contains(sigErr.Message, "token_not_routable") {
				log.Printf("[trader] %s not routable via Jupiter — pumpportal fallback", result.Mint[:8])
				tx, err = t.buyViaPumpPortal(result)
				break
			}
			// 502 from Jupiter — retry a few times for indexing delay, then fall back to pumpportal.
			// Signet currently returns 502 for both "not indexed yet" and "not routable" —
			// after exhausting retries we try pumpportal as a last resort.
			if sigErr.StatusCode == 502 {
				if attempt < maxRetries-1 {
					log.Printf("[trader] %s not yet indexed (attempt %d/%d), retrying in 8s", result.Mint[:8], attempt+1, maxRetries)
					time.Sleep(8 * time.Second)
					continue
				}
				log.Printf("[trader] %s still 502 after %d attempts — pumpportal fallback", result.Mint[:8], maxRetries)
				tx, err = t.buyViaPumpPortal(result)
			}
		}
		break
	}
	if err != nil {
		// Routing failures (no pool, not indexed, unroutable) are expected for many new tokens.
		// Log them but don't spam the user — only alert on truly unexpected errors.
		msg := err.Error()
		isRoutingFailure := strings.Contains(msg, "502") ||
			strings.Contains(msg, "token_not_routable") ||
			strings.Contains(msg, "Pool account not found") ||
			strings.Contains(msg, "pumpportal") ||
			strings.Contains(msg, "not found")
		log.Printf("[trader] entry failed for %s: %v", result.Mint[:8], err)
		if !isRoutingFailure {
			t.telegram.Alert(fmt.Sprintf("entry failed: %s\n%v", result.Mint[:8], err))
		}
		return
	}

	log.Printf("[trader] entered %s | tx=%s", result.Mint[:8], tx.TxHash)

	// Estimate entry price: position SOL / token balance (approximation)
	// Real balance fetch would require an extra RPC call — use position SOL as proxy for now.
	pos := &models.Position{
		ID:             tx.TxHash,
		Mint:           result.Mint,
		DevWallet:      result.DevWallet,
		WalletID:       t.walletID,
		EntryPriceSOL:  result.PositionSOL, // refined by monitor on first price tick
		EntryAmountSOL: result.PositionSOL,
		Score:          result.Total,
		Decision:       result.Decision,
		OpenedAt:       time.Now(),
		PeakPriceSOL:   result.PositionSOL,
	}

	t.markTrade()
	t.portfolio.Open(pos)
	t.telegram.Entered(pos)

	// Start post-entry monitor — uses Cricket Firefly for exodus detection
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFns.Store(result.Mint, cancel)
	m := monitor.New(pos, t.cricket, t.exitCh, t.monitorCfg)
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

	// Snapshot SOL balance before exit so we can compute real P&L
	var balBefore float64
	if sig.Partial == 0 {
		balBefore = t.Balance()
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
		balAfter := t.Balance()
		exitAmount := balAfter - balBefore
		if exitAmount <= 0 {
			// RPC failed or returned stale data — fall back to entry amount
			exitAmount = pos.EntryAmountSOL
		}
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

		t.markTrade()
		t.portfolio.Close(closed)
		t.telegram.Exited(closed)

		// Notify scalper so the slot is freed for re-entry if the pattern repeats
		if t.scalper != nil && closed.Reason != models.ExitManual {
			go t.scalper.OnPositionClosed(closed.Mint)
		}
	}
}


// Restore resumes monitoring for a position loaded from DB on startup.
// Does NOT execute a swap — the position was already entered in a previous run.
func (t *Trader) Restore(pos *models.Position) {
	t.portfolio.RestoreOpen(pos)
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFns.Store(pos.Mint, cancel)
	m := monitor.New(pos, t.cricket, t.exitCh, t.monitorCfg)
	go m.Watch(ctx)
	log.Printf("[trader] restored position %s from DB", pos.Mint[:8])
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

// Balance returns the wallet's current SOL balance via Signet.
// Returns 0 on failure.
func (t *Trader) markTrade() {
	t.lastTradeMu.Lock()
	t.lastTradeAt = time.Now()
	t.lastTradeMu.Unlock()
}

func (t *Trader) LastTradeAt() time.Time {
	t.lastTradeMu.Lock()
	defer t.lastTradeMu.Unlock()
	return t.lastTradeAt
}

// Balance fetches SOL balance via Signet — used for trade-critical checks.
func (t *Trader) Balance() float64 {
	if t.walletID == "" {
		return 0
	}
	b, err := t.signet.Wallets.Balance(t.walletID)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseFloat(b.NativeBalance, 64)
	if err != nil {
		return 0
	}
	return v
}

// BalanceViaRPC fetches SOL balance directly from Helius RPC.
// Used for polling/display so we don't waste Signet requests.
// BalanceViaRPC fetches SOL balance directly from Helius RPC.
// Returns -1 on any RPC error so callers can distinguish "failed" from "real zero balance".
func (t *Trader) BalanceViaRPC() float64 {
	if t.rpcURL == "" || t.walletAddress == "" {
		return t.Balance() // fallback to Signet
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBalance",
		"params":  []any{t.walletAddress, map[string]string{"commitment": "confirmed"}},
	})
	resp, err := http.Post(t.rpcURL, "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != 200 {
		return -1 // RPC failure — not a real zero balance
	}
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Value int64 `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1
	}
	return float64(result.Result.Value) / 1e9
}

// LatestTxHash returns the most recent transaction signature for this wallet via Helius.
func (t *Trader) LatestTxHash() string {
	if t.rpcURL == "" || t.walletAddress == "" {
		return ""
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSignaturesForAddress",
		"params":  []any{t.walletAddress, map[string]any{"limit": 1}},
	})
	resp, err := http.Post(t.rpcURL, "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		Result []struct {
			Signature string `json:"signature"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Result) > 0 {
		return result.Result[0].Signature
	}
	return ""
}

// buyViaPumpPortal builds a pump.fun buy transaction via pumpportal.fun and executes it
// through Signet's /execute endpoint. Called when /swap returns token_not_routable.
// Tries "pump" (bonding curve) first, then "pump-amm" (migrated tokens) on 400.
func (t *Trader) buyViaPumpPortal(result *models.ScoreResult) (*signet.TransactionResult, error) {
	if t.walletAddress == "" {
		return nil, fmt.Errorf("wallet address not set — cannot build pump.fun tx")
	}

	// Try bonding curve first, then migrated AMM pool on failure.
	pools := []string{"pump", "pump-amm"}

	var lastErr error
	for _, pool := range pools {
		tx, err := t.pumpPortalBuy(result.Mint, result.PositionSOL, pool)
		if err == nil {
			return tx, nil
		}
		log.Printf("[trader] pumpportal pool=%s failed for %s: %v", pool, result.Mint[:8], err)
		lastErr = err
	}
	return nil, lastErr
}

func (t *Trader) pumpPortalBuy(mint string, amountSOL float64, pool string) (*signet.TransactionResult, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"publicKey":        t.walletAddress,
		"action":           "buy",
		"mint":             mint,
		"denominatedInSol": true,
		"amount":           amountSOL,
		"slippage":         10, // 10% — wide for new tokens
		"priorityFee":      0.0005,
		"pool":             pool,
	})

	ppResp, err := http.Post(
		"https://pumpportal.fun/api/trade-local",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("pumpportal request failed: %w", err)
	}
	defer ppResp.Body.Close()

	txBytes, err := io.ReadAll(ppResp.Body)
	if err != nil {
		return nil, fmt.Errorf("pumpportal read failed: %w", err)
	}
	if ppResp.StatusCode != 200 {
		// Error detail is in the HTTP status line (not body) — use resp.Status
		return nil, fmt.Errorf("pumpportal %s", ppResp.Status)
	}

	txBase64 := base64.StdEncoding.EncodeToString(txBytes)
	tx, err := t.signet.Wallets.Execute(t.walletID, txBase64)
	if err != nil {
		return nil, fmt.Errorf("signet execute: %w", err)
	}

	log.Printf("[trader] pumpportal pool=%s success for %s | tx=%s", pool, mint[:8], tx.TxHash)
	return tx, nil
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
