package session

import log "github.com/sirupsen/logrus"

// logFields returns structured log fields consistent with pkg/grpc and pkg/websocket conventions.
func (cs *CallSession) logFields() log.Fields {
	return log.Fields{
		"session_id":   cs.ID,
		"channel_uuid": cs.ChannelUUID,
		"project_uuid": cs.ProjectUUID,
		"contact_urn":  cs.ContactURN,
		"state":        cs.CurrentState(),
	}
}
