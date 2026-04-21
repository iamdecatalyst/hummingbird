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
	"math"
	"math/big"
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
	"github.com/iamdecatalyst/hummingbird/orchestrator/util"
)

// compile-time check that alerts.Telegram still satisfies Notifier
var _ alerts.Notifier = (*alerts.Telegram)(nil)

// ScalperCloser is notified when a position closes so the scalper can free the slot.
type ScalperCloser interface {
	OnPositionClosed(mint string)
}

// Trader executes trades via Signet and manages position lifecycle.
type Trader struct {
	signet         *signet.Client
	walletID       string
	portfolio      *portfolio.Portfolio
	telegram       alerts.Notifier
	cricket        *cricket.Client // passed to per-position monitors
	scalper        ScalperCloser   // notified on close to free scalp slots
	monitorCfg     monitor.MonitorConfig
	minBalanceSOL  float64
	maxPositionSOL float64            // hard cap on per-trade SOL — clamps incoming ScoreResult
	onProgress     monitor.ProgressFn // optional — fired by monitor on TP advance for DB persistence

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
	maxPositionSOL float64,
	rpcURL string,
	onProgress monitor.ProgressFn,
) *Trader {
	t := &Trader{
		signet:         signetClient,
		walletID:       walletID,
		portfolio:      port,
		telegram:       tg,
		cricket:        cc,
		scalper:        sc,
		monitorCfg:     monitorCfg,
		minBalanceSOL:  minBalanceSOL,
		maxPositionSOL: maxPositionSOL,
		rpcURL:         rpcURL,
		onProgress:     onProgress,
		exitCh:         make(chan monitor.ExitSignal, 32),
	}

	// Fetch and cache the wallet's Solana public address for RPC balance lookups.
	if w, err := signetClient.Wallets.Get(walletID); err == nil {
		t.walletAddress = w.Address
	}

	util.Go("trader.processExits", t.processExits)
	return t
}

const entryCooldown = 45 * time.Second // min gap between entry attempts

