package main

import (
	"strings"
)

// ECS service task draining — the ECS service is a control-plane state machine
// (see ecs_service.go): it reaches runningCount == desiredCount synchronously
// and never launches ephemeral Docker containers to satisfy DesiredCount. Real
// container execution is RunTask's job. The only task work the service owns is
// draining: on DeleteService any task still tagged with the service's group is
// stopped so a deleted service leaves nothing running.

// ecsServiceTaskGroup is the Group tag ECS assigns to a task started for a
// service: "service:<service-name>".
func ecsServiceTaskGroup(serviceName string) string {
	return "service:" + serviceName
}

// ecsServiceRunningTasks returns the service's non-STOPPED tasks — the set a
// delete must drain.
func ecsServiceRunningTasks(clusterArn, group string) []ECSTask {
	tasks := ecsServiceTasksForGroup(clusterArn, group)
	out := tasks[:0]
	for _, t := range tasks {
		if t.LastStatus == ECSTaskStatusStopped {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ecsServiceTasksForGroup enumerates every task in the cluster whose Group
// matches the service's `service:<name>` tag, including STOPPED tasks.
func ecsServiceTasksForGroup(clusterArn, group string) []ECSTask {
	if group == "" {
		return nil
	}
	return ecsTasks.Filter(func(t ECSTask) bool {
		return t.ClusterArn == clusterArn && t.Group == group
	})
}

// ecsStopServiceTasks drains every non-STOPPED task for a service. Called from
// DeleteService so a deleted service's tasks don't keep running.
func ecsStopServiceTasks(svc ECSService) {
	group := ecsServiceTaskGroup(svc.ServiceName)
	for _, t := range ecsServiceRunningTasks(svc.ClusterArn, group) {
		stopECSTask(t.TaskID(), "Service deleted", "ServiceScheduler")
	}
}

// TaskID returns the bare task ID (trailing path segment of the TaskArn).
// Method on ECSTask so callers can pass task references around without
// re-parsing the ARN at every callsite.
func (t ECSTask) TaskID() string {
	arn := t.TaskArn
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}
