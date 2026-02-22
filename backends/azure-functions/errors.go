package azf

import (
	"fmt"

	"github.com/sockerless/api"
)

// mapAzureError converts common Azure errors to api errors.
func mapAzureError(err error, resource, id string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	switch {
	case containsAny(msg, "not found", "NotFound", "ResourceNotFound"):
		return &api.NotFoundError{Resource: resource, ID: id}
	case containsAny(msg, "already exists", "Conflict"):
		return &api.ConflictError{Message: fmt.Sprintf("%s %s already exists", resource, id)}
	case containsAny(msg, "InvalidParameter", "BadRequest"):
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
