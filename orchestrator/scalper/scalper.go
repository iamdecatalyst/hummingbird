// Package scalper finds second-wave entries using Cricket Firefly signals.
// Replaces the Python scalper service — no more DexScreener polling, no more
// manual pattern detection. Cricket's smart-money signal engine handles it.
package scalper

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/cricket"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

const (
	scanInterval  = 30 * time.Second
	scalpeSOL     = 0.05 // position size for scalp entries
)

// Dispatch is a function that routes a score result to one or more traders.
// Single-tenant: calls tr.Execute(result).
// Multi-tenant: calls each user's tr.Execute(result).
type Dispatch func(*models.ScoreResult)

// Scalper watches Cricket Firefly signals and dispatches scalp entries.
type Scalper struct {
	cricket  *cricket.Client
	dispatch Dispatch
	active   sync.Map // token_address → struct{} for dedup
}

// New creates a Scalper. dispatch is called for each actionable signal.
func New(cc *cricket.Client, dispatch Dispatch) *Scalper {
	return &Scalper{cricket: cc, dispatch: dispatch}
}

// SetDispatch updates the dispatch function. Safe to call before Run starts.
func (s *Scalper) SetDispatch(dispatch Dispatch) {
	s.dispatch = dispatch
}

// Run starts the signal scanning loop. Call in a goroutine.
func (s *Scalper) Run(ctx context.Context) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	log.Printf("[scalper] started — Cricket Firefly signals every %s", scanInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

// OnPositionClosed frees a token slot so it can be scalped again if the pattern repeats.
// Call this from trader when a scalp position closes.
func (s *Scalper) OnPositionClosed(mint string) {
	s.active.Delete(mint)
}

func (s *Scalper) scan(ctx context.Context) {
	scanCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	resp, err := s.cricket.FireflySignals(scanCtx)
	if err != nil {
		log.Printf("[scalper] signals error: %v", err)
		return
	}

	for _, sig := range resp.Data {
		if sig.SignalType != "accumulation" {
			continue
		}
		if sig.Strength == "weak" {
			continue
		}
		if _, active := s.active.Load(sig.TokenAddress); active {
			continue
		}

		log.Printf("[scalper] 🎯 accumulation on %s (%s) strength=%s smart_wallets=%d",
			sig.TokenSymbol, short(sig.TokenAddress), sig.Strength, sig.Evidence.SmartWalletsCount)

		s.active.Store(sig.TokenAddress, struct{}{})
		s.dispatch(&models.ScoreResult{
			Mint:        sig.TokenAddress,
			Total:       75,
			Decision:    "scalp",
			PositionSOL: scalpeSOL,
			Checks: map[string]models.CheckResult{
				"firefly": {
					Score:    sig.Evidence.AvgWalletScore,
					MaxScore: 100,
					Reason:   "accumulation/" + sig.Strength,
				},
			},
			ScoredAtMs: time.Now().UnixMilli(),
		})
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
