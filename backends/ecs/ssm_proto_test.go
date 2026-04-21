package ecs

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// craftSSMFrame builds a wire-format AgentMessage so the parser+ack tests
// don't depend on an actual SSM endpoint. Each call produces a frame with
// a distinct random UUID so thededupe in ssmDecoder doesn't
// collapse frames that the test intends to be sequential.
func craftSSMFrame(t *testing.T, msgType string, payloadType uint32, seq int64, payload []byte) []byte {
	t.Helper()
	if len(msgType) > ssmMessageTypeLen {
		t.Fatalf("msgType too long")
	}
	out := make([]byte, ssmFixedHeaderLen+len(payload))
	binary.BigEndian.PutUint32(out[0:4], ssmHeaderLen)
	copy(out[4:4+ssmMessageTypeLen], msgType)
	for i := 4 + len(msgType); i < 4+ssmMessageTypeLen; i++ {
		out[i] = ' '
	}
	binary.BigEndian.PutUint32(out[36:40], 1)
	binary.BigEndian.PutUint64(out[40:48], uint64(time.Now().UnixMilli()))
	binary.BigEndian.PutUint64(out[48:56], uint64(seq))
	binary.BigEndian.PutUint64(out[56:64], ssmFlagsSynFin)

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	copy(out[64:72], idBytes[0:8])
	copy(out[72:80], idBytes[8:16])

	digest := sha256.Sum256(payload)
	copy(out[80:112], digest[:])
	binary.BigEndian.PutUint32(out[112:116], payloadType)
	binary.BigEndian.PutUint32(out[116:120], uint32(len(payload)))
	copy(out[ssmFixedHeaderLen:], payload)
	return out
}

func TestParseSSMFrame_OutputStreamData_Stdout(t *testing.T) {
	wire := craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 7, []byte("hello\n"))
	f, err := parseSSMFrame(wire)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.MessageType != "output_stream_data" {
		t.Fatalf("messageType=%q", f.MessageType)
	}
	if f.PayloadType != ssmPayloadOutput {
		t.Fatalf("payloadType=%d", f.PayloadType)
	}
	if string(f.Payload) != "hello\n" {
		t.Fatalf("payload=%q", string(f.Payload))
	}
	if f.SequenceNumber != 7 {
		t.Fatalf("seq=%d", f.SequenceNumber)
	}
	if !ssmFrameDigestOK(f) {
		t.Fatal("digest mismatch")
	}
	id, ok := ssmTextStreamID(f)
	if !ok || id != 1 {
		t.Fatalf("expected stdout (1), got %d ok=%v", id, ok)
	}
}

func TestParseSSMFrame_OutputStreamData_Stderr(t *testing.T) {
	wire := craftSSMFrame(t, "output_stream_data", ssmPayloadStdErr, 3, []byte("oops"))
	f, _ := parseSSMFrame(wire)
	id, ok := ssmTextStreamID(f)
	if !ok || id != 2 {
		t.Fatalf("expected stderr (2), got %d ok=%v", id, ok)
	}
}

func TestParseSSMFrame_TooShort(t *testing.T) {
	if _, err := parseSSMFrame([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error on truncated frame")
	}
}

func TestParseSSMFrame_PayloadLengthMismatch(t *testing.T) {
	wire := craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("xyz"))
	// Truncate payload
	if _, err := parseSSMFrame(wire[:len(wire)-1]); err == nil {
		t.Fatal("expected error on payload truncation")
	}
}

func TestBuildSSMAck_RoundTrip(t *testing.T) {
	recv := &ssmFrame{
		MessageType:    "output_stream_data",
		MessageID:      uuid.MustParse("11111111-2222-4333-8444-555555555555"),
		SequenceNumber: 42,
	}
	wire, err := buildSSMAck(recv)
	if err != nil {
		t.Fatalf("buildSSMAck: %v", err)
	}
	parsed, err := parseSSMFrame(wire)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if parsed.MessageType != "acknowledge" {
		t.Fatalf("expected acknowledge, got %q", parsed.MessageType)
	}
	var body ssmAck
	if err := json.Unmarshal(parsed.Payload, &body); err != nil {
		t.Fatalf("unmarshal ack body: %v", err)
	}
	if body.AcknowledgedMessageType != "output_stream_data" {
		t.Fatalf("ack.MessageType=%q", body.AcknowledgedMessageType)
	}
	if body.AcknowledgedMessageId != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("ack.MessageId=%q", body.AcknowledgedMessageId)
	}
	if body.AcknowledgedMessageSequenceNumber != 42 {
		t.Fatalf("ack.Seq=%d", body.AcknowledgedMessageSequenceNumber)
	}
	if !body.IsSequentialMessage {
		t.Fatal("ack.IsSequentialMessage = false")
	}
	if !ssmFrameDigestOK(parsed) {
		t.Fatal("ack digest mismatch")
	}
}

func TestSSMTextStreamID_NonOutputType(t *testing.T) {
	wire := craftSSMFrame(t, "channel_closed", 0, 1, []byte(`{"Output":"done"}`))
	f, _ := parseSSMFrame(wire)
	if _, ok := ssmTextStreamID(f); ok {
		t.Fatal("expected non-output frame to return false")
	}
}

// fakeWire is an io.ReadWriteCloser that hands out pre-staged SSM frames
// to ssmDecoder reads and captures any acks/inputs the decoder writes.
type fakeWire struct {
	in       *bytes.Buffer
	out      bytes.Buffer
	closed   bool
	closeErr error
}

func newFakeWire(frames [][]byte) *fakeWire {
	buf := &bytes.Buffer{}
	for _, f := range frames {
		buf.Write(f)
	}
	return &fakeWire{in: buf}
}

func (w *fakeWire) Read(p []byte) (int, error)  { return w.in.Read(p) }
func (w *fakeWire) Write(p []byte) (int, error) { return w.out.Write(p) }
func (w *fakeWire) Close() error {
	w.closed = true
	return w.closeErr
}

// Reading three stdout frames should surface the concatenated bytes and
// emit three acks back through the wire.
func TestSSMDecoder_StdoutFlow(t *testing.T) {
	wire := newFakeWire([][]byte{
		craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("hel")),
		craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 2, []byte("lo\n")),
	})
	d := newSSMDecoder(wire)
	got := make([]byte, 0, 16)
	buf := make([]byte, 64)
	for i := 0; i < 4; i++ {
		n, err := d.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break
		}
	}
	if string(got) != "hello\n" {
		t.Fatalf("got %q want %q", string(got), "hello\n")
	}
	if d.lastStream() != 1 {
		t.Fatalf("lastStream=%d want 1", d.lastStream())
	}
	// Should have written two acks.
	written := wire.out.Bytes()
	count := 0
	off := 0
	for off+ssmFixedHeaderLen <= len(written) {
		f, err := parseSSMFrame(written[off:])
		if err != nil {
			break
		}
		if f.MessageType == "acknowledge" {
			count++
		}
		off += ssmFixedHeaderLen + int(f.PayloadLength)
	}
	if count != 2 {
		t.Fatalf("got %d acks, want 2", count)
	}
}
