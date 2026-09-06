package ecs

import (
	"io"
	"testing"
)

func readAllDecoder(t *testing.T, d *ssmDecoder) string {
	t.Helper()
	got, err := io.ReadAll(d)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(got)
}

// A non-TTY exec's shell prints the exit marker after the command's
// output. The decoder strips it even when it straddles frames, hands the
// client the output verbatim, and reports the marker's status.
func TestSSMDecoder_NonTTYMarkerAcrossFrames(t *testing.T) {
	frames := [][]byte{
		craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("hello\n__SOCK")),
		craftSSMFrame(t, "output_stream_data", ssmPayloadStdErr, 2, []byte("warn\n")),
		craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 3, []byte("EXIT:127:__")),
		craftSSMFrame(t, "channel_closed", 0, 4, nil),
	}
	d := newSSMDecoder(newFakeWire(frames))
	d.markerScan = true
	var reported = -2
	d.onExit = func(code int) { reported = code }
	if got := readAllDecoder(t, d); got != "hello\nwarn\n" {
		t.Fatalf("output %q, want %q", got, "hello\nwarn\n")
	}
	if reported != 127 {
		t.Fatalf("exit code %d, want 127", reported)
	}
}

// Output that merely looks like the start of a marker is released once
// the session ends without one, and the exit code is then unknown.
func TestSSMDecoder_NonTTYWithoutMarkerReportsUnknown(t *testing.T) {
	frames := [][]byte{
		craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("partial __SOCKEX")),
		craftSSMFrame(t, "channel_closed", 0, 2, nil),
	}
	d := newSSMDecoder(newFakeWire(frames))
	d.markerScan = true
	reported := 0
	d.onExit = func(code int) { reported = code }
	if got := readAllDecoder(t, d); got != "partial __SOCKEX" {
		t.Fatalf("output %q", got)
	}
	if reported != -1 {
		t.Fatalf("exit code %d, want -1", reported)
	}
}

// A TTY exec carries no marker; the session's exit-code frame is the
// exit status, and the output reaches the client untouched.
func TestSSMDecoder_TTYExitCodeFrame(t *testing.T) {
	frames := [][]byte{
		craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("$ ")),
		craftSSMFrame(t, "output_stream_data", ssmPayloadExitCode, 2, []byte("3")),
		craftSSMFrame(t, "channel_closed", 0, 3, nil),
	}
	d := newSSMDecoder(newFakeWire(frames))
	reported := 0
	d.onExit = func(code int) { reported = code }
	if got := readAllDecoder(t, d); got != "$ " {
		t.Fatalf("output %q", got)
	}
	if reported != 3 {
		t.Fatalf("exit code %d, want 3", reported)
	}
}

func TestSSMExitMarkerHoldback(t *testing.T) {
	cases := map[string]int{
		"":                        0,
		"hello\n":                 0,
		"hello\n_":                1,
		"hello\n__SOCKEXIT:":      11,
		"hello\n__SOCKEXIT:12":    13,
		"hello\n__SOCKEXIT:12:_":  15,
		"hello\n__SOCKEXIT:12:__": 16,
		"hello\n__SOCKEXIT:1234":  0,
		"hello\n__SOCKEXIT:x":     0,
		"_SOCKEXIT":               0,
	}
	for in, want := range cases {
		if got := ssmExitMarkerHoldback([]byte(in)); got != want {
			t.Errorf("holdback(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSplitTrailingSSMExitMarker(t *testing.T) {
	clean, code, ok := splitTrailingSSMExitMarker([]byte("out\n__SOCKEXIT:2:__\r\n"))
	if !ok || code != 2 || string(clean) != "out\n" {
		t.Fatalf("got clean=%q code=%d ok=%v", clean, code, ok)
	}
	if _, _, ok := splitTrailingSSMExitMarker([]byte("out\n__SOCKEXIT:2:__ trailing")); ok {
		t.Fatal("marker followed by text must not count")
	}
	if _, _, ok := splitTrailingSSMExitMarker([]byte("no marker")); ok {
		t.Fatal("absent marker reported")
	}
}
