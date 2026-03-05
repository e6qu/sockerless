package lambda

import (
	"fmt"
	"net/http"
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

	lambdaState, _ := s.Lambda.Get(id)
	logStreamPrefix := lambdaState.FunctionName

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// BUG-426: Early return if stdout suppressed
	if !params.ShouldWrite() {
		return
	}

	// BUG-435: Filter LogBuffers output through params
	if buf, ok := s.Store.LogBuffers.Load(id); ok {
		data := buf.([]byte)
		if len(data) > 0 {
			params.FilterBufferedOutput(w, data)
		}
	}

	// Query CloudWatch Logs using the function name as log group prefix
	logGroupName := fmt.Sprintf("/aws/lambda/%s", logStreamPrefix)

	// Try to find a log stream for this function
	streamsResult, err := s.aws.CloudWatch.DescribeLogStreams(s.ctx(), &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
		OrderBy:      "LastEventTime",
		Descending:   aws.Bool(true),
		Limit:        aws.Int32(1),
	})
	if err != nil {
		s.Logger.Debug().Err(err).Str("logGroup", logGroupName).Msg("failed to describe log streams")
		return
	}

	if len(streamsResult.LogStreams) == 0 {
		return
	}

	logStreamName := streamsResult.LogStreams[0].LogStreamName

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: logStreamName,
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

	result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to get log events")
		return
	}

	for _, event := range result.Events {
		s.writeLogEvent(w, event.Message, event.Timestamp, params)
	}
	core.FlushIfNeeded(w)

	// BUG-428: Follow mode support
	if !params.Follow {
		return
	}

	nextToken := result.NextForwardToken

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				// Fetch any remaining logs
				followInput := &cloudwatchlogs.GetLogEventsInput{
					LogGroupName:  aws.String(logGroupName),
					LogStreamName: logStreamName,
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

			followInput := &cloudwatchlogs.GetLogEventsInput{
				LogGroupName:  aws.String(logGroupName),
				LogStreamName: logStreamName,
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
	line := params.FormatLine(*message, ts)
	core.WriteMuxLine(w, line)
}
