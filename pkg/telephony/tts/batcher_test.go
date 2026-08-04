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
