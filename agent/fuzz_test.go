package agent

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

// FuzzMessageUnmarshal feeds arbitrary bytes through the same JSON envelope
// decode every WebSocket read loop performs (server.go / reverse.go /
// wsclient.go all do json.Unmarshal(data, &msg)). Malformed JSON must surface
// as an error the loop skips, never a panic.
func FuzzMessageUnmarshal(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"type":"exec","id":"1","cmd":["echo","hi"]}`),
		[]byte(`{"type":"stdin","id":"1","data":"aGVsbG8="}`),
		[]byte(`{"type":"stdin","id":"1","data":"!!!not base64!!!"}`),
		[]byte(`{"type":"resize","width":-1,"height":99999999}`),
		[]byte(`{"type":"signal","signal":"SIGKILL"}`),
		[]byte(`{"type":"exit","code":null}`),
		[]byte(`{}`),
		[]byte(``),
		[]byte(`null`),
		[]byte(`[]`),
		[]byte(`{"type":12345}`),
		[]byte(`{"code":1.5}`),
		// Numeric-field edge cases for Code *int / Width / Height: overflow,
		// float, exponent, and a deeply-nested object the decoder must reject
		// without unbounded recursion.
		[]byte(`{"code":9223372036854775808}`),
		[]byte(`{"width":1e309,"height":-1e309}`),
		[]byte(`{"code":-9999999999999999999}`),
		[]byte(`{"data":"QUFB"}`),
		[]byte(`{"type":"stdin","data":"////"}`),
		[]byte(strings.Repeat(`{"a":`, 2000) + "1" + strings.Repeat("}", 2000)),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var msg Message
		_ = json.Unmarshal(data, &msg)
	})
}

// FuzzRouterHandle drives the agent message router's no-write dispatch paths
// (stdin/close_stdin/signal/resize) with arbitrary parsed messages against an
// empty registry. These reach registry.Get (miss) → base64 stdin decode and
// signal parsing without writing to the connection, so a nil *websocket.Conn is
// never dereferenced. The decode + parse must never panic on attacker input.
func FuzzRouterHandle(f *testing.F) {
	type seed struct {
		typ, id, data, signal string
		width, height         int
	}
	seeds := []seed{
		{"stdin", "x", "aGVsbG8=", "", 0, 0},
		{"stdin", "x", "%%%bad%%%", "", 0, 0},
		{"stdin", "", "", "", 0, 0},
		{"signal", "x", "", "SIGKILL", 0, 0},
		{"signal", "x", "", "../../etc", 0, 0},
		{"resize", "x", "", "", -5, 1 << 30},
		{"close_stdin", "x", "", "", 0, 0},
	}
	for _, s := range seeds {
		f.Add(s.typ, s.id, s.data, s.signal, s.width, s.height)
	}
	// Only the message types whose handlers never write to conn on a registry
	// miss; exec/attach/unknown call sendError(conn,…) and need a live socket.
	noWrite := map[string]bool{
		TypeStdin: true, TypeCloseStdin: true, TypeSignal: true, TypeResize: true,
	}
	f.Fuzz(func(t *testing.T, typ, id, data, signal string, width, height int) {
		if !noWrite[typ] {
			return
		}
		registry := NewSessionRegistry()
		router := NewRouter(registry, nil, nil, zerolog.Nop())
		msg := &Message{Type: typ, ID: id, Data: data, Signal: signal, Width: width, Height: height}
		router.Handle(msg, nil, &sync.Mutex{})
	})
}

// FuzzParseSignal exercises the signal-name parser with arbitrary input.
func FuzzParseSignal(f *testing.F) {
	for _, s := range []string{"SIGKILL", "kill", "sigterm", "", "SIG", "SIGSIGSIG", "\x00", "TERM"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = parseSignal(s)
	})
}

// FuzzDetectENOSPC exercises the ENOSPC stderr scanner with arbitrary bytes.
func FuzzDetectENOSPC(f *testing.F) {
	f.Add([]byte("No space left on device"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\xff random"))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = DetectENOSPC(b)
		_ = AnnotateENOSPC(b, "lambda")
	})
}
