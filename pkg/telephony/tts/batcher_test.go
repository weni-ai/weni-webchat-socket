package tts

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingTTSClient struct {
	mu        sync.Mutex
	calls     []string
	languages []string
	errOn     map[int]error
}

func (r *recordingTTSClient) Synthesize(_ context.Context, text, _, language string) (<-chan []byte, error) {
	r.mu.Lock()
	idx := len(r.calls)
	r.calls = append(r.calls, text)
	r.languages = append(r.languages, language)
	err := r.errOn[idx]
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}

	ch := make(chan []byte, 1)
	ch <- []byte{byte(idx), 0x01, 0x02}
	close(ch)
	return ch, nil
}

func (r *recordingTTSClient) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recordingTTSClient) Languages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.languages...)
}

func drainBatcher(t *testing.T, b *TTSBatcher, final bool) {
	t.Helper()
	if final {
		b.Flush(true)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case chunk, ok := <-b.Output():
			if !ok {
				return
			}
			if chunk.StreamEnd {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for batcher to complete")
		}
	}
}

func TestTTSBatcherSentenceBoundaryBatching(t *testing.T) {
	client := &recordingTTSClient{}
	b := NewTTSBatcher(client, "voice-1", "en", 40)
	defer b.Close()

	b.Append("Hello. ")
	b.Append("How are you? ")
	b.Append("Great.")
	drainBatcher(t, b, true)

	calls := client.Calls()
	assert.Equal(t, []string{"Hello.", "How are you?", "Great."}, calls)
}

func TestTTSBatcherMinThresholdBatching(t *testing.T) {
	client := &recordingTTSClient{}
	longText := "This is a long sentence without punctuation until the end maybe"
	require.GreaterOrEqual(t, int64(len(longText)), int64(40))

	b := NewTTSBatcher(client, "voice-1", "en", 40)
	defer b.Close()

	b.Append(longText)

	deadline := time.After(2 * time.Second)
	for len(client.Calls()) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for min-threshold batch")
		case <-time.After(10 * time.Millisecond):
		}
	}

	calls := client.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, longText, calls[0])
}

func TestTTSBatcherMixedPunctuationAndThreshold(t *testing.T) {
	client := &recordingTTSClient{}
	b := NewTTSBatcher(client, "voice-1", "en", 10)
	defer b.Close()

	b.Append("Hi! ")
	b.Append("Ok")
	drainBatcher(t, b, true)

	calls := client.Calls()
	assert.Equal(t, []string{"Hi!", "Ok"}, calls)
}

func TestTTSBatcherFinalFlushRemainingBuffer(t *testing.T) {
	client := &recordingTTSClient{}
	b := NewTTSBatcher(client, "voice-1", "en", 40)
	defer b.Close()

	b.Append("No trailing boundary here")
	drainBatcher(t, b, true)

	calls := client.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "No trailing boundary here", calls[0])
}

func TestTTSBatcherSkipsNonSpeakableContent(t *testing.T) {
	client := &recordingTTSClient{}
	b := NewTTSBatcher(client, "voice-1", "en", 40)
	defer b.Close()

	b.Append("https://example.com/path. ")
	b.Append("👍🎉. ")
	b.Append("```go\nfmt.Println(\"hi\")\n```. ")
	b.Append("Actual speech.")
	drainBatcher(t, b, true)

	calls := client.Calls()
	assert.Equal(t, []string{"Actual speech."}, calls)
}

func TestTTSBatcherSkipsFailedBatchAndContinues(t *testing.T) {
	client := &recordingTTSClient{
		errOn: map[int]error{0: errors.New("tts unavailable")},
	}
	b := NewTTSBatcher(client, "voice-1", "en", 40)
	defer b.Close()

	b.Append("First sentence. ")
	b.Append("Second sentence.")
	drainBatcher(t, b, true)

	calls := client.Calls()
	assert.Equal(t, []string{"First sentence.", "Second sentence."}, calls)
}

