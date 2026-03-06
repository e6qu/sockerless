package ecs

import (
	"net/http"
	"strings"
	"time"

	core "github.com/sockerless/backend-core"
)

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
