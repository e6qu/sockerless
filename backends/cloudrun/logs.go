package cloudrun

import (
	"cloud.google.com/go/logging"
)

// extractLogLine gets the text content from a Cloud Logging entry.
// Only stdout/stderr from Cloud Run carry a string Payload; structured
// payloads (audit logs, system events) are rejected outright rather
// than %v-stringified so a misconfigured logName filter can never leak
// AuditLog protos into a container's stdout stream.
func extractLogLine(entry *logging.Entry) string {
	if entry.Payload == nil {
		return ""
	}
	if s, ok := entry.Payload.(string); ok {
		return s
	}
	return ""
}
