package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// ECS Service family — terraform-provider-aws declares `aws_ecs_service` for
// long-lived Fargate services (the common case; RunTask alone covers only
// one-shot tasks) and `aws_ecs_cluster_capacity_providers` for the cluster's
// capacity-provider config. The sim models the service as a control-plane
// object: it reaches ACTIVE with runningCount == desiredCount and a COMPLETED
// primary deployment, so create/update/delete and Describe/List round-trip.
// Real task placement/scheduling stays reserved for the container runtimes —
// Terraform only needs the control-plane state machine.

// ECSService is a control-plane model of an ECS service. Config blocks the sim
// doesn't interpret (network/load-balancer/registry config) are held as raw
// JSON so they round-trip byte-exact.
type ECSService struct {
	ServiceArn               string          `json:"serviceArn"`
	ServiceName              string          `json:"serviceName"`
	ClusterArn               string          `json:"clusterArn"`
	TaskDefinition           string          `json:"taskDefinition"`
	DesiredCount             int             `json:"desiredCount"`
	RunningCount             int             `json:"runningCount"`
	PendingCount             int             `json:"pendingCount"`
	Status                   string          `json:"status"`
	LaunchType               string          `json:"launchType,omitempty"`
	PlatformVersion          string          `json:"platformVersion,omitempty"`
	SchedulingStrategy       string          `json:"schedulingStrategy,omitempty"`
	RoleArn                  string          `json:"roleArn,omitempty"`
	PropagateTags            string          `json:"propagateTags,omitempty"`
	EnableExecuteCommand     bool            `json:"enableExecuteCommand,omitempty"`
	CreatedAt                float64         `json:"createdAt"`
	NetworkConfiguration     json.RawMessage `json:"networkConfiguration,omitempty"`
	LoadBalancers            json.RawMessage `json:"loadBalancers,omitempty"`
	ServiceRegistries        json.RawMessage `json:"serviceRegistries,omitempty"`
	DeploymentController     json.RawMessage `json:"deploymentController,omitempty"`
	CapacityProviderStrategy json.RawMessage `json:"capacityProviderStrategy,omitempty"`
	Deployments              []ECSDeployment `json:"deployments"`
	Tags                     []ECSTag        `json:"tags,omitempty"`
}

// ECSDeployment is the service's deployment record. A modeled service has a
// single PRIMARY deployment that immediately reports COMPLETED.
type ECSDeployment struct {
	Id             string  `json:"id"`
	Status         string  `json:"status"`
	TaskDefinition string  `json:"taskDefinition"`
	DesiredCount   int     `json:"desiredCount"`
	RunningCount   int     `json:"runningCount"`
	PendingCount   int     `json:"pendingCount"`
	RolloutState   string  `json:"rolloutState"`
	CreatedAt      float64 `json:"createdAt"`
	UpdatedAt      float64 `json:"updatedAt"`
}

var ecsServices sim.Store[ECSService]

func registerECSServices(r *sim.AWSRouter, srv *sim.Server) {
	ecsServices = sim.MakeStore[ECSService](srv.DB(), "ecs_services")

	r.Register("AmazonEC2ContainerServiceV20141113.CreateService", handleECSCreateService)
	r.Register("AmazonEC2ContainerServiceV20141113.DescribeServices", handleECSDescribeServices)
	r.Register("AmazonEC2ContainerServiceV20141113.ListServices", handleECSListServices)
	r.Register("AmazonEC2ContainerServiceV20141113.UpdateService", handleECSUpdateService)
	r.Register("AmazonEC2ContainerServiceV20141113.DeleteService", handleECSDeleteService)
	r.Register("AmazonEC2ContainerServiceV20141113.PutClusterCapacityProviders", handleECSPutClusterCapacityProviders)
}

// ecsClusterNameFromRef extracts the cluster name from a name or ARN,
// defaulting to "default" (the implicit cluster) when empty.
func ecsClusterNameFromRef(ref string) string {
	if ref == "" {
		return "default"
	}
	if strings.HasPrefix(ref, "arn:") {
		parts := strings.Split(ref, "/")
		return parts[len(parts)-1]
	}
	return ref
}

