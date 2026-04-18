// Package util provides cross-cutting helpers used by every orchestrator package.
package util

import (
	"log"
	"runtime/debug"
)

// Go runs fn in a new goroutine with panic recovery. A panic in any single
// per-user trader/monitor/bot goroutine would otherwise take down the whole
// orchestrator process and unwatch every user's open positions until restart.
//
// The label is included in the log so the source goroutine is identifiable.
func Go(label string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic recovered in %s] %v\n%s", label, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// ShortMint returns the first 8 chars of a mint address, or the full string if
// it's shorter. Use instead of raw `mint[:8]` slicing — attacker-controllable
// inputs (short mints from the unauthenticated path before B1, plus future
// surfaces) will panic on raw slicing.
func ShortMint(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
