package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVoiceErrorString(t *testing.T) {
	var nilErr *VoiceError
	assert.Empty(t, nilErr.Error())

	err := &VoiceError{Message: "stt unavailable"}
	assert.Equal(t, "stt unavailable", err.Error())
}

func TestRecoverable(t *testing.T) {
	assert.False(t, Recoverable(nil))
	assert.False(t, Recoverable(errors.New("generic")))

	assert.True(t, Recoverable(&VoiceError{Recoverable: true}))
	assert.False(t, Recoverable(&VoiceError{Recoverable: false}))
}
