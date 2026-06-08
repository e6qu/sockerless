package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	ClusterArn                        string          `json:"clusterArn"`
	ClusterName                       string          `json:"clusterName"`
	Status                            string          `json:"status"`
	RunningTasksCount                 int             `json:"runningTasksCount"`
	PendingTasksCount                 int             `json:"pendingTasksCount"`
	ActiveServicesCount               int             `json:"activeServicesCount"`
	RegisteredContainerInstancesCount int             `json:"registeredContainerInstancesCount"`
	CapacityProviders                 []string        `json:"capacityProviders,omitempty"`
	DefaultCapacityProviderStrategy   json.RawMessage `json:"defaultCapacityProviderStrategy,omitempty"`
	Tags                              []ECSTag        `json:"tags,omitempty"`
	// Settings (containerInsights) and Configuration (executeCommandConfiguration)
	// are stored raw so they round-trip exactly; DescribeClusters only surfaces
	// them when SETTINGS / CONFIGURATIONS is in the `include` list.
	Settings               json.RawMessage `json:"settings,omitempty"`
	Configuration          json.RawMessage `json:"configuration,omitempty"`
	ServiceConnectDefaults json.RawMessage `json:"serviceConnectDefaults,omitempty"`
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
	// healthCheck and secrets are decoded for the runtime (secret injection reads
	// Secrets); every other field rides the verbatim `raw` capture below.
	HealthCheck json.RawMessage `json:"healthCheck,omitempty"`
	Secrets     json.RawMessage `json:"secrets,omitempty"`

	// raw holds the exact bytes the client registered. The provider folds the
	// whole containerDefinitions JSON into a ForceNew hash, so dropping ANY
	// registered field (ulimits, dependsOn, linuxParameters, dockerLabels, user,
	// workingDirectory, privileged, stop/startTimeout, systemControls, …) forces
	// a new revision every plan. Echoing the captured bytes round-trips every
	// field faithfully while the typed fields above stay available to the runtime.
	raw json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the typed fields the runtime needs and captures the
