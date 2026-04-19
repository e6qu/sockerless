package core

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// CloudLogEntry represents a single log entry from a cloud logging service.
type CloudLogEntry struct {
	Timestamp time.Time
	Message   string
}

// CloudLogFetchFunc fetches log entries from a cloud logging service.
// cursor is opaque state from a previous call (nil on first call).
// Returns entries sorted by time and an updated cursor for follow-mode calls.
type CloudLogFetchFunc func(ctx context.Context, params CloudLogParams, cursor any) ([]CloudLogEntry, any, error)

// StreamCloudLogsOptions configures StreamCloudLogs behavior per backend type.
type StreamCloudLogsOptions struct {
	// CheckLogBuffers causes StreamCloudLogs to use in-memory LogBuffers if available.
	// Set true for FaaS backends where invocation output is captured in memory.
	CheckLogBuffers bool

	// AllowCreated tolerates containers in the "created" state (not yet
	// started) instead of rejecting the call. Used by ContainerAttach
	// on the create→attach→start docker flow: the attach stream must
	// open before start, so the container is still "created" when we
	// set up polling. The follow loop then waits for it to transition
	// to running and begin emitting logs.
	AllowCreated bool
}

// StreamCloudLogs implements the common ContainerLogs pattern for cloud backends.
// It handles container resolution, state checking, LogBuffers fallback,
// io.Pipe creation, formatting, tail filtering, and follow-mode polling.
// Writes raw text to the pipe — the HTTP handler adds Docker mux framing.
func StreamCloudLogs(s *BaseServer, containerID string, opts api.ContainerLogsOptions, fetch CloudLogFetchFunc, sopts StreamCloudLogsOptions) (io.ReadCloser, error) {
	// Resolve container via CloudState-aware path (stateless backends have no Store.Containers)
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}
	id := c.ID

	if c.State.Status == "created" && !sopts.AllowCreated {
		return nil, &api.InvalidParameterError{
			Message: "can not get logs from container which is dead or marked for removal",
		}
	}

	params := CloudLogParamsFromOpts(opts, c.Config.Labels)
	if !params.ShouldWrite() {
		return io.NopCloser(strings.NewReader("")), nil
	}

	// FaaS backends check in-memory log buffers first.
	if sopts.CheckLogBuffers {
		if bufData, bufOK := s.Store.LogBuffers.Load(id); bufOK {
			buf := bufData.([]byte)
			if len(buf) > 0 {
				return streamBufferedLogs(params, buf), nil
			}
		}
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close() //nolint:errcheck
		ctx := context.Background()

		entries, cursor, err := fetch(ctx, params, nil)
		if err != nil {
			return
		}

		// Apply tail filtering.
		if params.Tail >= 0 && len(entries) > params.Tail {
			entries = entries[len(entries)-params.Tail:]
		}

		for _, e := range entries {
			writeLogLine(pw, params.FormatLine(e.Message, e.Timestamp))
		}

		if !params.Follow {
			return
		}

		// Follow mode: poll every second.
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Stop following only when the container has reached a
			// terminal state. Pre-start (status "created") must keep
			// polling because attach-before-start is a valid docker
			// flow (used by `docker run` without `-d`). Treating
			// `!Running` as the exit trigger would drop the stream
			// before the container ever emits its first line.
			if cc, ccOK := s.ResolveContainerAuto(ctx, id); ccOK && isTerminalState(cc.State.Status) {
				// Final fetch before exit.
				entries, _, _ = fetch(ctx, params, cursor)
				for _, e := range entries {
					writeLogLine(pw, params.FormatLine(e.Message, e.Timestamp))
				}
				return
			}

			entries, cursor, err = fetch(ctx, params, cursor)
			if err != nil {
				continue
			}
			for _, e := range entries {
				writeLogLine(pw, params.FormatLine(e.Message, e.Timestamp))
			}
		}
	}()

	return pr, nil
}

// streamBufferedLogs creates a pipe reader that writes LogBuffers data
// filtered through CloudLogParams. Writes raw text (no mux framing).
func streamBufferedLogs(params CloudLogParams, buf []byte) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close() //nolint:errcheck

		raw := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
		now := time.Now().UTC()

		var filtered []string
		for _, line := range raw {
			if line == "" {
				continue
			}
			if !params.FilterByTime(now) {
				continue
			}
			filtered = append(filtered, line)
		}

		filtered = params.ApplyTail(filtered)

		for _, line := range filtered {
			writeLogLine(pw, params.FormatLine(line, now))
		}
	}()
	return pr
}

// writeLogLine writes a formatted log line as raw text to w.
func writeLogLine(w io.Writer, line string) {
	_, _ = io.WriteString(w, line)
}

// isTerminalState reports whether a container's State.Status indicates
// the container will not produce further logs. Pre-start ("created")
// and transient states like "running" / "restarting" / "paused" are
// non-terminal. Used by StreamCloudLogs follow mode to decide when to
// close the stream.
func isTerminalState(status string) bool {
	switch status {
	case "exited", "dead", "removing":
		return true
	default:
		return false
	}
}
