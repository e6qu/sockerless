package main

// SSM Session Manager AgentMessage emitter for the ECS exec WebSocket.
// The backend (backends/ecs/exec_cloud.go) expects the server side of
// the exec channel to speak the AWS Session Manager binary protocol:
// real AWS does, so the simulator has to as well (BUG-728). Wire layout
// matches backends/ecs/ssm_proto.go — see that file + AWS's
// session-manager-plugin source for the 120-byte fixed header format.

import (
	"crypto/sha256"
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	ssmHeaderLen        = 116
	ssmFixedHeaderLen   = ssmHeaderLen + 4 // 120 bytes through PayloadLength
	ssmMessageTypeLen   = 32
	ssmPayloadDigestLen = 32
	ssmFlagsSynFin      = uint64(3)
)

// MessageType constants (right-padded with spaces to 32 bytes).
const (
	ssmMTOutputStreamData = "output_stream_data"
	ssmMTChannelClosed    = "channel_closed"
)

// PayloadType constants. Backend decoder maps 1 → Docker stream 1
// (stdout) and 11 → stream 2 (stderr); 12 signals process exit.
const (
	ssmPayloadStdout   = uint32(1)
	ssmPayloadStderr   = uint32(11)
	ssmPayloadExitCode = uint32(12)
)

// outputSequence is a monotonic per-process counter applied to every
// emitted output_stream_data frame. Real AWS uses a per-session
// counter; the simulator uses process-global which is equivalent for
// single-session tests (what the e2e suite exercises).
var outputSequence int64

// buildSSMOutputFrame wraps a payload chunk in an output_stream_data
// AgentMessage. payloadType selects stdout (1), stderr (11), or
// exit-code (12).
func buildSSMOutputFrame(payloadType uint32, payload []byte) []byte {
	digest := sha256.Sum256(payload)
	out := make([]byte, ssmFixedHeaderLen+len(payload))
	binary.BigEndian.PutUint32(out[0:4], ssmHeaderLen)

	mt := []byte(ssmMTOutputStreamData)
	copy(out[4:4+ssmMessageTypeLen], mt)
	for i := 4 + len(mt); i < 4+ssmMessageTypeLen; i++ {
		out[i] = ' '
	}

	binary.BigEndian.PutUint32(out[36:40], 1) // schema version
	binary.BigEndian.PutUint64(out[40:48], uint64(time.Now().UnixMilli()))
	seq := atomic.AddInt64(&outputSequence, 1)
	binary.BigEndian.PutUint64(out[48:56], uint64(seq))
	binary.BigEndian.PutUint64(out[56:64], ssmFlagsSynFin)

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	copy(out[64:72], idBytes[0:8])
	copy(out[72:80], idBytes[8:16])

	copy(out[80:112], digest[:])
	binary.BigEndian.PutUint32(out[112:116], payloadType)
	binary.BigEndian.PutUint32(out[116:120], uint32(len(payload)))
	copy(out[ssmFixedHeaderLen:], payload)
	return out
}

// buildSSMChannelClosed builds the channel_closed frame that tells the
// backend decoder to terminate cleanly. Payload can be empty.
func buildSSMChannelClosed() []byte {
	payload := []byte{}
	digest := sha256.Sum256(payload)
	out := make([]byte, ssmFixedHeaderLen)
	binary.BigEndian.PutUint32(out[0:4], ssmHeaderLen)

	mt := []byte(ssmMTChannelClosed)
	copy(out[4:4+ssmMessageTypeLen], mt)
	for i := 4 + len(mt); i < 4+ssmMessageTypeLen; i++ {
		out[i] = ' '
	}

	binary.BigEndian.PutUint32(out[36:40], 1)
	binary.BigEndian.PutUint64(out[40:48], uint64(time.Now().UnixMilli()))
	seq := atomic.AddInt64(&outputSequence, 1)
	binary.BigEndian.PutUint64(out[48:56], uint64(seq))
	binary.BigEndian.PutUint64(out[56:64], ssmFlagsSynFin)

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	copy(out[64:72], idBytes[0:8])
	copy(out[72:80], idBytes[8:16])

	copy(out[80:112], digest[:])
	binary.BigEndian.PutUint32(out[112:116], 0) // PayloadType 0 for control
	binary.BigEndian.PutUint32(out[116:120], 0)
	return out
}
