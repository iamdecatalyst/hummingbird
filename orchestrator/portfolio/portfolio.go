package portfolio

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

// Portfolio manages all open positions and tracks P&L.
type Portfolio struct {
	mu sync.Mutex

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

	onOpen  func(*models.Position)
	onClose func(*models.ClosedPosition)
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

// SetPersistHooks registers callbacks invoked on Open and Close for DB persistence.
// Must be called before the portfolio is used. Either hook may be nil.
func (p *Portfolio) SetPersistHooks(onOpen func(*models.Position), onClose func(*models.ClosedPosition)) {
	p.onOpen = onOpen
	p.onClose = onClose
}

// CanEnter returns true if we're allowed to open a new position.
func (p *Portfolio) CanEnter() (bool, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshToday()

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
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.positions[mint]
	return ok
}

// Open records a new position and persists it via the onOpen hook if set.
func (p *Portfolio) Open(pos *models.Position) {
	p.mu.Lock()
	p.refreshToday()
	p.positions[pos.Mint] = pos
	hook := p.onOpen
	p.mu.Unlock()
	if hook != nil {
		go hook(pos)
	}
}

// RestoreOpen loads a position recovered from DB without triggering the onOpen hook.
func (p *Portfolio) RestoreOpen(pos *models.Position) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.positions[pos.Mint] = pos
}

// Get returns the position for a mint.
func (p *Portfolio) Get(mint string) (*models.Position, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pos, ok := p.positions[mint]
	return pos, ok
}

// Close records exit, updates P&L, and persists via the onClose hook if set.
func (p *Portfolio) Close(closed *models.ClosedPosition) {
	p.mu.Lock()
	p.refreshToday()
	delete(p.positions, closed.Mint)
	p.closed = append(p.closed, closed)

	p.totalPnL += closed.PnLSOL
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
	hook := p.onClose
	p.mu.Unlock()
	if hook != nil {
		go hook(closed)
	}
}

// Stats returns a snapshot of portfolio performance.
func (p *Portfolio) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()

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
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*models.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		out = append(out, pos)
	}
	return out
}

// Pause manually pauses the bot with a reason.
func (p *Portfolio) Pause(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = true
	p.pauseReason = reason
}

// Resume unpauses the bot.
func (p *Portfolio) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
	p.pauseReason = ""
}

// RecentClosed returns the last n closed positions (most recent first).
func (p *Portfolio) RecentClosed(n int) []*models.ClosedPosition {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.closed) == 0 {
		return nil
	}
	start := len(p.closed) - n
	if start < 0 {
		start = 0
	}
	out := make([]*models.ClosedPosition, len(p.closed)-start)
	for i, c := range p.closed[start:] {
		out[len(out)-1-i] = c // reverse: newest first
	}
	return out
}

// refreshToday resets daily P&L tracking when UTC date has changed.
// Must be called with p.mu held.
func (p *Portfolio) refreshToday() {
	today := time.Now().UTC().Format("2006-01-02")
	if today != p.todayDate {
		p.todayDate = today
		p.todayPnL = 0
		p.todayStartSOL = p.startingSOL + p.totalPnL
		// Lift a daily-loss pause — new day, fresh start
		if p.paused && strings.Contains(p.pauseReason, "daily loss limit") {
			p.paused = false
			p.pauseReason = ""
		}
		log.Printf("[portfolio] daily P&L reset — new trading day %s", today)
	}
}

type Stats struct {
	OpenPositions int     `json:"open_positions"`
	TotalTrades   int     `json:"total_trades"`
	Wins          int     `json:"wins"`
	Losses        int     `json:"losses"`
	WinRate       float64 `json:"win_rate"`
	TodayPnL      float64 `json:"today_pnl"`
	TotalPnL      float64 `json:"total_pnl"`
	Paused        bool    `json:"paused"`
	PauseReason   string  `json:"pause_reason"`
}
