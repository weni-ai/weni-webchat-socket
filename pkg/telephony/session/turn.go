package session

import (
	"strings"
	"time"
)

// Turn tracks a single caller/agent exchange within a CallSession.
type Turn struct {
	MsgID         string
	CommittedText string
	StartedAt     time.Time
	DeltaBuffer   strings.Builder
	BatchesIssued int
	Interrupted   bool
}
