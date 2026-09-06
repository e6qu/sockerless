package ecs

import "strings"

// The exit marker an ECS ExecuteCommand session's shell prints after the
// command's own output. ECS ExecuteCommand sessions are interactive and
// need not end with the SSM `output_stream_data` frame that carries an
// exit code, so the in-container shell reports `$?` itself.
const (
	ssmExitMarkerPrefix  = "__SOCKEXIT:"
	ssmExitMarkerSuffix  = ":__"
	ssmExitMarkerCommand = `; printf "` + ssmExitMarkerPrefix + `%d` + ssmExitMarkerSuffix + `" $?`
)

// ssmExitMarkerMaxLen bounds the bytes a marker can occupy: the prefix, an
// exit status of at most three digits, and the suffix.
const ssmExitMarkerMaxLen = len(ssmExitMarkerPrefix) + 3 + len(ssmExitMarkerSuffix)

// ssmExitMarkerHoldback returns how many trailing bytes of out could be
// the beginning of an exit marker and so must not be released to the
// client yet.
func ssmExitMarkerHoldback(out []byte) int {
	max := len(out)
	if max > ssmExitMarkerMaxLen {
		max = ssmExitMarkerMaxLen
	}
	for k := max; k > 0; k-- {
		if ssmExitMarkerCouldStart(string(out[len(out)-k:])) {
			return k
		}
	}
	return 0
}

// ssmExitMarkerCouldStart reports whether s is a prefix of some complete
// exit marker.
func ssmExitMarkerCouldStart(s string) bool {
	if len(s) <= len(ssmExitMarkerPrefix) {
		return strings.HasPrefix(ssmExitMarkerPrefix, s)
	}
	if !strings.HasPrefix(s, ssmExitMarkerPrefix) {
		return false
	}
	rest := s[len(ssmExitMarkerPrefix):]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > 3 {
		return false
	}
	return strings.HasPrefix(ssmExitMarkerSuffix, rest[digits:])
}

// splitTrailingSSMExitMarker takes a complete exit marker off the end of
// out (a trailing CR/LF after the marker is tolerated) and returns the
// bytes before it, verbatim, with the parsed exit status. ok is false
// when out does not end with a marker.
func splitTrailingSSMExitMarker(out []byte) (clean []byte, code int, ok bool) {
	s := strings.TrimRight(string(out), "\r\n")
	if !strings.HasSuffix(s, ssmExitMarkerSuffix) {
		return out, -1, false
	}
	body := s[:len(s)-len(ssmExitMarkerSuffix)]
	idx := strings.LastIndex(body, ssmExitMarkerPrefix)
	if idx < 0 {
		return out, -1, false
	}
	digits := body[idx+len(ssmExitMarkerPrefix):]
	if digits == "" || len(digits) > 3 {
		return out, -1, false
	}
	code = 0
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return out, -1, false
		}
		code = code*10 + int(ch-'0')
	}
	return out[:idx], code, true
}
