package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gorilla/websocket"

	sim "github.com/sockerless/simulator"
)

// ECS types

type ECSCluster struct {
	ClusterArn                        string `json:"clusterArn"`
	ClusterName                       string `json:"clusterName"`
	Status                            string `json:"status"`
	RunningTasksCount                 int    `json:"runningTasksCount"`
	PendingTasksCount                 int    `json:"pendingTasksCount"`
	ActiveServicesCount               int    `json:"activeServicesCount"`
	RegisteredContainerInstancesCount int    `json:"registeredContainerInstancesCount"`
}

type ECSContainerDefinition struct {
	Name              string               `json:"name"`
	Image             string               `json:"image"`
	Cpu               int                  `json:"cpu,omitempty"`
	Memory            int                  `json:"memory,omitempty"`
	MemoryReservation int                  `json:"memoryReservation,omitempty"`
	Essential         *bool                `json:"essential,omitempty"`
	Environment       []ECSKeyValuePair    `json:"environment,omitempty"`
	MountPoints       []ECSMountPoint      `json:"mountPoints,omitempty"`
	PortMappings      []ECSPortMapping     `json:"portMappings,omitempty"`
	LogConfiguration  *ECSLogConfiguration `json:"logConfiguration,omitempty"`
	EntryPoint        []string             `json:"entryPoint,omitempty"`
	Command           []string             `json:"command,omitempty"`
	PseudoTerminal    bool                 `json:"pseudoTerminal,omitempty"`
	Interactive       bool                 `json:"interactive,omitempty"`
}

type ECSKeyValuePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ECSMountPoint struct {
	SourceVolume  string `json:"sourceVolume"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

type ECSPortMapping struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
}

type ECSLogConfiguration struct {
	LogDriver string            `json:"logDriver"`
	Options   map[string]string `json:"options,omitempty"`
}

type ECSVolume struct {
	Name                   string              `json:"name"`
	EfsVolumeConfiguration *ECSEfsVolumeConfig `json:"efsVolumeConfiguration,omitempty"`
}

type ECSEfsVolumeConfig struct {
	FileSystemId        string                     `json:"fileSystemId"`
	RootDirectory       string                     `json:"rootDirectory,omitempty"`
	TransitEncryption   string                     `json:"transitEncryption,omitempty"`
	AuthorizationConfig *ECSEfsAuthorizationConfig `json:"authorizationConfig,omitempty"`
}

type ECSEfsAuthorizationConfig struct {
	AccessPointId string `json:"accessPointId,omitempty"`
	Iam           string `json:"iam,omitempty"`
}

type ECSTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ECSTaskDefinition struct {
	TaskDefinitionArn       string                   `json:"taskDefinitionArn"`
	Family                  string                   `json:"family"`
	Revision                int                      `json:"revision"`
	ContainerDefinitions    []ECSContainerDefinition `json:"containerDefinitions"`
	Cpu                     string                   `json:"cpu,omitempty"`
	Memory                  string                   `json:"memory,omitempty"`
	NetworkMode             string                   `json:"networkMode,omitempty"`
	RequiresCompatibilities []string                 `json:"requiresCompatibilities,omitempty"`
	ExecutionRoleArn        string                   `json:"executionRoleArn,omitempty"`
	TaskRoleArn             string                   `json:"taskRoleArn,omitempty"`
	Volumes                 []ECSVolume              `json:"volumes,omitempty"`
	Tags                    []ECSTag                 `json:"tags,omitempty"`
	Status                  string                   `json:"status"`
}

type ECSTaskContainer struct {
	ContainerArn      string                `json:"containerArn"`
	Name              string                `json:"name"`
	LastStatus        string                `json:"lastStatus"`
	ExitCode          *int                  `json:"exitCode,omitempty"`
	NetworkInterfaces []ECSNetworkInterface `json:"networkInterfaces,omitempty"`
}

type ECSNetworkInterface struct {
	AttachmentId       string `json:"attachmentId"`
	PrivateIpv4Address string `json:"privateIpv4Address"`
}

type ECSAttachment struct {
	Id      string            `json:"id"`
	Type    string            `json:"type"`
	Status  string            `json:"status"`
	Details []ECSKeyValuePair `json:"details,omitempty"`
}

type ECSTask struct {
	TaskArn              string                `json:"taskArn"`
	TaskDefinitionArn    string                `json:"taskDefinitionArn"`
	ClusterArn           string                `json:"clusterArn"`
	LastStatus           string                `json:"lastStatus"`
	DesiredStatus        string                `json:"desiredStatus"`
	Connectivity         string                `json:"connectivity,omitempty"`
	Containers           []ECSTaskContainer    `json:"containers"`
	CreatedAt            *float64              `json:"createdAt,omitempty"`
	StartedAt            *int64                `json:"startedAt,omitempty"`
	StoppedAt            *int64                `json:"stoppedAt,omitempty"`
	StopCode             string                `json:"stopCode,omitempty"`
	StoppedReason        string                `json:"stoppedReason,omitempty"`
	Attachments          []ECSAttachment       `json:"attachments,omitempty"`
	Tags                 []ECSTag              `json:"tags,omitempty"`
	LaunchType           string                `json:"launchType,omitempty"`
	Cpu                  string                `json:"cpu,omitempty"`
	Memory               string                `json:"memory,omitempty"`
	Group                string                `json:"group,omitempty"`
	EnableExecuteCommand bool                  `json:"enableExecuteCommand,omitempty"`
	NetworkConfiguration *ECSTaskNetworkConfig `json:"networkConfiguration,omitempty"`
}

type ECSTaskNetworkConfig struct {
	AwsvpcConfiguration *ECSTaskVpcConfig `json:"awsvpcConfiguration,omitempty"`
}

type ECSTaskVpcConfig struct {
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups"`
	AssignPublicIp string   `json:"assignPublicIp"`
}

// State stores
var (
	ecsClusters        sim.Store[ECSCluster]
	ecsTaskDefinitions sim.Store[ECSTaskDefinition]
	ecsTasks           sim.Store[ECSTask]
	ecsRevisionMu      sync.Mutex
	ecsRevisions       map[string]int // family -> latest revision
	ecsProcessHandles  sync.Map       // map[taskID]*sim.ContainerHandle
)

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func ecsArn(resourceType, id string) string {
	return fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:%s/%s", resourceType, id)
}

