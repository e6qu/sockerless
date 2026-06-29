package main

import (
	"context"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

// TestECSServiceScheduler_DerivesCountsFromTaskSet verifies the scheduler's
// count-derivation logic without any container runtime: given an ACTIVE service
// and a set of tasks in various statuses, ecsReconcileService updates the
// service's RunningCount and PendingCount to match the live task set and stops
// surplus tasks on scale-down.
func TestECSServiceScheduler_DerivesCountsFromTaskSet(t *testing.T) {
	// Isolate the test from any other package-level state.
	ecsClusters = sim.MakeStore[ECSCluster](nil, "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](nil, "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](nil, "ecs_tasks")
	ecsServices = sim.MakeStore[ECSService](nil, "ecs_services")
	ecsRevisions = map[string]int{}
	ecsSchedulerStarted = false

	cluster := ECSCluster{
		ClusterName: "sched-unit-cluster",
		ClusterArn:  ecsArn("cluster", "sched-unit-cluster"),
		Status:      "ACTIVE",
	}
	ecsClusters.Put(cluster.ClusterName, cluster)

	svc := ECSService{
		ServiceArn:     ecsArn("service", "sched-unit-cluster/sched-unit-svc"),
		ServiceName:    "sched-unit-svc",
		ClusterArn:     cluster.ClusterArn,
		TaskDefinition: "sched-unit-task",
		DesiredCount:   2,
		RunningCount:   0,
		PendingCount:   0,
		Status:         "ACTIVE",
	}
	key := ecsServiceKey("sched-unit-cluster", svc.ServiceName)
	ecsServices.Put(key, svc)

	group := ecsServiceTaskGroup("sched-unit-svc")
	startedBy := "ecs-svc/sched-unit-svc"

	// Seed two RUNNING tasks that belong to the service.
	task1 := makeECSTestTask(cluster.ClusterArn, group, startedBy, ECSTaskStatusRunning)
	task2 := makeECSTestTask(cluster.ClusterArn, group, startedBy, ECSTaskStatusRunning)
	ecsTasks.Put(task1.TaskID(), task1)
	ecsTasks.Put(task2.TaskID(), task2)

	// Reconcile: both tasks are RUNNING, so RunningCount must become 2.
	ecsReconcileService(context.Background(), svc)
	svc, _ = ecsServices.Get(key)
	if svc.RunningCount != 2 {
		t.Fatalf("expected RunningCount=2, got %d", svc.RunningCount)
	}
	if svc.PendingCount != 0 {
		t.Fatalf("expected PendingCount=0, got %d", svc.PendingCount)
	}

	// Mark one task as STOPPED and add a PENDING task.
	ecsTasks.Update(task1.TaskID(), func(t *ECSTask) { t.LastStatus = ECSTaskStatusStopped })
	task3 := makeECSTestTask(cluster.ClusterArn, group, startedBy, ECSTaskStatusPending)
	ecsTasks.Put(task3.TaskID(), task3)

	ecsReconcileService(context.Background(), svc)
	svc, _ = ecsServices.Get(key)
	if svc.RunningCount != 1 {
		t.Fatalf("expected RunningCount=1, got %d", svc.RunningCount)
	}
	if svc.PendingCount != 1 {
		t.Fatalf("expected PendingCount=1, got %d", svc.PendingCount)
	}

	// Scale to zero: scheduler must stop the remaining non-STOPPED task and
	// update counts synchronously.
	ecsServices.Update(key, func(s *ECSService) { s.DesiredCount = 0 })
	svc, _ = ecsServices.Get(key)
	ecsReconcileService(context.Background(), svc)
	svc, _ = ecsServices.Get(key)
	if svc.RunningCount != 0 {
		t.Fatalf("after scale to zero: expected RunningCount=0, got %d", svc.RunningCount)
	}
	if svc.PendingCount != 0 {
		t.Fatalf("after scale to zero: expected PendingCount=0, got %d", svc.PendingCount)
	}
}

// TestECSServiceScheduler_SkipsInactiveServices verifies the scheduler does
// not launch tasks for INACTIVE services.
func TestECSServiceScheduler_SkipsInactiveServices(t *testing.T) {
	ecsClusters = sim.MakeStore[ECSCluster](nil, "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](nil, "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](nil, "ecs_tasks")
	ecsServices = sim.MakeStore[ECSService](nil, "ecs_services")
	ecsRevisions = map[string]int{}
	ecsSchedulerStarted = false

	cluster := ECSCluster{
		ClusterName: "sched-inactive-cluster",
		ClusterArn:  ecsArn("cluster", "sched-inactive-cluster"),
		Status:      "ACTIVE",
	}
	ecsClusters.Put(cluster.ClusterName, cluster)

	svc := ECSService{
		ServiceArn:     ecsArn("service", "sched-inactive-cluster/sched-inactive-svc"),
		ServiceName:    "sched-inactive-svc",
		ClusterArn:     cluster.ClusterArn,
		TaskDefinition: "sched-inactive-task",
		DesiredCount:   1,
		Status:         "INACTIVE",
	}
	key := ecsServiceKey("sched-inactive-cluster", svc.ServiceName)
	ecsServices.Put(key, svc)

	ecsReconcileService(context.Background(), svc)

	if ecsTasks.Len() != 0 {
		t.Fatalf("INACTIVE service must not launch tasks; got %d tasks", ecsTasks.Len())
	}
}

// TestECSServiceScheduler_LaunchesTasksWithServiceMetadata verifies that
// ecsLaunchServiceTasks creates tasks tagged with the service's Group and
// StartedBy. The task launch itself goes through runECSTasks; we inspect the
// created tasks immediately, before the async container-start goroutines can
// alter their status.
func TestECSServiceScheduler_LaunchesTasksWithServiceMetadata(t *testing.T) {
	ecsClusters = sim.MakeStore[ECSCluster](nil, "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](nil, "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](nil, "ecs_tasks")
	ecsServices = sim.MakeStore[ECSService](nil, "ecs_services")
	ecsRevisions = map[string]int{"sched-launch-task": 1}
	ecsSchedulerStarted = false

	cluster := ECSCluster{
		ClusterName: "sched-launch-cluster",
		ClusterArn:  ecsArn("cluster", "sched-launch-cluster"),
		Status:      "ACTIVE",
	}
	ecsClusters.Put(cluster.ClusterName, cluster)

	td := ECSTaskDefinition{
		Family:               "sched-launch-task",
		TaskDefinitionArn:    ecsArn("task-definition", "sched-launch-task:1"),
		ContainerDefinitions: []ECSContainerDefinition{{Name: "app", Image: "scratch"}},
	}
	ecsTaskDefinitions.Put("sched-launch-task:1", td)

	svc := ECSService{
		ServiceArn:     ecsArn("service", "sched-launch-cluster/sched-launch-svc"),
		ServiceName:    "sched-launch-svc",
		ClusterArn:     cluster.ClusterArn,
		TaskDefinition: "sched-launch-task",
		DesiredCount:   2,
		Status:         "ACTIVE",
	}
	ecsServices.Put(ecsServiceKey("sched-launch-cluster", svc.ServiceName), svc)

	ecsLaunchServiceTasks(context.Background(), svc, 2)

	group := ecsServiceTaskGroup("sched-launch-svc")
	startedBy := "ecs-svc/sched-launch-svc"
	tasks := ecsServiceTasksForGroup(cluster.ClusterArn, group)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 launched tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Group != group {
			t.Fatalf("task Group mismatch: expected %q, got %q", group, task.Group)
		}
		if task.StartedBy != startedBy {
			t.Fatalf("task StartedBy mismatch: expected %q, got %q", startedBy, task.StartedBy)
		}
		wantTDArn := ecsArn("task-definition", "sched-launch-task:1")
		if task.TaskDefinitionArn != wantTDArn {
			t.Fatalf("task TaskDefinition mismatch: expected %q, got %q", wantTDArn, task.TaskDefinitionArn)
		}
	}
}

func makeECSTestTask(clusterArn, group, startedBy string, status ECSTaskStatus) ECSTask {
	taskID := generateUUID()
	return ECSTask{
		TaskArn:           ecsArn("task", clusterArn[strings.LastIndex(clusterArn, "/")+1:]+"/"+taskID),
		TaskDefinitionArn: ecsArn("task-definition", "sched-unit-task:1"),
		ClusterArn:        clusterArn,
		LastStatus:        status,
		DesiredStatus:     ECSTaskStatusRunning,
		Group:             group,
		StartedBy:         startedBy,
	}
}
