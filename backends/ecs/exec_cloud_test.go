package ecs

import (
	"io"
	"strings"
	"testing"
)

// mockRWC is a minimal io.ReadWriteCloser backed by a reader.
type mockRWC struct {
	reader io.Reader
	closed bool
}

func (m *mockRWC) Read(p []byte) (int, error)  { return m.reader.Read(p) }
func (m *mockRWC) Write(p []byte) (int, error) { return len(p), nil }
func (m *mockRWC) Close() error                { m.closed = true; return nil }

func TestMuxBridge_AddsStdoutHeader(t *testing.T) {
	inner := &mockRWC{reader: strings.NewReader("hello\n")}
	bridge := &muxBridge{rwc: inner}

	buf := make([]byte, 100)
	n, err := bridge.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected: 8-byte mux header + 6 bytes of "hello\n" = 14 bytes
	if n != 14 {
		t.Fatalf("expected 14 bytes, got %d", n)
	}

	// Stream type: stdout = 0x01
	if buf[0] != 0x01 {
		t.Fatalf("expected stream type 0x01, got 0x%02x", buf[0])
	}

	// Padding bytes should be zero
	if buf[1] != 0 || buf[2] != 0 || buf[3] != 0 {
		t.Fatalf("expected zero padding, got [%02x %02x %02x]", buf[1], buf[2], buf[3])
	}

	// Size as big-endian uint32 = 6
	size := int(buf[4])<<24 | int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7])
	if size != 6 {
		t.Fatalf("expected size 6, got %d", size)
	}

	// Payload
	if string(buf[8:14]) != "hello\n" {
		t.Fatalf("expected payload 'hello\\n', got %q", string(buf[8:14]))
	}
}

func TestMuxBridge_MultipleReads(t *testing.T) {
	// Two reads: first returns "abc", second returns "xyz"
	inner := &mockRWC{reader: strings.NewReader("abcxyz")}
	bridge := &muxBridge{rwc: inner}

	// First read
	buf := make([]byte, 100)
	n, err := bridge.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The inner reader returns all 6 bytes at once ("abcxyz"),
	// so we get one mux frame with all 6 bytes
	size := int(buf[4])<<24 | int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7])
	if size != 6 {
		t.Fatalf("expected size 6, got %d (n=%d)", size, n)
	}
	if string(buf[8:8+size]) != "abcxyz" {
		t.Fatalf("expected payload 'abcxyz', got %q", string(buf[8:8+size]))
	}
}

func TestMuxBridge_WritePassthrough(t *testing.T) {
	inner := &mockRWC{reader: strings.NewReader("")}
	bridge := &muxBridge{rwc: inner}

	n, err := bridge.Write([]byte("input"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes written, got %d", n)
	}
}

func TestMuxBridge_Close(t *testing.T) {
	inner := &mockRWC{reader: strings.NewReader("")}
	bridge := &muxBridge{rwc: inner}

	if err := bridge.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.closed {
		t.Fatal("expected inner closer to be called")
	}
}

func TestMuxBridge_SmallBuffer(t *testing.T) {
	// When the caller buffer is smaller than header+payload, the bridge
	// should buffer and return data across multiple Read calls.
	inner := &mockRWC{reader: strings.NewReader("hello")}
	bridge := &muxBridge{rwc: inner}

	// Read into a tiny buffer (4 bytes at a time)
	var got []byte
	buf := make([]byte, 4)
	for {
		n, err := bridge.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Total expected: 8 header + 5 payload = 13 bytes
		if len(got) >= 13 {
			break
		}
	}

	if len(got) < 13 {
		t.Fatalf("expected at least 13 bytes, got %d", len(got))
	}
	if got[0] != 0x01 {
		t.Fatalf("expected stdout stream type, got 0x%02x", got[0])
	}
	if string(got[8:13]) != "hello" {
		t.Fatalf("expected payload 'hello', got %q", string(got[8:13]))
	}
}