func registerECS(r *sim.AWSRouter, srv *sim.Server) {
	ecsClusters = sim.MakeStore[ECSCluster](srv.DB(), "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](srv.DB(), "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](srv.DB(), "ecs_tasks")
	ecsRevisions = make(map[string]int)

	r.Register("AmazonEC2ContainerServiceV20141113.CreateCluster", handleECSCreateCluster)
	r.Register("AmazonEC2ContainerServiceV20141113.DescribeClusters", handleECSDescribeClusters)
	r.Register("AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition", handleECSRegisterTaskDefinition)
	r.Register("AmazonEC2ContainerServiceV20141113.DeregisterTaskDefinition", handleECSDeregisterTaskDefinition)
	r.Register("AmazonEC2ContainerServiceV20141113.DescribeTaskDefinition", handleECSDescribeTaskDefinition)
	r.Register("AmazonEC2ContainerServiceV20141113.RunTask", handleECSRunTask)
	r.Register("AmazonEC2ContainerServiceV20141113.DescribeTasks", handleECSDescribeTasks)
	r.Register("AmazonEC2ContainerServiceV20141113.StopTask", handleECSStopTask)
	r.Register("AmazonEC2ContainerServiceV20141113.ListTasks", handleECSListTasks)
	r.Register("AmazonEC2ContainerServiceV20141113.DeleteCluster", handleECSDeleteCluster)
	r.Register("AmazonEC2ContainerServiceV20141113.ListTagsForResource", handleECSListTagsForResource)
	r.Register("AmazonEC2ContainerServiceV20141113.TagResource", handleECSTagResource)
	r.Register("AmazonEC2ContainerServiceV20141113.UntagResource", handleECSUntagResource)
	r.Register("AmazonEC2ContainerServiceV20141113.ExecuteCommand", handleECSExecuteCommand(srv))

	// Static WebSocket route for ECS exec sessions (session ID is a path param)
	srv.HandleFunc("GET /ecs-exec/{sessionId}", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionId")
		handleECSExecWebSocket(sessionID)(w, r)
	})

	// Archive upload endpoint: forward tar archive to the Docker container backing an ECS task
	srv.HandleFunc("PUT /sockerless/tasks/{taskId}/archive", func(w http.ResponseWriter, r *http.Request) {
		taskID := r.PathValue("taskId")
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "missing path query parameter", http.StatusBadRequest)
			return
		}

		// Poll for the container handle — it may not be stored yet if the
		// Docker container is still starting (async after RUNNING state).
		var handle *sim.ContainerHandle
		for i := 0; i < 20; i++ {
			if v, ok := ecsProcessHandles.Load(taskID); ok {
				handle = v.(*sim.ContainerHandle)
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if handle == nil {
			http.Error(w, "no running container for task "+taskID, http.StatusNotFound)
			return
		}

		cli := sim.DockerClient()
		if cli == nil {
			http.Error(w, "docker client not available", http.StatusInternalServerError)
			return
		}

		// Create target directory if it doesn't exist
		mkdirExec, mkdirErr := cli.ContainerExecCreate(r.Context(), handle.ContainerID, dockercontainer.ExecOptions{
			Cmd: []string{"mkdir", "-p", path},
		})
		if mkdirErr == nil {
			_ = cli.ContainerExecStart(r.Context(), mkdirExec.ID, dockercontainer.ExecStartOptions{})
		}

		err := cli.CopyToContainer(r.Context(), handle.ContainerID, path, r.Body, dockercontainer.CopyToContainerOptions{
			AllowOverwriteDirWithFile: true,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func handleECSCreateCluster(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClusterName string `json:"clusterName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ClusterName == "" {
		req.ClusterName = "default"
	}

	cluster := ECSCluster{
		ClusterArn:  ecsArn("cluster", req.ClusterName),
		ClusterName: req.ClusterName,
		Status:      "ACTIVE",
	}
	ecsClusters.Put(req.ClusterName, cluster)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"cluster": cluster,
	})
}

func handleECSDescribeClusters(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clusters []string `json:"clusters"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var clusters []ECSCluster
	var failures []map[string]string

	for _, nameOrArn := range req.Clusters {
		// Extract cluster name from ARN if needed
		name := nameOrArn
		if strings.HasPrefix(nameOrArn, "arn:") {
			parts := strings.Split(nameOrArn, "/")
			if len(parts) > 1 {
				name = parts[len(parts)-1]
			}
		}

		cluster, ok := ecsClusters.Get(name)
		if ok {
			// Update running task count
			runningCount := 0
			for _, t := range ecsTasks.List() {
				if t.ClusterArn == cluster.ClusterArn && t.LastStatus == "RUNNING" {
					runningCount++
				}
			}
			cluster.RunningTasksCount = runningCount
			clusters = append(clusters, cluster)
		} else {
			failures = append(failures, map[string]string{
				"arn":    ecsArn("cluster", name),
				"reason": "MISSING",
			})
		}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"clusters": clusters,
		"failures": failures,
	})
}

func handleECSRegisterTaskDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Family                  string                   `json:"family"`
		ContainerDefinitions    []ECSContainerDefinition `json:"containerDefinitions"`
		Cpu                     string                   `json:"cpu,omitempty"`
		Memory                  string                   `json:"memory,omitempty"`
		NetworkMode             string                   `json:"networkMode,omitempty"`
		RequiresCompatibilities []string                 `json:"requiresCompatibilities,omitempty"`
		ExecutionRoleArn        string                   `json:"executionRoleArn,omitempty"`
		TaskRoleArn             string                   `json:"taskRoleArn,omitempty"`
		Volumes                 []ECSVolume              `json:"volumes,omitempty"`
		Tags                    []ECSTag                 `json:"tags,omitempty"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Family == "" {
		sim.AWSError(w, "InvalidParameterException", "Family is required", http.StatusBadRequest)
		return
	}
	if len(req.ContainerDefinitions) == 0 {
		sim.AWSError(w, "InvalidParameterException", "At least one container definition is required", http.StatusBadRequest)
		return
	}

	// Validate Fargate CPU/memory combinations
	if hasFargate(req.RequiresCompatibilities) && req.Cpu != "" && req.Memory != "" {
		if err := validateFargateResources(req.Cpu, req.Memory); err != nil {
			sim.AWSError(w, "ClientException", err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Auto-increment revision
	ecsRevisionMu.Lock()
	ecsRevisions[req.Family]++
	revision := ecsRevisions[req.Family]
	ecsRevisionMu.Unlock()

	td := ECSTaskDefinition{
		TaskDefinitionArn:       fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task-definition/%s:%d", req.Family, revision),
		Family:                  req.Family,
		Revision:                revision,
		ContainerDefinitions:    req.ContainerDefinitions,
		Cpu:                     req.Cpu,
		Memory:                  req.Memory,
		NetworkMode:             req.NetworkMode,
		RequiresCompatibilities: req.RequiresCompatibilities,
		ExecutionRoleArn:        req.ExecutionRoleArn,
		TaskRoleArn:             req.TaskRoleArn,
		Volumes:                 req.Volumes,
		Tags:                    req.Tags,
		Status:                  "ACTIVE",
	}

	key := fmt.Sprintf("%s:%d", req.Family, revision)
	ecsTaskDefinitions.Put(key, td)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"taskDefinition": td,
	})
}

func handleECSDeregisterTaskDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskDefinition string `json:"taskDefinition"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.TaskDefinition == "" {
		sim.AWSError(w, "InvalidParameterException", "taskDefinition is required", http.StatusBadRequest)
		return
	}

	// Extract family:revision from ARN or direct reference
	key := req.TaskDefinition
	if strings.HasPrefix(key, "arn:") {
		parts := strings.Split(key, "/")
		if len(parts) > 1 {
			key = parts[len(parts)-1]
		}
	}

	found := ecsTaskDefinitions.Update(key, func(td *ECSTaskDefinition) {
		td.Status = "INACTIVE"
	})

	if !found {
		sim.AWSErrorf(w, "ClientException", http.StatusBadRequest,
			"Unable to describe task definition: %s", req.TaskDefinition)
		return
	}

	td, _ := ecsTaskDefinitions.Get(key)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"taskDefinition": td,
	})
}

func handleECSDescribeTaskDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskDefinition string `json:"taskDefinition"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.TaskDefinition == "" {
		sim.AWSError(w, "InvalidParameterException", "taskDefinition is required", http.StatusBadRequest)
		return
	}

	key := req.TaskDefinition
	if strings.HasPrefix(key, "arn:") {
		parts := strings.Split(key, "/")
		if len(parts) > 1 {
			key = parts[len(parts)-1]
		}
	}

	// If no revision specified, find the latest active one
	if !strings.Contains(key, ":") {
		ecsRevisionMu.Lock()
		rev, exists := ecsRevisions[key]
		ecsRevisionMu.Unlock()
		if exists {
			key = fmt.Sprintf("%s:%d", key, rev)
		}
	}

	td, ok := ecsTaskDefinitions.Get(key)
	if !ok {
		sim.AWSErrorf(w, "ClientException", http.StatusBadRequest,
			"Unable to describe task definition: %s", req.TaskDefinition)
		return
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"taskDefinition": td,
	})
}

func handleECSRunTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster              string   `json:"cluster"`
		TaskDefinition       string   `json:"taskDefinition"`
		Count                int      `json:"count"`
		LaunchType           string   `json:"launchType"`
		Group                string   `json:"group"`
		Tags                 []ECSTag `json:"tags,omitempty"`
		PropagateTags        string   `json:"propagateTags,omitempty"`
		EnableExecuteCommand bool     `json:"enableExecuteCommand,omitempty"`
		NetworkConfiguration *struct {
			AwsvpcConfiguration *struct {
				Subnets        []string `json:"subnets"`
				SecurityGroups []string `json:"securityGroups"`
				AssignPublicIp string   `json:"assignPublicIp"`
			} `json:"awsvpcConfiguration"`
		} `json:"networkConfiguration"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.TaskDefinition == "" {
		sim.AWSError(w, "InvalidParameterException", "taskDefinition is required", http.StatusBadRequest)
		return
	}
	if req.Count == 0 {
		req.Count = 1
	}
	if req.Cluster == "" {
		req.Cluster = "default"
	}

	// Resolve cluster name
	clusterName := req.Cluster
	if strings.HasPrefix(clusterName, "arn:") {
		parts := strings.Split(clusterName, "/")
		if len(parts) > 1 {
			clusterName = parts[len(parts)-1]
		}
	}

	cluster, ok := ecsClusters.Get(clusterName)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest,
			"Cluster not found: %s", req.Cluster)
		return
	}

	// Resolve task definition
	tdKey := req.TaskDefinition
	if strings.HasPrefix(tdKey, "arn:") {
		parts := strings.Split(tdKey, "/")
		if len(parts) > 1 {
			tdKey = parts[len(parts)-1]
		}
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

	// Validate security groups exist
	if req.NetworkConfiguration != nil && req.NetworkConfiguration.AwsvpcConfiguration != nil {
		for _, sgID := range req.NetworkConfiguration.AwsvpcConfiguration.SecurityGroups {
			if _, sgOK := ec2SecurityGroups.Get(sgID); !sgOK {
				sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
					"The security group '%s' does not exist", sgID)
				return
			}
		}
	}

	// Real ECS validates the subnet exists in EC2 and uses its CIDR for
	// task IP assignment. Pull the requested subnet up front; surface a
	// clean InvalidParameterException when the caller passes one we
	// don't know about (matches real AWS InvalidSubnetID.NotFound).
	var requestedSubnet string
	if req.NetworkConfiguration != nil && req.NetworkConfiguration.AwsvpcConfiguration != nil &&
		len(req.NetworkConfiguration.AwsvpcConfiguration.Subnets) > 0 {
		requestedSubnet = req.NetworkConfiguration.AwsvpcConfiguration.Subnets[0]
	}

	var tasks []ECSTask
	for i := 0; i < req.Count; i++ {
		_ = i
		taskID := generateUUID()
		taskArn := fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task/%s/%s", clusterName, taskID)

		eniID := generateUUID()
		var privateIP, subnetID string
		if requestedSubnet != "" {
			ip, ipErr := AllocateSubnetIP(requestedSubnet)
			if ipErr != nil {
				sim.AWSError(w, "InvalidParameterException", ipErr.Error(), http.StatusBadRequest)
				return
			}
			privateIP = ip
			subnetID = requestedSubnet
		}
		createdAt := float64(time.Now().Unix())

		var containers []ECSTaskContainer
		for _, cd := range td.ContainerDefinitions {
			containers = append(containers, ECSTaskContainer{
				ContainerArn: fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:container/%s", generateUUID()),
				Name:         cd.Name,
				LastStatus:   "PROVISIONING",
				NetworkInterfaces: []ECSNetworkInterface{
					{
						AttachmentId:       eniID,
						PrivateIpv4Address: privateIP,
					},
				},
			})
		}

		// Merge tags: request tags take priority, then inherited from task def
		var taskTags []ECSTag
		if req.PropagateTags == "TASK_DEFINITION" && len(td.Tags) > 0 {
			taskTags = append(taskTags, td.Tags...)
		}
		taskTags = append(taskTags, req.Tags...)

		attachmentDetails := []ECSKeyValuePair{
			{Name: "privateIPv4Address", Value: privateIP},
		}
		if subnetID != "" {
			attachmentDetails = append([]ECSKeyValuePair{{Name: "subnetId", Value: subnetID}}, attachmentDetails...)
		}

		task := ECSTask{
			TaskArn:              taskArn,
			TaskDefinitionArn:    td.TaskDefinitionArn,
			ClusterArn:           cluster.ClusterArn,
			LastStatus:           "PROVISIONING",
			DesiredStatus:        "RUNNING",
			Containers:           containers,
			CreatedAt:            &createdAt,
			Tags:                 taskTags,
			LaunchType:           req.LaunchType,
			Cpu:                  td.Cpu,
			Memory:               td.Memory,
			Group:                req.Group,
			EnableExecuteCommand: req.EnableExecuteCommand,
			Attachments: []ECSAttachment{
				{
					Id:      eniID,
					Type:    "ElasticNetworkInterface",
					Status:  "ATTACHING",
					Details: attachmentDetails,
				},
			},
		}

		// Store VPC network configuration from request
		if req.NetworkConfiguration != nil && req.NetworkConfiguration.AwsvpcConfiguration != nil {
			vpc := req.NetworkConfiguration.AwsvpcConfiguration
			task.NetworkConfiguration = &ECSTaskNetworkConfig{
				AwsvpcConfiguration: &ECSTaskVpcConfig{
					Subnets:        vpc.Subnets,
					SecurityGroups: vpc.SecurityGroups,
					AssignPublicIp: vpc.AssignPublicIp,
				},
			}
		}

		ecsTasks.Put(taskID, task)
		tasks = append(tasks, task)

		// Simulate async transition: PROVISIONING → PENDING → RUNNING
		go func(id string, td ECSTaskDefinition, taskTags []ECSTag) {
			// PROVISIONING → PENDING
			time.Sleep(100 * time.Millisecond)
			ecsTasks.Update(id, func(t *ECSTask) {
				t.LastStatus = "PENDING"
				for j := range t.Containers {
					t.Containers[j].LastStatus = "PENDING"
				}
			})

			// PENDING → RUNNING
			time.Sleep(400 * time.Millisecond)

			// Extract image, entrypoint, command, and env from first container definition
			var imageURI string
			var entrypoint, args []string
			var cmdEnv map[string]string
			if len(td.ContainerDefinitions) > 0 {
				cd := td.ContainerDefinitions[0]
				imageURI = cd.Image
				entrypoint = cd.EntryPoint
				args = cd.Command
				if len(cd.Environment) > 0 {
					cmdEnv = make(map[string]string, len(cd.Environment))
					for _, ev := range cd.Environment {
						cmdEnv[ev.Name] = ev.Value
					}
				}
			}

			// Mark task as RUNNING before starting containers
			now := time.Now().Unix()
			ecsTasks.Update(id, func(t *ECSTask) {
				t.LastStatus = "RUNNING"
				t.Connectivity = "CONNECTED"
				t.StartedAt = &now
				for j := range t.Containers {
					t.Containers[j].LastStatus = "RUNNING"
				}
				for j := range t.Attachments {
					t.Attachments[j].Status = "ATTACHED"
				}
			})

			// Inject CloudWatch logs for containers with awslogs log driver,
			// and pick a sink for the real container we start below.
			var sink sim.LogSink = discardLogSink{}
			for _, cd := range td.ContainerDefinitions {
				if cd.LogConfiguration == nil || cd.LogConfiguration.LogDriver != "awslogs" {
					continue
				}
				logGroup := cd.LogConfiguration.Options["awslogs-group"]
				streamPrefix := cd.LogConfiguration.Options["awslogs-stream-prefix"]
				if logGroup == "" || streamPrefix == "" {
					continue
				}
				logStreamName := fmt.Sprintf("%s/%s/%s", streamPrefix, cd.Name, id)
				nowMs := time.Now().UnixMilli()

				// Create log group if not exists
				if _, exists := cwLogGroups.Get(logGroup); !exists {
					cwLogGroups.Put(logGroup, CWLogGroup{
						LogGroupName: logGroup,
						Arn:          cwLogGroupArn(logGroup),
						CreationTime: nowMs,
					})
				}

				// Create log stream
				key := cwEventsKey(logGroup, logStreamName)
				cwLogStreams.Put(key, CWLogStream{
					LogStreamName:       logStreamName,
					LogGroupName:        logGroup,
					CreationTime:        nowMs,
					FirstEventTimestamp: nowMs,
					LastEventTimestamp:  nowMs,
					Arn:                 cwLogStreamArn(logGroup, logStreamName),
					UploadSequenceToken: "1",
				})

				// Insert initial log event
				cmdDesc := strings.Join(append(entrypoint, args...), " ")
				if cmdDesc == "" {
					cmdDesc = "container started"
				}
				cwLogEvents.Put(key, []CWLogEvent{
					{
						Timestamp:     nowMs,
						Message:       cmdDesc,
						IngestionTime: nowMs,
					},
				})

				sink = &cwLogSink{logGroup: logGroup, logStream: logStreamName}
				break
			}

			// Always start the real container when an image is specified —
			// task lifecycle (RUNNING → STOPPED) depends on handle.Wait()
			// returning, regardless of whether logs are configured.
			if imageURI != "" {
				wantTTY := false
				for _, tag := range taskTags {
					if tag.Key == "sockerless-tty" && tag.Value == "true" {
						wantTTY = true
						break
					}
				}
				// Build bind mounts from task definition volumes + container mount points.
				// For EFS volumes, translate to a real host path backed by the
				// simulator's EFS slice (file system or access point root
				// directory); otherwise fall through to a named Docker volume.
				var binds []string
				volMap := make(map[string]string) // volume name → docker bind source
				for _, v := range td.Volumes {
					if v.EfsVolumeConfiguration != nil {
						cfg := v.EfsVolumeConfiguration
						var host string
						if cfg.AuthorizationConfig != nil && cfg.AuthorizationConfig.AccessPointId != "" {
							host = EFSAccessPointHostDir(cfg.AuthorizationConfig.AccessPointId)
						}
						if host == "" && cfg.FileSystemId != "" {
							host = EFSFileSystemHostDir(cfg.FileSystemId)
							if cfg.RootDirectory != "" && cfg.RootDirectory != "/" {
								host = fmt.Sprintf("%s/%s", host, strings.TrimPrefix(cfg.RootDirectory, "/"))
							}
						}
						if host != "" {
							volMap[v.Name] = host
							continue
						}
					}
					volMap[v.Name] = v.Name // fall back to named Docker volume
				}
				if len(td.ContainerDefinitions) > 0 {
					for _, mp := range td.ContainerDefinitions[0].MountPoints {
						if src, ok := volMap[mp.SourceVolume]; ok {
							bind := src + ":" + mp.ContainerPath
							if mp.ReadOnly {
								bind += ":ro"
							}
							binds = append(binds, bind)
						}
					}
				}

				handle, err := sim.StartContainerSync(sim.ContainerConfig{
					Image:     sim.ResolveLocalImage(imageURI),
					Command:   entrypoint,
					Args:      args,
					Env:       cmdEnv,
					Name:      fmt.Sprintf("sockerless-sim-aws-task-%s", id[:12]),
					Labels:    map[string]string{"sockerless-sim-task": id},
					Tty:       wantTTY,
					OpenStdin: wantTTY,
					Binds:     binds,
				}, sink)
				if err != nil {
					stoppedAt := time.Now().Unix()
					ecsTasks.Update(id, func(t *ECSTask) {
						t.LastStatus = "STOPPED"
						t.DesiredStatus = "STOPPED"
						t.StoppedAt = &stoppedAt
						t.StopCode = "EssentialContainerExited"
						t.StoppedReason = fmt.Sprintf("Container start failed: %v", err)
						exitCode := -1
						for j := range t.Containers {
							t.Containers[j].LastStatus = "STOPPED"
							t.Containers[j].ExitCode = &exitCode
						}
					})
				} else {
					ecsProcessHandles.Store(id, handle)

					go func(taskID string, handle *sim.ContainerHandle) {
						result := handle.Wait()
						ecsProcessHandles.Delete(taskID)
						stoppedAt := time.Now().Unix()
						ecsTasks.Update(taskID, func(t *ECSTask) {
							if t.LastStatus == "STOPPED" {
								return // already stopped
							}
							t.LastStatus = "STOPPED"
							t.DesiredStatus = "STOPPED"
							t.StoppedAt = &stoppedAt
							t.StopCode = "EssentialContainerExited"
							t.StoppedReason = "Essential container in task exited"
							exitCode := result.ExitCode
							for j := range t.Containers {
								t.Containers[j].LastStatus = "STOPPED"
								t.Containers[j].ExitCode = &exitCode
							}
						})
					}(id, handle)
				}
			}

		}(taskID, td, taskTags)
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tasks":    tasks,
		"failures": []any{},
	})
}

func handleECSDescribeTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster string   `json:"cluster"`
		Tasks   []string `json:"tasks"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var tasks []ECSTask
	var failures []map[string]string

	for _, taskRef := range req.Tasks {
		// Extract task ID from ARN
		taskID := taskRef
		if strings.HasPrefix(taskRef, "arn:") {
			parts := strings.Split(taskRef, "/")
			if len(parts) > 0 {
				taskID = parts[len(parts)-1]
			}
		}

		task, ok := ecsTasks.Get(taskID)
		if ok {
			tasks = append(tasks, task)
		} else {
			failures = append(failures, map[string]string{
				"arn":    taskRef,
				"reason": "MISSING",
			})
		}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tasks":    tasks,
		"failures": failures,
	})
}

func handleECSStopTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster string `json:"cluster"`
		Task    string `json:"task"`
		Reason  string `json:"reason"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Task == "" {
		sim.AWSError(w, "InvalidParameterException", "task is required", http.StatusBadRequest)
		return
	}

	taskID := req.Task
	if strings.HasPrefix(taskID, "arn:") {
		parts := strings.Split(taskID, "/")
		if len(parts) > 0 {
			taskID = parts[len(parts)-1]
		}
	}

	// Stop running container if any
	if v, ok := ecsProcessHandles.LoadAndDelete(taskID); ok {
		handle := v.(*sim.ContainerHandle)
		sim.StopContainer(handle.ContainerID)
	}

	now := time.Now().Unix()
	found := ecsTasks.Update(taskID, func(t *ECSTask) {
		t.DesiredStatus = "STOPPED"
		t.LastStatus = "STOPPED"
		t.StoppedAt = &now
		t.StopCode = "UserInitiated"
		if req.Reason != "" {
			t.StoppedReason = req.Reason
		}
		exitCode := 0
		for j := range t.Containers {
			t.Containers[j].LastStatus = "STOPPED"
			t.Containers[j].ExitCode = &exitCode
		}
	})

	if !found {
		sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
			"Task not found: %s", req.Task)
		return
	}

	task, _ := ecsTasks.Get(taskID)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"task": task,
	})
}

func handleECSListTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster       string `json:"cluster"`
		Family        string `json:"family"`
		DesiredStatus string `json:"desiredStatus"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = "default"
	}
	if strings.HasPrefix(clusterName, "arn:") {
		parts := strings.Split(clusterName, "/")
		if len(parts) > 1 {
			clusterName = parts[len(parts)-1]
		}
	}

	clusterArn := ecsArn("cluster", clusterName)

	tasks := ecsTasks.Filter(func(t ECSTask) bool {
		if t.ClusterArn != clusterArn {
			return false
		}
		if req.Family != "" {
			// Check if task definition family matches
			td, ok := ecsTaskDefinitions.Get(extractTDKey(t.TaskDefinitionArn))
			if !ok || td.Family != req.Family {
				return false
			}
		}
		if req.DesiredStatus != "" && t.DesiredStatus != req.DesiredStatus {
			return false
		}
		return true
	})

	var taskArns []string
	for _, t := range tasks {
		taskArns = append(taskArns, t.TaskArn)
	}
	if taskArns == nil {
		taskArns = []string{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"taskArns": taskArns,
	})
}

func handleECSDeleteCluster(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster string `json:"cluster"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Cluster == "" {
		sim.AWSError(w, "InvalidParameterException", "cluster is required", http.StatusBadRequest)
		return
	}

	name := req.Cluster
	if strings.HasPrefix(name, "arn:") {
		parts := strings.Split(name, "/")
		if len(parts) > 1 {
			name = parts[len(parts)-1]
		}
	}

	cluster, ok := ecsClusters.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest,
			"Cluster not found: %s", req.Cluster)
		return
	}

	cluster.Status = "INACTIVE"
	ecsClusters.Delete(name)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"cluster": cluster,
	})
}

