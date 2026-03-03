package main

import (
	"testing"
)

func TestRingBufferPartialLine(t *testing.T) {
	rb := NewRingBuffer(10)

	// First write: partial line (no newline)
	rb.Write([]byte("hello wo"))
	// Second write: completes the line
	rb.Write([]byte("rld\n"))

	lines := rb.Lines(10)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "hello world" {
		t.Errorf("expected 'hello world', got %q", lines[0])
	}
}

func TestRingBufferPartialMultiChunk(t *testing.T) {
	rb := NewRingBuffer(10)

	// Three writes that span a line
	rb.Write([]byte("he"))
	rb.Write([]byte("ll"))
	rb.Write([]byte("o\n"))

	lines := rb.Lines(10)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "hello" {
		t.Errorf("expected 'hello', got %q", lines[0])
	}
}

func TestRingBufferPartialThenComplete(t *testing.T) {
	rb := NewRingBuffer(10)

	// First write: complete line + partial
	rb.Write([]byte("line1\npart"))
	// Second write: complete the partial + new line
	rb.Write([]byte("ial\nline3\n"))

	lines := rb.Lines(10)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" {
		t.Errorf("line 0: expected 'line1', got %q", lines[0])
	}
	if lines[1] != "partial" {
		t.Errorf("line 1: expected 'partial', got %q", lines[1])
	}
	if lines[2] != "line3" {
		t.Errorf("line 2: expected 'line3', got %q", lines[2])
	}
}

func TestRingBufferReset(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("line1\nline2\n"))
	lines := rb.Lines(10)
	if len(lines) != 2 {
		t.Fatalf("before reset: expected 2 lines, got %d", len(lines))
	}

	rb.Reset()
	lines = rb.Lines(10)
	if len(lines) != 0 {
		t.Fatalf("after reset: expected 0 lines, got %d: %v", len(lines), lines)
	}

	// Write after reset should work normally
	rb.Write([]byte("after\n"))
	lines = rb.Lines(10)
	if len(lines) != 1 {
		t.Fatalf("after reset+write: expected 1 line, got %d", len(lines))
	}
	if lines[0] != "after" {
		t.Errorf("expected 'after', got %q", lines[0])
	}
}

func TestRingBufferResetClearsPartial(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("partial"))
	rb.Reset()

	// Write a new line — should NOT include old partial
	rb.Write([]byte("clean\n"))
	lines := rb.Lines(10)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "clean" {
		t.Errorf("expected 'clean', got %q", lines[0])
	}
}

func TestRingBufferLinesNegative(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("line1\nline2\n"))

	// Lines(-1) should return nil, not panic
	lines := rb.Lines(-1)
	if lines != nil {
		t.Fatalf("expected nil for Lines(-1), got %v", lines)
	}

	// Lines(0) should also return nil
	lines = rb.Lines(0)
	if lines != nil {
		t.Fatalf("expected nil for Lines(0), got %v", lines)
	}
}

func TestRingBufferTrailingPartialNotStored(t *testing.T) {
	rb := NewRingBuffer(10)

	// Write with trailing partial — should NOT appear in Lines()
	rb.Write([]byte("complete\nincomplete"))

	lines := rb.Lines(10)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "complete" {
		t.Errorf("expected 'complete', got %q", lines[0])
	}

	// Now complete it
	rb.Write([]byte(" line\n"))
	lines = rb.Lines(10)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[1] != "incomplete line" {
		t.Errorf("expected 'incomplete line', got %q", lines[1])
	}
}
