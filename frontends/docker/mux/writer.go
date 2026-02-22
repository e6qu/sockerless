// Package mux implements Docker's multiplexed stream format.
//
// The format uses 8-byte headers: [stream_type, 0, 0, 0, size_big_endian_4bytes]
// stream_type: 0=stdin, 1=stdout, 2=stderr
// This is compatible with Docker SDK's stdcopy.StdCopy.
package mux

import (
	"encoding/binary"
	"io"
	"sync"
)

const (
	Stdin  byte = 0
	Stdout byte = 1
	Stderr byte = 2

	headerSize = 8
)

// Writer wraps an io.Writer and adds multiplexed stream headers.
type Writer struct {
	w      io.Writer
	mu     sync.Mutex
	stream byte
}

// NewWriter creates a new multiplexed writer for the given stream type.
func NewWriter(w io.Writer, stream byte) *Writer {
	return &Writer{w: w, stream: stream}
}

// Write writes data with a multiplexed header.
func (w *Writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	header := make([]byte, headerSize)
	header[0] = w.stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(p)))

	if _, err := w.w.Write(header); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

// WriteFrame writes a single frame with the given stream type and data.
func WriteFrame(w io.Writer, stream byte, data []byte) error {
	header := make([]byte, headerSize)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}