func TestTTSBatcherEmitsBatchEndMarkers(t *testing.T) {
	client := &recordingTTSClient{}
	b := NewTTSBatcher(client, "voice-1", "en", 40)
	defer b.Close()

	b.Append("One. Two.")
	b.Flush(true)

	var batchEnds int
	var streamEnd bool
	deadline := time.After(2 * time.Second)
	for !streamEnd {
		select {
		case chunk, ok := <-b.Output():
			if !ok {
				streamEnd = true
				break
			}
			if chunk.BatchEnd {
				batchEnds++
			}
			if chunk.StreamEnd {
				streamEnd = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for batcher output")
		}
	}

	assert.Equal(t, 2, batchEnds)
	assert.True(t, streamEnd)
}

func TestTTSBatcherCarriesResolvedLanguage(t *testing.T) {
	client := &recordingTTSClient{}
	b := NewTTSBatcher(client, "voice-1", "pt", 40)
	defer b.Close()

	b.Append("Olá. ")
	b.Append("Como vai?")
	drainBatcher(t, b, true)

	languages := client.Languages()
	require.Len(t, languages, 2)
	assert.Equal(t, []string{"pt", "pt"}, languages)
}

// TestTTSBatcherCreditEfficiencyParameterized verifies SC-005: TTS requests stay within
// roughly one per sentence across realistic multi-sentence agent responses.
func TestTTSBatcherCreditEfficiencyParameterized(t *testing.T) {
	cases := []struct {
		name     string
		deltas   []string
		language string
		minChars int64
		wantMin  int
		wantMax  int
	}{
		{
			name: "three short sentences as many small deltas",
			deltas: []string{
				"Hel", "lo. ", "How ", "are ", "you? ", "I ", "am ", "fine.",
			},
			wantMin: 3,
			wantMax: 4,
		},
		{
			name: "three long sentences one delta each",
			deltas: []string{
				"The product catalog includes thousands of items across multiple categories.",
				" Shipping options vary by region and order weight for each destination.",
				" Contact support if you need help choosing the right option.",
			},
			wantMin: 3,
			wantMax: 4,
		},
		{
			name: "punctuation edge cases",
			deltas: []string{
				"Wait... ", "Really?! ", "Yes, please.",
			},
			wantMin: 3,
			wantMax: 4,
		},
		{
			name:     "mixed language portuguese",
			language: "pt",
			deltas: []string{
				"Olá! ", "Como ", "posso ", "ajudar? ", "Estou ", "à ", "disposição.",
			},
			wantMin: 3,
			wantMax: 4,
		},
		{
			name: "question and exclamation mix",
			deltas: []string{
				"Can you hear me? ", "Great! ", "Let's continue.",
			},
			wantMin: 3,
			wantMax: 4,
		},
		{
			name: "five sentence response stays near one request per sentence",
			deltas: []string{
				"One. ", "Two. ", "Three. ", "Four. ", "Five.",
			},
			wantMin: 5,
			wantMax: 6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			language := tc.language
			if language == "" {
				language = "en"
			}
			minChars := tc.minChars
			if minChars == 0 {
				minChars = 40
			}

			client := &recordingTTSClient{}
			b := NewTTSBatcher(client, "voice-1", language, minChars)
			defer b.Close()

			for _, delta := range tc.deltas {
				b.Append(delta)
			}
			drainBatcher(t, b, true)

			calls := client.Calls()
			assert.GreaterOrEqual(t, len(calls), tc.wantMin,
				"expected at least %d TTS requests, got %d: %v", tc.wantMin, len(calls), calls)
			assert.LessOrEqual(t, len(calls), tc.wantMax,
				"SC-005 budget exceeded: expected at most %d TTS requests, got %d: %v", tc.wantMax, len(calls), calls)
		})
	}
}

func TestTTSBatcherDiscardCancelsInFlight(t *testing.T) {
	slowClient := &slowTTSClient{}
	b := NewTTSBatcher(slowClient, "voice-1", "en", 40)
	defer b.Close()

	b.Append("Long unfinished agent response without punctuation")
	time.Sleep(50 * time.Millisecond)
	b.Discard()

	deadline := time.After(500 * time.Millisecond)
	for !slowClient.WasCancelled() {
		select {
		case <-deadline:
			t.Fatal("expected in-flight TTS to be cancelled on discard")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type slowTTSClient struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	cancelled bool
}

func (c *slowTTSClient) Synthesize(ctx context.Context, _, _, _ string) (<-chan []byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	ch := make(chan []byte)
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		c.cancelled = true
		c.mu.Unlock()
		close(ch)
	}()
	return ch, nil
}

func (c *slowTTSClient) WasCancelled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancelled
}
