package audiosocket

import "errors"

// ErrChannelNotFound indicates the DID does not resolve to a configured PSTN channel.
var ErrChannelNotFound = errors.New("pstn channel not found for did")

// SessionRegistrar creates telephony sessions from registration requests.
type SessionRegistrar interface {
	Register(did, callerID, origin string) (sessionID string, err error)
}
