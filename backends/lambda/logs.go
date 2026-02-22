package lambda

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

	timestamps := r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true"
	tail := r.URL.Query().Get("tail")

	lambdaState, _ := s.Lambda.Get(id)
	logStreamPrefix := lambdaState.FunctionName

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

	startFromHead := limit == nil

	// Query CloudWatch Logs using the function name as log group prefix
	logGroupName := fmt.Sprintf("/aws/lambda/%s", logStreamPrefix)

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroupName),
		StartFromHead: aws.Bool(startFromHead),
	}
	if limit != nil {
		input.Limit = limit
	}

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

	input.LogStreamName = streamsResult.LogStreams[0].LogStreamName

	result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to get log events")
		return
	}

	for _, event := range result.Events {
		s.writeLogEvent(w, event.Message, event.Timestamp, timestamps)
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
	header := make([]byte, 8)
	header[0] = 1 // stdout
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	w.Write(header)
	w.Write(data)
}
