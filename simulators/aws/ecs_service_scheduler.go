package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// ECS service scheduler — reconciles each ACTIVE ECS service so that the
// number of RUNNING tasks tracks DesiredCount. Mirrors the real ECS
// scheduler: it observes the actual running task set, launches replacements
// when tasks stop, scales up on DesiredCount increase, scales down on
// decrease, and drains a service's tasks when it is deleted. The service's
// RunningCount is always derived from the live task set, never assumed.

// ecsSchedulerInterval is the reconciliation period. Real ECS reconciles
// continuously; 2s is a fast-enough tick for tests and clients polling
// DescribeServices while remaining cheap on an idle sim.
const ecsSchedulerInterval = 2 * time.Second

// ecsSchedulerStarted guards against starting two scheduler goroutines when
// buildSimulator is invoked more than once in one process (e.g. spec tests).
var ecsSchedulerStarted bool

// startECSScheduler launches the background reconciliation loop. It is safe
// to call multiple times — only the first call starts a goroutine.
func startECSScheduler() {
	if ecsSchedulerStarted {
		return
	}
	ecsSchedulerStarted = true
	go ecsSchedulerLoop()
}

func ecsSchedulerLoop() {
	t := time.NewTicker(ecsSchedulerInterval)
	defer t.Stop()
	for range t.C {
		ecsReconcileServices(context.Background())
	}
}

// ecsReconcileServices walks every ACTIVE service and aligns its RUNNING task
// count with DesiredCount. One pass: scale-down first (stop the surplus),
// then scale-up (launch the deficit), then refresh RunningCount +
// deployment state from the resulting task set.
func ecsReconcileServices(ctx context.Context) {
	for _, svc := range ecsServices.List() {
		ecsReconcileService(ctx, svc)
	}
}

// ecsServiceTaskGroup is the Group tag the scheduler / ECS assigns to a task
// started by a service: "service:<service-name>".
func ecsServiceTaskGroup(serviceName string) string {
	return "service:" + serviceName
}

func ecsReconcileService(ctx context.Context, svc ECSService) {
	if svc.Status != "ACTIVE" {
		// INACTIVE (deleted) services have their tasks drained at delete
		// time; the scheduler does not touch them.
		return
	}
	clusterName := ecsClusterNameFromRef(svc.ClusterArn)
	key := ecsServiceKey(clusterName, svc.ServiceName)
	group := ecsServiceTaskGroup(svc.ServiceName)

	running := ecsServiceRunningTasks(svc.ClusterArn, group)

	if len(running) > svc.DesiredCount {
		excess := len(running) - svc.DesiredCount
		// Real ECS stops the most recently launched tasks first when scaling
		// down (LIFO). The store returns tasks in insertion order, so walk
		// from the end.
		for i := len(running) - 1; i >= 0 && excess > 0; i-- {
			t := running[i]
			if t.LastStatus == ECSTaskStatusStopped {
				continue
			}
			stopECSTask(t.TaskID(), "Scheduler scaling down service", "ServiceScheduler")
			excess--
		}
	}

	// Recompute after the scale-down pass so the deficit accounts for tasks
	// that just transitioned to STOPPED.
	running = ecsServiceRunningTasks(svc.ClusterArn, group)
	deficit := svc.DesiredCount - len(running)
	if deficit > 0 {
		ecsLaunchServiceTasks(ctx, svc, deficit)
	}

	// Refresh counts from cloud state. Pending tasks (PROVISIONING/PENDING)
	// are tracked separately so DescribeServices reports a pendingCount that
	// matches what real ECS exposes while a freshly scaled-up service warms.
	running = ecsServiceTasksForGroup(svc.ClusterArn, group)
	runningCount, pendingCount := 0, 0
	for _, t := range running {
		switch t.LastStatus {
		case ECSTaskStatusRunning:
			runningCount++
		case ECSTaskStatusProvisioning, ECSTaskStatusPending:
			pendingCount++
		}
	}

	updated := false
	ecsServices.Update(key, func(s *ECSService) {
		if s.RunningCount != runningCount {
			s.RunningCount = runningCount
			updated = true
		}
		if s.PendingCount != pendingCount {
			s.PendingCount = pendingCount
			updated = true
		}
		if updated && len(s.Deployments) > 0 {
			now := float64(time.Now().Unix())
			s.Deployments[0].RunningCount = runningCount
			s.Deployments[0].PendingCount = pendingCount
			s.Deployments[0].DesiredCount = s.DesiredCount
			s.Deployments[0].UpdatedAt = now
		}
	})
}

// ecsLaunchServiceTasks starts `count` tasks for the service through the
// shared RunTask code path, tagging each with the service's Group and
// StartedBy so the scheduler can identify and reconcile them.
func ecsLaunchServiceTasks(ctx context.Context, svc ECSService, count int) {
	netCfg := decodeServiceNetworkConfig(svc.NetworkConfiguration)
	// RunTask caps each call at 10 tasks; chunk so a service with a large
	// DesiredCount still converges in one tick.
	for remaining := count; remaining > 0; {
		batch := remaining
		if batch > 10 {
			batch = 10
		}
		in := ecsRunTaskInput{
			Cluster:              svc.ClusterArn,
			TaskDefinition:       svc.TaskDefinition,
			Count:                batch,
			Group:                ecsServiceTaskGroup(svc.ServiceName),
			LaunchType:           svc.LaunchType,
			NetworkConfiguration: netCfg,
			StartedBy:            "ecs-svc/" + svc.ServiceName,
		}
		if _, rerr := runECSTasks(ctx, in); rerr != nil {
			// A launch failure surfaces through the task's StoppedReason
			// (runECSTasks records it on the task); the scheduler will
			// retry the deficit on the next tick. Don't spam stderr here.
			break
		}
		remaining -= batch
	}
}

// decodeServiceNetworkConfig parses a service's raw networkConfiguration
// block (captured verbatim at create/update time) into the typed shape
// runECSTasks consumes. Returns nil when the block is absent or malformed.
func decodeServiceNetworkConfig(raw json.RawMessage) *ECSTaskNetworkConfig {
	if len(raw) == 0 {
		return nil
	}
	var cfg struct {
		AwsvpcConfiguration *ECSTaskVpcConfig `json:"awsvpcConfiguration"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	if cfg.AwsvpcConfiguration == nil {
		return nil
	}
	return &ECSTaskNetworkConfig{AwsvpcConfiguration: cfg.AwsvpcConfiguration}
}

// ecsServiceRunningTasks returns the service's non-STOPPED tasks — the set
// the scheduler treats as occupying a desired-count slot.
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
// matches the service's `service:<name>` tag, including STOPPED tasks so the
// scheduler can refresh RunningCount from a single source of truth.
func ecsServiceTasksForGroup(clusterArn, group string) []ECSTask {
	if group == "" {
		return nil
	}
	return ecsTasks.Filter(func(t ECSTask) bool {
		return t.ClusterArn == clusterArn && t.Group == group
	})
}

// ecsStopServiceTasks drains every non-STOPPED task for a service. Called
// from DeleteService so a deleted service's tasks don't keep running.
func ecsStopServiceTasks(svc ECSService) {
	group := ecsServiceTaskGroup(svc.ServiceName)
	for _, t := range ecsServiceRunningTasks(svc.ClusterArn, group) {
		stopECSTask(t.TaskID(), "Service deleted", "ServiceScheduler")
	}
}

// TaskID returns the bare task ID (trailing path segment of the TaskArn).
// Method on ECSTask so the scheduler can pass task references around without
// re-parsing the ARN at every callsite.
func (t ECSTask) TaskID() string {
	arn := t.TaskArn
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}
