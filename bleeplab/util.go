package bleeplab

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	return nil
}

// newToken returns a deterministic-but-unique token for id+prefix. The
// runner treats tokens as opaque, so a readable value aids debugging.
func newToken(prefix string, id int) string {
	return fmt.Sprintf("%s-%08d", prefix, id)
}
