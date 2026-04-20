package ecs

// SSM Session Manager binary protocol decoder + ack writer.
//
// Wire format reference: AWS open-source `session-manager-plugin`,
// `src/message/clientmessage.go` + `src/message/messageparser.go`.
// Header is 116 bytes big-endian, payload follows. See BUG-717.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// ssmHeaderLen is the wire `HeaderLength` field value, which equals the
	// offset of the PayloadLength field. The full fixed-size header
	// (everything before the variable Payload) is ssmHeaderLen+4 = 120 bytes.
	ssmHeaderLen        = 116
	ssmFixedHeaderLen   = ssmHeaderLen + 4 // 120: through PayloadLength
	ssmMessageTypeLen   = 32
	ssmPayloadDigestLen = 32
	ssmFlagsSynFin      = uint64(3) // SYN | FIN
)

// SSM MessageType strings (right-padded with spaces to 32 bytes).
const (
	ssmMTOutputStreamData = "output_stream_data"
	ssmMTInputStreamData  = "input_stream_data"
	ssmMTAcknowledge      = "acknowledge"
	ssmMTChannelClosed    = "channel_closed"
	ssmMTStartPublication = "start_publication"
	ssmMTPausePublication = "pause_publication"
)

// SSM PayloadType (uint32). For output_stream_data:
//
//	1 = stdout text, 2 = error text, 11 = stderr text, 12 = exit code.
const (
	ssmPayloadOutput   = uint32(1)
	ssmPayloadError    = uint32(2)
	ssmPayloadStdErr   = uint32(11)
	ssmPayloadExitCode = uint32(12)
)

// ssmFrame is a parsed SSM AgentMessage / ClientMessage.
type ssmFrame struct {
	HeaderLength   uint32
	MessageType    string // trimmed of trailing spaces
	SchemaVersion  uint32
	CreatedDateMS  uint64
	SequenceNumber int64
	Flags          uint64
	MessageID      uuid.UUID
	PayloadDigest  []byte
	PayloadType    uint32
	PayloadLength  uint32
	Payload        []byte
}

// ssmAck is the JSON body inside an `acknowledge` frame's payload.
type ssmAck struct {
	AcknowledgedMessageType           string `json:"AcknowledgedMessageType"`
	AcknowledgedMessageId             string `json:"AcknowledgedMessageId"`
	AcknowledgedMessageSequenceNumber int64  `json:"AcknowledgedMessageSequenceNumber"`
	IsSequentialMessage               bool   `json:"IsSequentialMessage"`
}

// parseSSMFrame deserializes a single SSM AgentMessage from raw WebSocket
// bytes. Returns an error if the buffer is shorter than the declared
// header+payload length.
func parseSSMFrame(b []byte) (*ssmFrame, error) {
	if len(b) < ssmFixedHeaderLen {
		return nil, fmt.Errorf("ssm frame too short: %d bytes < %d fixed header", len(b), ssmFixedHeaderLen)
	}
	f := &ssmFrame{
		HeaderLength:  binary.BigEndian.Uint32(b[0:4]),
		MessageType:   strings.TrimRight(string(b[4:4+ssmMessageTypeLen]), " \x00"),
		SchemaVersion: binary.BigEndian.Uint32(b[36:40]),
		CreatedDateMS: binary.BigEndian.Uint64(b[40:48]),
		// SequenceNumber is signed int64 at offset 48
		SequenceNumber: int64(binary.BigEndian.Uint64(b[48:56])),
		Flags:          binary.BigEndian.Uint64(b[56:64]),
		PayloadDigest:  append([]byte(nil), b[80:80+ssmPayloadDigestLen]...),
		PayloadType:    binary.BigEndian.Uint32(b[112:116]),
		PayloadLength:  binary.BigEndian.Uint32(b[116:120]),
	}

	// MessageId: two int64 halves at offsets 64 (MSB) and 72 (LSB),
	// reassembled into the Java-style UUID byte order.
	var idBytes [16]byte
	copy(idBytes[0:8], b[64:72])
	copy(idBytes[8:16], b[72:80])
	f.MessageID = uuid.UUID(idBytes)

	if len(b) < ssmFixedHeaderLen+int(f.PayloadLength) {
		return nil, fmt.Errorf("ssm frame truncated: have %d bytes, need %d+payload (PayloadLength=%d)", len(b), ssmFixedHeaderLen, f.PayloadLength)
	}
	f.Payload = append([]byte(nil), b[ssmFixedHeaderLen:ssmFixedHeaderLen+int(f.PayloadLength)]...)
	return f, nil
}