// handleECSTagResource implements `AmazonEC2ContainerServiceV20141113.TagResource`.
// `mergeECSTagsByKey` adds new tags + overwrites existing keys;
// missing tags persist. Real ECS rejects TagResource on STOPPED
// tasks; we mirror that behaviour so the recovery.go "skip STOPPED"
// logic exercises the same gate.
func handleECSTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string   `json:"resourceArn"`
		Tags        []ECSTag `json:"tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceArn == "" {
		sim.AWSError(w, "InvalidParameterException", "resourceArn is required", http.StatusBadRequest)
		return
	}

	// Task ARN: tag the task in-place. Real ECS rejects TagResource
	// on STOPPED tasks with InvalidParameterException; mirror that.
	if strings.Contains(req.ResourceArn, ":task/") {
		parts := strings.Split(req.ResourceArn, "/")
		if len(parts) == 0 {
			sim.AWSError(w, "InvalidParameterException", "malformed task ARN", http.StatusBadRequest)
			return
		}
		taskID := parts[len(parts)-1]
		task, ok := ecsTasks.Get(taskID)
		if !ok {
			sim.AWSError(w, "ClusterNotFoundException", "task not found: "+req.ResourceArn, http.StatusBadRequest)
			return
		}
		if task.LastStatus == "STOPPED" || task.LastStatus == "DEPROVISIONING" {
			sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
				"The specified task is not in a state to be tagged: %s", task.LastStatus)
			return
		}
		task.Tags = mergeECSTagsByKey(task.Tags, req.Tags)
		ecsTasks.Put(taskID, task)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Task-definition ARN: tag the task-def.
	if strings.Contains(req.ResourceArn, ":task-definition/") {
		key := extractTDKey(req.ResourceArn)
		td, ok := ecsTaskDefinitions.Get(key)
		if !ok {
			sim.AWSError(w, "ClientException", "task definition not found", http.StatusBadRequest)
			return
		}
		td.Tags = mergeECSTagsByKey(td.Tags, req.Tags)
		ecsTaskDefinitions.Put(key, td)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Other resource types (cluster, service, container-instance) —
	// not used by sockerless today; surface a clear error rather
	// than silently succeeding (no fakes / no fallbacks).
	sim.AWSError(w, "InvalidParameterException", "tag-target type not implemented in sim: "+req.ResourceArn, http.StatusBadRequest)
}

// handleECSUntagResource implements `AmazonEC2ContainerServiceV20141113.UntagResource`.
// Companion to TagResource; removes the named tags. Same STOPPED-task
// rejection rule.
func handleECSUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string   `json:"resourceArn"`
		TagKeys     []string `json:"tagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceArn == "" || len(req.TagKeys) == 0 {
		sim.AWSError(w, "InvalidParameterException", "resourceArn and tagKeys are required", http.StatusBadRequest)
		return
	}
	keep := func(tags []ECSTag) []ECSTag {
		drop := make(map[string]struct{}, len(req.TagKeys))
		for _, k := range req.TagKeys {
			drop[k] = struct{}{}
		}
		out := tags[:0]
		for _, t := range tags {
			if _, gone := drop[t.Key]; gone {
				continue
			}
			out = append(out, t)
		}
		return out
	}
	if strings.Contains(req.ResourceArn, ":task/") {
		parts := strings.Split(req.ResourceArn, "/")
		taskID := parts[len(parts)-1]
		task, ok := ecsTasks.Get(taskID)
		if !ok {
			sim.AWSError(w, "ClusterNotFoundException", "task not found", http.StatusBadRequest)
			return
		}
		if task.LastStatus == "STOPPED" || task.LastStatus == "DEPROVISIONING" {
			sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
				"The specified task is not in a state to be tagged: %s", task.LastStatus)
			return
		}
		task.Tags = keep(task.Tags)
		ecsTasks.Put(taskID, task)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if strings.Contains(req.ResourceArn, ":task-definition/") {
		key := extractTDKey(req.ResourceArn)
		td, ok := ecsTaskDefinitions.Get(key)
		if !ok {
			sim.AWSError(w, "ClientException", "task definition not found", http.StatusBadRequest)
			return
		}
		td.Tags = keep(td.Tags)
		ecsTaskDefinitions.Put(key, td)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	sim.AWSError(w, "InvalidParameterException", "untag-target type not implemented in sim: "+req.ResourceArn, http.StatusBadRequest)
}

// mergeECSTagsByKey combines `existing` with `incoming`: any key
// present in both is overwritten by the `incoming` value (matching
// real ECS TagResource semantics — "If existing tags on a resource
// are not specified in the request parameters, they aren't changed").
func mergeECSTagsByKey(existing, incoming []ECSTag) []ECSTag {
	byKey := make(map[string]ECSTag, len(existing)+len(incoming))
	for _, t := range existing {
		byKey[t.Key] = t
	}
	for _, t := range incoming {
		byKey[t.Key] = t
	}
	out := make([]ECSTag, 0, len(byKey))
	for _, t := range byKey {
		out = append(out, t)
	}
	return out
}

func handleECSListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string `json:"resourceArn"`
	}
	_ = sim.ReadJSON(r, &req)

	var tags []ECSTag

	// Check if it's a task definition ARN
	if strings.Contains(req.ResourceArn, ":task-definition/") {
		key := extractTDKey(req.ResourceArn)
		if td, ok := ecsTaskDefinitions.Get(key); ok {
			tags = td.Tags
		}
	}

	// Check if it's a task ARN
	if strings.Contains(req.ResourceArn, ":task/") {
		parts := strings.Split(req.ResourceArn, "/")
		if len(parts) > 0 {
			taskID := parts[len(parts)-1]
			if task, ok := ecsTasks.Get(taskID); ok {
				tags = task.Tags
			}
		}
	}

	if tags == nil {
		tags = []ECSTag{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tags": tags,
	})
}

// ecsExecSessions tracks active ECS exec sessions for WebSocket handlers.
var ecsExecSessions sync.Map // map[sessionID]ecsExecSession

type ecsExecSession struct {
	taskID            string
	command           string
	dockerContainerID string
}

// ssmStreamWriter wraps chunks in an SSM output_stream_data AgentMessage
// frame before sending over the WebSocket. The backend's decoder
// parses these frames to reconstruct the Docker-mux'd stream;
// sending raw bytes silently produces empty exec output.
type ssmStreamWriter struct {
	conn        *websocket.Conn
	payloadType uint32 // 1 = stdout, 11 = stderr
	mu          *sync.Mutex
}

