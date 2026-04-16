package audio

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func TestNeedsConversion(t *testing.T) {
	tests := []struct {
		fileType string
		want     bool
	}{
		{"webm", true},
		{"mp3", false},
		{"ogg", false},
		{"wav", false},
		{"mp4", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.fileType, func(t *testing.T) {
			assert.Equal(t, tt.want, NeedsConversion(tt.fileType))
		})
	}
}

func TestConvertWebMToMP3_InvalidInput(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not installed")
	}

	buf, err := ConvertWebMToMP3(bytes.NewReader([]byte("not valid webm")))
	require.Error(t, err)
	assert.Nil(t, buf)
}
