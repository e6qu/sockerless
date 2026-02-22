package gcf

import (
	"fmt"

	"github.com/sockerless/api"
)

// mapGCPError converts common GCP errors to api errors.
func mapGCPError(err error, resource, id string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	switch {
	case containsAny(msg, "not found", "NotFound", "404"):
		return &api.NotFoundError{Resource: resource, ID: id}
	case containsAny(msg, "already exists", "AlreadyExists", "409"):
		return &api.ConflictError{Message: fmt.Sprintf("%s %s already exists", resource, id)}
	case containsAny(msg, "InvalidArgument", "invalid", "400"):
		return &api.InvalidParameterError{Message: msg}
	default:
		return fmt.Errorf("%s %s: %w", resource, id, err)
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