// ecsServiceNameFromRef extracts the service name from a name or ARN.
func ecsServiceNameFromRef(ref string) string {
	if strings.HasPrefix(ref, "arn:") {
		parts := strings.Split(ref, "/")
		return parts[len(parts)-1]
	}
	return ref
}

func ecsServiceKey(cluster, service string) string { return cluster + "/" + service }

func handleECSCreateService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster                  string          `json:"cluster"`
		ServiceName              string          `json:"serviceName"`
		TaskDefinition           string          `json:"taskDefinition"`
		DesiredCount             *int            `json:"desiredCount"`
		LaunchType               string          `json:"launchType"`
		PlatformVersion          string          `json:"platformVersion"`
		SchedulingStrategy       string          `json:"schedulingStrategy"`
		Role                     string          `json:"role"`
		PropagateTags            string          `json:"propagateTags"`
		EnableExecuteCommand     bool            `json:"enableExecuteCommand"`
		NetworkConfiguration     json.RawMessage `json:"networkConfiguration"`
		LoadBalancers            json.RawMessage `json:"loadBalancers"`
		ServiceRegistries        json.RawMessage `json:"serviceRegistries"`
		DeploymentController     json.RawMessage `json:"deploymentController"`
		CapacityProviderStrategy json.RawMessage `json:"capacityProviderStrategy"`
		Tags                     []ECSTag        `json:"tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceName == "" {
		sim.AWSError(w, "InvalidParameterException", "serviceName is required", http.StatusBadRequest)
		return
	}
	if req.TaskDefinition == "" {
		sim.AWSError(w, "InvalidParameterException", "taskDefinition is required", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	cluster, ok := ecsClusters.Get(clusterName)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest,
			"Cluster not found: %s", clusterName)
		return
	}
	key := ecsServiceKey(clusterName, req.ServiceName)
	if existing, ok := ecsServices.Get(key); ok && existing.Status == "ACTIVE" {
		sim.AWSErrorf(w, "InvalidParameterException", http.StatusBadRequest,
			"Creating a service named %q in cluster %q is not permitted while a service of the same name already exists.",
			req.ServiceName, clusterName)
		return
	}
	desired := 0
	if req.DesiredCount != nil {
		desired = *req.DesiredCount
	}
	strategy := req.SchedulingStrategy
	if strategy == "" {
		strategy = "REPLICA"
	}
	now := float64(time.Now().Unix())
	svc := ECSService{
		ServiceArn:               ecsArn("service", clusterName+"/"+req.ServiceName),
		ServiceName:              req.ServiceName,
		ClusterArn:               cluster.ClusterArn,
		TaskDefinition:           req.TaskDefinition,
		DesiredCount:             desired,
		RunningCount:             desired, // modeled: tasks are immediately running
		PendingCount:             0,
		Status:                   "ACTIVE",
		LaunchType:               req.LaunchType,
		PlatformVersion:          req.PlatformVersion,
		SchedulingStrategy:       strategy,
		RoleArn:                  req.Role,
		PropagateTags:            req.PropagateTags,
		EnableExecuteCommand:     req.EnableExecuteCommand,
		CreatedAt:                now,
		NetworkConfiguration:     req.NetworkConfiguration,
		LoadBalancers:            req.LoadBalancers,
		ServiceRegistries:        req.ServiceRegistries,
		DeploymentController:     req.DeploymentController,
		CapacityProviderStrategy: req.CapacityProviderStrategy,
		Tags:                     req.Tags,
		Deployments:              []ECSDeployment{ecsPrimaryDeployment(req.TaskDefinition, desired, now)},
	}
	ecsServices.Put(key, svc)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"service": svc})
}

func ecsPrimaryDeployment(taskDef string, desired int, now float64) ECSDeployment {
	return ECSDeployment{
		Id:             "ecs-svc/" + generateUUID(),
		Status:         "PRIMARY",
		TaskDefinition: taskDef,
		DesiredCount:   desired,
		RunningCount:   desired,
		PendingCount:   0,
		RolloutState:   "COMPLETED",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func handleECSDescribeServices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster  string   `json:"cluster"`
		Services []string `json:"services"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	services := make([]ECSService, 0, len(req.Services))
	failures := make([]map[string]string, 0)
	for _, ref := range req.Services {
		name := ecsServiceNameFromRef(ref)
		if svc, ok := ecsServices.Get(ecsServiceKey(clusterName, name)); ok {
			services = append(services, svc)
		} else {
			failures = append(failures, map[string]string{
				"arn":    ecsArn("service", clusterName+"/"+name),
				"reason": "MISSING",
			})
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"services": services,
		"failures": failures,
	})
}