// buildSSMAck produces the binary AgentMessage that acknowledges receipt
// of an output_stream_data frame, keeping the SSM session alive.
func buildSSMAck(received *ssmFrame) ([]byte, error) {
	body := ssmAck{
		AcknowledgedMessageType:           received.MessageType,
		AcknowledgedMessageId:             received.MessageID.String(),
		AcknowledgedMessageSequenceNumber: received.SequenceNumber,
		IsSequentialMessage:               true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(payload)

	out := make([]byte, ssmFixedHeaderLen+len(payload))
	binary.BigEndian.PutUint32(out[0:4], ssmHeaderLen) // header length

	// MessageType padded with spaces to 32 bytes
	mt := []byte(ssmMTAcknowledge)
	if len(mt) > ssmMessageTypeLen {
		mt = mt[:ssmMessageTypeLen]
	}
	copy(out[4:4+ssmMessageTypeLen], mt)
	for i := 4 + len(mt); i < 4+ssmMessageTypeLen; i++ {
		out[i] = ' '
	}

	binary.BigEndian.PutUint32(out[36:40], 1) // schema version
	binary.BigEndian.PutUint64(out[40:48], uint64(time.Now().UnixMilli()))
	binary.BigEndian.PutUint64(out[48:56], 0)              // sequence (always 0 for ack)
	binary.BigEndian.PutUint64(out[56:64], ssmFlagsSynFin) // flags

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	copy(out[64:72], idBytes[0:8])
	copy(out[72:80], idBytes[8:16])

	copy(out[80:112], digest[:])
	binary.BigEndian.PutUint32(out[112:116], 0) // payload type 0 for ack
	binary.BigEndian.PutUint32(out[116:120], uint32(len(payload)))
	copy(out[ssmFixedHeaderLen:], payload)
	return out, nil
}

// ssmIsTextOutput reports whether a payload should be treated as
// stream output for the Docker mux'd reader. Returns the Docker
// stream ID (1=stdout, 2=stderr) when applicable.
func ssmTextStreamID(f *ssmFrame) (byte, bool) {
	if f.MessageType != ssmMTOutputStreamData {
		return 0, false
	}
	switch f.PayloadType {
	case ssmPayloadOutput:
		return 1, true
	case ssmPayloadStdErr, ssmPayloadError:
		return 2, true
	}
	return 0, false
}

// ssmFrameDigestOK verifies that the embedded SHA-256 digest matches the
// payload bytes — useful for tests and defensive parsing.
func ssmFrameDigestOK(f *ssmFrame) bool {
	got := sha256.Sum256(f.Payload)
	return hex.EncodeToString(got[:]) == hex.EncodeToString(f.PayloadDigest)
}

// inputSequence is a monotonically-increasing per-process sequence number
// for outbound input_stream_data frames.
var inputSequence int64

// buildSSMInput wraps a stdin payload in an `input_stream_data`
// AgentMessage with PayloadType=1 (raw bytes).
func buildSSMInput(payload []byte) ([]byte, error) {
	digest := sha256.Sum256(payload)
	out := make([]byte, ssmFixedHeaderLen+len(payload))
	binary.BigEndian.PutUint32(out[0:4], ssmHeaderLen)

	mt := []byte(ssmMTInputStreamData)
	copy(out[4:4+ssmMessageTypeLen], mt)
	for i := 4 + len(mt); i < 4+ssmMessageTypeLen; i++ {
		out[i] = ' '
	}

	binary.BigEndian.PutUint32(out[36:40], 1)
	binary.BigEndian.PutUint64(out[40:48], uint64(time.Now().UnixMilli()))
	inputSequence++
	binary.BigEndian.PutUint64(out[48:56], uint64(inputSequence))
	binary.BigEndian.PutUint64(out[56:64], ssmFlagsSynFin)

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	copy(out[64:72], idBytes[0:8])
	copy(out[72:80], idBytes[8:16])

	copy(out[80:112], digest[:])
	binary.BigEndian.PutUint32(out[112:116], ssmPayloadOutput) // PayloadType=1 (raw)
	binary.BigEndian.PutUint32(out[116:120], uint32(len(payload)))
	copy(out[ssmFixedHeaderLen:], payload)
	return out, nil
}
