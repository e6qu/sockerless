package ecs

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ssmFrameCapture is a per-session debug logger of every SSM WS frame
// the backend reads and writes. Activated by setting
// `SOCKERLESS_ECS_SSM_CAPTURE_DIR=/some/dir`. One file per session,
// hex-dumped frames, no PII. Off by default. The capture is purely
// observational — it never alters control flow.
type ssmFrameCapture struct {
	w  *os.File
	mu sync.Mutex
}

func openSSMCapture(taskARN, cmd string) *ssmFrameCapture {
	dir := os.Getenv("SOCKERLESS_ECS_SSM_CAPTURE_DIR")
	if dir == "" {
		return &ssmFrameCapture{}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &ssmFrameCapture{}
	}
	short := taskARN
	if i := len(short) - 12; i > 0 {
		short = short[i:]
	}
	name := fmt.Sprintf("ssm-%s-%s.log", time.Now().Format("20060102-150405.000"), short)
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return &ssmFrameCapture{}
	}
	fmt.Fprintf(f, "# task=%s\n# cmd=%q\n# t0=%s\n", taskARN, cmd, time.Now().Format(time.RFC3339Nano))
	return &ssmFrameCapture{w: f}
}

func (c *ssmFrameCapture) Close() {
	if c == nil || c.w == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.w.Close()
	c.w = nil
}

func (c *ssmFrameCapture) frame(label string, raw []byte, meta string) {
	if c == nil || c.w == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, "## %s | %s | bytes=%d\n%s\n", label, meta, len(raw), hex.Dump(raw))
}

func (c *ssmFrameCapture) note(format string, args ...any) {
	if c == nil || c.w == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, "## note: "+format+"\n", args...)
}
