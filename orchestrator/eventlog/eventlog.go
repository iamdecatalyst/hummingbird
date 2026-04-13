// Package eventlog provides a per-user event log with an optional persist callback.
package eventlog

import (
	"sync"
	"time"
)

const maxEvents = 500

type Event struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`              // ENTER EXIT START STOP ALERT INFO
	Token   string    `json:"token,omitempty"`
	AmtSOL  float64   `json:"amount_sol,omitempty"`
	PnLSOL  float64   `json:"pnl_sol,omitempty"`
	PnLPct  float64   `json:"pnl_pct,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	TxHash  string    `json:"tx_hash,omitempty"`
	Message string    `json:"message"`
}

// Log is a per-user event log. persist is called on every new event (may be nil).
type Log struct {
	mu      sync.RWMutex
	events  []Event
	persist func(Event)
}

func New(persist func(Event)) *Log {
	return &Log{persist: persist}
}

func (l *Log) Emit(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	l.mu.Lock()
	l.events = append(l.events, e)
	if len(l.events) > maxEvents {
		l.events = l.events[len(l.events)-maxEvents:]
	}
	l.mu.Unlock()
	if l.persist != nil {
		l.persist(e)
	}
}

// Load pre-populates the log from stored events without calling persist.
func (l *Log) Load(events []Event) {
	if len(events) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(events, l.events...)
	if len(l.events) > maxEvents {
		l.events = l.events[len(l.events)-maxEvents:]
	}
}

// All returns events newest-first.
func (l *Log) All() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Event, len(l.events))
	for i, e := range l.events {
		out[len(l.events)-1-i] = e
	}
	return out
}

// global is used only in single-tenant mode.
var global = New(nil)

func Emit(e Event) { global.Emit(e) }
func All() []Event { return global.All() }
