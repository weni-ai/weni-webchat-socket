package session

import (
	"sync"
	"time"

	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
)

const setupKeepaliveInterval = 20 * time.Millisecond

// setupAudioKeepalive streams silent AudioSocket frames while the gateway prepares
// a call. Asterisk's app_audiosocket aborts after 2 s without channel or socket activity.
type setupAudioKeepalive struct {
	conn     audiosocket.AudioSocketConn
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	paused   bool
}

func startSetupAudioKeepalive(conn audiosocket.AudioSocketConn) *setupAudioKeepalive {
	if conn == nil {
		return nil
	}
	k := &setupAudioKeepalive{
		conn: conn,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go k.run()
	return k
}

func (k *setupAudioKeepalive) run() {
	defer close(k.done)

	silence := make([]byte, audioFrameSize)
	ticker := time.NewTicker(setupKeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-k.stop:
			return
		case <-ticker.C:
			if k.isPaused() {
				continue
			}
			if err := k.conn.WriteAudio(silence); err != nil {
				return
			}
		}
	}
}

func (k *setupAudioKeepalive) isPaused() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.paused
}

func (k *setupAudioKeepalive) Pause() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.paused = true
	k.mu.Unlock()
}

func (k *setupAudioKeepalive) Resume() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.paused = false
	k.mu.Unlock()
}

func (k *setupAudioKeepalive) Stop() {
	if k == nil {
		return
	}
	k.stopOnce.Do(func() {
		close(k.stop)
	})
	<-k.done
}
