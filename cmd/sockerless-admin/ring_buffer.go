package main

import "sync"

// RingBuffer is a thread-safe ring buffer that captures the last N lines.
type RingBuffer struct {
	mu      sync.Mutex
	lines   []string
	pos     int
	cap     int
	full    bool
	partial string // carry-over buffer for incomplete lines
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		lines: make([]string, capacity),
		cap:   capacity,
	}
}

// Write implements io.Writer. It splits input on newlines and stores each line.
func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Split on newlines
	start := 0
	first := true
	for i, b := range p {
		if b == '\n' {
			line := string(p[start:i])
			if first && rb.partial != "" {
				line = rb.partial + line
				rb.partial = ""
			}
			first = false
			rb.lines[rb.pos] = line
			rb.pos++
			if rb.pos >= rb.cap {
				rb.pos = 0
				rb.full = true
			}
			start = i + 1
		}
	}
	// Remaining bytes (partial line without trailing newline) — carry over
	if start < len(p) {
		tail := string(p[start:])
		if first && rb.partial != "" {
			rb.partial += tail
		} else {
			rb.partial = tail
		}
	}

	return len(p), nil
}

// Reset clears all lines and resets the buffer to empty.
func (rb *RingBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for i := range rb.lines {
		rb.lines[i] = ""
	}
	rb.pos = 0
	rb.full = false
	rb.partial = ""
}

// Lines returns the last n lines from the buffer.
func (rb *RingBuffer) Lines(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	total := rb.pos
	if rb.full {
		total = rb.cap
	}
	if n <= 0 {
		return nil
	}
	if n > total {
		n = total
	}
	if n == 0 {
		return nil
	}

	result := make([]string, n)
	if rb.full {
		// Read from (pos - n) wrapping around
		start := rb.pos - n
		if start < 0 {
			start += rb.cap
		}
		for i := 0; i < n; i++ {
			idx := (start + i) % rb.cap
			result[i] = rb.lines[idx]
		}
	} else {
		start := rb.pos - n
		for i := 0; i < n; i++ {
			result[i] = rb.lines[start+i]
		}
	}
	return result
}