// verbatim bytes so DescribeTaskDefinition can echo every registered field.
func (c *ECSContainerDefinition) UnmarshalJSON(data []byte) error {
	type alias ECSContainerDefinition
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*c = ECSContainerDefinition(a)
	c.raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON re-emits the captured bytes verbatim when present, so no field is
// silently dropped on read-back. Containers built in-process (no capture) fall
// back to the typed encoding.
func (c ECSContainerDefinition) MarshalJSON() ([]byte, error) {
	if len(c.raw) > 0 {
		return c.raw, nil
	}
	type alias ECSContainerDefinition
	return json.Marshal(alias(c))
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
	ConfiguredAtLaunch     bool                `json:"configuredAtLaunch,omitempty"`
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

type ECSTaskVolumeConfiguration struct {
	Name             string                                `json:"name"`
	ManagedEBSVolume *ECSTaskManagedEBSVolumeConfiguration `json:"managedEBSVolume,omitempty"`
}

type ECSTaskManagedEBSVolumeConfiguration struct {
	Encrypted         bool                                `json:"encrypted,omitempty"`
	KmsKeyId          string                              `json:"kmsKeyId,omitempty"`
	VolumeType        string                              `json:"volumeType,omitempty"`
	SizeInGiB         int                                 `json:"sizeInGiB,omitempty"`
	SnapshotId        string                              `json:"snapshotId,omitempty"`
	RoleArn           string                              `json:"roleArn,omitempty"`
	TerminationPolicy *ECSTaskManagedEBSTerminationPolicy `json:"terminationPolicy,omitempty"`
	TagSpecifications []ECSTaskManagedEBSTagSpecification `json:"tagSpecifications,omitempty"`
}

type ECSTaskManagedEBSTerminationPolicy struct {
	DeleteOnTermination *bool `json:"deleteOnTermination,omitempty"`
}

type ECSTaskManagedEBSTagSpecification struct {
	ResourceType string   `json:"resourceType,omitempty"`
	Tags         []ECSTag `json:"tags,omitempty"`
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
	// Top-level knobs the provider reads back (all ForceNew); each was dropped on
	// register, so aws_ecs_task_definition.{runtime_platform,ephemeral_storage,
	// proxy_configuration,pid_mode,ipc_mode,placement_constraints,…} drifted into
	// a new revision every plan. Nested objects ride RawMessage (verbatim).
	RuntimePlatform       json.RawMessage `json:"runtimePlatform,omitempty"`
	EphemeralStorage      json.RawMessage `json:"ephemeralStorage,omitempty"`
	ProxyConfiguration    json.RawMessage `json:"proxyConfiguration,omitempty"`
	PlacementConstraints  json.RawMessage `json:"placementConstraints,omitempty"`
	InferenceAccelerators json.RawMessage `json:"inferenceAccelerators,omitempty"`
	PidMode               string          `json:"pidMode,omitempty"`
	IpcMode               string          `json:"ipcMode,omitempty"`
	EnableFaultInjection  *bool           `json:"enableFaultInjection,omitempty"`
	// Compatibilities is the AWS-computed launch-type list (distinct from the
	// requiresCompatibilities input). requiresAttributes is intentionally NOT
	// modelled: it requires AWS's capability-attribute engine, no stable client
	// reads it, and fabricating a list would be a fake.
	Compatibilities []string `json:"compatibilities,omitempty"`
	// Tags are internal-only: real AWS does not carry them inside the
	// taskDefinition object — they surface at the response top level (from
	// RegisterTaskDefinition always, DescribeTaskDefinition only with
	// include=TAGS). Serializing them here would be silently dropped by the SDK
	// model, so the provider would still see no tags. See ecsTaskDefTagsResponse.
	Tags   []ECSTag `json:"-"`
	Status string   `json:"status"`
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
	ecsProcessHandles  sync.Map       // map[taskID]*ecsTaskProcesses
)

type ecsTaskProcesses struct {
	MainContainerName string
	Handles           map[string]*sim.ContainerHandle
}

func (p *ecsTaskProcesses) firstHandle() *sim.ContainerHandle {
	if p == nil {
		return nil
	}
	if p.MainContainerName != "" {
		if h := p.Handles[p.MainContainerName]; h != nil {
			return h
		}
	}
	for _, h := range p.Handles {
		return h
	}
	return nil
}

func (p *ecsTaskProcesses) handleFor(containerName string) *sim.ContainerHandle {
	if p == nil {
		return nil
	}
	if containerName != "" {
		return p.Handles[containerName]
	}
	return p.firstHandle()
}

func stopECSTaskProcesses(p *ecsTaskProcesses) {
	if p == nil {
		return
	}
	for _, h := range p.Handles {
		if h != nil {
			sim.StopContainer(h.ContainerID)
		}
	}
}

func cleanupECSTaskProcesses(taskID string, p *ecsTaskProcesses) {
	stopECSTaskProcesses(p)
	ec2DetachRealECSTaskNIC(context.Background(), taskID)
}

func requestStopECSTaskProcesses(p *ecsTaskProcesses) {
	if p == nil {
		return
	}
	for _, h := range p.Handles {
		if h != nil {
			go sim.StopContainer(h.ContainerID)
		}
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func ecsArn(resourceType, id string) string {
	return fmt.Sprintf("arn:aws:ecs:"+awsRegion()+":"+awsAccountID()+":%s/%s", resourceType, id)
}

func registerECS(r *sim.AWSRouter, srv *sim.Server) {
	ecsClusters = sim.MakeStore[ECSCluster](srv.DB(), "ecs_clusters")
	ecsTaskDefinitions = sim.MakeStore[ECSTaskDefinition](srv.DB(), "ecs_task_definitions")
	ecsTasks = sim.MakeStore[ECSTask](srv.DB(), "ecs_tasks")
	ecsRevisions = make(map[string]int)

	r.Register("AmazonEC2ContainerServiceV20141113.CreateCluster", handleECSCreateCluster)
	r.Register("AmazonEC2ContainerServiceV20141113.DescribeClusters", handleECSDescribeClusters)
	r.Register("AmazonEC2ContainerServiceV20141113.UpdateCluster", handleECSUpdateCluster)
	r.Register("AmazonEC2ContainerServiceV20141113.UpdateClusterSettings", handleECSUpdateClusterSettings)
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

	// ECS Service family + cluster capacity providers (aws_ecs_service /
	// aws_ecs_cluster_capacity_providers).
	registerECSServices(r, srv)

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
				handle = v.(*ecsTaskProcesses).firstHandle()
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
		ClusterName                     string          `json:"clusterName"`
		Tags                            []ECSTag        `json:"tags"`
		CapacityProviders               []string        `json:"capacityProviders"`
		DefaultCapacityProviderStrategy json.RawMessage `json:"defaultCapacityProviderStrategy"`
		Settings                        json.RawMessage `json:"settings"`
		Configuration                   json.RawMessage `json:"configuration"`
		ServiceConnectDefaults          json.RawMessage `json:"serviceConnectDefaults"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ClusterName == "" {
		req.ClusterName = "default"
	}

	cluster := ECSCluster{
		ClusterArn:                      ecsArn("cluster", req.ClusterName),
		ClusterName:                     req.ClusterName,
		Status:                          "ACTIVE",
		Tags:                            req.Tags,
		CapacityProviders:               req.CapacityProviders,
		DefaultCapacityProviderStrategy: req.DefaultCapacityProviderStrategy,
		Settings:                        req.Settings,
		Configuration:                   req.Configuration,
		ServiceConnectDefaults:          req.ServiceConnectDefaults,
	}
	ecsClusters.Put(req.ClusterName, cluster)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"cluster": cluster,
	})
}

func handleECSDescribeClusters(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clusters []string `json:"clusters"`
		Include  []string `json:"include"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	includeSettings, includeConfig := false, false
	for _, inc := range req.Include {
		switch inc {
		case "SETTINGS":
			includeSettings = true
		case "CONFIGURATIONS":
			includeConfig = true
		}
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
			activeServices := 0
			for _, s := range ecsServices.List() {
				if s.ClusterArn == cluster.ClusterArn && s.Status == "ACTIVE" {
					activeServices++
				}
			}
			cluster.ActiveServicesCount = activeServices
			// settings / configuration only surface when explicitly included,
			// matching real DescribeClusters.
			if !includeSettings {
				cluster.Settings = nil
			}
			if !includeConfig {
				cluster.Configuration = nil
			}
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

// handleECSUpdateCluster updates a cluster's settings / configuration /
// serviceConnectDefaults in place. Without it, any change to
// aws_ecs_cluster.{setting,configuration,service_connect_defaults} forced
// recreation.
func handleECSUpdateCluster(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster                string          `json:"cluster"`
		Settings               json.RawMessage `json:"settings"`
		Configuration          json.RawMessage `json:"configuration"`
		ServiceConnectDefaults json.RawMessage `json:"serviceConnectDefaults"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name := ecsClusterNameFromRef(req.Cluster)
	cluster, ok := ecsClusters.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest, "Cluster not found: %s", req.Cluster)
		return
	}
	if req.Settings != nil {
		cluster.Settings = req.Settings
	}
	if req.Configuration != nil {
		cluster.Configuration = req.Configuration
	}
	if req.ServiceConnectDefaults != nil {
		cluster.ServiceConnectDefaults = req.ServiceConnectDefaults
	}
	ecsClusters.Put(name, cluster)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"cluster": cluster})
}

// handleECSUpdateClusterSettings updates only the cluster settings
// (containerInsights). The provider uses it for aws_ecs_cluster.setting changes.
func handleECSUpdateClusterSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster  string          `json:"cluster"`
		Settings json.RawMessage `json:"settings"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name := ecsClusterNameFromRef(req.Cluster)
	cluster, ok := ecsClusters.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest, "Cluster not found: %s", req.Cluster)
		return
	}
	if req.Settings != nil {
		cluster.Settings = req.Settings
	}
	ecsClusters.Put(name, cluster)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"cluster": cluster})
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
		RuntimePlatform         json.RawMessage          `json:"runtimePlatform,omitempty"`
		EphemeralStorage        json.RawMessage          `json:"ephemeralStorage,omitempty"`
		ProxyConfiguration      json.RawMessage          `json:"proxyConfiguration,omitempty"`
		PlacementConstraints    json.RawMessage          `json:"placementConstraints,omitempty"`
		InferenceAccelerators   json.RawMessage          `json:"inferenceAccelerators,omitempty"`
		PidMode                 string                   `json:"pidMode,omitempty"`
		IpcMode                 string                   `json:"ipcMode,omitempty"`
		EnableFaultInjection    *bool                    `json:"enableFaultInjection,omitempty"`
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
		TaskDefinitionArn:       fmt.Sprintf("arn:aws:ecs:"+awsRegion()+":"+awsAccountID()+":task-definition/%s:%d", req.Family, revision),
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
		RuntimePlatform:         req.RuntimePlatform,
		EphemeralStorage:        req.EphemeralStorage,
		ProxyConfiguration:      req.ProxyConfiguration,
		PlacementConstraints:    req.PlacementConstraints,
		InferenceAccelerators:   req.InferenceAccelerators,
		PidMode:                 req.PidMode,
		IpcMode:                 req.IpcMode,
		EnableFaultInjection:    req.EnableFaultInjection,
		Compatibilities:         ecsComputeCompatibilities(req.NetworkMode, req.RequiresCompatibilities),
		Tags:                    req.Tags,
		Status:                  "ACTIVE",
	}

	key := fmt.Sprintf("%s:%d", req.Family, revision)
	ecsTaskDefinitions.Put(key, td)

	// Real RegisterTaskDefinition echoes the tags at the response top level.
	resp := map[string]any{"taskDefinition": td}
	if len(td.Tags) > 0 {
		resp["tags"] = td.Tags
	}
	sim.WriteJSON(w, http.StatusOK, resp)
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
		TaskDefinition string   `json:"taskDefinition"`
		Include        []string `json:"include"`
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

	// Tags surface at the response top level only when include=TAGS — this is
	// the path terraform-provider-aws reads task-definition tags on refresh.
	resp := map[string]any{"taskDefinition": td}
	if len(td.Tags) > 0 && ecsIncludeHasTags(req.Include) {
		resp["tags"] = td.Tags
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

// ecsComputeCompatibilities derives the AWS-computed launch-type list. Every
// task definition is EC2-compatible; FARGATE additionally requires the awsvpc
// network mode. requiresCompatibilities is always a subset of the result.
func ecsComputeCompatibilities(networkMode string, requires []string) []string {
	set := map[string]bool{"EC2": true}
	for _, c := range requires {
		set[strings.ToUpper(c)] = true
	}
	if strings.EqualFold(networkMode, "awsvpc") {
		set["FARGATE"] = true
	}
	var out []string
	for _, c := range []string{"EC2", "FARGATE", "EXTERNAL"} {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

// ecsIncludeHasTags reports whether the DescribeTaskDefinition include list
// requests TAGS (case-insensitive, matching the AWS enum).
func ecsIncludeHasTags(include []string) bool {
	for _, v := range include {
		if strings.EqualFold(v, "TAGS") {
			return true
		}
	}
	return false
}

type ecsRequestError struct {
	code    string
	message string
	status  int
}

func ecsConfiguredAtLaunchVolumes(td ECSTaskDefinition) map[string]bool {
	out := map[string]bool{}
	for _, vol := range td.Volumes {
		if vol.ConfiguredAtLaunch {
			out[vol.Name] = true
		}
	}
	return out
}

func ecsManagedEBSTags(specs []ECSTaskManagedEBSTagSpecification) []EC2Tag {
	var tags []EC2Tag
	for _, spec := range specs {
		if spec.ResourceType != "" && spec.ResourceType != "volume" {
			continue
		}
		for _, tag := range spec.Tags {
			tags = append(tags, EC2Tag(tag))
		}
	}
	return tags
}

func ecsTaskDetail(details []ECSKeyValuePair, name string) string {
	for _, detail := range details {
		if detail.Name == name {
			return detail.Value
		}
	}
	return ""
}

func ecsPrepareManagedEBSVolumes(ctx context.Context, td ECSTaskDefinition, configs []ECSTaskVolumeConfiguration, taskID, requestedSubnet string) (map[string]string, []ECSAttachment, *ecsRequestError) {
	allowed := ecsConfiguredAtLaunchVolumes(td)
	hosts := map[string]string{}
	var attachments []ECSAttachment
	if len(configs) == 0 {
		return hosts, attachments, nil
	}

	az := awsAvailabilityZone()
	if requestedSubnet != "" {
		if subnet, ok := ec2Subnets.Get(requestedSubnet); ok && subnet.AvailabilityZone != "" {
			az = subnet.AvailabilityZone
		}
	}

	for _, cfg := range configs {
		if cfg.Name == "" {
			return nil, nil, &ecsRequestError{"InvalidParameterException", "volumeConfigurations.name is required", http.StatusBadRequest}
		}
		if !allowed[cfg.Name] {
			return nil, nil, &ecsRequestError{"ClientException", fmt.Sprintf("Volume %s is not configuredAtLaunch in the task definition", cfg.Name), http.StatusBadRequest}
		}
		managed := cfg.ManagedEBSVolume
		if managed == nil {
			return nil, nil, &ecsRequestError{"ClientException", fmt.Sprintf("Volume %s requires managedEBSVolume", cfg.Name), http.StatusBadRequest}
		}

		size := managed.SizeInGiB
		if size == 0 {
			size = 8
		}
		snapshotID := managed.SnapshotId
		var snapshotDockerVolumeName string
		var snapshotHostPath string
		if snapshotID != "" {
			snap, ok := ec2Snapshots.Get(snapshotID)
			if !ok {
				return nil, nil, &ecsRequestError{"InvalidParameterException", fmt.Sprintf("Snapshot not found: %s", snapshotID), http.StatusBadRequest}
			}
			if snap.State != "completed" {
				return nil, nil, &ecsRequestError{"InvalidParameterException", fmt.Sprintf("Snapshot is not completed: %s", snapshotID), http.StatusBadRequest}
			}
			if size < snap.VolumeSize {
				size = snap.VolumeSize
			}
			snapshotDockerVolumeName = snap.DockerVolumeName
			snapshotHostPath = snap.HostPath
		}
		volumeType := managed.VolumeType
		if volumeType == "" {
			volumeType = "gp3"
		}
		deleteOnTermination := true
		if managed.TerminationPolicy != nil && managed.TerminationPolicy.DeleteOnTermination != nil {
			deleteOnTermination = *managed.TerminationPolicy.DeleteOnTermination
		}

		volumeID := ec2ID("vol")
		now := time.Now().UTC().Format(time.RFC3339)
		vol := EC2Volume{
			VolumeId:         volumeID,
			DockerVolumeName: ebsECSDockerVolumeName(volumeID),
			Size:             size,
			SnapshotId:       snapshotID,
			AvailabilityZone: az,
			State:            "in-use",
			CreateTime:       now,
			VolumeType:       volumeType,
			Encrypted:        managed.Encrypted,
			Tags:             ecsManagedEBSTags(managed.TagSpecifications),
			Attachments: []EC2VolumeAttachment{{
				VolumeId:            volumeID,
				InstanceId:          taskID,
				Device:              cfg.Name,
				State:               "attached",
				AttachTime:          now,
				DeleteOnTermination: deleteOnTermination,
			}},
		}
		// Restore snapshot data into the new Docker named volume when present.
		// Docker auto-creates the destination volume on first container use so no
		// explicit VolumeCreate is needed — the copy container triggers creation.
		if snapshotDockerVolumeName != "" {
			if err := ebsCopyDockerVolumes(ctx, snapshotDockerVolumeName, vol.DockerVolumeName); err != nil {
				return nil, nil, &ecsRequestError{"InternalError", fmt.Sprintf("could not restore managed EBS snapshot data: %v", err), http.StatusInternalServerError}
			}
		} else if snapshotHostPath != "" {
			// Snapshot came from an EC2/Firecracker volume (host-path); fall back to
			// directory copy. Only works in on-host topology where the sim process runs
			// on the same machine as the Docker host.
			if err := ebsPrepareVolumeHostPath(&vol); err != nil {
				return nil, nil, &ecsRequestError{"InternalError", fmt.Sprintf("could not create managed EBS volume data path: %v", err), http.StatusInternalServerError}
			}
			if err := ebsCopyDir(vol.HostPath, snapshotHostPath); err != nil {
				return nil, nil, &ecsRequestError{"InternalError", fmt.Sprintf("could not restore managed EBS snapshot data: %v", err), http.StatusInternalServerError}
			}
			// Use host-path bind-mount for this volume since the data is on-disk.
			ec2Volumes.Put(volumeID, vol)
			hosts[cfg.Name] = vol.HostPath
			attachments = append(attachments, ECSAttachment{
				Id:     "ebs-" + volumeID,
				Type:   "AmazonElasticBlockStorage",
				Status: "ATTACHING",
				Details: []ECSKeyValuePair{
					{Name: "volumeName", Value: cfg.Name},
					{Name: "volumeId", Value: volumeID},
					{Name: "deleteOnTermination", Value: strconv.FormatBool(deleteOnTermination)},
				},
			})
			continue
		}
		ec2Volumes.Put(volumeID, vol)
		hosts[cfg.Name] = vol.DockerVolumeName
		attachments = append(attachments, ECSAttachment{
			Id:     "ebs-" + volumeID,
			Type:   "AmazonElasticBlockStorage",
			Status: "ATTACHING",
			Details: []ECSKeyValuePair{
				{Name: "volumeName", Value: cfg.Name},
				{Name: "volumeId", Value: volumeID},
				{Name: "deleteOnTermination", Value: strconv.FormatBool(deleteOnTermination)},
			},
		})
	}
	return hosts, attachments, nil
}

func ecsCleanupTaskManagedEBS(task *ECSTask) {
	for i := range task.Attachments {
		att := &task.Attachments[i]
		if att.Type != "AmazonElasticBlockStorage" {
			continue
		}
		volumeID := ecsTaskDetail(att.Details, "volumeId")
		if volumeID == "" {
			continue
		}
		deleteOnTermination, _ := strconv.ParseBool(ecsTaskDetail(att.Details, "deleteOnTermination"))
		if deleteOnTermination {
			if vol, ok := ec2Volumes.Get(volumeID); ok {
				if vol.DockerVolumeName != "" {
					ebsRemoveDockerVolume(vol.DockerVolumeName)
				} else {
					_ = os.RemoveAll(vol.HostPath)
				}
			}
			ec2Volumes.Delete(volumeID)
		} else {
			ec2Volumes.Update(volumeID, func(vol *EC2Volume) {
				vol.State = "available"
				vol.Attachments = nil
			})
		}
		att.Status = "DETACHED"
	}
}

func handleECSRunTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster              string                       `json:"cluster"`
		TaskDefinition       string                       `json:"taskDefinition"`
		Count                int                          `json:"count"`
		LaunchType           string                       `json:"launchType"`
		Group                string                       `json:"group"`
		Tags                 []ECSTag                     `json:"tags,omitempty"`
		PropagateTags        string                       `json:"propagateTags,omitempty"`
		EnableExecuteCommand bool                         `json:"enableExecuteCommand,omitempty"`
		VolumeConfigurations []ECSTaskVolumeConfiguration `json:"volumeConfigurations,omitempty"`
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
		taskArn := fmt.Sprintf("arn:aws:ecs:"+awsRegion()+":"+awsAccountID()+":task/%s/%s", clusterName, taskID)

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
				ContainerArn: fmt.Sprintf("arn:aws:ecs:"+awsRegion()+":"+awsAccountID()+":container/%s", generateUUID()),
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

		taskVolumeHosts, ebsAttachments, ebsErr := ecsPrepareManagedEBSVolumes(r.Context(), td, req.VolumeConfigurations, taskID, requestedSubnet)
		if ebsErr != nil {
			sim.AWSError(w, ebsErr.code, ebsErr.message, ebsErr.status)
			return
		}

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
		task.Attachments = append(task.Attachments, ebsAttachments...)

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
		go func(id string, td ECSTaskDefinition, taskTags []ECSTag, taskVolumeHosts map[string]string) {
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
				cmdDesc := strings.Join(append(cd.EntryPoint, cd.Command...), " ")
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

			processes, err := startECSTaskContainers(id, td, taskTags, taskVolumeHosts, sink)
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
					ecsCleanupTaskManagedEBS(t)
				})
			} else if processes != nil {
				ecsProcessHandles.Store(id, processes)

				for name, handle := range processes.Handles {
					go func(taskID, containerName string, handle *sim.ContainerHandle) {
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
							ecsCleanupTaskManagedEBS(t)
						})
						cleanupECSTaskProcesses(taskID, processes)
					}(id, name, handle)
				}
			}

		}(taskID, td, taskTags, taskVolumeHosts)
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tasks":    tasks,
		"failures": []any{},
	})
}

