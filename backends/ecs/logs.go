package ecs

import (
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

	params := core.ParseCloudLogParams(r, c.Config.Labels)

	logStreamPrefix := id[:12]
	logStreamName := fmt.Sprintf("%s/main/%s", logStreamPrefix, s.getTaskID(id))

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// BUG-426: Early return if stdout suppressed (all cloud logs are stdout)
	if !params.ShouldWrite() {
		return
	}

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(s.config.LogGroup),
		LogStreamName: aws.String(logStreamName),
		StartFromHead: aws.Bool(params.CloudLogTailInt32() == nil),
	}
	if limit := params.CloudLogTailInt32(); limit != nil {
		input.Limit = limit
	}
	// BUG-423: Apply since as StartTime
	if ms := params.SinceMillis(); ms != nil {
		input.StartTime = ms
	}
	// BUG-424: Apply until as EndTime
	if ms := params.UntilMillis(); ms != nil {
		input.EndTime = ms
	}

	// Fetch initial logs
	result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
	if err != nil {
		s.Logger.Debug().Err(err).Str("stream", logStreamName).Msg("failed to get log events")
		return
	}

	for _, event := range result.Events {
		s.writeLogEvent(w, event.Message, event.Timestamp, params)
	}
	core.FlushIfNeeded(w)

	nextToken := result.NextForwardToken

	if !params.Follow {
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
				// Fetch any remaining logs — BUG-433: no since/until on follow queries
				followInput := &cloudwatchlogs.GetLogEventsInput{
					LogGroupName:  aws.String(s.config.LogGroup),
					LogStreamName: aws.String(logStreamName),
					StartFromHead: aws.Bool(true),
					NextToken:     nextToken,
				}
				result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), followInput)
				if err == nil {
					for _, event := range result.Events {
						s.writeLogEvent(w, event.Message, event.Timestamp, params)
					}
				}
				return
			}

			// BUG-433: Follow queries use NextToken only, no since/until
			followInput := &cloudwatchlogs.GetLogEventsInput{
				LogGroupName:  aws.String(s.config.LogGroup),
				LogStreamName: aws.String(logStreamName),
				StartFromHead: aws.Bool(true),
				NextToken:     nextToken,
			}
			result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), followInput)
			if err != nil {
				continue
			}

			for _, event := range result.Events {
				s.writeLogEvent(w, event.Message, event.Timestamp, params)
			}
			if len(result.Events) > 0 {
				core.FlushIfNeeded(w)
			}
			nextToken = result.NextForwardToken
		}
	}
}

func (s *Server) writeLogEvent(w http.ResponseWriter, message *string, timestamp *int64, params core.CloudLogParams) {
	if message == nil {
		return
	}
	var ts time.Time
	if timestamp != nil {
		ts = time.UnixMilli(*timestamp)
	}
	// BUG-427: FormatLine handles timestamps + details
	line := params.FormatLine(*message, ts)
	core.WriteMuxLine(w, line)
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
