package mux

import (
	"encoding/binary"
	"io"
)

// Frame represents a single multiplexed frame.
type Frame struct {
	Stream byte
	Data   []byte
}

// Reader reads multiplexed stream frames.
type Reader struct {
	r io.Reader
}

// NewReader creates a new multiplexed reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

// ReadFrame reads the next frame from the stream.
func (r *Reader) ReadFrame() (*Frame, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(r.r, header); err != nil {
		return nil, err
	}

	stream := header[0]
	size := binary.BigEndian.Uint32(header[4:])

	data := make([]byte, size)
	if _, err := io.ReadFull(r.r, data); err != nil {
		return nil, err
	}

	return &Frame{Stream: stream, Data: data}, nil
}

// Demux reads all frames and copies stdout/stderr to the given writers.
func Demux(r io.Reader, stdout, stderr io.Writer) error {
	reader := NewReader(r)
	for {
		frame, err := reader.ReadFrame()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		switch frame.Stream {
		case Stdout:
			if stdout != nil {
				if _, err := stdout.Write(frame.Data); err != nil {
					return err
				}
			}
		case Stderr:
			if stderr != nil {
				if _, err := stderr.Write(frame.Data); err != nil {
					return err
				}
			}
		}
	}
}
