package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// StartTask runs a task on the specific container instances the caller names,
// rather than letting the scheduler place it (RunTask). It is the EC2-launch-type
// placement primitive: the caller has its own ECS agents (RegisterContainerInstance)
// and assigns the task directly. The sim creates a task associated with the named
// instances, reaching RUNNING as a control-plane object.

func registerECSStartTask(r *sim.AWSRouter, srv *sim.Server) {
	r.Register("AmazonEC2ContainerServiceV20141113.StartTask", handleECSStartTask)
}

func handleECSStartTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster              string           `json:"cluster"`
		ContainerInstances   []string         `json:"containerInstances"`
		TaskDefinition       string           `json:"taskDefinition"`
		Group                string           `json:"group"`
		StartedBy            string           `json:"startedBy"`
		ReferenceId          string           `json:"referenceId"`
		EnableExecuteCommand bool             `json:"enableExecuteCommand"`
		EnableECSManagedTags bool             `json:"enableECSManagedTags"`
		PropagateTags        string           `json:"propagateTags"`
		Tags                 []ECSTag         `json:"tags"`
		Overrides            *ECSTaskOverride `json:"overrides"`
		NetworkConfiguration json.RawMessage  `json:"networkConfiguration"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.TaskDefinition == "" {
		sim.AWSError(w, "InvalidParameterException", "taskDefinition is required", http.StatusBadRequest)
		return
	}
	if len(req.ContainerInstances) == 0 {
		sim.AWSError(w, "InvalidParameterException", "containerInstances cannot be empty", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	cluster, ok := ecsClusters.Get(clusterName)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest, "Cluster not found: %s", req.Cluster)
		return
	}

	// Resolve task definition (latest revision when a bare family is given).
	tdKey := req.TaskDefinition
	if strings.HasPrefix(tdKey, "arn:") {
		parts := strings.Split(tdKey, "/")
		tdKey = parts[len(parts)-1]
	}
	if !strings.Contains(tdKey, ":") {
		ecsRevisionMu.Lock()
		rev, exists := ecsRevisions[tdKey]
		ecsRevisionMu.Unlock()
		if exists {
			tdKey = fmt.Sprintf("%s:%d", tdKey, rev)
		}
	}
	td, ok := ecsTaskDefinitions.Get(tdKey)
	if !ok {
		sim.AWSErrorf(w, "ClientException", http.StatusBadRequest,
			"Unable to describe task definition: %s", req.TaskDefinition)
		return
	}

	var tasks []ECSTask
	var failures []map[string]string
	for _, instanceRef := range req.ContainerInstances {
		instID := ecsContainerInstanceID(instanceRef)
		ci, ciOK := ecsContainerInstances.Get(ecsContainerInstanceKey(clusterName, instID))
		if !ciOK {
			failures = append(failures, map[string]string{
				"arn":    ecsArn("container-instance", clusterName+"/"+instID),
				"reason": "MISSING",
			})
			continue
		}

		taskID := generateUUID()
		taskArn := fmt.Sprintf("arn:aws:ecs:"+awsRegion()+":"+awsAccountID()+":task/%s/%s", clusterName, taskID)
		createdAt := float64(time.Now().Unix())
		startedAt := time.Now().Unix()

		var containers []ECSTaskContainer
		for _, cd := range td.ContainerDefinitions {
			containers = append(containers, ECSTaskContainer{
				ContainerArn: fmt.Sprintf("arn:aws:ecs:"+awsRegion()+":"+awsAccountID()+":container/%s", generateUUID()),
				Name:         cd.Name,
				LastStatus:   "RUNNING",
			})
		}

		var taskTags []ECSTag
		if req.PropagateTags == "TASK_DEFINITION" && len(td.Tags) > 0 {
			taskTags = append(taskTags, td.Tags...)
		}
		taskTags = append(taskTags, req.Tags...)

		task := ECSTask{
			TaskArn:              taskArn,
			TaskDefinitionArn:    td.TaskDefinitionArn,
			ClusterArn:           cluster.ClusterArn,
			LastStatus:           ECSTaskStatusRunning,
			DesiredStatus:        ECSTaskStatusRunning,
			Connectivity:         "CONNECTED",
			Containers:           containers,
			CreatedAt:            &createdAt,
			StartedAt:            &startedAt,
			Tags:                 taskTags,
			LaunchType:           "EC2",
			Cpu:                  ecsTaskCPU(td, req.Overrides),
			Memory:               ecsTaskMemory(td, req.Overrides),
			Group:                req.Group,
			Overrides:            req.Overrides,
			EnableExecuteCommand: req.EnableExecuteCommand,
			StartedBy:            req.StartedBy,
			ContainerInstanceArn: ci.ContainerInstanceArn,
		}
		ecsTasks.Put(taskID, task)
		ecsContainerInstances.Update(ecsContainerInstanceKey(clusterName, instID), func(c *ECSContainerInstance) {
			c.RunningTasksCount++
		})
		tasks = append(tasks, task)
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tasks":    ecsTasksWire(tasks),
		"failures": failures,
	})
}
