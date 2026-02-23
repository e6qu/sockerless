package api

import (
	"fmt"
	"net/http"
)

// StatusCoder is implemented by errors that have an associated HTTP status code.
type StatusCoder interface {
	StatusCode() int
}

// ErrorResponse is the JSON error response body matching Docker's format.
type ErrorResponse struct {
	Message string `json:"message"`
}

// NotFoundError indicates a requested resource was not found.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("No such %s: %s", e.Resource, e.ID)
}

func (e *NotFoundError) StatusCode() int {
	return http.StatusNotFound
}

// ConflictError indicates a conflict (e.g., container name already in use).
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

func (e *ConflictError) StatusCode() int {
	return http.StatusConflict
}

// InvalidParameterError indicates an invalid request parameter.
type InvalidParameterError struct {
	Message string
}

func (e *InvalidParameterError) Error() string {
	return e.Message
}

func (e *InvalidParameterError) StatusCode() int {
	return http.StatusBadRequest
}

// NotImplementedError indicates an unimplemented endpoint.
type NotImplementedError struct {
	Message string
}

func (e *NotImplementedError) Error() string {
	if e.Message == "" {
		return "not implemented"
	}
	return e.Message
}

func (e *NotImplementedError) StatusCode() int {
	return http.StatusNotImplemented
}

// NotModifiedError indicates the resource has not been modified.
type NotModifiedError struct{}

func (e *NotModifiedError) Error() string {
	return "not modified"
}

func (e *NotModifiedError) StatusCode() int {
	return http.StatusNotModified
}

// ServerError indicates an internal server error.
type ServerError struct {
	Message string
}

func (e *ServerError) Error() string {
	return e.Message
}

func (e *ServerError) StatusCode() int {
	return http.StatusInternalServerError
}
