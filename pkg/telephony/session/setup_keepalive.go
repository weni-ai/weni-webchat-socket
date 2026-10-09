package session

import (
	"sync"
	"time"

	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
)

const audioKeepaliveInterval = 20 * time.Millisecond

// audioKeepalive streams silent AudioSocket frames so Asterisk sees socket activity.
// app_audiosocket aborts after 2 s without channel or socket activity.
type audioKeepalive struct {
	conn     audiosocket.AudioSocketConn
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	paused   bool
}

func startAudioKeepalive(conn audiosocket.AudioSocketConn) *audioKeepalive {
	if conn == nil {
		return nil
	}
	k := &audioKeepalive{
		conn: conn,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go k.run()
	return k
}

// startSetupAudioKeepalive is kept for tests.
func startSetupAudioKeepalive(conn audiosocket.AudioSocketConn) *audioKeepalive {
	return startAudioKeepalive(conn)
}

func (k *audioKeepalive) run() {
	defer close(k.done)

	silence := make([]byte, audioFrameSize)
	ticker := time.NewTicker(audioKeepaliveInterval)
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

func (k *audioKeepalive) isPaused() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.paused
}

func (k *audioKeepalive) Pause() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.paused = true
	k.mu.Unlock()
}

func (k *audioKeepalive) Resume() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.paused = false
	k.mu.Unlock()
}

func (k *audioKeepalive) Stop() {
	if k == nil {
		return
	}
	k.stopOnce.Do(func() {
		close(k.stop)
	})
	<-k.done
}

func (cs *CallSession) ensureAudioKeepalive() {
	if cs == nil || cs.Conn == nil {
		return
	}
	cs.keepaliveMu.Lock()
	defer cs.keepaliveMu.Unlock()
	if cs.audioKeepalive != nil {
		return
	}
	cs.audioKeepalive = startAudioKeepalive(cs.Conn)
}

func (cs *CallSession) stopAudioKeepalive() {
	if cs == nil {
		return
	}
	cs.keepaliveMu.Lock()
	k := cs.audioKeepalive
	cs.audioKeepalive = nil
	cs.keepaliveMu.Unlock()
	k.Stop()
}

func (cs *CallSession) pauseAudioKeepalive() {
	cs.keepaliveMu.Lock()
	k := cs.audioKeepalive
	cs.keepaliveMu.Unlock()
	k.Pause()
}

func (cs *CallSession) resumeAudioKeepalive() {
	cs.keepaliveMu.Lock()
	k := cs.audioKeepalive
	cs.keepaliveMu.Unlock()
	k.Resume()
}
