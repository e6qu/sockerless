package main

import (
	"strings"
	"testing"
	"time"

	sim "github.com/sockerless/simulator"
)

// TestECSStopServiceTasks_DrainsNonStopped verifies the delete-time drain: given
// a service and a set of tasks in various statuses tagged with the service's
// group, ecsStopServiceTasks stops every non-STOPPED task (RUNNING and PENDING)
// and leaves already-STOPPED tasks untouched. Tasks that belong to a different
// group must not be drained.
func TestECSStopServiceTasks_DrainsNonStopped(t *testing.T) {
	// Isolate from any other package-level state. The drain's task-stop events
	// schedule an asynchronous service reconciliation that reads the scheduler,
	// deployment-record, and alarm stores, so they must be real stores even
	// though the drain itself never touches them.
	ecsClusters = sim.MakeStore[ECSCluster](nil, "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](nil, "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](nil, "ecs_tasks")
	ecsServices = sim.MakeStore[ECSService](nil, "ecs_services")
	ecsServiceSchedulerStates = sim.MakeStore[ECSServiceSchedulerState](nil, "ecs_service_scheduler_states")
	ecsServiceDeployments = sim.MakeStore[ECSServiceDeploymentRec](nil, "ecs_service_deployments")
	cwAlarms = sim.MakeStore[CWAlarm](nil, "cw_alarms")
	ecsRevisions = map[string]int{}

	cluster := ECSCluster{
		ClusterName: "drain-cluster",
		ClusterArn:  ecsArn("cluster", "drain-cluster"),
		Status:      "ACTIVE",
	}
	ecsClusters.Put(cluster.ClusterName, cluster)

	svc := ECSService{
		ServiceArn:     ecsArn("service", "drain-cluster/drain-svc"),
		ServiceName:    "drain-svc",
		ClusterArn:     cluster.ClusterArn,
		TaskDefinition: "drain-task",
		DesiredCount:   2,
		Status:         "ACTIVE",
	}
	ecsServices.Put(ecsServiceKey("drain-cluster", svc.ServiceName), svc)

	group := ecsServiceTaskGroup("drain-svc")
	startedBy := "ecs-svc/drain-svc"

	running := makeECSTestTask(cluster.ClusterArn, group, startedBy, ECSTaskStatusRunning)
	pending := makeECSTestTask(cluster.ClusterArn, group, startedBy, ECSTaskStatusPending)
	alreadyStopped := makeECSTestTask(cluster.ClusterArn, group, startedBy, ECSTaskStatusStopped)
	// A task in a different group must be left alone.
	otherGroup := makeECSTestTask(cluster.ClusterArn, ecsServiceTaskGroup("other-svc"), "ecs-svc/other-svc", ECSTaskStatusRunning)
	for _, task := range []ECSTask{running, pending, alreadyStopped, otherGroup} {
		ecsTasks.Put(task.TaskID(), task)
	}

	ecsStopServiceTasks(svc)

	assertStatus := func(name, id string, want ECSTaskStatus) {
		got, ok := ecsTasks.Get(id)
		if !ok {
			t.Fatalf("%s task %s missing after drain", name, id)
		}
		if got.LastStatus != want {
			t.Fatalf("%s task: expected LastStatus=%s, got %s", name, want, got.LastStatus)
		}
	}
	assertStatus("running", running.TaskID(), ECSTaskStatusStopped)
	assertStatus("pending", pending.TaskID(), ECSTaskStatusStopped)
	assertStatus("already-stopped", alreadyStopped.TaskID(), ECSTaskStatusStopped)
	assertStatus("other-group", otherGroup.TaskID(), ECSTaskStatusRunning)
}

func makeECSTestTask(clusterArn, group, startedBy string, status ECSTaskStatus) ECSTask {
	taskID := generateUUID()
	return ECSTask{
		TaskArn:           ecsArn("task", clusterArn[strings.LastIndex(clusterArn, "/")+1:]+"/"+taskID),
		TaskDefinitionArn: ecsArn("task-definition", "drain-task:1"),
		ClusterArn:        clusterArn,
		LastStatus:        status,
		DesiredStatus:     ECSTaskStatusRunning,
		Group:             group,
		StartedBy:         startedBy,
	}
}

