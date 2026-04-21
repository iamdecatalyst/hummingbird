package models

import "time"

// ScoreResult is produced by the Cricket scorer and passed to traders.
type ScoreResult struct {
	Mint        string                 `json:"mint"`
	DevWallet   string                 `json:"dev_wallet"`
	Platform    string                 `json:"platform"` // pump_fun | moonshot | raydium_launchlab | boop | etc.
	Total       int                    `json:"total"`
	Decision    string                 `json:"decision"` // skip | small | medium | full | scalp | swing
	PositionSOL float64                `json:"position_sol"`
	Checks      map[string]CheckResult `json:"checks"`
	ScoredAtMs  int64                  `json:"scored_at_ms"`

	// Cricket metadata for rich Telegram broadcast (populated by Python scorer)
	Rating                string   `json:"rating"`                            // low | moderate | high | critical
	MintAuthorityRevoked  *bool    `json:"mint_authority_revoked,omitempty"`
	FreezeAuthorityRevoked *bool   `json:"freeze_authority_revoked,omitempty"`
	BondingFillPct        *float64 `json:"bonding_fill_pct,omitempty"`
	DevSupplyPct          *float64 `json:"dev_supply_pct,omitempty"`
	Top10HolderPct        *float64 `json:"top_10_holder_pct,omitempty"`
	DeployerWalletAgeDays *int     `json:"deployer_wallet_age_days,omitempty"`
	DeployerPriorLaunches *int     `json:"deployer_prior_launches,omitempty"`
	FireflyScore          *int     `json:"firefly_score,omitempty"`
	FireflyWinRate        *float64 `json:"firefly_win_rate,omitempty"`
	ScanFlags             []string `json:"scan_flags,omitempty"`
	AISummary             string   `json:"ai_summary,omitempty"` // AI 2-3 sentence analysis (hunter+ tier)
}

type CheckResult struct {
	Score    int    `json:"score"`
	MaxScore int    `json:"max_score"`
	Reason   string `json:"reason"`
}

// Position tracks an open trade.
type Position struct {
	ID             string    `json:"id"`
	Mint           string    `json:"mint"`
	DevWallet      string    `json:"dev_wallet"`
	WalletID       string    `json:"wallet_id"`
	EntryPriceSOL  float64   `json:"entry_price_sol"`
	EntryAmountSOL float64   `json:"entry_amount_sol"`
	TokenBalance   float64   `json:"token_balance"`
	Score          int       `json:"score"`
	Decision       string    `json:"decision"`
	OpenedAt       time.Time `json:"opened_at"`

	Platform string `json:"platform"` // pump_fun | moonshot | raydium_launchlab | boop | etc.

	// Exit tracking
	PeakPriceSOL    float64 `json:"peak_price_sol"`
	TakeProfitLevel int     `json:"take_profit_level"`
}

// ExitReason describes why a position was closed.
type ExitReason string

const (
	ExitTakeProfit   ExitReason = "take_profit"
	ExitStopLoss     ExitReason = "stop_loss"
	ExitRugDetected  ExitReason = "rug_detected"
	ExitNoLiquidity  ExitReason = "no_liquidity"
	ExitTimeout      ExitReason = "timeout"
	ExitManual       ExitReason = "manual"
)

// ClosedPosition is a completed trade with final P&L.
type ClosedPosition struct {
	Position
	ExitPriceSOL  float64    `json:"exit_price_sol"`
	ExitAmountSOL float64    `json:"exit_amount_sol"`
	PnLSOL        float64    `json:"pnl_sol"`
	PnLPercent    float64    `json:"pnl_percent"`
	Reason        ExitReason `json:"reason"`
	ClosedAt      time.Time  `json:"closed_at"`
	TxHash        string     `json:"tx_hash"`
}
