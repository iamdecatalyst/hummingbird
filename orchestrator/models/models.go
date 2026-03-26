package models

import "time"

// ScoreResult is received from the Python scorer.
type ScoreResult struct {
	Mint        string                 `json:"mint"`
	Total       int                    `json:"total"`
	Decision    string                 `json:"decision"` // skip | small | medium | full
	PositionSOL float64                `json:"position_sol"`
	Checks      map[string]CheckResult `json:"checks"`
	ScoredAtMs  int64                  `json:"scored_at_ms"`
}

type CheckResult struct {
	Score    int    `json:"score"`
	MaxScore int    `json:"max_score"`
	Reason   string `json:"reason"`
}

// Position tracks an open trade.
type Position struct {
	ID             string
	Mint           string
	WalletID       string
	EntryPriceSOL  float64 // price in SOL at entry
	EntryAmountSOL float64 // SOL spent to enter
	TokenBalance   float64 // token units held
	Score          int
	OpenedAt       time.Time

	// Exit tracking
	PeakPriceSOL    float64
	TakeProfitLevel int // 0 = none hit, 1 = first hit (2x), 2 = second (5x)
}

// ExitReason describes why a position was closed.
type ExitReason string

const (
	ExitTakeProfit  ExitReason = "take_profit"
	ExitStopLoss    ExitReason = "stop_loss"
	ExitRugDetected ExitReason = "rug_detected"
	ExitTimeout     ExitReason = "timeout"
	ExitManual      ExitReason = "manual"
)

// ClosedPosition is a completed trade with final P&L.
type ClosedPosition struct {
	Position
	ExitPriceSOL   float64
	ExitAmountSOL  float64
	PnLSOL         float64
	PnLPercent     float64
	Reason         ExitReason
	ClosedAt       time.Time
	TxHash         string
}
