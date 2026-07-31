package tts

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	log "github.com/sirupsen/logrus"
)

// AudioChunk is a PCM fragment emitted by TTSBatcher for sequential playback.
type AudioChunk struct {
	PCM         []byte
	BatchEnd    bool
	StreamEnd   bool
	Interrupted bool
}

// TTSBatcher accumulates delta text and issues TTS requests at sentence boundaries.
type TTSBatcher struct {
	mu         sync.Mutex
	buffer     strings.Builder
	minChars   int64
	voiceID    string
	language   string
	client     TTSStreamClient
	out        chan AudioChunk
	batchQueue chan string
	ctx        context.Context
	cancel     context.CancelFunc
	finalFlush      bool
	streamEndSent   bool
	pending         int32
	closed          bool
	workerDone      chan struct{}
}

// NewTTSBatcher creates a batcher that synthesizes batches sequentially.
func NewTTSBatcher(client TTSStreamClient, voiceID, language string, minChars int64) *TTSBatcher {
	if minChars <= 0 {
		minChars = 40
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &TTSBatcher{
		minChars:   minChars,
		voiceID:    voiceID,
		language:   language,
		client:     client,
		out:        make(chan AudioChunk, 16),
		batchQueue: make(chan string, 8),
		ctx:        ctx,
		cancel:     cancel,
		workerDone: make(chan struct{}),
	}
	go b.worker()
	return b
}

// Append adds delta text and extracts batches when boundaries or thresholds are met.
func (b *TTSBatcher) Append(delta string) {
	if delta == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.buffer.WriteString(delta)
	b.extractBatchesLocked()
}

// Flush emits remaining buffered text. When final is true, signals stream completion after all batches finish.
func (b *TTSBatcher) Flush(final bool) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	if final {
		remaining := strings.TrimSpace(b.buffer.String())
		b.buffer.Reset()
		if remaining != "" && isSpeakable(remaining) {
			b.enqueueLocked(remaining)
		}
		b.finalFlush = true
	}
	b.mu.Unlock()
	b.checkComplete()
}

// Output returns the channel of audio chunks for sequential playback.
func (b *TTSBatcher) Output() <-chan AudioChunk {
	return b.out
}

// Discard cancels in-flight synthesis, drops queued output, and clears buffered text.
// Used by barge-in to stop agent playback immediately.
func (b *TTSBatcher) Discard() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.buffer.Reset()
	b.finalFlush = false
	b.streamEndSent = true
	b.mu.Unlock()

	b.cancel()

	for {
		select {
		case <-b.out:
		default:
			goto drained
		}
	}
drained:

	select {
	case b.out <- AudioChunk{Interrupted: true}:
	default:
	}
}

// Close stops the batcher and closes the output channel.
func (b *TTSBatcher) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()

	b.cancel()
	close(b.batchQueue)
	<-b.workerDone
	close(b.out)
}

func (b *TTSBatcher) extractBatchesLocked() {
	for {
		text := b.buffer.String()
		if text == "" {
			return
		}

		if idx := findSentenceEnd(text); idx >= 0 {
			batch := strings.TrimSpace(text[:idx+1])
			remainder := text[idx+1:]
			b.buffer.Reset()
			b.buffer.WriteString(remainder)
			if batch != "" && isSpeakable(batch) {
				b.enqueueLocked(batch)
			}
			continue
		}

		if int64(len(text)) >= b.minChars {
			batch := strings.TrimSpace(text)
			b.buffer.Reset()
			if batch != "" && isSpeakable(batch) {
				b.enqueueLocked(batch)
			}
		}
		return
	}
}

func (b *TTSBatcher) enqueueLocked(text string) {
	atomic.AddInt32(&b.pending, 1)
	select {
	case b.batchQueue <- text:
	case <-b.ctx.Done():
		atomic.AddInt32(&b.pending, -1)
	}
}

func (b *TTSBatcher) worker() {
	defer close(b.workerDone)
	for text := range b.batchQueue {
		b.synthesizeBatch(text)
		if atomic.AddInt32(&b.pending, -1) == 0 {
			b.checkComplete()
		}
	}
}

func (b *TTSBatcher) synthesizeBatch(text string) {
	audioCh, err := b.client.Synthesize(b.ctx, text, b.voiceID, b.language)
	if err != nil {
		log.WithFields(log.Fields{
			"text_len": len(text),
		}).WithError(err).Warn("telephony: TTS batch synthesis failed, skipping")
		b.emitBatchEnd()
		return
	}

	for chunk := range audioCh {
		select {
		case b.out <- AudioChunk{PCM: chunk}:
		case <-b.ctx.Done():
			return
		}
	}
	b.emitBatchEnd()
}

func (b *TTSBatcher) emitBatchEnd() {
	select {
	case b.out <- AudioChunk{BatchEnd: true}:
	case <-b.ctx.Done():
	}
}

func (b *TTSBatcher) checkComplete() {
	b.mu.Lock()
	if !b.finalFlush || b.closed || b.streamEndSent {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	if atomic.LoadInt32(&b.pending) != 0 {
		return
	}

	b.mu.Lock()
	if b.streamEndSent {
		b.mu.Unlock()
		return
	}
	b.streamEndSent = true
	b.mu.Unlock()

	select {
	case b.out <- AudioChunk{StreamEnd: true}:
	case <-b.ctx.Done():
	}
}

func findSentenceEnd(text string) int {
	runes := []rune(text)
	for i, r := range runes {
		switch r {
		case '!', '?':
			return i
		case '.':
			if i == len(runes)-1 {
				return i
			}
			if unicode.IsSpace(runes[i+1]) {
				return i
			}
		}
	}
	return -1
}

func isSpeakable(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if isURLOnly(trimmed) || isCodeFenceOnly(trimmed) || isEmojiOnly(trimmed) {
		return false
	}
	return true
}

func isURLOnly(text string) bool {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimRight(trimmed, ".,!?")
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isCodeFenceOnly(text string) bool {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimRight(trimmed, ".,!?")
	return strings.HasPrefix(trimmed, "```") && strings.HasSuffix(trimmed, "```")
}

func isEmojiOnly(text string) bool {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimRight(trimmed, ".,!?")
	if trimmed == "" {
		return true
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}
