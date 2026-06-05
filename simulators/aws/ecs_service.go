package main

import (
	"encoding/json"
	"net/http"
	"sort"
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
	r.Register("AmazonEC2ContainerServiceV20141113.ListClusters", handleECSListClusters)
	r.Register("AmazonEC2ContainerServiceV20141113.ListTaskDefinitions", handleECSListTaskDefinitions)
	r.Register("AmazonEC2ContainerServiceV20141113.ListTaskDefinitionFamilies", handleECSListTaskDefinitionFamilies)
	r.Register("AmazonEC2ContainerServiceV20141113.DescribeCapacityProviders", handleECSDescribeCapacityProviders)
}

// ecsBuiltInCapacityProviders are the two AWS-managed providers every account
// has; they are always ACTIVE and need no cluster association.
var ecsBuiltInCapacityProviders = []string{"FARGATE", "FARGATE_SPOT"}

// handleECSDescribeCapacityProviders is the read-back for capacity providers
// (BUG-1479) — without it `aws_ecs_cluster_capacity_providers` shows spurious
// drift on the post-apply plan. The sim has no standalone capacity-provider
// store (providers live as names on clusters via PutClusterCapacityProviders),
// so it resolves the built-in FARGATE/FARGATE_SPOT plus any custom provider
// referenced by a cluster. A requested name that resolves to neither is a
// failure, mirroring real ECS.
func handleECSDescribeCapacityProviders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CapacityProviders []string `json:"capacityProviders"`
	}
	_ = sim.ReadJSON(r, &req)

	known := map[string]bool{}
	for _, name := range ecsBuiltInCapacityProviders {
		known[name] = true
	}
	for _, c := range ecsClusters.List() {
		for _, cp := range c.CapacityProviders {
			known[ecsCapacityProviderName(cp)] = true
		}
	}

	requested := req.CapacityProviders
	if len(requested) == 0 {
		for name := range known {
			requested = append(requested, name)
		}
		sort.Strings(requested)
	}

	providers := make([]map[string]any, 0, len(requested))
	failures := make([]map[string]any, 0)
	for _, ref := range requested {
		name := ecsCapacityProviderName(ref)
		if !known[name] {
			failures = append(failures, map[string]any{
				"arn":    ecsArn("capacity-provider", name),
				"reason": "MISSING",
			})
			continue
		}
		providers = append(providers, map[string]any{
			"capacityProviderArn": ecsArn("capacity-provider", name),
			"name":                name,
			"status":              "ACTIVE",
			"tags":                []any{},
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"capacityProviders": providers,
		"failures":          failures,
	})
}

// ecsCapacityProviderName normalizes a capacity provider name or ARN to its
// bare name.
func ecsCapacityProviderName(ref string) string {
	if strings.HasPrefix(ref, "arn:") {
		parts := strings.Split(ref, "/")
		return parts[len(parts)-1]
	}
	return ref
}

// handleECSListTaskDefinitionFamilies aggregates the distinct families across
// the registered task definitions (BUG-1479) — the family companion to
// ListTaskDefinitions. status (ACTIVE/INACTIVE/ALL) selects which revisions a
// family must have to be listed; familyPrefix narrows by name prefix.
func handleECSListTaskDefinitionFamilies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FamilyPrefix string `json:"familyPrefix"`
		Status       string `json:"status"`
		MaxResults   int    `json:"maxResults"`
		NextToken    string `json:"nextToken"`
	}
	_ = sim.ReadJSON(r, &req)

	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	// active[family] is true if the family has ≥1 ACTIVE revision.
	active := map[string]bool{}
	seen := map[string]bool{}
	for _, td := range ecsTaskDefinitions.List() {
		seen[td.Family] = true
		if td.Status == "ACTIVE" {
			active[td.Family] = true
		}
	}
	families := make([]string, 0, len(seen))
	for family := range seen {
		if req.FamilyPrefix != "" && !strings.HasPrefix(family, req.FamilyPrefix) {
			continue
		}
		switch status {
		case "ACTIVE":
			if !active[family] {
				continue
			}
		case "INACTIVE":
			if active[family] {
				continue
			}
		}
		families = append(families, family)
	}
	sort.Strings(families)
	page, next := awsPage(families, req.NextToken, req.MaxResults, 100)
	out := map[string]any{"families": page}
	if next != "" {
		out["nextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
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

// ecsServiceFromARN resolves a service ARN (arn:...:service/<cluster>/<name>,
// or the older arn:...:service/<name> form) to its stored service.
func ecsServiceFromARN(arn string) (clusterName, key string, svc ECSService, ok bool) {
	idx := strings.Index(arn, ":service/")
	if idx < 0 {
		return "", "", ECSService{}, false
	}
	rest := arn[idx+len(":service/"):]
	if parts := strings.SplitN(rest, "/", 2); len(parts) == 2 {
		clusterName, key = parts[0], ecsServiceKey(parts[0], parts[1])
	} else {
		clusterName, key = "default", ecsServiceKey("default", parts[0])
	}
	svc, ok = ecsServices.Get(key)
	return clusterName, key, svc, ok
}

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
		Cluster    string `json:"cluster"`
		MaxResults int    `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	clusterName := ecsClusterNameFromRef(req.Cluster)
	all := make([]string, 0)
	for _, svc := range ecsServices.List() {
		if ecsClusterNameFromRef(svc.ClusterArn) == clusterName && svc.Status == "ACTIVE" {
			all = append(all, svc.ServiceArn)
		}
	}
	sort.Strings(all)
	page, next := awsPage(all, req.NextToken, req.MaxResults, 100)
	out := map[string]any{"serviceArns": page}
	if next != "" {
		out["nextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleECSListClusters(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxResults int    `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	_ = sim.ReadJSON(r, &req)
	all := make([]string, 0)
	for _, c := range ecsClusters.List() {
		all = append(all, c.ClusterArn)
	}
	sort.Strings(all)
	page, next := awsPage(all, req.NextToken, req.MaxResults, 100)
	out := map[string]any{"clusterArns": page}
	if next != "" {
		out["nextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleECSListTaskDefinitions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FamilyPrefix string `json:"familyPrefix"`
		MaxResults   int    `json:"maxResults"`
		NextToken    string `json:"nextToken"`
	}
	_ = sim.ReadJSON(r, &req)
	all := make([]string, 0)
	for _, td := range ecsTaskDefinitions.List() {
		if req.FamilyPrefix != "" && !strings.HasPrefix(td.Family, req.FamilyPrefix) {
			continue
		}
		all = append(all, td.TaskDefinitionArn)
	}
	sort.Strings(all)
	page, next := awsPage(all, req.NextToken, req.MaxResults, 100)
	out := map[string]any{"taskDefinitionArns": page}
	if next != "" {
		out["nextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
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
