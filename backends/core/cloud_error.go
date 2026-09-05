package core

import (
	"github.com/sockerless/api"
)

// CloudErrorPatterns are the substrings a cloud SDK's error messages
// carry for each Docker error class. Each cloud family declares its own.
type CloudErrorPatterns struct {
	NotFound []string
	Conflict []string
	Invalid  []string
}

// MapCloudError converts a cloud SDK error into the Docker API error the
// client expects, by matching the message against the cloud's patterns.
// An error that matches nothing is returned as a ServerError carrying the
// message.
func MapCloudError(err error, resource, id string, patterns CloudErrorPatterns) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case ContainsAny(msg, patterns.NotFound...):
		return &api.NotFoundError{Resource: resource, ID: id}
	case ContainsAny(msg, patterns.Conflict...):
		return &api.ConflictError{Message: msg}
	case ContainsAny(msg, patterns.Invalid...):
		return &api.InvalidParameterError{Message: msg}
	default:
		return &api.ServerError{Message: msg}
	}
}

// ContainsAny reports whether s contains any of the substrings.
func ContainsAny(s string, substrs ...string) bool {
	return containsAny(s, substrs...)
}
