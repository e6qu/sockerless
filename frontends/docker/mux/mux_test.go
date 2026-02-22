package mux

import (
	"bytes"
	"io"
	"testing"
)

func TestWriterReader(t *testing.T) {
	var buf bytes.Buffer

	// Write some frames
	w := NewWriter(&buf, Stdout)
	w.Write([]byte("hello"))
	w.Write([]byte(" world"))

	w2 := NewWriter(&buf, Stderr)
	w2.Write([]byte("error msg"))

	// Read them back
	reader := NewReader(&buf)

	frame1, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame1.Stream != Stdout || string(frame1.Data) != "hello" {
		t.Errorf("frame1: got stream=%d data=%q, want stream=1 data=hello", frame1.Stream, frame1.Data)
	}

	frame2, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame2.Stream != Stdout || string(frame2.Data) != " world" {
		t.Errorf("frame2: got stream=%d data=%q, want stream=1 data=' world'", frame2.Stream, frame2.Data)
	}

	frame3, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame3.Stream != Stderr || string(frame3.Data) != "error msg" {
		t.Errorf("frame3: got stream=%d data=%q, want stream=2 data='error msg'", frame3.Stream, frame3.Data)
	}

	_, err = reader.ReadFrame()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestDemux(t *testing.T) {
	var buf bytes.Buffer
	WriteFrame(&buf, Stdout, []byte("out1\n"))
	WriteFrame(&buf, Stderr, []byte("err1\n"))
	WriteFrame(&buf, Stdout, []byte("out2\n"))

	var stdout, stderr bytes.Buffer
	if err := Demux(&buf, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	if stdout.String() != "out1\nout2\n" {
		t.Errorf("stdout: got %q, want %q", stdout.String(), "out1\nout2\n")
	}
	if stderr.String() != "err1\n" {
		t.Errorf("stderr: got %q, want %q", stderr.String(), "err1\n")
	}
}

func TestWriteFrame(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, Stdout, []byte("test")); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()
	if len(data) != 12 { // 8 header + 4 data
		t.Fatalf("expected 12 bytes, got %d", len(data))
	}
	if data[0] != 1 { // stdout
		t.Errorf("stream type: got %d, want 1", data[0])
	}
	if data[4] != 0 || data[5] != 0 || data[6] != 0 || data[7] != 4 {
		t.Errorf("size bytes: got %v, want [0 0 0 4]", data[4:8])
	}
	if string(data[8:]) != "test" {
		t.Errorf("data: got %q, want %q", string(data[8:]), "test")
	}
}