// ecsPauseImage is the image for the netns pause container — a long-lived sleep
// that owns the task's VPC network namespace. Defaults to busybox from ECR
// (always has `sleep`, avoids Docker Hub throttling); override with the env var.
func ecsPauseImage() string {
	if v := os.Getenv("SOCKERLESS_ECS_PAUSE_IMAGE"); v != "" {
		return v
	}
	return "public.ecr.aws/docker/library/busybox:latest"
}

// startECSPauseContainer launches the netns pause container (Fargate-style): a
// long-lived process that holds the task's network namespace so the ENI can be
// plumbed in with no start-race, then shared by every task container.
func startECSPauseContainer(taskID string, td ECSTaskDefinition, sink sim.LogSink) (*sim.ContainerHandle, error) {
	img := sim.ResolveLocalImage(ecsPauseImage())
	platform, err := localImagePlatform(context.Background(), img)
	if err != nil {
		return nil, fmt.Errorf("resolve pause image platform: %w", err)
	}
	return sim.StartContainerSync(sim.ContainerConfig{
		Image:        img,
		Architecture: platform,
		Command:      []string{"sleep"},
		Args:         []string{"2147483647"},
		Name:         fmt.Sprintf("sockerless-sim-aws-task-%s-pause", taskID[:12]),
		Labels: map[string]string{
			"sockerless-sim-task":       taskID,
			"sockerless-sim-task-pause": "true",
		},
		Sandbox: sim.SandboxFargate,
	}, sink)
}

