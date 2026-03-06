package gcf

import (
	"fmt"

	"cloud.google.com/go/logging"
)

// extractLogLine gets the text content from a Cloud Logging entry.
func extractLogLine(entry *logging.Entry) string {
	if entry.Payload == nil {
		return ""
	}
	switch p := entry.Payload.(type) {
	case string:
		return p
	default:
		return fmt.Sprintf("%v", p)
	}
}
