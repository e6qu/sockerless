package ecs

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Status == "created" {
		core.WriteError(w, &api.InvalidParameterError{
			Message: "can not get logs from container which is dead or marked for removal",
		})
		return
	}

	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"
	timestamps := r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true"
	tail := r.URL.Query().Get("tail")

	logStreamPrefix := id[:12]
	logStreamName := fmt.Sprintf("%s/main/%s", logStreamPrefix, s.getTaskID(id))

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// Determine limit
	var limit *int32
	if tail != "" && tail != "all" {
		n := int32(100)
		fmt.Sscanf(tail, "%d", &n)
		limit = &n
	}

	var nextToken *string
	startFromHead := limit == nil

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(s.config.LogGroup),
		LogStreamName: aws.String(logStreamName),
		StartFromHead: aws.Bool(startFromHead),
	}
	if limit != nil {
		input.Limit = limit
	}

	// Fetch initial logs
	result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
	if err != nil {
		// If log stream doesn't exist yet, return empty
		s.Logger.Debug().Err(err).Str("stream", logStreamName).Msg("failed to get log events")
		return
	}

	for _, event := range result.Events {
		s.writeLogEvent(w, event.Message, event.Timestamp, timestamps)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	nextToken = result.NextForwardToken

	if !follow {
		return
	}

	// Follow mode: poll for new events
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Check if container has stopped
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				// Fetch any remaining logs
				input := &cloudwatchlogs.GetLogEventsInput{
					LogGroupName:  aws.String(s.config.LogGroup),
					LogStreamName: aws.String(logStreamName),
					StartFromHead: aws.Bool(true),
					NextToken:     nextToken,
				}
				result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
				if err == nil {
					for _, event := range result.Events {
						s.writeLogEvent(w, event.Message, event.Timestamp, timestamps)
					}
				}
				return
			}

			input := &cloudwatchlogs.GetLogEventsInput{
				LogGroupName:  aws.String(s.config.LogGroup),
				LogStreamName: aws.String(logStreamName),
				StartFromHead: aws.Bool(true),
				NextToken:     nextToken,
			}
			result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
			if err != nil {
				continue
			}

			for _, event := range result.Events {
				s.writeLogEvent(w, event.Message, event.Timestamp, timestamps)
			}
			if len(result.Events) > 0 {
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			nextToken = result.NextForwardToken
		}
	}
}

func (s *Server) writeLogEvent(w http.ResponseWriter, message *string, timestamp *int64, addTimestamp bool) {
	if message == nil {
		return
	}
	line := *message
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}

	if addTimestamp && timestamp != nil {
		ts := time.UnixMilli(*timestamp).UTC().Format(time.RFC3339Nano)
		line = ts + " " + line
	}

	data := []byte(line)
	// Docker mux header: [stream_type, 0, 0, 0, size (4 bytes big-endian)]
	header := make([]byte, 8)
	header[0] = 1 // stdout
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	w.Write(header)
	w.Write(data)
}

// getTaskID extracts the task ID from the stored ECS state.
func (s *Server) getTaskID(containerID string) string {
	ecsState, ok := s.ECS.Get(containerID)
	if !ok || ecsState.TaskARN == "" {
		return "unknown"
	}
	// TaskARN format: arn:aws:ecs:region:account:task/cluster/taskid
	parts := strings.Split(ecsState.TaskARN, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ecsState.TaskARN
}
