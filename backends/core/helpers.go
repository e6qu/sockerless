package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/sockerless/api"
)

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// WriteError writes an error response with the appropriate HTTP status code.
func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if sc, ok := err.(api.StatusCoder); ok {
		status = sc.StatusCode()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(api.ErrorResponse{Message: err.Error()})
}

// ReadJSON reads and decodes a JSON request body.
func ReadJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// ParseFilters decodes the Docker SDK filter JSON which uses map[string]map[string]bool format.
func ParseFilters(filtersStr string) map[string][]string {
	if filtersStr == "" {
		return nil
	}
	// Docker SDK >= 1.22 sends {"label":{"key=val":true}}
	var mapFormat map[string]map[string]bool
	if err := json.Unmarshal([]byte(filtersStr), &mapFormat); err == nil {
		result := make(map[string][]string)
		for key, vals := range mapFormat {
			for val := range vals {
				result[key] = append(result[key], val)
			}
		}
		return result
	}
	// Legacy format: {"label":["key=val"]}
	var listFormat map[string][]string
	if err := json.Unmarshal([]byte(filtersStr), &listFormat); err == nil {
		return listFormat
	}
	return nil
}

// GenerateID generates a random 64-character hex ID.
func GenerateID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateName generates a random 12-character hex name.
func GenerateName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateToken generates a random 64-character hex token.
func GenerateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
