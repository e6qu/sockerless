package gcpcommon

import "cloud.google.com/go/logging"

// ExtractLogLine returns the text of a Cloud Logging entry. Only a
// workload's stdout and stderr carry a string payload; a structured
// payload (an audit log, a system event) yields the empty string rather
// than a stringified proto, so a misconfigured log filter can never leak
// platform records into a container's log stream.
func ExtractLogLine(entry *logging.Entry) string {
	if entry.Payload == nil {
		return ""
	}
	if s, ok := entry.Payload.(string); ok {
		return s
	}
	return ""
}