func startECSTaskContainers(taskID string, td ECSTaskDefinition, taskTags []ECSTag, taskVolumeHosts map[string]string, sink sim.LogSink) (*ecsTaskProcesses, error) {
	if len(td.ContainerDefinitions) == 0 {
		return nil, nil
	}

	wantTTY := false
	for _, tag := range taskTags {
		if tag.Key == "sockerless-tty" && tag.Value == "true" {
			wantTTY = true
			break
		}
	}

	volMap := make(map[string]string)
	for _, v := range td.Volumes {
		if host, ok := taskVolumeHosts[v.Name]; ok {
			volMap[v.Name] = host
			continue
		}
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
		volMap[v.Name] = v.Name
	}

	processes := &ecsTaskProcesses{
		MainContainerName: td.ContainerDefinitions[0].Name,
		Handles:           make(map[string]*sim.ContainerHandle, len(td.ContainerDefinitions)),
	}
	// awsvpc networking. netns tier (Linux + CAP_NET_ADMIN): a pause container
	// holds the task's VPC network namespace (a long-lived sleep, so the ENI is
	// plumbed with no start-race), the ENI veth is attached into it, and every
	// task container shares that netns — overlapping VPC CIDRs work natively with
	// the real ENI IP. Otherwise the cross-platform Docker-network tier pins the
	// first container to a per-VPC bridge at the ENI IP.
	eniIP, subnetID, hasENI := ecsTaskENIInfo(taskID)
	netnsTier := ec2ECSRealNetAvailable()
	var sharedNetMode string
	if netnsTier && hasENI {
		pause, perr := startECSPauseContainer(taskID, td, sink)
		if perr != nil {
			return nil, perr
		}
		processes.Handles["__pause__"] = pause
		if derr := sim.DisconnectContainerNetworks(pause.ContainerID); derr != nil {
			cleanupECSTaskProcesses(taskID, processes)
			return nil, fmt.Errorf("disconnect task netns pause from Docker networks: %w", derr)
		}
		pid, perr := sim.ContainerPID(pause.ContainerID)
		if perr != nil {
			cleanupECSTaskProcesses(taskID, processes)
			return nil, fmt.Errorf("task netns pause pid: %w", perr)
		}
		if aerr := ec2AttachRealECSTaskNIC(context.Background(), taskID, subnetID, pid, eniIP); aerr != nil {
			cleanupECSTaskProcesses(taskID, processes)
			return nil, fmt.Errorf("attach task to VPC netns: %w", aerr)
		}
		sharedNetMode = "container:" + pause.ContainerID
	}
	var mainDockerID string

	for i, cd := range td.ContainerDefinitions {
		if cd.Image == "" {
			continue
		}
		cmdEnv := make(map[string]string, len(cd.Environment))
		for _, ev := range cd.Environment {
			cmdEnv[ev.Name] = ev.Value
		}
		// Resolve the container definition's `secrets` (valueFrom →
		// SecretsManager/SSM) at launch and inject them as env vars, exactly as
		// real ECS does — indistinguishable from `environment` to the container.
		for name, val := range resolveECSContainerSecrets(cd.Secrets) {
			cmdEnv[name] = val
		}
		if sharedNetMode != "" {
			cmdEnv = rewriteHostDockerInternalEnv(cmdEnv)
		}
		var binds []string
		for _, mp := range cd.MountPoints {
			if src, ok := volMap[mp.SourceVolume]; ok {
				bind := src + ":" + mp.ContainerPath
				if mp.ReadOnly {
					bind += ":ro"
				}
				binds = append(binds, bind)
			}
		}

		containerName := fmt.Sprintf("sockerless-sim-aws-task-%s", taskID[:12])
		if i > 0 {
			containerName = fmt.Sprintf("%s-%s", containerName, cd.Name)
		}
		localImage := sim.ResolveLocalImage(cd.Image)
		platform, err := localImagePlatform(context.Background(), localImage)
		if err != nil {
			cleanupECSTaskProcesses(taskID, processes)
			return nil, fmt.Errorf("resolve task container %q image platform: %w", cd.Name, err)
		}

		cfg := sim.ContainerConfig{
			Image:        localImage,
			Architecture: platform,
			Command:      cd.EntryPoint,
			Args:         cd.Command,
			Env:          mergeEnv(cmdEnv, hostMetadataEnv(taskID)),
			Name:         containerName,
			Labels: map[string]string{
				"sockerless-sim-task":           taskID,
				"sockerless-sim-task-container": cd.Name,
			},
			Tty:       wantTTY || cd.PseudoTerminal,
			OpenStdin: wantTTY || cd.Interactive,
			Binds:     binds,
			Sandbox:   sim.SandboxFargate,
		}
		switch {
		case sharedNetMode != "":
			// netns tier: share the pause container's ENI netns.
			cfg.NetworkMode = sharedNetMode
		case i == 0:
			cfg.ExtraHosts = hostMetadataExtraHosts()
			if !netnsTier {
				netName, dockerIP, ok, nerr := ecsTaskVPCNetwork(taskID)
				if nerr != nil {
					cleanupECSTaskProcesses(taskID, processes)
					return nil, nerr
				}
				if ok {
					cfg.Network = netName
					cfg.IPAddress = dockerIP
				}
			}
		case mainDockerID != "":
			cfg.NetworkMode = "container:" + mainDockerID
		}

		handle, err := sim.StartContainerSync(cfg, sink)
		if err != nil {
			cleanupECSTaskProcesses(taskID, processes)
			return nil, fmt.Errorf("start task container %q: %w", cd.Name, err)
		}
		if i == 0 {
			mainDockerID = handle.ContainerID
		}
		processes.Handles[cd.Name] = handle
	}

	if len(processes.Handles) == 0 {
		return nil, nil
	}
	return processes, nil
}

