package ecs

import (
	"strings"
)

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