// A rollback leaves the failed deployment alongside the replacement, which real
// Amazon ECS also does — while the old tasks drain. What real ECS then does, and
// this simulator did not, is drop it once the new deployment is stable, so
// DescribeServices reports exactly one deployment, PRIMARY.
//
// Holding the failed entry for the life of the service is not cosmetic: any
// deployment gate that waits for every deployment to report COMPLETED waits for
// ever against a service that is healthy and serving. The infra repository's
// post-simulator-upgrade rollout script does exactly that, and burned its full
// 120-poll budget while the service it was watching had been ready for minutes.
func TestECSRefreshServiceState_RetiresSupersededDeploymentOnceStable(t *testing.T) {
	ecsClusters = sim.MakeStore[ECSCluster](nil, "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](nil, "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](nil, "ecs_tasks")
	ecsServices = sim.MakeStore[ECSService](nil, "ecs_services")
	ecsServiceSchedulerStates = sim.MakeStore[ECSServiceSchedulerState](nil, "ecs_service_scheduler_states")
	ecsServiceDeployments = sim.MakeStore[ECSServiceDeploymentRec](nil, "ecs_service_deployments")
	cwAlarms = sim.MakeStore[CWAlarm](nil, "cw_alarms")
	ecsRevisions = map[string]int{}

	cluster := ECSCluster{
		ClusterName: "roll-cluster",
		ClusterArn:  ecsArn("cluster", "roll-cluster"),
		Status:      "ACTIVE",
	}
	ecsClusters.Put(cluster.ClusterName, cluster)

	definitionArn := ecsArn("task-definition", "drain-task:1")
	// The store is keyed family:revision, not by ARN; keying it by ARN leaves the
	// lookup empty, every task then looks like it belongs to an older definition,
	// and the rollout never reaches COMPLETED.
	ecsTaskDefinitions.Put("drain-task:1", ECSTaskDefinition{
		TaskDefinitionArn: definitionArn,
		Family:            "drain-task",
		Revision:          1,
		Status:            "ACTIVE",
	})

	svc := ECSService{
		ServiceArn:     ecsArn("service", "roll-cluster/roll-svc"),
		ServiceName:    "roll-svc",
		ClusterArn:     cluster.ClusterArn,
		TaskDefinition: definitionArn,
		DesiredCount:   1,
		Status:         "ACTIVE",
		Deployments: []ECSDeployment{
			{Id: "ecs-svc/primary", Status: "PRIMARY", TaskDefinition: definitionArn, DesiredCount: 1},
			// The superseded rollout, exactly as ecsStartServiceDeploymentRollback leaves it.
			{Id: "ecs-svc/failed", Status: "ACTIVE", TaskDefinition: definitionArn, RolloutState: "FAILED",
				RolloutStateReason: "tasks failed to start"},
		},
	}
	key := ecsServiceKey("roll-cluster", svc.ServiceName)
	ecsServices.Put(key, svc)

	// One healthy task on the current definition, nothing left from any other:
	// the service is stable and the replaced deployment has drained.
	task := makeECSTestTask(cluster.ClusterArn, ecsServiceTaskGroup("roll-svc"), "ecs-svc/roll-svc", ECSTaskStatusRunning)
	task.TaskDefinitionArn = definitionArn
	// Healthy means started long enough ago to be past the steady-state window;
	// a task that started a moment ago is not yet counted.
	startedAt := time.Now().Add(-10 * ecsServiceSteadyStateWindow).Unix()
	task.StartedAt = &startedAt
	ecsTasks.Put(task.TaskID(), task)

	ecsRefreshServiceState(key)

	got, ok := ecsServices.Get(key)
	if !ok {
		t.Fatal("service missing after refresh")
	}
	if len(got.Deployments) != 1 {
		states := make([]string, 0, len(got.Deployments))
		for _, d := range got.Deployments {
			states = append(states, d.Id+"/"+d.Status+"/"+d.RolloutState)
		}
		t.Fatalf("expected exactly one deployment once stable, got %d: %s",
			len(got.Deployments), strings.Join(states, ", "))
	}
	if got.Deployments[0].Status != "PRIMARY" {
		t.Errorf("surviving deployment status = %q, want PRIMARY", got.Deployments[0].Status)
	}
	if got.Deployments[0].RolloutState != "COMPLETED" {
		t.Errorf("surviving deployment rolloutState = %q, want COMPLETED", got.Deployments[0].RolloutState)
	}
}
