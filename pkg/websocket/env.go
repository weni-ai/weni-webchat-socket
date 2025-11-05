package websocket

import (
    "fmt"
    "os"
    "time"
)

// DetectPodID returns the current pod/instance identifier using
// POD_NAME, then HOSTNAME, falling back to a time-based unique ID.
func DetectPodID() string {
    if v := os.Getenv("POD_NAME"); v != "" {
        return v
    }
    if v := os.Getenv("HOSTNAME"); v != "" {
        return v
    }
    return fmt.Sprintf("pod-%d", time.Now().UnixNano())
}


