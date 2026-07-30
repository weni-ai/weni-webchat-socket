package stt

// EventKind identifies an STT session event type.
type EventKind int

const (
	EventPartialTranscript EventKind = iota
	EventCommittedTranscript
	EventClosed
)

// Event is a tagged union of STT session events.
type Event struct {
	Kind EventKind

	PartialTranscript   PartialTranscript
	CommittedTranscript CommittedTranscript
	Closed              Closed
}

// PartialTranscript carries in-progress recognition text.
type PartialTranscript struct {
	Text string
}

// CommittedTranscript carries a finalized recognition result.
type CommittedTranscript struct {
	Text string
}

// Closed signals the STT session ended.
type Closed struct {
	Err error
}

// STTSession abstracts a gateway-side ElevenLabs STT WebSocket session.
type STTSession interface {
	Send(audio []byte) error
	Events() <-chan Event
	Close() error
}
