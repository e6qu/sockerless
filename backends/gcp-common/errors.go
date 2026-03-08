package gcpcommon

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// MapGCPError converts common GCP SDK errors to api error types.
func MapGCPError(err error, resource, id string) error {
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
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
