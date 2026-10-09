package session

import (
	"sync/atomic"
	"time"
)

// BargeInController gates barge-in triggers to the Speaking state and invokes
// the configured callback when caller speech is detected via STT partials.
type BargeInController struct {
	armed     atomic.Bool
	onTrigger func(triggeredAt time.Time)
}

// NewBargeInController creates a controller that invokes onTrigger when armed.
func NewBargeInController(onTrigger func(triggeredAt time.Time)) *BargeInController {
	return &BargeInController{onTrigger: onTrigger}
}

// SetArmed toggles whether partial transcripts should trigger barge-in.
func (b *BargeInController) SetArmed(armed bool) {
	b.armed.Store(armed)
}

// IsArmed reports whether barge-in triggers are currently accepted.
func (b *BargeInController) IsArmed() bool {
	return b.armed.Load()
}

// Trigger invokes the barge-in callback when armed.
func (b *BargeInController) Trigger() {
	if !b.armed.Load() {
		return
	}
	if b.onTrigger != nil {
		b.onTrigger(time.Now())
	}
}
