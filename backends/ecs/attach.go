package ecs

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// ContainerAttach streams CloudWatch logs for an ECS-backed container as
// a Docker attach-compatible io.ReadWriteCloser. Reads return stdout
// bytes (mux-framed for non-TTY containers, raw for TTY), the stream
// closes when the ECS task transitions to STOPPED. Writes are
// discarded — Fargate tasks have no remote stdin channel.
//
// This overrides the default delegation in backend_delegates.go because
// BaseServer.ContainerAttach returns an immediately-EOF pipe for
// stateless cloud backends, which caused docker CLI to hang after
// POST /containers/{id}/attach (BUG-692).
func (s *Server) ContainerAttach(ref string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	// Best-effort: if the task hasn't been launched yet, return a writer
	// that blocks on the task starting. For the common
	// docker-run-create-attach-start sequence, we have to return here
	// before the POST /start lands, so we can't reference the log stream
	// yet. The fetch closure tolerates stream-not-found by returning
	// empty entries; StreamCloudLogs retries on its follow tick.
	fetch := s.buildCloudWatchFetcher(c.ID)

	// Configure log options to match attach semantics:
	//   - Follow until the container stops
	//   - Stdout by default (Docker attach defaults to stdout+stderr)
	//   - No tail (stream from the start)
	logOpts := api.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Stream,
	}

	// Attach tolerates the pre-start "created" state so the docker
	// run flow (create → attach → start) works; the follow loop waits
	// for the container to transition to running before emitting logs.
	logReader, err := core.StreamCloudLogs(s.BaseServer, ref, logOpts, fetch, core.StreamCloudLogsOptions{AllowCreated: true})
	if err != nil {
		return nil, err
	}

	rwc := &attachStream{reader: logReader}

	if c.Config.Tty {
		return rwc, nil
	}
	return &muxBridge{rwc: rwc}, nil
}

// attachStream adapts an io.ReadCloser (CloudWatch log follower) to
// an io.ReadWriteCloser by discarding writes. Matches the behavior
// expected by docker attach for containers with no stdin channel.
type attachStream struct {
	reader io.ReadCloser
}

func (a *attachStream) Read(p []byte) (int, error)  { return a.reader.Read(p) }
func (a *attachStream) Write(p []byte) (int, error) { return len(p), nil }
func (a *attachStream) Close() error                { return a.reader.Close() }

// buildCloudWatchFetcher returns a CloudLogFetchFunc closure that reads
// log events for the given container's ECS task. Shared between
// ContainerLogs and ContainerAttach. Tolerates a missing log stream
// by returning empty entries — the follow loop will retry.
func (s *Server) buildCloudWatchFetcher(containerID string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		taskID := s.getTaskID(containerID)
		if taskID == "unknown" {
			// Task not yet running; caller polls and will retry.
			return nil, cursor, nil
		}
		logStreamName := fmt.Sprintf("%s/main/%s", containerID[:12], taskID)

		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(s.config.LogGroup),
			LogStreamName: aws.String(logStreamName),
			StartFromHead: aws.Bool(true),
		}
		if cursor != nil {
			input.NextToken = cursor.(*string)
		} else {
			input.StartFromHead = aws.Bool(params.CloudLogTailInt32() == nil)
			if limit := params.CloudLogTailInt32(); limit != nil {
				input.Limit = limit
			}
			if ms := params.SinceMillis(); ms != nil {
				input.StartTime = ms
			}
			if ms := params.UntilMillis(); ms != nil {
				input.EndTime = ms
			}
		}

		result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
		if err != nil {
			// Tolerate "log stream not found" during the window between
			// task start and first log emission; the follow loop will retry.
			return nil, cursor, nil
		}

		var entries []core.CloudLogEntry
		for _, event := range result.Events {
			if event.Message == nil {
				continue
			}
			var ts time.Time
			if event.Timestamp != nil {
				ts = time.UnixMilli(*event.Timestamp)
			}
			entries = append(entries, core.CloudLogEntry{Timestamp: ts, Message: *event.Message})
		}
		return entries, result.NextForwardToken, nil
	}
}
