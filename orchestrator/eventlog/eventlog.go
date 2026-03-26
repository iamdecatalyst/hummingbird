// Package eventlog provides a global in-memory ring buffer of bot events.
package eventlog

import (
	"sync"
	"time"
)

const maxEvents = 200

type Event struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`              // ENTER EXIT START STOP ALERT INFO
	Token   string    `json:"token,omitempty"`
	AmtSOL  float64   `json:"amount_sol,omitempty"`
	PnLSOL  float64   `json:"pnl_sol,omitempty"`
	PnLPct  float64   `json:"pnl_pct,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message"`
}

var (
	mu     sync.RWMutex
	events []Event
)

func Emit(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	mu.Lock()
	defer mu.Unlock()
	events = append(events, e)
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}
}

// All returns events newest-first.
func All() []Event {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Event, len(events))
	for i, e := range events {
		out[len(events)-1-i] = e
	}
	return out
}