// ecsVPCNetworkName is the Docker network backing a VPC.
func ecsVPCNetworkName(vpcID string) string { return "sockerless-sim-vpc-" + vpcID }

// ecsTaskENIInfo reads a task's awsvpc ENI IP + subnet from its attachment.
// Returns ok=false for tasks without an awsvpc ENI.
func ecsTaskENIInfo(taskID string) (eniIP, subnetID string, ok bool) {
	task, found := ecsTasks.Get(taskID)
	if !found {
		return "", "", false
	}
	for _, att := range task.Attachments {
		if att.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, d := range att.Details {
			switch d.Name {
			case "subnetId":
				subnetID = d.Value
			case "privateIPv4Address":
				eniIP = d.Value
			}
		}
	}
	return eniIP, subnetID, eniIP != "" && subnetID != ""
}

// ecsTaskVPCNetwork resolves the VPC Docker network + ENI IP for an awsvpc task,
// ensuring the network exists. Returns ok=false for tasks with no awsvpc ENI
// (bridge/host tasks use default Docker networking). A network-provisioning
// failure (e.g. a CIDR Docker can't allocate) is returned so the launch fails
// loudly rather than running with a silently-wrong address.
func ecsTaskVPCNetwork(taskID string) (networkName, eniIP string, ok bool, err error) {
	task, found := ecsTasks.Get(taskID)
	if !found {
		return "", "", false, nil
	}
	var subnetID string
	for _, att := range task.Attachments {
		if att.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, d := range att.Details {
			switch d.Name {
			case "subnetId":
				subnetID = d.Value
			case "privateIPv4Address":
				eniIP = d.Value
			}
		}
	}
	if subnetID == "" || eniIP == "" {
		return "", "", false, nil
	}
	subnet, ok := ec2Subnets.Get(subnetID)
	if !ok {
		return "", "", false, nil
	}
	vpc, ok := ec2Vpcs.Get(subnet.VpcId)
	if !ok || vpc.CidrBlock == "" {
		return "", "", false, nil
	}
	name := ecsVPCNetworkName(subnet.VpcId)
	if _, nerr := sim.EnsureVPCNetwork(name, vpc.CidrBlock); nerr != nil {
		return "", "", false, fmt.Errorf("provision VPC network for %s: %w", subnet.VpcId, nerr)
	}
	return name, eniIP, true, nil
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
		requestStopECSTaskProcesses(v.(*ecsTaskProcesses))
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
		ecsCleanupTaskManagedEBS(t)
	})

	if !found {
		sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
			"Task not found: %s", req.Task)
		return
	}

	// Tear down the task's VPC veth (netns tier) after cloud-visible state is
	// updated; Docker/netns cleanup can take seconds on CI.
	go ec2DetachRealECSTaskNIC(context.Background(), taskID)

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
		NextToken     string `json:"nextToken"`
		MaxResults    int    `json:"maxResults"`
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
	sortBy(tasks, func(t ECSTask) string { return t.TaskArn })

	page, next := awsPage(tasks, req.NextToken, req.MaxResults, 100)

	taskArns := make([]string, 0, len(page))
	for _, t := range page {
		taskArns = append(taskArns, t.TaskArn)
	}

	out := map[string]any{"taskArns": taskArns}
	if next != "" {
		out["nextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
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

	// Cluster ARN.
	if strings.Contains(req.ResourceArn, ":cluster/") {
		name := ecsClusterNameFromRef(req.ResourceArn)
		cluster, ok := ecsClusters.Get(name)
		if !ok {
			sim.AWSError(w, "ClusterNotFoundException", "cluster not found: "+req.ResourceArn, http.StatusBadRequest)
			return
		}
		cluster.Tags = mergeECSTagsByKey(cluster.Tags, req.Tags)
		ecsClusters.Put(name, cluster)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Service ARN (arn:...:service/<cluster>/<service>).
	if strings.Contains(req.ResourceArn, ":service/") {
		clusterName, key, svc, ok := ecsServiceFromARN(req.ResourceArn)
		_ = clusterName
		if !ok {
			sim.AWSError(w, "ServiceNotFoundException", "service not found: "+req.ResourceArn, http.StatusBadRequest)
			return
		}
		svc.Tags = mergeECSTagsByKey(svc.Tags, req.Tags)
		ecsServices.Put(key, svc)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}

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
	if strings.Contains(req.ResourceArn, ":cluster/") {
		name := ecsClusterNameFromRef(req.ResourceArn)
		if cluster, ok := ecsClusters.Get(name); ok {
			cluster.Tags = keep(cluster.Tags)
			ecsClusters.Put(name, cluster)
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if strings.Contains(req.ResourceArn, ":service/") {
		if _, key, svc, ok := ecsServiceFromARN(req.ResourceArn); ok {
			svc.Tags = keep(svc.Tags)
			ecsServices.Put(key, svc)
		}
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
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	var tags []ECSTag

	// Check if it's a task definition ARN
	if strings.Contains(req.ResourceArn, ":task-definition/") {
		key := extractTDKey(req.ResourceArn)
		if td, ok := ecsTaskDefinitions.Get(key); ok {
			tags = td.Tags
		}
	}

	switch {
	case strings.Contains(req.ResourceArn, ":task/"):
		parts := strings.Split(req.ResourceArn, "/")
		if task, ok := ecsTasks.Get(parts[len(parts)-1]); ok {
			tags = task.Tags
		}
	case strings.Contains(req.ResourceArn, ":task-definition/"):
		if td, ok := ecsTaskDefinitions.Get(extractTDKey(req.ResourceArn)); ok {
			tags = td.Tags
		}
	case strings.Contains(req.ResourceArn, ":cluster/"):
		if cluster, ok := ecsClusters.Get(ecsClusterNameFromRef(req.ResourceArn)); ok {
			tags = cluster.Tags
		}
	case strings.Contains(req.ResourceArn, ":service/"):
		if _, _, svc, ok := ecsServiceFromARN(req.ResourceArn); ok {
			tags = svc.Tags
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

// resolveECSContainerSecrets resolves a container definition's `secrets` array
// (`[{"name","valueFrom"}]`) to name→value pairs by fetching each `valueFrom`
// from Secrets Manager or SSM Parameter Store, as real ECS does at task launch.
func resolveECSContainerSecrets(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var secrets []struct {
		Name      string `json:"name"`
		ValueFrom string `json:"valueFrom"`
	}
	if err := json.Unmarshal(raw, &secrets); err != nil {
		return out
	}
	for _, s := range secrets {
		if s.Name == "" || s.ValueFrom == "" {
			continue
		}
		if v, ok := resolveECSSecretValue(s.ValueFrom); ok {
			out[s.Name] = v
		}
	}
	return out
}

// resolveECSSecretValue resolves a single ECS secret `valueFrom` reference.
// Secrets Manager: arn:aws:secretsmanager:…:secret:name-suffix[:jsonKey:stage:id]
// — an optional jsonKey selects a field from a JSON SecretString. SSM:
// arn:aws:ssm:…:parameter/name or a bare /name.
func resolveECSSecretValue(valueFrom string) (string, bool) {
	if strings.Contains(valueFrom, ":secretsmanager:") {
		parts := strings.Split(valueFrom, ":")
		if len(parts) < 7 {
			return "", false
		}
		baseARN := strings.Join(parts[:7], ":")
		secret, ok := resolveSMSecret(baseARN)
		if !ok {
			return "", false
		}
		val := secret.SecretString
		if len(parts) >= 8 && parts[7] != "" {
			var m map[string]any
			if json.Unmarshal([]byte(val), &m) == nil {
				if jv, ok := m[parts[7]]; ok {
					return fmt.Sprint(jv), true
				}
			}
		}
		return val, true
	}
	// SSM Parameter Store (ARN or bare name).
	name := valueFrom
	if i := strings.Index(valueFrom, ":parameter"); i >= 0 {
		name = valueFrom[i+len(":parameter"):]
	}
	if p, ok := ssmParams.Get(ensureLeadingSlash(name)); ok {
		return p.Value, true
	}
	return "", false
}

// handleECSExecuteCommand returns a handler that implements ECS ExecuteCommand.
// It creates a session and registers a WebSocket handler for command execution.
func handleECSExecuteCommand(srv *sim.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Cluster     string `json:"cluster"`
			Task        string `json:"task"`
			Container   string `json:"container"`
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
		// Real ECS rejects exec unless the task was started with
		// enableExecuteCommand=true (the SSM exec agent is only injected then).
		if !task.EnableExecuteCommand {
			sim.AWSError(w, "InvalidParameterException",
				"The execute command failed because execute command was not enabled when the task was run or the execute command agent isn't running. Wait and try again or run a new task with execute command enabled and try again.",
				http.StatusBadRequest)
			return
		}

		sessionID := generateUUID()

		// Store the session
		// Look up the Docker container ID for this task (may need to wait briefly
		// for the container to start — it starts async after RUNNING transition)
		var dockerContainerID string
		for i := 0; i < 20; i++ {
			if v, ok := ecsProcessHandles.Load(taskID); ok {
				handle := v.(*ecsTaskProcesses).handleFor(req.Container)
				if handle == nil {
					break
				}
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

		// No fallback: ExecuteCommand requires the task's Docker
		// container. The sim never `os/exec`s the command on the sim
		// host — that would run against the wrong "host" entirely
		// (sim-binary host, not the Fargate-shaped task container).
		// See feedback_sim_host_model.md.
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
				"ECS ExecuteCommand requires a running Docker container for the task"))
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
