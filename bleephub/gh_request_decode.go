package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// decodeIssueLabelsBody decodes the body of the "add labels to an issue"
// endpoint. Real GitHub (and go-github / gh) accept EITHER a bare JSON array
// `["bug","help wanted"]` OR the object form `{"labels":[...]}`. Returns the
// label names, or false (after writing a 400) on malformed JSON. An empty body
// is treated as no labels.
func decodeIssueLabelsBody(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return nil, false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, true
	}
	if trimmed[0] == '[' {
		var labels []string
		if err := json.Unmarshal(trimmed, &labels); err != nil {
			writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
			return nil, false
		}
		return labels, true
	}
	var obj struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return nil, false
	}
	return obj.Labels, true
}

// request body decoding tolerant of string-coerced booleans + integers.
// Real GitHub's REST API accepts both:
//   - `{"private": false}`              (typed JSON boolean)
//   - `{"private": "false"}`            (string-coerced; what `gh api -f` sends)
//
// Bleephub must accept the same so `gh` CLI works natively. This isn't a
// fallback — it's the GitHub API spec the official client relies on.
//
// Use the `flexBool` / `flexInt` field types in request structs instead of
// `bool` / `int`. They Marshal back to the typed form, so JSON responses
// keep the proper shape.

// flexBool decodes either `true`/`false` (typed) or `"true"`/`"false"` (string).
// Empty string → false (matches Rails strong params coercion).
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*b = false
		return nil
	}
	// Typed boolean
	if data[0] == 't' || data[0] == 'f' {
		var v bool
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*b = flexBool(v)
		return nil
	}
	// String-coerced
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "true", "1", "yes":
		*b = true
	case "false", "0", "no", "":
		*b = false
	default:
		return &json.UnmarshalTypeError{Value: s, Type: nil}
	}
	return nil
}

func (b flexBool) MarshalJSON() ([]byte, error) {
	if b {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}

// flexInt decodes either typed numbers or string-coerced ints.
type flexInt int

func (i *flexInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*i = 0
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*i = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*i = flexInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*i = flexInt(n)
	return nil
}

func (i flexInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Itoa(int(i))), nil
}

// coerceBool extracts a bool from an interface{} that may be a bool, a
// "true"/"false" string, or a 0/1 number. Returns (value, found).
// Used in handlers that decode the body into map[string]interface{} (e.g.
// PATCH /repos/{o}/{r} which accepts a wide set of optional fields).
func coerceBool(v interface{}) (bool, bool) {
	switch x := v.(type) {
	case nil:
		return false, false
	case bool:
		return x, true
	case string:
		switch x {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	case float64:
		return x != 0, true
	}
	return false, false
}

// flexIntSlice decodes []int but tolerates a single int (string-coerced or typed),
// matching how `gh api -f key=val` sends each `-f` as a separate field.
type flexIntSlice []int

func (s *flexIntSlice) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = nil
		return nil
	}
	// Typed array
	if data[0] == '[' {
		var raw []json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		out := make([]int, 0, len(raw))
		for _, r := range raw {
			var n flexInt
			if err := json.Unmarshal(r, &n); err != nil {
				return err
			}
			out = append(out, int(n))
		}
		*s = out
		return nil
	}
	// Single scalar — wrap into one-element slice
	var n flexInt
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*s = []int{int(n)}
	return nil
}
