package portfolio

import (
	"fmt"
	"sync"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

// Portfolio manages all open positions and tracks P&L.
type Portfolio struct {
	mu sync.RWMutex

	positions map[string]*models.Position // mint → position
	closed    []*models.ClosedPosition

	startingSOL    float64
	todayStartSOL  float64
	todayDate      string
	totalPnL       float64
	todayPnL       float64
	wins           int
	losses         int

	maxConcurrent   int
	maxDailyLoss    float64 // e.g. 0.30 = stop if down 30% today
	paused          bool
	pauseReason     string
}

func New(startingSOL float64, maxConcurrent int, maxDailyLoss float64) *Portfolio {
	today := time.Now().Format("2006-01-02")
	return &Portfolio{
		positions:     make(map[string]*models.Position),
		startingSOL:   startingSOL,
		todayStartSOL: startingSOL,
		todayDate:     today,
		maxConcurrent: maxConcurrent,
		maxDailyLoss:  maxDailyLoss,
	}
}

// CanEnter returns true if we're allowed to open a new position.
func (p *Portfolio) CanEnter() (bool, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.paused {
		return false, "bot paused: " + p.pauseReason
	}
	if len(p.positions) >= p.maxConcurrent {
		return false, fmt.Sprintf("max concurrent positions reached (%d)", p.maxConcurrent)
	}
	return true, ""
}

// AlreadyOpen returns true if we already have a position in this mint.
func (p *Portfolio) AlreadyOpen(mint string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.positions[mint]
	return ok
}

// Open records a new position.
func (p *Portfolio) Open(pos *models.Position) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.positions[pos.Mint] = pos
}

// Get returns the position for a mint.
func (p *Portfolio) Get(mint string) (*models.Position, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pos, ok := p.positions[mint]
	return pos, ok
}

// Close records exit and updates P&L.
func (p *Portfolio) Close(closed *models.ClosedPosition) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.positions, closed.Mint)
	p.closed = append(p.closed, closed)

	p.totalPnL += closed.PnLSOL
	p.refreshToday()
	p.todayPnL += closed.PnLSOL

	if closed.PnLSOL >= 0 {
		p.wins++
	} else {
		p.losses++
	}

	// Daily loss check
	if p.todayPnL < 0 {
		lossPercent := (-p.todayPnL) / p.todayStartSOL
		if lossPercent >= p.maxDailyLoss {
			p.paused = true
			p.pauseReason = fmt.Sprintf("daily loss limit hit (%.0f%%)", lossPercent*100)
		}
	}
}

// Stats returns a snapshot of portfolio performance.
func (p *Portfolio) Stats() Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := p.wins + p.losses
	winRate := 0.0
	if total > 0 {
		winRate = float64(p.wins) / float64(total) * 100
	}

	return Stats{
		OpenPositions: len(p.positions),
		TotalTrades:   total,
		Wins:          p.wins,
		Losses:        p.losses,
		WinRate:       winRate,
		TodayPnL:      p.todayPnL,
		TotalPnL:      p.totalPnL,
		Paused:        p.paused,
		PauseReason:   p.pauseReason,
	}
}

// OpenPositions returns a snapshot of all open positions.
func (p *Portfolio) OpenPositions() []*models.Position {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*models.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		out = append(out, pos)
	}
	return out
}

// Resume unpauses the bot.
func (p *Portfolio) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
	p.pauseReason = ""
}

func (p *Portfolio) refreshToday() {
	today := time.Now().Format("2006-01-02")
	if today != p.todayDate {
		p.todayDate = today
		p.todayPnL = 0
		p.todayStartSOL = p.startingSOL + p.totalPnL
	}
}

type Stats struct {
	OpenPositions int
	TotalTrades   int
	Wins          int
	Losses        int
	WinRate       float64
	TodayPnL      float64
	TotalPnL      float64
	Paused        bool
	PauseReason   string
}
