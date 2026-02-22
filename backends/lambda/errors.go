package lambda

import (
	"fmt"

	"github.com/sockerless/api"
)

// mapAWSError converts common AWS errors to api errors.
func mapAWSError(err error, resource, id string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	switch {
	case containsAny(msg, "not found", "does not exist", "ResourceNotFoundException"):
		return &api.NotFoundError{Resource: resource, ID: id}
	case containsAny(msg, "already exists", "ConflictException", "ResourceConflictException"):
		return &api.ConflictError{Message: fmt.Sprintf("%s %s already exists", resource, id)}
	case containsAny(msg, "InvalidParameterValueException", "ValidationException"):
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