func (w *ssmStreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	frame := buildSSMOutputFrame(w.payloadType, p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return 0, err
	}
	return len(p), nil
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleECSExecuteCommand returns a handler that implements ECS ExecuteCommand.
// It creates a session and registers a WebSocket handler for command execution.
func handleECSExecuteCommand(srv *sim.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Cluster     string `json:"cluster"`
			Task        string `json:"task"`
			Command     string `json:"command"`
			Interactive bool   `json:"interactive"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.Task == "" {
			sim.AWSError(w, "InvalidParameterException", "task is required", http.StatusBadRequest)
			return
		}
		if req.Command == "" {
			sim.AWSError(w, "InvalidParameterException", "command is required", http.StatusBadRequest)
			return
		}

		// Extract task ID from ARN
		taskID := req.Task
		if strings.HasPrefix(taskID, "arn:") {
			parts := strings.Split(taskID, "/")
			if len(parts) > 0 {
				taskID = parts[len(parts)-1]
			}
		}

		// Verify task exists and is RUNNING
		task, ok := ecsTasks.Get(taskID)
		if !ok {
			sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
				"Task not found: %s", req.Task)
			return
		}
		if task.LastStatus != "RUNNING" {
			sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
				"Execute command is not supported on task in %s status", task.LastStatus)
			return
		}

		sessionID := generateUUID()

		// Store the session
		// Look up the Docker container ID for this task (may need to wait briefly
		// for the container to start — it starts async after RUNNING transition)
		var dockerContainerID string
		for i := 0; i < 20; i++ {
			if v, ok := ecsProcessHandles.Load(taskID); ok {
				handle := v.(*sim.ContainerHandle)
				dockerContainerID = handle.ContainerID
				break
			}
			time.Sleep(250 * time.Millisecond)
		}

		ecsExecSessions.Store(sessionID, ecsExecSession{
			taskID:            taskID,
			command:           req.Command,
			dockerContainerID: dockerContainerID,
		})

		// Determine host from the incoming request
		host := r.Host
		if host == "" {
			host = "localhost:4566"
		}
		streamURL := fmt.Sprintf("ws://%s/ecs-exec/%s", host, sessionID)

		// WebSocket endpoint is registered statically as /ecs-exec/{sessionId}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"session": map[string]any{
				"sessionId":  sessionID,
				"streamUrl":  streamURL,
				"tokenValue": "token-" + sessionID[:8],
			},
		})
	}
}

// handleECSExecWebSocket returns a handler for an ECS exec WebSocket session.
// It upgrades the connection and bridges stdin/stdout/stderr of the command.
func handleECSExecWebSocket(sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessVal, ok := ecsExecSessions.LoadAndDelete(sessionID)
		if !ok {
			http.Error(w, "session not found or already used", http.StatusNotFound)
			return
		}
		sess := sessVal.(ecsExecSession)

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck

		// Execute command inside the real Docker container
		if sess.dockerContainerID != "" {
			cli := sim.DockerClient()
			if cli != nil {
				// Always wrap the entire received string as a shell script.
				// Backends now wrap commands in `sh -c '<script>'` before
				// sending to ECS.ExecuteCommand (real AWS exec()s argv[0]
				// and rejects shell builtins / pipes / env-expansion). The
				// previous "unwrap if it starts with sh -c " path stripped
				// `-c ` then handed the remaining bytes to Docker exec
				// verbatim, which left the surrounding single quotes
				// intact — `'echo …'` was then exec()'d as a single
				// command name and produced "sh: 'echo …': not found".
				// Treat the whole received string as one shell script
				// regardless of whether the backend already wrapped it;
				// double-wrapping is correct (the inner shell parses the
				// outer script and dispatches the inner shell itself).
				execCmd := []string{"sh", "-c", sess.command}
				execCfg := dockercontainer.ExecOptions{
					Cmd:          execCmd,
					AttachStdin:  true,
					AttachStdout: true,
					AttachStderr: true,
				}
				execResp, err := cli.ContainerExecCreate(r.Context(), sess.dockerContainerID, execCfg)
				if err != nil {
					_ = conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
					return
				}
				attach, err := cli.ContainerExecAttach(r.Context(), execResp.ID, dockercontainer.ExecAttachOptions{})
				if err != nil {
					_ = conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
					return
				}
				defer attach.Close()

				// Bridge: WebSocket → Docker exec stdin. The backend wraps
				// stdin in SSM `input_stream_data` AgentMessage frames; real
				// ssm-agent decodes the frame, forwards only the payload to
				// the user process, and closes the user's stdin when the
				// frame's FIN flag is set so readers like `cat`, `tar`, and
				// `gzip` see EOF. Match that contract.
				go func() {
					defer attach.CloseWrite() //nolint:errcheck
					for {
						_, msg, rerr := conn.ReadMessage()
						if rerr != nil {
							return
						}
						payload, mt, fin, perr := decodeSSMInputFrame(msg)
						if perr != nil {
							// Not a parseable SSM frame — skip silently.
							// Real ssm-agent ignores unrecognized frames.
							continue
						}
						if mt != ssmMTInputStreamData {
							continue
						}
						if len(payload) > 0 {
							if _, werr := attach.Conn.Write(payload); werr != nil {
								return
							}
						}
						if fin {
							return
						}
					}
				}()

				// Bridge: Docker exec → WebSocket wrapped in SSM
				// AgentMessage frames. The backend's SSM decoder
				// (backends/ecs/exec_cloud.go, will only see
				// output if each chunk arrives as a proper
				// output_stream_data frame.
				writeMu := &sync.Mutex{}
				stdoutWriter := &ssmStreamWriter{conn: conn, payloadType: ssmPayloadStdout, mu: writeMu}
				stderrWriter := &ssmStreamWriter{conn: conn, payloadType: ssmPayloadStderr, mu: writeMu}
				_, _ = stdcopy.StdCopy(stdoutWriter, stderrWriter, attach.Reader)

				// Real AWS Session Manager sends an output_stream_data
				// frame with PayloadType=12 carrying the exec process's
				// exit code before the channel is closed. Match that so
				// the backend decoder sees the true exit status.
				exitCode := 0
				if inspect, err := cli.ContainerExecInspect(r.Context(), execResp.ID); err == nil {
					exitCode = inspect.ExitCode
				}
				writeMu.Lock()
				_ = conn.WriteMessage(websocket.BinaryMessage,
					buildSSMOutputFrame(ssmPayloadExitCode, []byte(strconv.Itoa(exitCode))))
				// Then signal channel close so the decoder unwinds cleanly.
				_ = conn.WriteMessage(websocket.BinaryMessage, buildSSMChannelClosed())
				writeMu.Unlock()

				_ = conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(5*time.Second),
				)
				return
			}
		}

		// Fallback: local process (only if no Docker container — should not happen)
		var cmd *exec.Cmd
		if strings.Contains(sess.command, " ") {
			cmd = exec.Command("sh", "-c", sess.command)
		} else {
			cmd = exec.Command(sess.command)
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
			return
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
			return
		}

		if err := cmd.Start(); err != nil {
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
			return
		}

		// Bridge stdout → WebSocket
		done := make(chan struct{}, 2)
		writeMu := sync.Mutex{}

		sendWS := func(data []byte) {
			writeMu.Lock()
			defer writeMu.Unlock()
			_ = conn.WriteMessage(websocket.BinaryMessage, data)
		}

		pipeToWS := func(reader io.Reader) {
			defer func() { done <- struct{}{} }()
			buf := make([]byte, 4096)
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					sendWS(buf[:n])
				}
				if err != nil {
					return
				}
			}
		}

		go pipeToWS(stdout)
		go pipeToWS(stderr)

		// Bridge WebSocket → stdin
		go func() {
			defer stdin.Close()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if _, err := stdin.Write(msg); err != nil {
					return
				}
			}
		}()

		// Wait for stdout and stderr to drain
		<-done
		<-done

		// Wait for process to finish
		_ = cmd.Wait()

		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
}

// extractTDKey extracts "family:revision" from a task definition ARN.
func extractTDKey(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return arn
}

// discardLogSink drops log lines. Used when a task definition has no
// awslogs configuration — the container still runs (so task lifecycle
// transitions to STOPPED) but its stdout/stderr aren't captured.
type discardLogSink struct{}

func (discardLogSink) WriteLog(sim.LogLine) {}

// cwLogSink implements sim.LogSink and writes log lines to CloudWatch.
type cwLogSink struct {
	logGroup  string
	logStream string
}

func (s *cwLogSink) WriteLog(line sim.LogLine) {
	key := cwEventsKey(s.logGroup, s.logStream)
	nowMs := time.Now().UnixMilli()
	cwLogEvents.Update(key, func(events *[]CWLogEvent) {
		*events = append(*events, CWLogEvent{
			Timestamp:     nowMs,
			Message:       line.Text,
			IngestionTime: nowMs,
		})
	})
}

// Fargate CPU/memory validation. Valid combinations per AWS docs.
// Lower tiers (256, 512) have explicit valid values; higher tiers use ranges.
type fargateCombo struct {
	cpu        int
	memOptions []int // explicit valid values (nil = use range)
	memMin     int
	memMax     int
	memInc     int
}

var fargateCombos = []fargateCombo{
	{256, []int{512, 1024, 2048}, 0, 0, 0},
	{512, []int{1024, 2048, 3072, 4096}, 0, 0, 0},
	{1024, nil, 2048, 8192, 1024},
	{2048, nil, 4096, 16384, 1024},
	{4096, nil, 8192, 30720, 1024},
	{8192, nil, 16384, 61440, 4096},
	{16384, nil, 32768, 122880, 8192},
}

func hasFargate(compatibilities []string) bool {
	for _, c := range compatibilities {
		if strings.EqualFold(c, "FARGATE") {
			return true
		}
	}
	return false
}

func validateFargateResources(cpuStr, memStr string) error {
	cpu, err := strconv.Atoi(cpuStr)
	if err != nil {
		return fmt.Errorf("invalid cpu value: %s", cpuStr)
	}
	mem, err := strconv.Atoi(memStr)
	if err != nil {
		return fmt.Errorf("invalid memory value: %s", memStr)
	}

	for _, combo := range fargateCombos {
		if combo.cpu != cpu {
			continue
		}
		if len(combo.memOptions) > 0 {
			for _, opt := range combo.memOptions {
				if opt == mem {
					return nil
				}
			}
			return fmt.Errorf("invalid memory value %d for cpu %d, valid values: %v", mem, cpu, combo.memOptions)
		}
		if mem >= combo.memMin && mem <= combo.memMax && (mem-combo.memMin)%combo.memInc == 0 {
			return nil
		}
		return fmt.Errorf("invalid memory value %d for cpu %d, valid range: %d-%d in %d increments",
			mem, cpu, combo.memMin, combo.memMax, combo.memInc)
	}
	return fmt.Errorf("invalid cpu value %d, valid values: 256, 512, 1024, 2048, 4096, 8192, 16384", cpu)
}
