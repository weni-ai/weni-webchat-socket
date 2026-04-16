package audio

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

// ConvertWebMToMP3 converts WebM audio data to MP3 using ffmpeg.
// It reads from the provided reader and returns a reader with the MP3 data.
// Requires ffmpeg to be installed on the system.
func ConvertWebMToMP3(input io.Reader) (*bytes.Buffer, error) {
	cmd := exec.Command(
		"ffmpeg",
		"-i", "pipe:0",
		"-vn",
		"-codec:a", "libmp3lame",
		"-q:a", "4",
		"-f", "mp3",
		"pipe:1",
	)

	cmd.Stdin = input

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.WithField("stderr", stderr.String()).WithError(err).Error("ffmpeg: WebM to MP3 conversion failed")
		return nil, fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	return &stdout, nil
}

// NeedsConversion reports whether the given MIME subtype is a WebM variant
// that should be converted to MP3 before upload.
func NeedsConversion(fileType string) bool {
	return fileType == "webm"
}
