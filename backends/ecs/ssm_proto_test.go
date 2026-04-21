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
	// AWS Java-style putUuid: LSL (UUID bytes 8-15) at offset 64, MSL at 72.
	copy(out[64:72], idBytes[8:16])
	copy(out[72:80], idBytes[0:8])

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

// TestBuildSSMAck_FlagsAndUUIDWireOrder pins the two wire-format
// details the live AWS agent cares about: Flags=3 (SYN|FIN) in the
// 8-byte Flags slot, and the UUID packed as LeastSignificantLong at
// offset 64 + MostSignificantLong at offset 72 (AWS Java-style putUuid).
// Without both, the agent retransmits output_stream_data frames
// indefinitely.
func TestBuildSSMAck_FlagsAndUUIDWireOrder(t *testing.T) {
	recv := &ssmFrame{
		MessageType:    "output_stream_data",
		MessageID:      uuid.MustParse("aabbccdd-eeff-4011-8022-334455667788"),
		SequenceNumber: 1,
	}
	wire, err := buildSSMAck(recv)
	if err != nil {
		t.Fatalf("buildSSMAck: %v", err)
	}
	if got := binary.BigEndian.Uint64(wire[56:64]); got != ssmFlagsSynFin {
		t.Errorf("Flags = %d at offset 56, want %d (SYN|FIN)", got, ssmFlagsSynFin)
	}
	// The ack uses a fresh UUID — we don't know its bytes, but we do
	// know the packing rule: re-parse and assert the parsed UUID's
	// string form matches the MSL half at offset 72 and LSL at 64.
	lsl := wire[64:72]
	msl := wire[72:80]
	var uuidBytes [16]byte
	copy(uuidBytes[0:8], msl)
	copy(uuidBytes[8:16], lsl)
	parsed, err := parseSSMFrame(wire)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if uuid.UUID(uuidBytes).String() != parsed.MessageID.String() {
		t.Errorf("UUID wire layout mismatch: manual=%s, parsed=%s", uuid.UUID(uuidBytes), parsed.MessageID)
	}
}

// TestBuildSSMInput_UUIDWireOrder — same invariant for input_stream_data.
func TestBuildSSMInput_UUIDWireOrder(t *testing.T) {
	wire, err := buildSSMInput([]byte("stdin"))
	if err != nil {
		t.Fatalf("buildSSMInput: %v", err)
	}
	parsed, err := parseSSMFrame(wire)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	var uuidBytes [16]byte
	copy(uuidBytes[0:8], wire[72:80])  // MSL
	copy(uuidBytes[8:16], wire[64:72]) // LSL
	if uuid.UUID(uuidBytes).String() != parsed.MessageID.String() {
		t.Errorf("input_stream_data UUID wire layout mismatch: manual=%s, parsed=%s", uuid.UUID(uuidBytes), parsed.MessageID)
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

// TestSSMDecoder_StdoutFlow — two output_stream_data frames surface as
// concatenated stdout bytes; the decoder emits two acks back, each one
// referencing the right MessageId + SequenceNumber, using the wire
// format AWS's agent accepts (Flags=3, UUID packed LSL-then-MSL).
func TestSSMDecoder_StdoutFlow(t *testing.T) {
	f1 := craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("hel"))
	f2 := craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 2, []byte("lo\n"))
	wire := newFakeWire([][]byte{f1, f2})
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

	// Extract the two acks off the wire and validate every field the
	// agent checks.
	origIDs := []string{}
	for _, frame := range [][]byte{f1, f2} {
		p, _ := parseSSMFrame(frame)
		origIDs = append(origIDs, p.MessageID.String())
	}
	written := wire.out.Bytes()
	acks := []*ssmFrame{}
	for off := 0; off+ssmFixedHeaderLen <= len(written); {
		f, err := parseSSMFrame(written[off:])
		if err != nil {
			break
		}
		if f.MessageType == "acknowledge" {
			acks = append(acks, f)
			// Agent-critical: Flags must be 3 (SYN|FIN).
			flags := binary.BigEndian.Uint64(written[off+56 : off+64])
			if flags != ssmFlagsSynFin {
				t.Errorf("ack #%d Flags=%d, want %d", len(acks), flags, ssmFlagsSynFin)
			}
			// Agent-critical: JSON body references the ORIGINAL frame's UUID.
			var body ssmAck
			if err := json.Unmarshal(f.Payload, &body); err != nil {
				t.Fatalf("ack #%d unmarshal: %v", len(acks), err)
			}
			want := origIDs[len(acks)-1]
			if body.AcknowledgedMessageId != want {
				t.Errorf("ack #%d AcknowledgedMessageId=%s, want %s", len(acks), body.AcknowledgedMessageId, want)
			}
			if body.AcknowledgedMessageSequenceNumber != int64(len(acks)) {
				t.Errorf("ack #%d seq=%d, want %d", len(acks), body.AcknowledgedMessageSequenceNumber, len(acks))
			}
			if !body.IsSequentialMessage {
				t.Errorf("ack #%d IsSequentialMessage=false", len(acks))
			}
		}
		off += ssmFixedHeaderLen + int(f.PayloadLength)
	}
	if len(acks) != 2 {
		t.Fatalf("got %d acks, want 2", len(acks))
	}
}

// TestSSMDecoder_NoDedupeWorkaround — prior behaviour deduped frames
// by MessageID because the agent retransmitted when its ack didn't
// match. With the corrected ack format that workaround is gone;
// duplicate frames (if any ever arrive) reach the caller once per
// arrival. This test pins the post-fix behaviour so reintroducing
// dedupe would fail CI.
func TestSSMDecoder_NoDedupeWorkaround(t *testing.T) {
	same := craftSSMFrame(t, "output_stream_data", ssmPayloadOutput, 1, []byte("x"))
	// Intentionally send the same frame twice.
	wire := newFakeWire([][]byte{same, append([]byte(nil), same...)})
	d := newSSMDecoder(wire)
	got := make([]byte, 0, 4)
	buf := make([]byte, 16)
	for i := 0; i < 3; i++ {
		n, err := d.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break
		}
	}
	if string(got) != "xx" {
		t.Fatalf("got %q, want %q (if this shrinks to \"x\" a dedupe workaround has been reintroduced)", got, "xx")
	}
}