func handleECSListServices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster string `json:"cluster"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	arns := make([]string, 0)
	for _, svc := range ecsServices.List() {
		if ecsClusterNameFromRef(svc.ClusterArn) == clusterName && svc.Status == "ACTIVE" {
			arns = append(arns, svc.ServiceArn)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"serviceArns": arns})
}

func handleECSUpdateService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster              string          `json:"cluster"`
		Service              string          `json:"service"`
		TaskDefinition       string          `json:"taskDefinition"`
		DesiredCount         *int            `json:"desiredCount"`
		NetworkConfiguration json.RawMessage `json:"networkConfiguration"`
		EnableExecuteCommand *bool           `json:"enableExecuteCommand"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	key := ecsServiceKey(clusterName, ecsServiceNameFromRef(req.Service))
	svc, ok := ecsServices.Get(key)
	if !ok {
		sim.AWSErrorf(w, "ServiceNotFoundException", http.StatusBadRequest,
			"Service not found: %s", req.Service)
		return
	}
	if req.TaskDefinition != "" {
		svc.TaskDefinition = req.TaskDefinition
	}
	if req.DesiredCount != nil {
		svc.DesiredCount = *req.DesiredCount
		svc.RunningCount = *req.DesiredCount
	}
	if req.NetworkConfiguration != nil {
		svc.NetworkConfiguration = req.NetworkConfiguration
	}
	if req.EnableExecuteCommand != nil {
		svc.EnableExecuteCommand = *req.EnableExecuteCommand
	}
	now := float64(time.Now().Unix())
	svc.Deployments = []ECSDeployment{ecsPrimaryDeployment(svc.TaskDefinition, svc.DesiredCount, now)}
	ecsServices.Put(key, svc)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"service": svc})
}

func handleECSDeleteService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster string `json:"cluster"`
		Service string `json:"service"`
		Force   bool   `json:"force"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	key := ecsServiceKey(clusterName, ecsServiceNameFromRef(req.Service))
	svc, ok := ecsServices.Get(key)
	if !ok {
		sim.AWSErrorf(w, "ServiceNotFoundException", http.StatusBadRequest,
			"Service not found: %s", req.Service)
		return
	}
	// Real ECS drains then marks the service INACTIVE; DescribeServices keeps
	// returning it as INACTIVE, which is how terraform-provider-aws confirms the
	// delete converged.
	svc.Status = "INACTIVE"
	svc.DesiredCount = 0
	svc.RunningCount = 0
	svc.PendingCount = 0
	ecsServices.Put(key, svc)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"service": svc})
}

func handleECSPutClusterCapacityProviders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster                         string          `json:"cluster"`
		CapacityProviders               []string        `json:"capacityProviders"`
		DefaultCapacityProviderStrategy json.RawMessage `json:"defaultCapacityProviderStrategy"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	cluster, ok := ecsClusters.Get(clusterName)
	if !ok {
		sim.AWSErrorf(w, "ClusterNotFoundException", http.StatusBadRequest,
			"Cluster not found: %s", clusterName)
		return
	}
	if req.CapacityProviders == nil {
		req.CapacityProviders = []string{}
	}
	cluster.CapacityProviders = req.CapacityProviders
	cluster.DefaultCapacityProviderStrategy = req.DefaultCapacityProviderStrategy
	ecsClusters.Put(clusterName, cluster)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"cluster": cluster})
}