// Execute is called when the scorer decides to enter a trade.
// Handles both sniper ("small"/"medium"/"full") and scalper ("scalp") decisions.
func (t *Trader) Execute(result *models.ScoreResult) {
	if result.Decision == "skip" || result.PositionSOL <= 0 {
		return
	}

	// Clamp to per-user max position size. Without this, the user's configured cap
	// (set in Telegram or web) would be cosmetic and the score result's raw size used.
	if t.maxPositionSOL > 0 && result.PositionSOL > t.maxPositionSOL {
		log.Printf("[trader] clamping %s position %.3f → %.3f SOL (user cap)", result.Mint[:8], result.PositionSOL, t.maxPositionSOL)
		result.PositionSOL = t.maxPositionSOL
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

	util.Go("trader.enter:"+util.ShortMint(result.Mint), func() { t.enter(result) })
}

func (t *Trader) enter(result *models.ScoreResult) {
	log.Printf("[trader] entering %s | score=%d | %.3f SOL", result.Mint[:8], result.Total, result.PositionSOL)

	// Min balance guard — check before swap so users' floor is respected.
	// Buffer covers ATA rent + priority fee + network fee so the user actually stays
	// above their configured floor after the swap settles.
	const swapFeeBuffer = 0.005 // SOL — empirical: ~0.002 priority + 0.002 ATA + dust
	if t.minBalanceSOL > 0 {
		balance := t.Balance()
		if balance-result.PositionSOL-swapFeeBuffer < t.minBalanceSOL {
			log.Printf("[trader] skip %s — would drop below min balance (balance=%.3f pos=%.3f buffer=%.3f min=%.3f)",
				result.Mint[:8], balance, result.PositionSOL, swapFeeBuffer, t.minBalanceSOL)
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

	var tx *signet.TransactionResult
	var err error

	// Scalp entries skip PumpPortal — tokens are 8-45 min old, already on Raydium/Jupiter,
	// no indexing delay needed. Sniper entries for pump_fun try PumpPortal first (bonding curve,
	// no Jupiter delay), then fall back to Jupiter.
	if result.Platform == "pump_fun" && result.Decision != "scalp" {
		log.Printf("[trader] pump_fun — trying PumpPortal directly for %s", result.Mint[:8])
		tx, err = t.buyViaPumpPortal(result)
		if err != nil {
			log.Printf("[trader] PumpPortal failed for %s: %v — falling back to Jupiter", result.Mint[:8], err)
			err = nil // clear so Jupiter path runs
		} else {
			goto entryDone
		}
	}

	// Non-pump_fun or PumpPortal fallback: use Signet/Jupiter.
	// Sniper entries wait 10s for Jupiter indexing. Scalp tokens are old enough to skip the wait.
	if result.Decision != "scalp" {
		time.Sleep(10 * time.Second)
	}

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

entryDone:
	if err != nil {
		// Routing failures (no pool, not indexed, unroutable) are expected for many new tokens.
		// Log them but don't spam the user — only alert on truly unexpected errors.
		msg := err.Error()
		isRoutingFailure := strings.Contains(msg, "502") ||
			strings.Contains(msg, "token_not_routable") ||
			strings.Contains(msg, "Pool account not found") ||
			strings.Contains(msg, "pumpportal") ||
			strings.Contains(msg, "not found") ||
			strings.Contains(msg, "context deadline exceeded") ||
			strings.Contains(msg, "timeout") ||
			strings.Contains(msg, "connection reset")
		log.Printf("[trader] entry failed for %s: %v", result.Mint[:8], err)
		if !isRoutingFailure {
			t.telegram.Alert(fmt.Sprintf("entry failed: %s\n%v", result.Mint[:8], err))
		}
		return
	}

	log.Printf("[trader] entered %s | tx=%s", result.Mint[:8], tx.TxHash)

	// Compute true per-token entry price = SOL spent / UI tokens received.
	// Without this, EntryPriceSOL would hold the lamport notional (~0.10) and the monitor
	// would compare it against DexScreener's per-token price (~1e-7), instant-stop-outing
	// every position. Retry a few times — the just-created ATA needs RPC indexing time.
	var entryPrice, tokenUIBal float64
	for attempt := 0; attempt < 3; attempt++ {
		time.Sleep(2 * time.Second)
		raw, dec, balErr := t.tokenBalance(result.Mint)
		if balErr == nil && raw > 0 {
			tokenUIBal = float64(raw) / math.Pow10(int(dec))
			if tokenUIBal > 0 {
				entryPrice = result.PositionSOL / tokenUIBal
				break
			}
		}
	}
	if entryPrice == 0 {
		log.Printf("[trader] %s could not fetch entry token balance after swap — monitor will use first price tick as baseline", result.Mint[:8])
	}

	pos := &models.Position{
		ID:             tx.TxHash,
		Mint:           result.Mint,
		DevWallet:      result.DevWallet,
		WalletID:       t.walletID,
		Platform:       result.Platform,
		EntryPriceSOL:  entryPrice,    // 0 = monitor will capture from first DexScreener tick
		EntryAmountSOL: result.PositionSOL,
		TokenBalance:   tokenUIBal,
		Score:          result.Total,
		Decision:       result.Decision,
		OpenedAt:       time.Now(),
		PeakPriceSOL:   entryPrice,    // matches entry; monitor will update
	}

	t.markTrade()
	t.portfolio.Open(pos)
	t.telegram.Entered(pos)

	// Start post-entry monitor — uses Cricket Firefly for exodus detection
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFns.Store(result.Mint, cancel)
	m := monitor.New(pos, t.cricket, t.exitCh, t.monitorCfg, t.onProgress)
	util.Go("monitor.Watch:"+util.ShortMint(result.Mint), func() { m.Watch(ctx) })
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

	// Try exit up to 3 times — RPC blips and pool delays can cause transient failures.
	var tx *signet.TransactionResult
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			log.Printf("[trader] exit retry %d/3 for %s in 5s", attempt+1, sig.Mint[:8])
			time.Sleep(5 * time.Second)
		}
		// Escalate slippage on retries — Token 2022 transfer fees and volatile
		// pump-amm pools often fail at 5%; 1000bps on retry recovers many of these.
		slippage := 500 + (attempt * 500)
		tx, err = t.signet.Wallets.Swap(t.walletID, signet.SwapParams{
			FromToken:       sig.Mint,
			ToToken:         "SOL",
			Amount:          sellAmount,
			SlippageBps:     slippage,
			DeadlineSeconds: 20,
		})
		if err != nil {
			var sigErr *signet.SignetError
			isRoutable := errors.As(err, &sigErr) && sigErr.StatusCode != 502 &&
				!strings.Contains(sigErr.Message, "token_not_routable")
			if !isRoutable {
				log.Printf("[trader] exit swap unroutable for %s, trying pumpportal sell", sig.Mint[:8])
				tx, err = t.sellViaPumpPortal(sig.Mint, sig.Partial)
			}
		}
		if err == nil {
			break
		}
		log.Printf("[trader] exit attempt %d failed for %s: %v", attempt+1, sig.Mint[:8], err)
	}

	if err != nil {
		// All 3 attempts failed — write off as total loss and free the slot.
		// Better to record the loss honestly than leave a ghost position blocking trades.
		log.Printf("[trader] exit failed after 3 attempts for %s — writing off as total loss", sig.Mint[:8])
		if sig.Partial == 0 {
			t.telegram.Alert(fmt.Sprintf("🪦 *Position Written Off*\n`%s`\nAll exit attempts failed — token likely rugged. Recorded as total loss.\n_%v_", sig.Mint[:8], err))
			closed := &models.ClosedPosition{
				Position:      *pos,
				ExitPriceSOL:  0,
				ExitAmountSOL: 0,
				PnLSOL:        -pos.EntryAmountSOL,
				PnLPercent:    -100,
				Reason:        models.ExitNoLiquidity,
				ClosedAt:      time.Now(),
				TxHash:        "writeoff",
			}
			t.portfolio.Close(closed)
			if t.scalper != nil {
				go t.scalper.OnPositionClosed(closed.Mint)
			}
		} else {
			t.telegram.Alert(fmt.Sprintf("⚠️ PARTIAL EXIT FAILED: %s\n%v", sig.Mint[:8], err))
		}
		return
	}

	// Only fully close the position on full exits
	if sig.Partial == 0 {
		// Retry the Balance call — RPC blips are common right after a swap settles.
		// If it stays broken we surface the loss honestly rather than reporting a
		// fake breakeven (which would silently hide rugs from users).
		var balAfter, exitAmount float64
		var pnl, pnlPct float64
		var balOK bool
		for attempt := 0; attempt < 3; attempt++ {
			balAfter = t.Balance()
			exitAmount = balAfter - balBefore
			if exitAmount > 0 {
				balOK = true
				break
			}
			time.Sleep(time.Duration(1+attempt) * time.Second)
		}
		if balOK {
			pnl = exitAmount - pos.EntryAmountSOL
			pnlPct = (pnl / pos.EntryAmountSOL) * 100
		} else {
			// RPC didn't come back. Don't lie to the user — record the worst-case
			// (treat as total loss) so the win-rate stat doesn't get inflated by
			// fake-breakeven entries. Telegram alert flags it for manual review.
			log.Printf("[trader] %s exit balance fetch failed after retries — recording as unknown loss", util.ShortMint(pos.Mint))
			t.telegram.Alert(fmt.Sprintf("⚠️ Exit P&L unknown for %s — RPC didn't return balance. Check Solscan: %s", util.ShortMint(pos.Mint), tx.TxHash))
			exitAmount = 0
			pnl = -pos.EntryAmountSOL
			pnlPct = -100
		}

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
	m := monitor.New(pos, t.cricket, t.exitCh, t.monitorCfg, t.onProgress)
	util.Go("monitor.Watch:"+util.ShortMint(pos.Mint), func() { m.Watch(ctx) })
	log.Printf("[trader] restored position %s from DB", util.ShortMint(pos.Mint))
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

// Close exits a single position by mint address (e.g. from Telegram "Close Now" button).
func (t *Trader) Close(mint string, reason models.ExitReason) {
	select {
	case t.exitCh <- monitor.ExitSignal{Mint: mint, Reason: reason, Partial: 0}:
	default:
		log.Printf("[trader] Close: exit channel full for %s", mint[:8])
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
func (t *Trader) MaxPositionSOL() float64 { return t.maxPositionSOL }

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

	if err := validatePumpPortalTx(txBytes, t.walletAddress); err != nil {
		return nil, fmt.Errorf("pumpportal tx rejected by safety check: %w", err)
	}

	txBase64 := base64.StdEncoding.EncodeToString(txBytes)
	tx, err := t.signet.Wallets.Execute(t.walletID, txBase64)
	if err != nil {
		return nil, fmt.Errorf("signet execute: %w", err)
	}

	log.Printf("[trader] pumpportal pool=%s success for %s | tx=%s", pool, mint[:8], tx.TxHash)
	return tx, nil
}

// sellViaPumpPortal sells a token via pumpportal.fun + Signet /execute.
// partial: 0 = sell all, 0.4 = sell 40%, etc.
func (t *Trader) sellViaPumpPortal(mint string, partial float64) (*signet.TransactionResult, error) {
	if t.walletAddress == "" {
		return nil, fmt.Errorf("wallet address not set")
	}

	// Get actual token balance from Helius so pumpportal knows how many to sell.
	tokenAmt, _, err := t.tokenBalance(mint)
	if err != nil || tokenAmt == 0 {
		return nil, fmt.Errorf("could not get token balance for %s: %w", mint[:8], err)
	}
	sellAmt := tokenAmt
	if partial > 0 && partial < 1 {
		sellAmt = uint64(float64(tokenAmt) * partial)
	}

	pools := []string{"pump", "pump-amm"}
	var lastErr error
	for _, pool := range pools {
		reqBody, _ := json.Marshal(map[string]any{
			"publicKey":        t.walletAddress,
			"action":           "sell",
			"mint":             mint,
			"denominatedInSol": false,
			"amount":           sellAmt,
			"slippage":         10,
			"priorityFee":      0.0005,
			"pool":             pool,
		})
		ppResp, err := http.Post("https://pumpportal.fun/api/trade-local", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			lastErr = err
			continue
		}
		txBytes, _ := io.ReadAll(ppResp.Body)
		ppResp.Body.Close()
		if ppResp.StatusCode != 200 {
			log.Printf("[trader] pumpportal sell pool=%s failed for %s: %s", pool, mint[:8], ppResp.Status)
			lastErr = fmt.Errorf("pumpportal sell %s", ppResp.Status)
			continue
		}
		if err := validatePumpPortalTx(txBytes, t.walletAddress); err != nil {
			lastErr = fmt.Errorf("pumpportal sell tx rejected by safety check: %w", err)
			log.Printf("[trader] %v", lastErr)
			continue
		}
		tx, err := t.signet.Wallets.Execute(t.walletID, base64.StdEncoding.EncodeToString(txBytes))
		if err != nil {
			lastErr = err
			continue
		}
		log.Printf("[trader] pumpportal sell pool=%s success for %s | tx=%s", pool, mint[:8], tx.TxHash)
		return tx, nil
	}
	return nil, lastErr
}

// tokenBalance returns the raw token amount and decimals held by this wallet for a mint.
// Caller computes UI amount via raw / 10^decimals.
func (t *Trader) tokenBalance(mint string) (uint64, uint8, error) {
	if t.rpcURL == "" || t.walletAddress == "" {
		return 0, 0, fmt.Errorf("RPC or wallet address not set")
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTokenAccountsByOwner",
		"params": []any{
			t.walletAddress,
			map[string]string{"mint": mint},
			map[string]string{"encoding": "jsonParsed"},
		},
	})
	resp, err := http.Post(t.rpcURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Value []struct {
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								TokenAmount struct {
									Amount   string `json:"amount"`
									Decimals uint8  `json:"decimals"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, err
	}
	if len(result.Result.Value) == 0 {
		return 0, 0, fmt.Errorf("no token account found for mint %s", mint[:8])
	}
	info := result.Result.Value[0].Account.Data.Parsed.Info
	var amt uint64
	fmt.Sscan(info.TokenAmount.Amount, &amt)
	return amt, info.TokenAmount.Decimals, nil
}

// WalletAddress returns the cached Solana public key for this trader's wallet.
func (t *Trader) WalletAddress() string { return t.walletAddress }

// Holding is a single SPL token balance in the wallet.
type Holding struct {
	Mint     string  `json:"mint"`
	UIAmount float64 `json:"ui_amount"`
	Decimals int     `json:"decimals"`
}

// Holdings returns all non-zero SPL token balances in the wallet.
// Holdings returns all non-zero SPL token balances in the wallet.
// Queries both the legacy Token program and Token-2022 program.
func (t *Trader) Holdings() ([]Holding, error) {
	if t.rpcURL == "" || t.walletAddress == "" {
		return nil, fmt.Errorf("RPC or wallet address not set")
	}

	programs := []string{
		"TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", // SPL Token
		"TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb", // Token-2022
	}

	type tokenAccountResult struct {
		Result struct {
			Value []struct {
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								Mint        string `json:"mint"`
								TokenAmount struct {
									UIAmount float64 `json:"uiAmount"`
									Decimals int     `json:"decimals"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
	}

	var out []Holding
	for _, programID := range programs {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "getTokenAccountsByOwner",
			"params": []any{
				t.walletAddress,
				map[string]string{"programId": programID},
				map[string]string{"encoding": "jsonParsed"},
			},
		})
		resp, err := http.Post(t.rpcURL, "application/json", bytes.NewReader(body))
		if err != nil {
			continue // try next program
		}
		var result tokenAccountResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		for _, v := range result.Result.Value {
			info := v.Account.Data.Parsed.Info
			if info.TokenAmount.UIAmount > 0 {
				out = append(out, Holding{
					Mint:     info.Mint,
					UIAmount: info.TokenAmount.UIAmount,
					Decimals: info.TokenAmount.Decimals,
				})
			}
		}
	}
	return out, nil
}

// EnsureWallet creates the Solana trading wallet if it doesn't exist yet.
// ForceSell swaps 100% of a token back to SOL regardless of whether HB has an
// open position for it. Used for manual recovery of stuck tokens.
// Tries Jupiter first (via Signet), falls back to pumpportal.
func (t *Trader) ForceSell(mint string) (string, error) {
	tx, err := t.signet.Wallets.Swap(t.walletID, signet.SwapParams{
		FromToken:       mint,
		ToToken:         "SOL",
		Amount:          "100%",
		SlippageBps:     1000, // 10% — wider slippage for manual force-sell
		DeadlineSeconds: 30,
	})
	if err != nil {
		var sigErr *signet.SignetError
		notRoutable := !errors.As(err, &sigErr) || sigErr.StatusCode == 502 ||
			strings.Contains(sigErr.Message, "token_not_routable")
		if notRoutable {
			tx, err = t.sellViaPumpPortal(mint, 0)
		}
	}
	if err != nil {
		return "", err
	}
	t.markTrade()
	return tx.TxHash, nil
}

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

// validatePumpPortalTx parses the wire-format Solana tx returned by pumpportal.fun
// and rejects anything that could drain the wallet via SystemProgram::Transfer
// (or any other SystemProgram instruction at top-level), or whose fee payer is
// not our wallet. This is the only check between a compromised pumpportal API
// and Signet broadcasting whatever bytes it gets.
//
// SystemProgram::CreateAccount and Allocate are allowed — pump-amm uses them for ATA
// initialization on first buy. Only Transfer (discriminant 2) and TransferWithSeed (11)
// are blocked as those drain SOL directly.
func validatePumpPortalTx(txBytes []byte, expectedFromBase58 string) error {
	if len(txBytes) < 64 {
		return fmt.Errorf("tx too short (%d bytes)", len(txBytes))
	}
	expectedFrom, err := Base58Decode(expectedFromBase58)
	if err != nil {
		return fmt.Errorf("decode wallet address: %w", err)
	}
	if len(expectedFrom) != 32 {
		return fmt.Errorf("wallet address wrong length: %d", len(expectedFrom))
	}

	buf := txBytes

	// Signatures section: compact-u16 count, then count * 64 bytes
	sigCount, n, err := compactU16(buf)
	if err != nil {
		return fmt.Errorf("sig count: %w", err)
	}
	buf = buf[n:]
	if sigCount > 16 {
		return fmt.Errorf("absurd signature count: %d", sigCount)
	}
	if len(buf) < sigCount*64 {
		return fmt.Errorf("truncated signatures (need %d, have %d)", sigCount*64, len(buf))
	}
	buf = buf[sigCount*64:]

	// Optional v0 version byte (high bit set)
	if len(buf) == 0 {
		return fmt.Errorf("missing message body")
	}
	if buf[0]&0x80 != 0 {
		buf = buf[1:]
	}

	// Header: 3 bytes (numRequiredSignatures, numReadonlySigned, numReadonlyUnsigned)
	if len(buf) < 3 {
		return fmt.Errorf("truncated header")
	}
	buf = buf[3:]

	// Account keys
	accCount, n, err := compactU16(buf)
	if err != nil {
		return fmt.Errorf("acc count: %w", err)
	}
	buf = buf[n:]
	if accCount == 0 || accCount > 256 {
		return fmt.Errorf("absurd account count: %d", accCount)
	}
	if len(buf) < accCount*32 {
		return fmt.Errorf("truncated account keys")
	}
	accountKeys := make([][]byte, accCount)
	for i := 0; i < accCount; i++ {
		accountKeys[i] = buf[:32]
		buf = buf[32:]
	}

	// Fee payer = accountKeys[0] must be our wallet
	if !bytes.Equal(accountKeys[0], expectedFrom) {
		return fmt.Errorf("fee payer mismatch: tx fee payer is not our wallet")
	}

	// Recent blockhash: 32 bytes
	if len(buf) < 32 {
		return fmt.Errorf("truncated blockhash")
	}
	buf = buf[32:]

	// Instructions
	ixCount, n, err := compactU16(buf)
	if err != nil {
		return fmt.Errorf("ix count: %w", err)
	}
	buf = buf[n:]
	if ixCount == 0 || ixCount > 32 {
		return fmt.Errorf("ix count out of expected range: %d", ixCount)
	}

	var systemProgram [32]byte // all-zero pubkey

	for i := 0; i < ixCount; i++ {
		if len(buf) < 1 {
			return fmt.Errorf("truncated ix %d", i)
		}
		progIdx := int(buf[0])
		buf = buf[1:]
		if progIdx >= accCount {
			return fmt.Errorf("ix %d: program index %d out of range", i, progIdx)
		}
		isSystemProgram := bytes.Equal(accountKeys[progIdx], systemProgram[:])

		// Skip account indices section
		accIdxCount, n, err := compactU16(buf)
		if err != nil {
			return fmt.Errorf("ix %d acc count: %w", i, err)
		}
		buf = buf[n:]
		if len(buf) < accIdxCount {
			return fmt.Errorf("ix %d truncated accounts", i)
		}
		buf = buf[accIdxCount:]

		// Read instruction data
		dataLen, n, err := compactU16(buf)
		if err != nil {
			return fmt.Errorf("ix %d data len: %w", i, err)
		}
		buf = buf[n:]
		if len(buf) < dataLen {
			return fmt.Errorf("ix %d truncated data", i)
		}
		ixData := buf[:dataLen]
		buf = buf[dataLen:]

		if isSystemProgram {
			// Allow Transfer (2) — used for SOL→WSOL wrapping in pump-amm swaps.
			// Allow CreateAccount (0) — used for ATA initialization.
			// Block only TransferWithSeed (11) — no legitimate swap use case.
			if dataLen < 4 {
				return fmt.Errorf("ix %d: SystemProgram instruction too short", i)
			}
			discriminant := uint32(ixData[0]) | uint32(ixData[1])<<8 | uint32(ixData[2])<<16 | uint32(ixData[3])<<24
			if discriminant == 11 {
				return fmt.Errorf("ix %d: SystemProgram::TransferWithSeed denied", i)
			}
		}
	}

	return nil
}

// compactU16 decodes Solana's variable-length u16 (1-3 bytes).
func compactU16(buf []byte) (val int, n int, err error) {
	if len(buf) == 0 {
		return 0, 0, fmt.Errorf("empty")
	}
	val = int(buf[0] & 0x7F)
	if buf[0]&0x80 == 0 {
		return val, 1, nil
	}
	if len(buf) < 2 {
		return 0, 0, fmt.Errorf("truncated")
	}
	val |= int(buf[1]&0x7F) << 7
	if buf[1]&0x80 == 0 {
		return val, 2, nil
	}
	if len(buf) < 3 {
		return 0, 0, fmt.Errorf("truncated")
	}
	val |= int(buf[2]&0x03) << 14
	return val, 3, nil
}

// Base58Decode decodes a Bitcoin/Solana base58 string into bytes.
const b58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func Base58Decode(s string) ([]byte, error) {
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	var leadingZeros int
	for _, c := range s {
		if c != '1' {
			break
		}
		leadingZeros++
	}
	num := new(big.Int)
	base := big.NewInt(58)
	for _, c := range s {
		idx := strings.IndexRune(b58Alphabet, c)
		if idx < 0 {
			return nil, fmt.Errorf("invalid char %q", c)
		}
		num.Mul(num, base)
		num.Add(num, big.NewInt(int64(idx)))
	}
	out := append(make([]byte, leadingZeros), num.Bytes()...)
	return out, nil
}
