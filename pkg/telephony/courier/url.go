package courier

import "strings"

const ReceivePath = "/c/tph/receive"

// ReceiveURL builds the fixed Courier inbound endpoint for PSTN transcripts.
func ReceiveURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	return base + ReceivePath
}
