package audiosocket

import "errors"

// ErrChannelNotFound indicates the DID does not resolve to a configured PSTN channel.
var ErrChannelNotFound = errors.New("pstn channel not found for did")

// ErrSTTDependencyDown indicates channel resolution succeeded but STT cannot be initialized.
var ErrSTTDependencyDown = errors.New("stt dependency unavailable")

// SessionRegistrar creates telephony sessions from registration requests.
type SessionRegistrar interface {
	Register(did, callerID, origin string) (sessionID string, err error)
}
